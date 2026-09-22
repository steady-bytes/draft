use dioxus::html::input_data::MouseButton;
use dioxus::prelude::*;
use std::collections::{HashMap, HashSet};
use std::sync::atomic::{AtomicU64, Ordering};

use gloo_timers::future::TimeoutFuture;
use tonic_web_wasm_client::Client as WasmClient;

use draft_api::hook::core_registry_key_value_v1::key_value_service_client::KeyValueServiceClient;
use draft_api::hook::core_registry_service_discovery_v1::{
    filter, use_service_discovery_service_service, Filter, QueryRequest,
};
use draft_api::proto::core_control_plane_networking_v1::{
    networking_service_client::NetworkingServiceClient, ListRoutesRequest, Route as GwRoute,
};
use draft_api::proto::core_message_broker_actors_v1::{
    topology_client::TopologyClient, GetTopologyRequest, GetTopologyResponse, WatchTopologyRequest,
};
use draft_api::proto::core_registry_key_value_v1::{GetRequest, SetRequest, Value as KvValue};
use draft_api::proto::core_registry_service_discovery_v1::{
    service_discovery_service_client::ServiceDiscoveryServiceClient, Process, ProcessHealthState,
    ProcessRunningState, WatchRequest,
};
use prost::Message as _;
use prost_types::Any;

use crate::components::{TopologyData, TopologyEdge, TopologyNode};

/// The KV key layout positions are persisted under — a plain JSON-encoded
/// `LayoutBlob`, using the same generic `Value{data: string}` shape
/// `leader`/`fuse_address` already use elsewhere in this store (see
/// docs/architecture/cluster-live-topology-implementation-plan.md's
/// "no new proto messages" non-goal).
const LAYOUT_KV_KEY: &str = "cluster/layout";
const LAYOUT_LOCAL_STORAGE_KEY: &str = "cluster_layout";
const LAYOUT_VALUE_TYPE_URL: &str = "type.googleapis.com/core.registry.key_value.v1.Value";
/// How often Gateway routes are re-polled — ListRoutes has no Watch RPC, and
/// routes change far less often than process health or event volume.
const GATEWAY_POLL_MS: u32 = 10_000;

// ── ID generator ─────────────────────────────────────────────────────────────

static UID: AtomicU64 = AtomicU64::new(1);
fn next_id() -> String {
    format!("n{}", UID.fetch_add(1, Ordering::Relaxed))
}
fn uid_val() -> u64 {
    UID.load(Ordering::Relaxed)
}

// ── Node kind ─────────────────────────────────────────────────────────────────

#[derive(Clone, Debug, PartialEq)]
enum NodeKind {
    Catalyst,
    Blueprint,
    Fuse,
    Service,
}

impl NodeKind {
    fn sym(&self) -> &'static str {
        match self {
            Self::Catalyst => "Ca",
            Self::Blueprint => "Bp",
            Self::Fuse => "Fs",
            Self::Service => "Sv",
        }
    }
    fn el(&self) -> &'static str {
        match self {
            Self::Catalyst => "CATALYST",
            Self::Blueprint => "BLUEPRINT",
            Self::Fuse => "FUSE",
            Self::Service => "SERVICE",
        }
    }
    fn num(&self) -> &'static str {
        match self {
            Self::Catalyst => "01",
            Self::Blueprint => "02",
            Self::Fuse => "03",
            Self::Service => "··",
        }
    }
    fn role(&self) -> &'static str {
        match self {
            Self::Catalyst => "EVENT BUS",
            Self::Blueprint => "SERVICE REGISTRY",
            Self::Fuse => "API GATEWAY",
            Self::Service => "WORKLOAD",
        }
    }
    fn color(&self) -> &'static str {
        match self {
            Self::Catalyst => "#58a6ff",
            Self::Blueprint => "#b48cff",
            Self::Fuse => "#ffb454",
            Self::Service => "#9fb3c2",
        }
    }
}

// ── Data model ────────────────────────────────────────────────────────────────

#[derive(Clone, Debug, PartialEq)]
struct RoutingRule {
    method: String,
    path: String,
    target: String,
}

#[derive(Clone, Debug, PartialEq)]
struct Topic {
    name: String,
    retention: String,
    partitions: u32,
}

/// Live nodes are derived from real cluster data (Registry/Gateway/Topology)
/// on every refresh and can't be deleted/toggled from the canvas — deleting
/// the row on screen doesn't stop the real process. Manual nodes are the
/// original hand-authored kind: fully editable, never touched by a live
/// refresh. See docs/architecture/cluster-live-topology-implementation-plan.md.
#[derive(Clone, Debug, PartialEq)]
enum NodeOrigin {
    Live,
    Manual,
}

#[derive(Clone, Debug, PartialEq)]
struct ClNode {
    id: String,
    kind: NodeKind,
    name: String,
    x: f64,
    y: f64,
    online: bool,
    rules: Vec<RoutingRule>,
    topics: Vec<Topic>,
    host: String,
    port: u16,
    protocol: String,
    ttl: String,
    origin: NodeOrigin,
}

/// live_node_id is a stable, content-addressed id for a live node — always
/// `live:<name>`, the same on every re-derivation, so drag positions and
/// wires attached to it survive a live-data refresh instead of being
/// recreated (and re-randomized) every time. Never collides with a manual
/// node's `next_id()`-generated `n<N>` id.
fn live_node_id(name: &str) -> String {
    format!("live:{name}")
}

impl ClNode {
    fn new(kind: NodeKind, name: &str, x: f64, y: f64) -> Self {
        let (rules, topics, host, port, ttl) = match &kind {
            NodeKind::Fuse => (
                vec![
                    RoutingRule {
                        method: "GET".into(),
                        path: "/api/auth/*".into(),
                        target: "auth-svc".into(),
                    },
                    RoutingRule {
                        method: "POST".into(),
                        path: "/api/billing/*".into(),
                        target: "billing-svc".into(),
                    },
                ],
                vec![],
                String::new(),
                8080u16,
                String::new(),
            ),
            NodeKind::Catalyst => (
                vec![],
                vec![
                    Topic {
                        name: "agent.events".into(),
                        retention: "72h".into(),
                        partitions: 6,
                    },
                    Topic {
                        name: "billing.tx".into(),
                        retention: "30d".into(),
                        partitions: 3,
                    },
                ],
                String::new(),
                8080,
                String::new(),
            ),
            NodeKind::Blueprint => (vec![], vec![], String::new(), 8080, "15s".into()),
            NodeKind::Service => (
                vec![],
                vec![],
                format!("{name}.cluster.local"),
                8080,
                String::new(),
            ),
        };
        Self {
            id: next_id(),
            kind,
            name: name.into(),
            x,
            y,
            online: true,
            rules,
            topics,
            host,
            port,
            protocol: "grpc".into(),
            ttl,
            origin: NodeOrigin::Manual,
        }
    }
    /// A node derived from real cluster data — see `derive_nodes`.
    fn new_live(kind: NodeKind, name: &str, x: f64, y: f64) -> Self {
        Self {
            id: live_node_id(name),
            kind,
            name: name.into(),
            x,
            y,
            online: false,
            rules: vec![],
            topics: vec![],
            host: String::new(),
            port: 0,
            protocol: "grpc".into(),
            ttl: String::new(),
            origin: NodeOrigin::Live,
        }
    }
    fn cx(&self) -> f64 {
        self.x + NW / 2.0
    }
    fn wire_count(&self, ts: &[ClTrace]) -> usize {
        ts.iter()
            .filter(|t| t.kind == TraceKind::Wire && (t.from == self.id || t.to == self.id))
            .count()
    }
    fn event_count(&self, ts: &[ClTrace]) -> usize {
        ts.iter()
            .filter(|t| t.kind == TraceKind::Event && (t.from == self.id || t.to == self.id))
            .count()
    }
}

#[derive(Clone, Debug, PartialEq)]
enum TraceKind {
    Wire,
    Event,
}

#[derive(Clone, Debug, PartialEq)]
struct ClTrace {
    id: String,
    from: String,
    to: String,
    kind: TraceKind,
    topic: Option<String>,
}

impl ClTrace {
    fn wire(a: &str, b: &str) -> Self {
        Self {
            id: next_id(),
            from: a.into(),
            to: b.into(),
            kind: TraceKind::Wire,
            topic: None,
        }
    }
    fn event(f: &str, t: &str, tp: &str) -> Self {
        Self {
            id: next_id(),
            from: f.into(),
            to: t.into(),
            kind: TraceKind::Event,
            topic: Some(tp.into()),
        }
    }
}

// ── Interaction state ──────────────────────────────────────────────────────────

#[derive(Clone, Debug, Default, PartialEq)]
enum Drag {
    #[default]
    None,
    Pan {
        sx: f64,
        sy: f64,
        px: f64,
        py: f64,
    },
    Node {
        id: String,
        sx: f64,
        sy: f64,
        nx: f64,
        ny: f64,
    },
    Pad {
        from_id: String,
        fx: f64,
        fy: f64,
        fd: i32,
        tx: f64,
        ty: f64,
    },
}

#[derive(Clone, Debug, PartialEq)]
enum MenuFor {
    Canvas { wx: f64, wy: f64 },
    Node(String),
}

#[derive(Clone, Debug, PartialEq)]
struct Menu {
    x: f64,
    y: f64,
    for_: MenuFor,
}

// ── Geometry ──────────────────────────────────────────────────────────────────

const NW: f64 = 178.0;
const NH: f64 = 84.0;

fn port_of(nx: f64, ny: f64, other_cx: f64) -> (f64, f64, i32) {
    if other_cx >= nx + NW / 2.0 {
        (nx + NW, ny + NH / 2.0, 1)
    } else {
        (nx, ny + NH / 2.0, -1)
    }
}

fn route_pts(sx: f64, sy: f64, sd: i32, ex: f64, ey: f64, ed: i32) -> Vec<(f64, f64)> {
    const STUB: f64 = 14.0;
    let (p1x, p1y) = (sx + sd as f64 * STUB, sy);
    let (p4x, p4y) = (ex + ed as f64 * STUB, ey);
    let mut pts = vec![(sx, sy), (p1x, p1y)];
    let (dx, dy) = (p4x - p1x, p4y - p1y);
    let (adx, ady) = (dx.abs(), dy.abs());
    let sgx: f64 = if dx >= 0.0 { 1.0 } else { -1.0 };
    let sgy: f64 = if dy >= 0.0 { 1.0 } else { -1.0 };
    if ady < 0.5 {
    } else if sgx == sd as f64 && adx >= ady {
        pts.push((p1x + sgx * (adx - ady), p1y));
    } else if sgx == sd as f64 && ady > adx {
        pts.push((p1x + sgx * adx, p1y + sgy * adx));
    } else {
        let mid_y = p1y + sgy * (ady / 2.0).max(40.0);
        pts.push((p1x, mid_y));
        pts.push((p4x, mid_y));
    }
    pts.push((p4x, p4y));
    pts.push((ex, ey));
    let mut out: Vec<(f64, f64)> = Vec::new();
    for &p in &pts {
        if out.last().map_or(true, |&q: &(f64, f64)| {
            (p.0 - q.0).abs() > 0.25 || (p.1 - q.1).abs() > 0.25
        }) {
            out.push(p);
        }
    }
    out
}

fn pts_path(pts: &[(f64, f64)]) -> String {
    pts.iter()
        .enumerate()
        .map(|(i, (x, y))| {
            if i == 0 {
                format!("M {x:.1} {y:.1}")
            } else {
                format!("L {x:.1} {y:.1}")
            }
        })
        .collect::<Vec<_>>()
        .join(" ")
}

fn poly_len(pts: &[(f64, f64)]) -> f64 {
    pts.windows(2)
        .map(|w| {
            let ((x0, y0), (x1, y1)) = (w[0], w[1]);
            ((x1 - x0).powi(2) + (y1 - y0).powi(2)).sqrt()
        })
        .sum()
}

fn point_at(pts: &[(f64, f64)], mut d: f64) -> (f64, f64, f64) {
    for w in pts.windows(2) {
        let ((x0, y0), (x1, y1)) = (w[0], w[1]);
        let seg = ((x1 - x0).powi(2) + (y1 - y0).powi(2)).sqrt();
        if d <= seg {
            let t = if seg > 0.0 { d / seg } else { 0.0 };
            return (
                x0 + (x1 - x0) * t,
                y0 + (y1 - y0) * t,
                (y1 - y0).atan2(x1 - x0),
            );
        }
        d -= seg;
    }
    let &(lx, ly) = pts.last().unwrap_or(&(0.0, 0.0));
    (lx, ly, 0.0)
}

fn hex_alpha(hex: &str, a: f64) -> String {
    let h = hex.trim_start_matches('#');
    let r = u8::from_str_radix(&h[0..2], 16).unwrap_or(61);
    let g = u8::from_str_radix(&h[2..4], 16).unwrap_or(220);
    let b = u8::from_str_radix(&h[4..6], 16).unwrap_or(151);
    format!("rgba({r},{g},{b},{a:.3})")
}

fn snap8(v: f64) -> f64 {
    (v / 8.0).round() * 8.0
}

// ── Live data → graph derivation ────────────────────────────────────────────
// See docs/architecture/cluster-live-topology-implementation-plan.md for the
// full rationale behind the identity join and edge derivation below.

/// Recovers a logical service name from a CloudEvent `source` string (e.g.
/// `/services/bench` -> `bench`), the convention documented in
/// docs/architecture/wide-events.md and used by every real producer in this
/// repo. Falls back to the raw source when it doesn't match the convention,
/// so an unresolved topology node is still shown rather than dropped.
fn service_name_from_source(source: &str) -> &str {
    source
        .strip_prefix("/services/")
        .or_else(|| source.strip_prefix("/plugins/"))
        .unwrap_or(source)
}

fn kind_for_name(name: &str) -> NodeKind {
    match name {
        "blueprint" => NodeKind::Blueprint,
        "catalyst" => NodeKind::Catalyst,
        "fuse" => NodeKind::Fuse,
        _ => NodeKind::Service,
    }
}

/// Deterministic lane-based placement for a node with no known position yet
/// (not a full force-directed layout — a ~15-node cluster diagram doesn't
/// need one; revisit only if this reads poorly in practice). Core services
/// get fixed anchor points; everything else fills a grid.
fn auto_position(kind: &NodeKind, service_index: usize) -> (f64, f64) {
    match kind {
        NodeKind::Blueprint => (470.0, 40.0),
        NodeKind::Fuse => (120.0, 280.0),
        NodeKind::Catalyst => (830.0, 280.0),
        NodeKind::Service => {
            let col = (service_index % 3) as f64;
            let row = (service_index / 3) as f64;
            (120.0 + col * 220.0, 460.0 + row * 120.0)
        }
    }
}

/// Builds the full node list from Registry (`processes`, the authoritative
/// "what's actually running" list) plus any Topology producer/consumer that
/// doesn't resolve to a registered process. `existing` is the current
/// on-screen node list — a node found there (by id) keeps its position and
/// other user-editable fields; only genuinely new nodes get auto-placed.
/// `saved_positions` seeds a brand-new node's position from a persisted
/// layout before falling back to `auto_position`. Manual nodes in `existing`
/// pass through untouched.
fn derive_nodes(
    processes: &HashMap<String, Process>,
    topology: &TopologyData,
    existing: &[ClNode],
    saved_positions: &HashMap<String, (f64, f64)>,
) -> Vec<ClNode> {
    let by_id: HashMap<&str, &ClNode> = existing.iter().map(|n| (n.id.as_str(), n)).collect();

    let mut by_name: HashMap<String, Vec<&Process>> = HashMap::new();
    for p in processes.values() {
        by_name.entry(p.name.clone()).or_default().push(p);
    }

    let mut result: Vec<ClNode> = Vec::new();
    let mut seen_names: HashSet<String> = HashSet::new();
    let mut service_idx = 0usize;

    let mut names: Vec<&String> = by_name.keys().collect();
    names.sort();
    for name in names {
        let instances = &by_name[name];
        let id = live_node_id(name);
        seen_names.insert(name.clone());
        let online = instances
            .iter()
            .any(|p| p.running_state == ProcessRunningState::ProcessRunning as i32)
            && instances
                .iter()
                .any(|p| p.health_state == ProcessHealthState::ProcessHealthy as i32);
        let ip = instances
            .first()
            .map(|p| p.ip_address.clone())
            .unwrap_or_default();
        let kind = kind_for_name(name);

        let is_new = by_id.get(id.as_str()).is_none();
        let mut node = match by_id.get(id.as_str()) {
            Some(existing_node) => (*existing_node).clone(),
            None => {
                let (x, y) = saved_positions
                    .get(&id)
                    .copied()
                    .unwrap_or_else(|| auto_position(&kind, service_idx));
                ClNode::new_live(kind.clone(), name, x, y)
            }
        };
        if matches!(kind, NodeKind::Service) && is_new {
            service_idx += 1;
        }
        node.online = online;
        node.host = ip;
        result.push(node);
    }

    for tn in topology.producers.iter().chain(topology.consumers.iter()) {
        let name = service_name_from_source(&tn.id).to_string();
        if seen_names.contains(&name) {
            continue;
        }
        seen_names.insert(name.clone());
        let id = live_node_id(&name);
        let kind = kind_for_name(&name);
        let node = match by_id.get(id.as_str()) {
            Some(existing_node) => (*existing_node).clone(),
            None => {
                let (x, y) = saved_positions
                    .get(&id)
                    .copied()
                    .unwrap_or_else(|| auto_position(&kind, service_idx));
                if matches!(kind, NodeKind::Service) {
                    service_idx += 1;
                }
                ClNode::new_live(kind, &name, x, y)
            }
        };
        result.push(node);
    }

    for n in existing.iter().filter(|n| n.origin == NodeOrigin::Manual) {
        result.push(n.clone());
    }

    result
}

/// Builds wires (from Gateway routes) and animated event traces (from
/// Topology edges) against the just-derived `nodes` list. Wires run
/// Fuse -> resolved route target, matching `Route.endpoint.host:port`
/// against `Process.ip_address` (== `advertise_address`, see
/// docs/architecture/service-registry-identity.md) to find the target.
/// Event traces run two hops per Topology edge, `producer -> Catalyst` and
/// `Catalyst -> consumer` — correct by construction, since every Topology
/// edge is by definition two CloudEvents through Catalyst, never a direct
/// call. Manual wires/events (attached to a Manual-origin node on either
/// end) are preserved from `existing`.
fn derive_traces(
    nodes: &[ClNode],
    routes: &[GwRoute],
    processes: &HashMap<String, Process>,
    topology: &TopologyData,
    existing: &[ClTrace],
) -> Vec<ClTrace> {
    let live_by_name: HashMap<&str, &str> = nodes
        .iter()
        .filter(|n| n.origin == NodeOrigin::Live)
        .map(|n| (n.name.as_str(), n.id.as_str()))
        .collect();

    let mut traces: Vec<ClTrace> = Vec::new();

    if let Some(&fuse_id) = live_by_name.get("fuse") {
        for route in routes {
            let Some(endpoint) = &route.endpoint else {
                continue;
            };
            let target_addr = format!("{}:{}", endpoint.host, endpoint.port);
            let Some(target_process) = processes.values().find(|p| p.ip_address == target_addr)
            else {
                continue;
            };
            let Some(&target_id) = live_by_name.get(target_process.name.as_str()) else {
                continue;
            };
            if target_id == fuse_id {
                continue;
            }
            traces.push(ClTrace::wire(fuse_id, target_id));
        }
    }

    if let Some(&catalyst_id) = live_by_name.get("catalyst") {
        for edge in &topology.edges {
            let producer_name = service_name_from_source(&edge.producer_id);
            let consumer_name = service_name_from_source(&edge.consumer_id);
            let Some(&producer_id) = live_by_name.get(producer_name) else {
                continue;
            };
            let Some(&consumer_id) = live_by_name.get(consumer_name) else {
                continue;
            };
            traces.push(ClTrace::event(producer_id, catalyst_id, &edge.event_type));
            traces.push(ClTrace::event(catalyst_id, consumer_id, &edge.event_type));
        }
    }

    // Manual traces (either endpoint a Manual node) pass through untouched.
    let manual_ids: HashSet<&str> = nodes
        .iter()
        .filter(|n| n.origin == NodeOrigin::Manual)
        .map(|n| n.id.as_str())
        .collect();
    for t in existing {
        if manual_ids.contains(t.from.as_str()) || manual_ids.contains(t.to.as_str()) {
            traces.push(t.clone());
        }
    }

    traces
}

/// Maps Catalyst's `GetTopology`/`WatchTopology` response onto the shared
/// `TopologyData` shape `views/topology.rs` already renders from — kept as a
/// small local copy rather than a shared export, since it's just a field
/// mapping and not worth a cross-module visibility change.
fn topology_from_response(resp: GetTopologyResponse) -> TopologyData {
    let producers = resp
        .producers
        .into_iter()
        .map(|n| TopologyNode {
            id: n.id.clone(),
            name: if n.name.is_empty() { n.id } else { n.name },
        })
        .collect();
    let consumers = resp
        .consumers
        .into_iter()
        .map(|n| TopologyNode {
            id: n.id.clone(),
            name: if n.name.is_empty() { n.id } else { n.name },
        })
        .collect();
    let edges = resp
        .edges
        .into_iter()
        .map(|e| TopologyEdge {
            producer_id: e.producer_source,
            consumer_id: e.consumer_source,
            event_type: e.event_type,
            vol: e.vol,
        })
        .collect();
    TopologyData {
        producers,
        consumers,
        edges,
    }
}

/// Re-derives `nodes`/`traces` from the latest snapshot of the three live
/// sources, merging against whatever's currently on screen (see
/// `derive_nodes`/`derive_traces`) so drag positions and manual nodes/wires
/// survive. A plain function (not a closure) so every fetch-completion site
/// can call it the same way without capture/`Copy` gymnastics — Dioxus
/// signals are `Copy`, so passing them by value here is cheap.
fn recompute(
    mut nodes: Signal<Vec<ClNode>>,
    mut traces: Signal<Vec<ClTrace>>,
    processes: &HashMap<String, Process>,
    routes: &[GwRoute],
    topology: &TopologyData,
    saved_positions: &HashMap<String, (f64, f64)>,
) {
    let existing_nodes = nodes.peek().clone();
    let existing_traces = traces.peek().clone();
    let new_nodes = derive_nodes(processes, topology, &existing_nodes, saved_positions);
    let new_traces = derive_traces(&new_nodes, routes, processes, topology, &existing_traces);
    nodes.set(new_nodes);
    traces.set(new_traces);
}

// ── Layout persistence (localStorage + Blueprint KV) ────────────────────────
// Finishes what docs/architecture/cluster-graph-persistence.md started but
// never wired up against this (later, pure-Dioxus) renderer — see
// docs/architecture/cluster-live-topology-implementation-plan.md Phase 5.
// Reuses the existing generic `Value{data: string}` KV shape (JSON-encoded)
// rather than adding a new proto message.

#[derive(Clone, Debug, Default, serde::Serialize, serde::Deserialize)]
struct LayoutBlob {
    positions: HashMap<String, (f64, f64)>,
    pan_x: f64,
    pan_y: f64,
    zoom: f64,
}

fn local_storage() -> Option<web_sys::Storage> {
    web_sys::window()?.local_storage().ok()?
}

fn load_layout_from_local_storage() -> Option<LayoutBlob> {
    let raw = local_storage()?.get_item(LAYOUT_LOCAL_STORAGE_KEY).ok()??;
    serde_json::from_str(&raw).ok()
}

fn save_layout_to_local_storage(layout: &LayoutBlob) {
    if let (Some(storage), Ok(raw)) = (local_storage(), serde_json::to_string(layout)) {
        let _ = storage.set_item(LAYOUT_LOCAL_STORAGE_KEY, &raw);
    }
}

async fn load_layout_from_kv() -> Option<LayoutBlob> {
    let mut client = KeyValueServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
    let resp = client
        .get(GetRequest {
            key: LAYOUT_KV_KEY.to_string(),
            value: Some(Any {
                type_url: LAYOUT_VALUE_TYPE_URL.to_string(),
                value: vec![],
            }),
        })
        .await
        .ok()?;
    let any = resp.into_inner().value?;
    let val = KvValue::decode(any.value.as_slice()).ok()?;
    serde_json::from_str(&val.data).ok()
}

async fn save_layout_to_kv(layout: &LayoutBlob) {
    let Ok(raw) = serde_json::to_string(layout) else {
        return;
    };
    let mut client = KeyValueServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
    let any = Any {
        type_url: LAYOUT_VALUE_TYPE_URL.to_string(),
        value: KvValue { data: raw }.encode_to_vec(),
    };
    let _ = client
        .set(SetRequest {
            key: LAYOUT_KV_KEY.to_string(),
            value: Some(any),
        })
        .await;
}

/// localStorage is written first (fast, always available) then Blueprint KV
/// (source of truth across browsers/sessions) — if the KV call fails
/// (Blueprint down, network error), the localStorage write still landed.
fn persist_layout(layout: LayoutBlob) {
    save_layout_to_local_storage(&layout);
    spawn(async move {
        save_layout_to_kv(&layout).await;
    });
}

// ── CSS for animations + helpers ──────────────────────────────────────────────

const CLUSTER_CSS: &str = r#"
@keyframes cv-flow { from { stroke-dashoffset: 56; } to { stroke-dashoffset: 0; } }
.cv-node { cursor: grab; user-select: none; -webkit-user-select: none; }
.cv-node:active { cursor: grabbing; }
.cv-pad { opacity: 0; transition: opacity .12s; cursor: crosshair; }
.cv-node:hover .cv-pad { opacity: 1; }
.cv-ctl { opacity: 0; transition: opacity .12s; }
.cv-node:hover .cv-ctl { opacity: 1; }
.cv-flow-dot { stroke-dasharray: 4 52; animation: cv-flow 1.3s linear infinite; }
.cv-mi:hover { background: rgba(61,220,151,0.10); color: #3ddc97; }
.cv-mi-danger:hover { background: rgba(229,83,75,0.12); color: #e5534b; }
@media (prefers-reduced-motion: reduce) { .cv-flow-dot { animation: none; } }
"#;

// ─────────────────────────────────────────────────────────────────────────────
// Main Cluster component
// ─────────────────────────────────────────────────────────────────────────────

#[component]
pub fn Cluster() -> Element {
    let mut nodes = use_signal(Vec::<ClNode>::new);
    let mut traces = use_signal(Vec::<ClTrace>::new);
    let mut pan_x = use_signal(|| 0.0_f64);
    let mut pan_y = use_signal(|| 0.0_f64);
    let mut zoom = use_signal(|| 1.0_f64);
    let mut snap = use_signal(|| true);
    let mut flow = use_signal(|| true);
    let mut primary = use_signal(|| "#3ddc97".to_string());
    let mut drag = use_signal(Drag::default);
    let mut menu = use_signal(|| Option::<Menu>::None);
    let mut drawer = use_signal(|| Option::<String>::None);
    let mut coord = use_signal(|| (0.0_f64, 0.0_f64));
    let mut stg_off = use_signal(|| (0.0_f64, 0.0_f64)); // (left, top)
    // Edge labels (eg. an Event trace's topic) render as a hover tooltip
    // instead of always-on text, revealed by hovering either endpoint node
    // -- see TraceSvg's show_tip comment for why hovering the edge line
    // itself isn't also wired up.
    let mut hovered_node = use_signal(|| Option::<String>::None);

    // ── Live data (Registry, Gateway, Topology) ──────────────────────────────
    let mut processes: Signal<HashMap<String, Process>> = use_signal(HashMap::new);
    let mut routes: Signal<Vec<GwRoute>> = use_signal(Vec::new);
    let mut topology_data: Signal<TopologyData> = use_signal(TopologyData::default);
    let mut saved_positions: Signal<HashMap<String, (f64, f64)>> = use_signal(HashMap::new);
    let mut layout_loaded = use_signal(|| false);

    // Load the persisted layout once on mount — Blueprint KV first (source of
    // truth across browsers/sessions), localStorage as a fallback that still
    // works if Blueprint is unreachable. Position lookups in derive_nodes
    // only matter for a node's very first appearance this session, so this
    // must land before the first live-data-driven recompute; layout_loaded
    // gates that first recompute below.
    use_effect(move || {
        spawn(async move {
            let layout = load_layout_from_kv()
                .await
                .or_else(load_layout_from_local_storage);
            if let Some(layout) = layout {
                saved_positions.set(layout.positions);
                pan_x.set(layout.pan_x);
                pan_y.set(layout.pan_y);
                if layout.zoom > 0.0 {
                    zoom.set(layout.zoom);
                }
            }
            layout_loaded.set(true);
        });
    });

    // ── Registry: Query + Watch (mirrors views/service_registry.rs) ─────────
    let sd_service = use_service_discovery_service_service();
    let sd_query_request = use_signal(|| QueryRequest {
        filter: Some(Filter {
            attribute: Some(filter::Attribute::All(String::new())),
        }),
    });
    let sd_query_result = sd_service.query(sd_query_request);

    // Each block below only *updates* its own raw-data signal — recomputation
    // of nodes/traces happens exclusively in the single reactive effect after
    // this section. Keeping those two responsibilities separate avoids a
    // subtle self-triggering loop: this effect both reads and writes
    // `processes`, so if it also reactively read `processes()` again (e.g. to
    // call `recompute` directly) it would re-run itself on every write,
    // forever — `nodes`/`traces` are the only signals the recompute effect
    // writes, and it deliberately never reads them back (see `recompute`'s
    // use of `.peek()`), which is what keeps that effect's dependency set
    // (the raw data signals) disjoint from what it writes.
    use_effect(move || {
        if let Some(Ok(ref resp)) = *sd_query_result.read() {
            processes.set(resp.data.clone());
        }
    });

    let grpc_config = use_context::<dioxus_grpc::GrpcConfig>();
    use_coroutine(move |_rx: UnboundedReceiver<()>| {
        let host = grpc_config.host.clone();
        async move {
            let mut client = ServiceDiscoveryServiceClient::new(WasmClient::new(host));
            let Ok(response) = client.watch(WatchRequest {}).await else {
                return;
            };
            let mut stream = response.into_inner();
            loop {
                match stream.message().await {
                    Ok(Some(msg)) => {
                        if msg.removed {
                            if let Some(process) = msg.process {
                                processes.with_mut(|m| {
                                    m.remove(&process.pid);
                                });
                            }
                        } else if let Some(process) = msg.process {
                            processes.with_mut(|m| {
                                m.insert(process.pid.clone(), process);
                            });
                        }
                    }
                    Ok(None) | Err(_) => break,
                }
            }
        }
    });

    // ── Gateway: polled — ListRoutes has no Watch RPC ────────────────────────
    use_coroutine(move |_rx: UnboundedReceiver<()>| async move {
        loop {
            let mut client =
                NetworkingServiceClient::new(WasmClient::new(crate::FUSE_DOMAIN.clone()));
            if let Ok(resp) = client.list_routes(ListRoutesRequest {}).await {
                routes.set(resp.into_inner().routes);
            }
            TimeoutFuture::new(GATEWAY_POLL_MS).await;
        }
    });

    // ── Event topology: GetTopology + WatchTopology (mirrors views/topology.rs) ─
    use_effect(move || {
        let host = crate::CATALYST_DOMAIN.clone();
        spawn(async move {
            let mut client = TopologyClient::new(WasmClient::new(host.clone()));
            if let Ok(resp) = client.get_topology(GetTopologyRequest {}).await {
                topology_data.set(topology_from_response(resp.into_inner()));
            }
            let Ok(resp) = client.watch_topology(WatchTopologyRequest {}).await else {
                return;
            };
            let mut stream = resp.into_inner();
            loop {
                match stream.message().await {
                    Ok(Some(_)) => {
                        let mut refresh = TopologyClient::new(WasmClient::new(host.clone()));
                        if let Ok(r) = refresh.get_topology(GetTopologyRequest {}).await {
                            topology_data.set(topology_from_response(r.into_inner()));
                        }
                    }
                    Ok(None) | Err(_) => break,
                }
            }
        });
    });

    // The single place nodes/traces get recomputed — reactively depends on
    // every raw-data signal above (and `layout_loaded`) but deliberately
    // never reads `nodes`/`traces` themselves (recompute uses `.peek()` for
    // those), so writing to them here can't re-trigger this same effect.
    use_effect(move || {
        if layout_loaded() {
            recompute(
                nodes,
                traces,
                &processes(),
                &routes(),
                &topology_data(),
                &saved_positions(),
            );
        }
    });

    use_effect(move || {
        use wasm_bindgen::closure::Closure;
        use wasm_bindgen::JsCast;
        if let Some(win) = web_sys::window() {
            if let Some(doc) = win.document() {
                if let Some(el) = doc.get_element_by_id("cv-stage") {
                    let r = el.get_bounding_client_rect();
                    stg_off.set((r.left(), r.top()));
                    let cb = Closure::<dyn FnMut(web_sys::MouseEvent)>::new(
                        |ev: web_sys::MouseEvent| {
                            ev.prevent_default();
                        },
                    );
                    el.add_event_listener_with_callback("contextmenu", cb.as_ref().unchecked_ref())
                        .ok();
                    cb.forget();
                }
            }
        }
    });

    // ── Derived ────────────────────────────────────────────────────────────
    let px = pan_x();
    let py = pan_y();
    let zk = zoom();
    let snap_on = snap();
    let flow_on = flow();
    let pri = primary();
    let (cx, cy) = coord();
    let node_ct = nodes.read().len();
    let trace_ct = traces.read().len();
    let zoom_pct = (zk * 100.0).round() as i32;

    // ── Grid background ───────────────────────────────────────────────────
    let cell = 16.0 * zk;
    let major = 80.0 * zk;
    let gx = ((px % cell) + cell) % cell;
    let gy = ((py % cell) + cell) % cell;
    let gX = ((px % major) + major) % major;
    let gY = ((py % major) + major) % major;
    // Neutral gray grid — no color tint
    let fc = "rgba(255,255,255,0.035)".to_string(); // minor lines
    let mc = "rgba(255,255,255,0.09)".to_string(); // major lines
    let grid_bg = format!(
        "background-color:#0a0d10;\
         background-image:repeating-linear-gradient({mc} 0 1px,transparent 1px 100%),\
         repeating-linear-gradient(90deg,{mc} 0 1px,transparent 1px 100%),\
         repeating-linear-gradient({fc} 0 1px,transparent 1px 100%),\
         repeating-linear-gradient(90deg,{fc} 0 1px,transparent 1px 100%);\
         background-size:{major}px {major}px,{major}px {major}px,{cell}px {cell}px,{cell}px {cell}px;\
         background-position:{gX}px {gY}px,{gX}px {gY}px,{gx}px {gy}px,{gx}px {gy}px;"
    );

    // ── Helpers ───────────────────────────────────────────────────────────
    let to_world = move |sx: f64, sy: f64| -> (f64, f64) {
        let (ol, ot) = stg_off();
        ((sx - ol - pan_x()) / zoom(), (sy - ot - pan_y()) / zoom())
    };
    let node_at = move |wx: f64, wy: f64| -> Option<String> {
        nodes
            .read()
            .iter()
            .find(|n| wx >= n.x && wx <= n.x + NW && wy >= n.y && wy <= n.y + NH)
            .map(|n| n.id.clone())
    };

    // ── Stage pointer handlers ─────────────────────────────────────────────
    let on_stage_down = move |ev: Event<PointerData>| {
        if ev.data().trigger_button() != Some(MouseButton::Primary) {
            return;
        }
        let c = ev.data().client_coordinates();
        drag.set(Drag::Pan {
            sx: c.x,
            sy: c.y,
            px: pan_x(),
            py: pan_y(),
        });
        menu.set(None);
    };

    let on_move = move |ev: Event<PointerData>| {
        let c = ev.data().client_coordinates();
        let (sx, sy) = (c.x, c.y);
        let (wx, wy) = to_world(sx, sy);
        coord.set((wx, wy));
        match drag() {
            Drag::Pan {
                sx: s0x,
                sy: s0y,
                px: p0x,
                py: p0y,
            } => {
                pan_x.set(p0x + sx - s0x);
                pan_y.set(p0y + sy - s0y);
            }
            Drag::Node {
                id,
                sx: s0x,
                sy: s0y,
                nx: n0x,
                ny: n0y,
            } => {
                let zk = zoom();
                let mut nx = n0x + (sx - s0x) / zk;
                let mut ny = n0y + (sy - s0y) / zk;
                if snap() {
                    nx = snap8(nx);
                    ny = snap8(ny);
                }
                nodes.with_mut(|ns| {
                    if let Some(n) = ns.iter_mut().find(|n| n.id == id) {
                        n.x = nx;
                        n.y = ny;
                    }
                });
            }
            Drag::Pad {
                from_id,
                fx,
                fy,
                fd,
                ..
            } => {
                drag.set(Drag::Pad {
                    from_id,
                    fx,
                    fy,
                    fd,
                    tx: wx,
                    ty: wy,
                });
            }
            Drag::None => {}
        }
    };

    let on_up = move |_ev: Event<PointerData>| {
        if let Drag::Pad {
            ref from_id,
            tx,
            ty,
            ..
        } = drag()
        {
            let from_id = from_id.clone();
            if let Some(to_id) = node_at(tx, ty) {
                if to_id != from_id {
                    let dup = traces.read().iter().any(|t| {
                        t.kind == TraceKind::Wire
                            && ((t.from == from_id && t.to == to_id)
                                || (t.from == to_id && t.to == from_id))
                    });
                    if !dup {
                        traces.with_mut(|ts| ts.push(ClTrace::wire(&from_id, &to_id)));
                    }
                }
            }
        }
        // Persist layout on drag-end for either a node move or a pan — this
        // is the only place positions/pan are considered "settled" rather
        // than mid-gesture. Zoom (wheel) isn't persisted on every tick to
        // avoid write-spam; it rides along whenever a drag next settles.
        if matches!(drag(), Drag::Node { .. } | Drag::Pan { .. }) {
            let positions: HashMap<String, (f64, f64)> = nodes
                .read()
                .iter()
                .map(|n| (n.id.clone(), (n.x, n.y)))
                .collect();
            persist_layout(LayoutBlob {
                positions,
                pan_x: pan_x(),
                pan_y: pan_y(),
                zoom: zoom(),
            });
        }
        drag.set(Drag::None);
        if let Some(w) = web_sys::window() {
            if let Some(d) = w.document() {
                if let Some(el) = d.get_element_by_id("cv-stage") {
                    let r = el.get_bounding_client_rect();
                    stg_off.set((r.left(), r.top()));
                }
            }
        }
    };

    let on_wheel = move |ev: Event<WheelData>| {
        let dy = ev.data().delta().strip_units().y;
        // Scale the zoom factor by how far this event actually moved rather
        // than a flat ±10% per event regardless of magnitude — a trackpad
        // fires many more, smaller-delta events per physical scroll gesture
        // than a mouse wheel does, so a flat per-event factor compounded
        // into a much faster-feeling zoom on trackpad. Clamped so a single
        // large mouse-wheel notch still can't jump too far either.
        const ZOOM_SENSITIVITY: f64 = 0.0012;
        let f = (1.0 - dy * ZOOM_SENSITIVITY).clamp(0.9, 1.1);
        let nk = (zoom() * f).clamp(0.25, 3.0);
        let c = ev.data().client_coordinates();
        let (ol, ot) = stg_off();
        let (wx, wy) = ((c.x - ol - pan_x()) / zoom(), (c.y - ot - pan_y()) / zoom());
        pan_x.set(c.x - ol - wx * nk);
        pan_y.set(c.y - ot - wy * nk);
        zoom.set(nk);
    };

    let on_ctx = move |ev: Event<MouseData>| {
        let c = ev.data().client_coordinates();
        let (wx, wy) = to_world(c.x, c.y);
        menu.set(Some(Menu {
            x: c.x,
            y: c.y,
            for_: MenuFor::Canvas { wx, wy },
        }));
    };

    let snap_bdr = if snap_on {
        format!("color-mix(in srgb,{pri} 45%,transparent)")
    } else {
        "rgba(255,255,255,.07)".to_string()
    };
    let snap_col = if snap_on {
        pri.clone()
    } else {
        "#5d6a76".to_string()
    };
    let snap_lbl = if snap_on { "SNAP: ON" } else { "SNAP: OFF" };
    let flow_bdr = if flow_on {
        format!("color-mix(in srgb,{pri} 45%,transparent)")
    } else {
        "rgba(255,255,255,.07)".to_string()
    };
    let flow_col = if flow_on {
        pri.clone()
    } else {
        "#5d6a76".to_string()
    };
    let flow_lbl = if flow_on { "FLOW: ON" } else { "FLOW: OFF" };

    rsx! {
        style { "{CLUSTER_CSS}" }
        div {
            class: "relative flex flex-col overflow-hidden",
            style: "height:calc(100vh - 4rem);background:#0a0d10;font-family:'JetBrains Mono',ui-monospace,'SFMono-Regular',Menlo,Consolas,monospace;font-size:12px;color:#d7e0e8;",

            // ── Topbar ────────────────────────────────────────────────────
            div {
                style: "height:42px;display:flex;align-items:center;gap:18px;padding:0 16px;\
                        background:linear-gradient(180deg,rgba(10,13,16,.96),rgba(10,13,16,.82));\
                        border-bottom:1px solid rgba(255,255,255,.07);flex-shrink:0;z-index:50;",
                // Wordmark
                div { style: "display:flex;align-items:baseline;gap:8px;letter-spacing:.18em;",
                    b { style: "color:{pri};font-weight:700;", "DRAFT" }
                    span { style: "color:#5d6a76;font-size:10px;", "// CLUSTER VIEW" }
                }
                // Color swatches
                div { style: "display:flex;gap:6px;margin-left:6px;",
                    for (col, _) in [("#3ddc97",""), ("#e5534b",""), ("#58a6ff",""), ("#b48cff",""), ("#ffb454","")] {
                        {
                            let c = col.to_string(); let is_on = pri == col;
                            let outline = if is_on { format!("outline:2px solid {col};") } else { String::new() };
                            rsx! {
                                div {
                                    style: "width:14px;height:14px;background:{col};border:1px solid rgba(255,255,255,.07);cursor:pointer;box-sizing:border-box;{outline}",
                                    onclick: move |_| primary.set(c.clone()),
                                }
                            }
                        }
                    }
                }
                div { style: "flex:1;" }
                button {
                    style: "background:none;border:1px solid {snap_bdr};color:{snap_col};font:inherit;font-size:10px;letter-spacing:.12em;padding:5px 10px;cursor:pointer;",
                    onclick: move |_| snap.set(!snap()),
                    "{snap_lbl}"
                }
                button {
                    style: "background:none;border:1px solid {flow_bdr};color:{flow_col};font:inherit;font-size:10px;letter-spacing:.12em;padding:5px 10px;cursor:pointer;",
                    onclick: move |_| flow.set(!flow()),
                    "{flow_lbl}"
                }
                span { style: "color:#5d6a76;font-size:10px;letter-spacing:.12em;", "NODES " b { style: "color:#d7e0e8;", "{node_ct}" } }
                span { style: "color:#5d6a76;font-size:10px;letter-spacing:.12em;", "TRACES " b { style: "color:#d7e0e8;", "{trace_ct}" } }
                span { style: "color:#5d6a76;font-size:10px;letter-spacing:.12em;", "ZOOM " b { style: "color:#d7e0e8;", "{zoom_pct}%" } }
            }

            // ── Stage ──────────────────────────────────────────────────────────
            div {
                id: "cv-stage",
                class: "relative flex-1 overflow-hidden",
                style: "{grid_bg}",
                prevent_default: "oncontextmenu onwheel",
                onpointerdown: on_stage_down,
                onpointermove: on_move,
                onpointerup:   on_up,
                onwheel:       on_wheel,
                oncontextmenu: on_ctx,

                // ── World (pan/zoom container) ────────────────────────────────
                div {
                    style: "position:absolute;left:0;top:0;transform-origin:0 0;\
                            transform:translate({px}px,{py}px) scale({zk});\
                            width:0;height:0;pointer-events:none;",

                    // SVG trace layer — width/height 1 + overflow:visible keeps paths in world space
                    svg {
                        style: "position:absolute;left:0;top:0;pointer-events:none;overflow:visible;",
                        width: "1",
                        height: "1",
                        overflow: "visible",
                        TraceSvg {
                            nodes: nodes(), traces: traces(), primary: pri.clone(), flow_on, drag: drag(),
                            hovered_node: hovered_node(),
                        }
                    }

                    // Node cards
                    for node in nodes() {
                        {
                            let nid      = node.id.clone();
                            let nid_ctx  = node.id.clone();
                            let nid_cfg  = node.id.clone();
                            let nid_del  = node.id.clone();
                            let nid_pad  = node.id.clone();
                            let nid_hov  = node.id.clone();
                            let wc  = node.wire_count(&traces());
                            let ec  = node.event_count(&traces());
                            let nx0 = node.x;
                            let ny0 = node.y;
                            rsx! {
                                NodeCard {
                                    node:        node.clone(),
                                    wire_count:  wc,
                                    event_count: ec,
                                    on_node_down: move |coords: (f64,f64)| {
                                        let cur_x = nodes.read().iter().find(|n|n.id==nid).map(|n|n.x).unwrap_or(nx0);
                                        let cur_y = nodes.read().iter().find(|n|n.id==nid).map(|n|n.y).unwrap_or(ny0);
                                        drag.set(Drag::Node { id: nid.clone(), sx: coords.0, sy: coords.1, nx: cur_x, ny: cur_y });
                                    },
                                    on_context: move |coords: (f64,f64)| {
                                        menu.set(Some(Menu { x: coords.0, y: coords.1, for_: MenuFor::Node(nid_ctx.clone()) }));
                                    },
                                    on_config: move |_| { drawer.set(Some(nid_cfg.clone())); },
                                    on_delete: move |_| {
                                        let id = nid_del.clone();
                                        traces.with_mut(|ts| ts.retain(|t| t.from != id && t.to != id));
                                        nodes.with_mut(|ns| ns.retain(|n| n.id != id));
                                        if drawer() == Some(id) { drawer.set(None); }
                                    },
                                    on_pad_down: move |(side, sx, sy): (i32, f64, f64)| {
                                        let id = nid_pad.clone();
                                        let (fx, fy, fd) = nodes.read().iter().find(|n|n.id==id)
                                            .map(|n| if side==1 { (n.x+NW, n.y+NH/2.0, 1i32) } else { (n.x, n.y+NH/2.0, -1i32) })
                                            .unwrap_or((0.0,0.0,1));
                                        drag.set(Drag::Pad { from_id: id, fx, fy, fd, tx: fx, ty: fy });
                                        let _ = (sx, sy);
                                    },
                                    on_hover: move |entering: bool| {
                                        if entering {
                                            hovered_node.set(Some(nid_hov.clone()));
                                        } else if hovered_node() == Some(nid_hov.clone()) {
                                            hovered_node.set(None);
                                        }
                                    },
                                }
                            }
                        }
                    }
                }

                // ── Context menu ──────────────────────────────────────────────
                if let Some(m) = menu() {
                    CtxMenu {
                        m,
                        nodes:    nodes(),
                        traces:   traces(),
                        snap_on,
                        primary:  pri.clone(),
                        on_close: move |_| menu.set(None),
                        on_add: move |(kind, name, wx, wy): (NodeKind, String, f64, f64)| {
                            nodes.with_mut(|ns| ns.push(ClNode::new(kind, &name, snap8(wx), snap8(wy))));
                            menu.set(None);
                        },
                        on_drawer: move |id: String| { drawer.set(Some(id)); menu.set(None); },
                        on_snap:   move |_| { snap.set(!snap()); menu.set(None); },
                        on_reset:  move |_| { pan_x.set(0.0); pan_y.set(0.0); zoom.set(1.0); menu.set(None); },
                        on_delete: move |id: String| {
                            traces.with_mut(|ts| ts.retain(|t| t.from != id && t.to != id));
                            nodes.with_mut(|ns| ns.retain(|n| n.id != id));
                            menu.set(None);
                        },
                        on_toggle_online: move |id: String| {
                            nodes.with_mut(|ns| { if let Some(n) = ns.iter_mut().find(|n|n.id==id) { n.online = !n.online; } });
                            menu.set(None);
                        },
                        on_disconnect: move |id: String| {
                            traces.with_mut(|ts| ts.retain(|t| t.from != id && t.to != id));
                            menu.set(None);
                        },
                        on_reg_bp: move |svc: String| {
                            if let Some(bp) = nodes.read().iter().find(|n|n.kind==NodeKind::Blueprint).map(|n|n.id.clone()) {
                                let dup = traces.read().iter().any(|t|t.kind==TraceKind::Wire&&((t.from==svc&&t.to==bp)||(t.from==bp&&t.to==svc)));
                                if !dup { traces.with_mut(|ts| ts.push(ClTrace::wire(&svc, &bp))); }
                            }
                            menu.set(None);
                        },
                        on_pub: move |(nid, cat_id, tp): (String, String, String)| {
                            let dup = traces.read().iter().any(|t|t.kind==TraceKind::Event&&t.from==nid&&t.to==cat_id&&t.topic.as_deref()==Some(&tp));
                            if !dup { traces.with_mut(|ts| ts.push(ClTrace::event(&nid, &cat_id, &tp))); }
                            menu.set(None);
                        },
                        on_sub: move |(nid, cat_id, tp): (String, String, String)| {
                            let dup = traces.read().iter().any(|t|t.kind==TraceKind::Event&&t.from==cat_id&&t.to==nid&&t.topic.as_deref()==Some(&tp));
                            if !dup { traces.with_mut(|ts| ts.push(ClTrace::event(&cat_id, &nid, &tp))); }
                            menu.set(None);
                        },
                    }
                }

                // ── Config drawer ─────────────────────────────────────────────
                if let Some(ref nid) = drawer() {
                    if let Some(node) = nodes.read().iter().find(|n|&n.id==nid).cloned() {
                        {
                            let nid_r = node.id.clone();
                            let nid_t = node.id.clone();
                            let nid_e = node.id.clone();
                            let nid_l = node.id.clone();
                            rsx! {
                                ConfigDrawer {
                                    node:    node.clone(),
                                    nodes:   nodes(),
                                    traces:  traces(),
                                    on_close: move |_| drawer.set(None),
                                    on_rules: move |rules: Vec<RoutingRule>| {
                                        nodes.with_mut(|ns| { if let Some(n) = ns.iter_mut().find(|n|n.id==nid_r) { n.rules = rules; } });
                                    },
                                    on_topics: move |topics: Vec<Topic>| {
                                        nodes.with_mut(|ns| { if let Some(n) = ns.iter_mut().find(|n|n.id==nid_t) { n.topics = topics; } });
                                    },
                                    on_endpoint: move |(h,p,proto): (String,u16,String)| {
                                        nodes.with_mut(|ns| { if let Some(n) = ns.iter_mut().find(|n|n.id==nid_e) { n.host=h; n.port=p; n.protocol=proto; } });
                                    },
                                    on_ttl: move |ttl: String| {
                                        nodes.with_mut(|ns| { if let Some(n) = ns.iter_mut().find(|n|n.id==nid_l) { n.ttl=ttl; } });
                                    },
                                    on_rm_trace: move |tid: String| {
                                        traces.with_mut(|ts| ts.retain(|t|t.id!=tid));
                                    },
                                }
                            }
                        }
                    }
                }
            }

            // ── Status bar ─────────────────────────────────────────────────────
            div {
                style: "height:30px;display:flex;align-items:center;gap:22px;padding:0 16px;\
                        background:rgba(10,13,16,.92);border-top:1px solid rgba(255,255,255,.07);\
                        color:#5d6a76;font-size:10px;letter-spacing:.14em;flex-shrink:0;",
                span { style: "min-width:150px;color:{pri};", "X {cx:.0} · Y {cy:.0}" }
                span { style: "opacity:.35;", "|" }
                span { "RIGHT-CLICK · CONTEXT MENU" }
                span { style: "opacity:.35;", "|" }
                span { "DRAG BG · PAN" }
                span { style: "opacity:.35;", "|" }
                span { "SCROLL · ZOOM" }
                span { style: "opacity:.35;", "|" }
                span { "DRAG PAD ▸ NODE · CONNECT" }
                span { style: "opacity:.35;", "|" }
                span { span { style: "color:{pri};", "━" } " WIRE  " span { style: "color:#58a6ff;", "┄▸" } " EVENT" }
            }
        }
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// SVG trace layer
// ─────────────────────────────────────────────────────────────────────────────

#[component]
fn TraceSvg(
    nodes: Vec<ClNode>,
    traces: Vec<ClTrace>,
    primary: String,
    flow_on: bool,
    drag: Drag,
    hovered_node: Option<String>,
) -> Element {
    rsx! {
        for tr in &traces {
            {
                let a = nodes.iter().find(|n| n.id == tr.from);
                let b = nodes.iter().find(|n| n.id == tr.to);
                if let (Some(a), Some(b)) = (a, b) {
                    let (sx,sy,sd) = port_of(a.x, a.y, b.cx());
                    let (ex,ey,ed) = port_of(b.x, b.y, a.cx());
                    let pts = route_pts(sx,sy,sd, ex,ey,ed);
                    let d   = pts_path(&pts);
                    let len = poly_len(&pts);
                    match tr.kind {
                        TraceKind::Wire => {
                            let p  = &primary;
                            let gc = hex_alpha(p, 0.18);
                            let lc = hex_alpha(p, 0.90);
                            rsx! {
                                path { d: "{d}", stroke: "{gc}", stroke_width: "4.5", fill: "none", stroke_linecap: "round", stroke_linejoin: "round" }
                                path { d: "{d}", stroke: "{lc}", stroke_width: "1.5", fill: "none", stroke_linecap: "round", stroke_linejoin: "round" }
                                WirePads { pts: pts.clone(), color: lc.clone() }
                                ViaDots  { pts: pts.clone(), color: p.clone() }
                            }
                        }
                        TraceKind::Event => {
                            let col  = "#58a6ff";
                            let gc   = hex_alpha(col, 0.13);
                            let lc   = hex_alpha(col, 0.80);
                            let topic = tr.topic.clone().unwrap_or_default().to_uppercase();
                            let last  = *pts.last().unwrap_or(&(0.0,0.0));
                            let first = pts[0];
                            let mid   = if len > 0.0 { point_at(&pts, len/2.0) } else { (0.0,0.0,0.0) };
                            // Shown as a hover tooltip, not always-on text -- reveal it when the
                            // pointer is over either endpoint node. Hovering the edge line itself
                            // was attempted (an invisible hit overlay, both as raw SVG shapes and
                            // as absolutely-positioned divs, matching NodeCard's own working
                            // pattern) and dropped: no event type ever reached it despite
                            // confirmed-correct geometry and hit-testing (elementFromPoint agreed
                            // every time) -- an unresolved Dioxus-web limitation for this kind of
                            // dynamically-generated element, not a geometry or code bug. Every
                            // edge has two endpoint nodes, so hovering either one already reveals
                            // it.
                            let show_tip = hovered_node.as_deref() == Some(tr.from.as_str())
                                || hovered_node.as_deref() == Some(tr.to.as_str());
                            rsx! {
                                path { d: "{d}", stroke: "{gc}", stroke_width: "4.5", fill: "none", stroke_linecap: "round", stroke_linejoin: "round" }
                                path { d: "{d}", stroke: "{lc}", stroke_width: "1.3", fill: "none", stroke_dasharray: "6 5", stroke_linecap: "round" }
                                if flow_on { path { class: "cv-flow-dot", d: "{d}", stroke: "{col}", stroke_width: "2.5", fill: "none" } }
                                Chevrons { pts: pts.clone(), color: col.to_string() }
                                ViaDots  { pts: pts.clone(), color: col.to_string() }
                                // Producer pad (filled square)
                                rect { x: "{first.0-3.5:.1}", y: "{first.1-3.5:.1}", width: "7", height: "7", fill: "{col}", opacity: "0.95" }
                                // Consumer pad (open ring)
                                circle { cx: "{last.0:.1}", cy: "{last.1:.1}", r: "3.5", stroke: "{col}", stroke_width: "1.5", fill: "none", opacity: "0.95" }
                                // Topic label
                                if !topic.is_empty() && show_tip {
                                    rect { x: "{mid.0-30.0:.1}", y: "{mid.1-7.0:.1}", width: "60", height: "13", fill: "#0a0d10" }
                                    rect { x: "{mid.0-29.5:.1}", y: "{mid.1-6.5:.1}", width: "59", height: "12", fill: "none", stroke: "{hex_alpha(col,0.4)}", stroke_width: "1" }
                                    text {
                                        x: "{mid.0:.1}", y: "{mid.1+1.0:.1}",
                                        text_anchor: "middle", dominant_baseline: "middle",
                                        fill: "{hex_alpha(col,0.95)}",
                                        font_family: "'JetBrains Mono',ui-monospace,monospace",
                                        font_size: "7", letter_spacing: "0.05em",
                                        "{topic}"
                                    }
                                }
                            }
                        }
                    }
                } else { rsx! {} }
            }
        }
        // Ghost trace while dragging pad
        if let Drag::Pad { fx, fy, fd, tx, ty, .. } = drag {
            {
                let pts = route_pts(fx,fy,fd, tx,ty, if tx>=fx { -1 } else { 1 });
                let d   = pts_path(&pts);
                rsx! { path { d: "{d}", stroke: "rgba(61,220,151,0.55)", stroke_width: "1.5", fill: "none", stroke_dasharray: "5 4", stroke_linecap: "round" } }
            }
        }
    }
}

#[component]
fn WirePads(pts: Vec<(f64, f64)>, color: String) -> Element {
    if pts.len() < 2 {
        return rsx! {};
    }
    let (s, e) = (pts[0], *pts.last().unwrap());
    rsx! {
        rect { x: "{s.0-3.5:.1}", y: "{s.1-3.5:.1}", width: "7", height: "7", fill: "{color}" }
        rect { x: "{e.0-3.5:.1}", y: "{e.1-3.5:.1}", width: "7", height: "7", fill: "{color}" }
    }
}

#[component]
fn ViaDots(pts: Vec<(f64, f64)>, color: String) -> Element {
    let outer = hex_alpha(&color, 0.95);
    if pts.len() < 3 {
        return rsx! {};
    }
    rsx! {
        for &(vx,vy) in pts[1..pts.len()-1].iter() {
            circle { cx: "{vx:.1}", cy: "{vy:.1}", r: "2.6", fill: "{outer}" }
            circle { cx: "{vx:.1}", cy: "{vy:.1}", r: "1.1", fill: "#0a0d10" }
        }
    }
}

#[component]
fn Chevrons(pts: Vec<(f64, f64)>, color: String) -> Element {
    let len = poly_len(&pts);
    let mut positions: Vec<(f64, f64, f64)> = Vec::new();
    let mut d = 34.0;
    while d < len - 26.0 {
        positions.push(point_at(&pts, d));
        d += 64.0;
    }
    rsx! {
        for (cx, cy, angle) in positions {
            { let deg = angle.to_degrees();
              rsx! { path { d: "M -3,-3.2 L 1.6,0 L -3,3.2", transform: "translate({cx:.1},{cy:.1}) rotate({deg:.1})", stroke: "{color}", stroke_width: "1.4", fill: "none", opacity: "0.85" } }
            }
        }
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// Node card
// ─────────────────────────────────────────────────────────────────────────────

#[component]
fn NodeCard(
    node: ClNode,
    wire_count: usize,
    event_count: usize,
    on_node_down: EventHandler<(f64, f64)>,
    on_context: EventHandler<(f64, f64)>,
    on_config: EventHandler<()>,
    on_delete: EventHandler<()>,
    on_pad_down: EventHandler<(i32, f64, f64)>,
    on_hover: EventHandler<bool>,
) -> Element {
    let col = node.kind.color();
    let sym = node.kind.sym();
    let el = node.kind.el();
    let num = node.kind.num();
    let role = node.kind.role();
    let name = node.name.to_uppercase();
    let led_style = if node.online {
        "background:#3ddc97;box-shadow:0 0 6px #3ddc97;"
    } else {
        "background:#e5534b;box-shadow:0 0 6px #e5534b;"
    };
    let st = if node.online { "ONLINE" } else { "OFFLINE" };
    let links = {
        let mut p = Vec::new();
        if wire_count > 0 {
            p.push(format!(
                "{wire_count} LINK{}",
                if wire_count > 1 { "S" } else { "" }
            ));
        }
        if event_count > 0 {
            p.push(format!("{event_count} EVT"));
        }
        if p.is_empty() {
            "NO LINKS".into()
        } else {
            p.join(" · ")
        }
    };
    let bk_base = format!("position:absolute;width:9px;height:9px;border-color:{col};opacity:.9;");
    let bk_tl = format!("{bk_base}top:-1px;left:-1px;border-top:2px solid;border-left:2px solid;");
    let bk_tr =
        format!("{bk_base}top:-1px;right:-1px;border-top:2px solid;border-right:2px solid;");
    let bk_bl =
        format!("{bk_base}bottom:-1px;left:-1px;border-bottom:2px solid;border-left:2px solid;");
    let bk_br =
        format!("{bk_base}bottom:-1px;right:-1px;border-bottom:2px solid;border-right:2px solid;");
    // Manual nodes get a dashed border so it's obvious at a glance which
    // nodes on screen are real (solid) vs. hand-added annotations (dashed).
    let border = if node.origin == NodeOrigin::Manual {
        "border:1px dashed rgba(255,255,255,.22);"
    } else {
        "border:1px solid rgba(255,255,255,.07);"
    };

    rsx! {
        div {
            class: "cv-node",
            style: "position:absolute;left:{node.x}px;top:{node.y}px;width:{NW}px;min-height:{NH}px;\
                    background:linear-gradient(180deg,#10161c,#0e1318);{border}\
                    padding:10px 12px 8px;box-sizing:border-box;pointer-events:auto;",
            prevent_default: "oncontextmenu",
            onpointerdown: move |ev| {
                if ev.data().trigger_button() != Some(MouseButton::Primary) { return; }
                ev.stop_propagation();
                let c = ev.data().client_coordinates();
                on_node_down.call((c.x, c.y));
            },
            oncontextmenu: move |ev| {
                ev.stop_propagation();
                ev.prevent_default();
                let c = ev.data().client_coordinates();
                on_context.call((c.x, c.y));
            },
            onmouseenter: move |_| on_hover.call(true),
            onmouseleave: move |_| on_hover.call(false),

            // Bracket corners
            div { style: "{bk_tl}" }
            div { style: "{bk_tr}" }
            div { style: "{bk_bl}" }
            div { style: "{bk_br}" }

            // Head
            div { style: "display:flex;gap:10px;align-items:flex-start;",
                div {
                    style: "flex:0 0 36px;height:40px;border:1px solid {col};\
                            display:flex;flex-direction:column;align-items:center;justify-content:center;\
                            position:relative;color:{col};background:color-mix(in srgb,{col} 7%,transparent);",
                    span { style: "position:absolute;top:1px;left:3px;font-size:7px;opacity:.75;", "{num}" }
                    span { style: "font-size:15px;font-weight:700;line-height:1;", "{sym}" }
                    span { style: "font-size:6px;letter-spacing:.08em;margin-top:3px;opacity:.8;", "{el}" }
                }
                div { style: "flex:1;min-width:0;",
                    div { style: "font-size:12px;font-weight:600;letter-spacing:.08em;color:#d7e0e8;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;", "{name}" }
                    div { style: "font-size:8px;letter-spacing:.2em;color:#5d6a76;margin-top:4px;", "{role}" }
                }
            }

            // Meta
            div { style: "display:flex;align-items:center;gap:7px;margin-top:8px;font-size:8px;letter-spacing:.16em;color:#5d6a76;",
                span { style: "width:5px;height:5px;border-radius:50%;display:inline-block;{led_style}" }
                span { "{st}" }
                span { style: "margin-left:auto;color:{col};", "{links}" }
            }

            // Corner controls (CSS hover reveal)
            div {
                class: "cv-ctl",
                style: "position:absolute;top:-9px;right:6px;display:flex;gap:4px;",
                button {
                    style: "width:18px;height:18px;background:#0e1318;border:1px solid rgba(255,255,255,.07);\
                            color:#5d6a76;font:inherit;font-size:10px;line-height:1;cursor:pointer;",
                    onclick: move |_| on_config.call(()),
                    title: "configure",
                    "≡"
                }
                button {
                    style: "width:18px;height:18px;background:#0e1318;border:1px solid rgba(255,255,255,.07);\
                            color:#5d6a76;font:inherit;font-size:10px;line-height:1;cursor:pointer;",
                    onclick: move |_| on_delete.call(()),
                    title: "remove",
                    "×"
                }
            }

            // Left pad
            div {
                class: "cv-pad",
                style: "position:absolute;width:9px;height:9px;background:#0a0d10;\
                        border:1.5px solid {col};left:-5px;top:50%;transform:translateY(-50%);z-index:5;",
                onpointerdown: move |ev| {
                    ev.stop_propagation();
                    let c = ev.data().client_coordinates();
                    on_pad_down.call((-1, c.x, c.y));
                },
            }
            // Right pad
            div {
                class: "cv-pad",
                style: "position:absolute;width:9px;height:9px;background:#0a0d10;\
                        border:1.5px solid {col};right:-5px;top:50%;transform:translateY(-50%);z-index:5;",
                onpointerdown: move |ev| {
                    ev.stop_propagation();
                    let c = ev.data().client_coordinates();
                    on_pad_down.call((1, c.x, c.y));
                },
            }
        }
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// Context menu
// ─────────────────────────────────────────────────────────────────────────────

#[component]
fn CtxMenu(
    m: Menu,
    nodes: Vec<ClNode>,
    traces: Vec<ClTrace>,
    snap_on: bool,
    primary: String,
    on_close: EventHandler<()>,
    on_add: EventHandler<(NodeKind, String, f64, f64)>,
    on_drawer: EventHandler<String>,
    on_snap: EventHandler<()>,
    on_reset: EventHandler<()>,
    on_delete: EventHandler<String>,
    on_toggle_online: EventHandler<String>,
    on_disconnect: EventHandler<String>,
    on_reg_bp: EventHandler<String>,
    on_pub: EventHandler<(String, String, String)>,
    on_sub: EventHandler<(String, String, String)>,
) -> Element {
    let (mx, my) = (m.x, m.y);
    let mi = "display:flex;align-items:center;gap:10px;padding:6px 12px;cursor:pointer;color:#d7e0e8;font-size:11px;letter-spacing:.06em;";
    let hd = "padding:7px 12px 6px;font-size:9px;letter-spacing:.2em;color:#5d6a76;border-bottom:1px solid rgba(255,255,255,.07);margin-bottom:4px;display:flex;gap:8px;align-items:center;";
    let dv = "height:1px;background:rgba(255,255,255,.07);margin:4px 0;";

    let body: Element = match m.for_ {
        MenuFor::Canvas { wx, wy } => {
            let n = uid_val();
            let cat_name = format!("catalyst-{n:02}");
            let bp_name = format!("blueprint-{n:02}");
            let fuse_name = format!("fuse-{n:02}");
            let svc_name = format!("svc-{n:02}");
            let snap_lbl = if snap_on {
                "SNAP TO GRID · ON"
            } else {
                "SNAP TO GRID · OFF"
            };
            rsx! {
                div { style: "{hd}", span { style: "color:{primary};font-weight:700;", "◈" } span { "CANVAS" } }
                div { class: "cv-mi", style: "{mi}", onclick: move |_| on_add.call((NodeKind::Catalyst,  cat_name.clone(),  wx, wy)), "ADD · CATALYST" }
                div { class: "cv-mi", style: "{mi}", onclick: move |_| on_add.call((NodeKind::Blueprint, bp_name.clone(),   wx, wy)), "ADD · BLUEPRINT" }
                div { class: "cv-mi", style: "{mi}", onclick: move |_| on_add.call((NodeKind::Fuse,      fuse_name.clone(), wx, wy)), "ADD · FUSE" }
                div { class: "cv-mi", style: "{mi}", onclick: move |_| on_add.call((NodeKind::Service,   svc_name.clone(),  wx, wy)), "ADD · SERVICE" }
                div { style: "{dv}" }
                div { class: "cv-mi", style: "{mi}", onclick: move |_| on_snap.call(()), "{snap_lbl}" }
                div { class: "cv-mi", style: "{mi}", onclick: move |_| on_reset.call(()), "RESET VIEW" }
            }
        }
        MenuFor::Node(ref nid) => {
            if let Some(node) = nodes.iter().find(|n| &n.id == nid).cloned() {
                let col = node.kind.color();
                let sym = node.kind.sym();
                let title = format!("{} · {}", node.name.to_uppercase(), node.kind.role());
                let nid = nid.clone();
                let nid1 = nid.clone();
                let nid2 = nid.clone();
                let nid3 = nid.clone();
                let online_lbl = if node.online {
                    "MARK OFFLINE"
                } else {
                    "MARK ONLINE"
                };

                let type_items: Element = match node.kind {
                    NodeKind::Fuse => {
                        let rc = node.rules.len();
                        let id = nid.clone();
                        rsx! {
                            div { class: "cv-mi", style: "{mi}", onclick: move |_| on_drawer.call(id.clone()),
                                "ROUTING RULES…" span { style: "margin-left:auto;color:#5d6a76;font-size:9px;", "{rc}" }
                            }
                        }
                    }
                    NodeKind::Catalyst => {
                        let tc = node.topics.len();
                        let id = nid.clone();
                        rsx! {
                            div { class: "cv-mi", style: "{mi}", onclick: move |_| on_drawer.call(id.clone()),
                                "TOPICS…" span { style: "margin-left:auto;color:#5d6a76;font-size:9px;", "{tc}" }
                            }
                        }
                    }
                    NodeKind::Blueprint => {
                        let id = nid.clone();
                        rsx! { div { class: "cv-mi", style: "{mi}", onclick: move |_| on_drawer.call(id.clone()), "SERVICE REGISTRY…" } }
                    }
                    NodeKind::Service => {
                        let cats: Vec<(String, Vec<String>)> = nodes
                            .iter()
                            .filter(|n| n.kind == NodeKind::Catalyst)
                            .map(|c| {
                                (
                                    c.id.clone(),
                                    c.topics.iter().map(|t| t.name.clone()).collect(),
                                )
                            })
                            .collect();
                        let has_bp = nodes.iter().any(|n| n.kind == NodeKind::Blueprint);
                        let id = nid.clone();
                        let id_ep = nid.clone();
                        let id_bp = nid.clone();
                        rsx! {
                            div { class: "cv-mi", style: "{mi}", onclick: move |_| on_drawer.call(id_ep.clone()), "ENDPOINT CONFIG…" }
                            for (cat_id, topics) in cats.clone() {
                                for tp in topics {
                                    {
                                        let n = id.clone(); let c = cat_id.clone(); let t = tp.clone();
                                        let lbl = format!("PUB · {}", tp.to_uppercase());
                                        rsx! { div { class: "cv-mi", style: "{mi}", onclick: move |_| on_pub.call((n.clone(), c.clone(), t.clone())), "{lbl}" } }
                                    }
                                }
                            }
                            for (cat_id, topics) in cats {
                                for tp in topics {
                                    {
                                        let n = id.clone(); let c = cat_id.clone(); let t = tp.clone();
                                        let lbl = format!("SUB · {}", tp.to_uppercase());
                                        rsx! { div { class: "cv-mi", style: "{mi}", onclick: move |_| on_sub.call((n.clone(), c.clone(), t.clone())), "{lbl}" } }
                                    }
                                }
                            }
                            if has_bp {
                                div { class: "cv-mi", style: "{mi}", onclick: move |_| on_reg_bp.call(id_bp.clone()), "REGISTER WITH BLUEPRINT" }
                            }
                        }
                    }
                };

                let is_live = node.origin == NodeOrigin::Live;
                rsx! {
                    div { style: "{hd}", span { style: "color:{col};font-weight:700;", "{sym}" } span { "{title}" } }
                    {type_items}
                    div { style: "{dv}" }
                    if is_live {
                        // Live nodes reflect real cluster state — their existence and
                        // online/offline status come from the Registry, not the canvas.
                        div { style: "padding:6px 12px;font-size:9px;letter-spacing:.1em;color:#5d6a76;line-height:1.6;",
                            "LIVE NODE · TRACKS THE SERVICE REGISTRY"
                        }
                    } else {
                        div { class: "cv-mi", style: "{mi}", onclick: move |_| on_toggle_online.call(nid1.clone()), "{online_lbl}" }
                        div { class: "cv-mi", style: "{mi}", onclick: move |_| on_disconnect.call(nid2.clone()), "DISCONNECT ALL" }
                        div { style: "{dv}" }
                        div { class: "cv-mi cv-mi-danger", style: "{mi}color:#e5534b;",
                            onclick: move |_| on_delete.call(nid3.clone()), "REMOVE NODE"
                        }
                    }
                }
            } else {
                rsx! {}
            }
        }
    };

    rsx! {
        div { style: "position:fixed;inset:0;z-index:99;", onclick: move |_| on_close.call(()), }
        div {
            style: "position:fixed;left:{mx}px;top:{my}px;z-index:100;min-width:208px;\
                    background:rgba(13,18,23,.97);border:1px solid rgba(255,255,255,.07);\
                    box-shadow:0 12px 32px rgba(0,0,0,.55);padding:4px 0;",
            onclick: move |ev| ev.stop_propagation(),
            {body}
        }
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// Config drawer
// ─────────────────────────────────────────────────────────────────────────────

#[component]
fn ConfigDrawer(
    node: ClNode,
    nodes: Vec<ClNode>,
    traces: Vec<ClTrace>,
    on_close: EventHandler<()>,
    on_rules: EventHandler<Vec<RoutingRule>>,
    on_topics: EventHandler<Vec<Topic>>,
    on_endpoint: EventHandler<(String, u16, String)>,
    on_ttl: EventHandler<String>,
    on_rm_trace: EventHandler<String>,
) -> Element {
    let col = node.kind.color();
    let sym = node.kind.sym();
    let role = format!("{} · {}", node.kind.el(), node.kind.role());
    let name = node.name.to_uppercase();

    let fld  = "width:100%;background:#0a0f13;border:1px solid rgba(255,255,255,.07);color:#d7e0e8;font:inherit;font-size:10px;padding:5px 6px;outline:none;box-sizing:border-box;";
    let sec = "font-size:9px;letter-spacing:.22em;color:#5d6a76;margin:14px 0 8px;display:block;";
    let note = format!("margin-top:14px;padding:9px 10px;border-left:2px solid {col};background:rgba(255,255,255,.025);font-size:9px;line-height:1.6;letter-spacing:.06em;color:#5d6a76;");
    let add  = "width:100%;margin-top:10px;background:none;cursor:pointer;font:inherit;border:1px dashed rgba(61,220,151,.45);color:#3ddc97;font-size:10px;letter-spacing:.14em;padding:7px;";
    let kv =
        "display:grid;grid-template-columns:86px 1fr;gap:8px;align-items:center;margin-bottom:8px;";
    let lbl = "font-size:9px;letter-spacing:.16em;color:#5d6a76;";
    let th   = "font-size:8px;letter-spacing:.18em;color:#5d6a76;text-align:left;padding:4px 6px;border-bottom:1px solid rgba(255,255,255,.07);font-weight:500;";
    let td = "padding:4px 3px;border-bottom:1px solid rgba(255,255,255,.04);";

    let body: Element = match node.kind {
        NodeKind::Fuse => {
            let rules = node.rules.clone();
            let targets: Vec<String> = nodes
                .iter()
                .filter(|n| n.id != node.id && n.kind != NodeKind::Fuse)
                .map(|n| n.name.clone())
                .collect();
            rsx! {
                span { style: "{sec}", "ROUTING RULES" }
                table { style: "width:100%;border-collapse:collapse;",
                    tr {
                        th { style: "{th}width:62px;", "METHOD" }
                        th { style: "{th}", "PATH PREFIX" }
                        th { style: "{th}width:88px;", "TARGET" }
                        th { style: "width:18px;border-bottom:1px solid rgba(255,255,255,.07);" }
                    }
                    for (i, rule) in rules.iter().enumerate() {
                        {
                            let r0 = rules.clone(); let r1 = rules.clone(); let r2 = rules.clone(); let r_del = rules.clone();
                            let t0 = targets.clone();
                            let (method, path, target) = (rule.method.clone(), rule.path.clone(), rule.target.clone());
                            rsx! {
                                tr {
                                    td { style: "{td}", select { style: "{fld}", value: "{method}",
                                        onchange: { let mut a=r0; move |ev:Event<FormData>| { a[i].method=ev.data().value(); on_rules.call(a.clone()); } },
                                        for m in ["GET","POST","PUT","DELETE","*"] { option { value:"{m}", selected: method==m, "{m}" } }
                                    }}
                                    td { style: "{td}", input { style: "{fld}", value: "{path}",
                                        oninput: { let mut a=r1; move |ev:Event<FormData>| { a[i].path=ev.data().value(); on_rules.call(a.clone()); } }
                                    }}
                                    td { style: "{td}", select { style: "{fld}", value: "{target}",
                                        onchange: { let mut a=r2; move |ev:Event<FormData>| { a[i].target=ev.data().value(); on_rules.call(a.clone()); } },
                                        if !t0.contains(&target) { option { value: "{target}", "{target}" } }
                                        for t in &t0 { option { value: "{t}", selected: *t==target, "{t}" } }
                                    }}
                                    td { style: "{td}", button { style: "background:none;border:none;color:#5d6a76;cursor:pointer;font:inherit;",
                                        onclick: { let mut a=r_del; move |_| { a.remove(i); on_rules.call(a.clone()); } }, "×"
                                    }}
                                }
                            }
                        }
                    }
                }
                button { style: "{add}",
                    onclick: { let mut a=rules; let tgt=targets.first().cloned().unwrap_or_else(||"svc".into());
                               move |_| { a.push(RoutingRule{method:"GET".into(),path:"/api/".into(),target:tgt.clone()}); on_rules.call(a.clone()); } },
                    "+ ADD RULE"
                }
                div { style: "{note}", "RULES RESOLVE TARGETS THROUGH BLUEPRINT. FIRST MATCH WINS." }
            }
        }
        NodeKind::Catalyst => {
            let topics = node.topics.clone();
            let evs: Vec<ClTrace> = traces
                .iter()
                .filter(|t| t.kind == TraceKind::Event && (t.from == node.id || t.to == node.id))
                .cloned()
                .collect();
            let nid = node.id.clone();
            rsx! {
                span { style: "{sec}", "TOPICS" }
                table { style: "width:100%;border-collapse:collapse;",
                    tr {
                        th { style: "{th}", "NAME" }
                        th { style: "{th}width:64px;", "RETAIN" }
                        th { style: "{th}width:46px;", "PARTS" }
                        th { style: "width:18px;border-bottom:1px solid rgba(255,255,255,.07);" }
                    }
                    for (i, tp) in topics.iter().enumerate() {
                        {
                            let t0=topics.clone(); let t1=topics.clone(); let t2=topics.clone(); let t_del=topics.clone();
                            let (tname, tret, tpart) = (tp.name.clone(), tp.retention.clone(), tp.partitions);
                            rsx! {
                                tr {
                                    td { style: "{td}", input { style: "{fld}", value: "{tname}", oninput: { let mut a=t0; move |ev:Event<FormData>| { a[i].name=ev.data().value(); on_topics.call(a.clone()); } } }}
                                    td { style: "{td}", input { style: "{fld}", value: "{tret}",  oninput: { let mut a=t1; move |ev:Event<FormData>| { a[i].retention=ev.data().value(); on_topics.call(a.clone()); } } }}
                                    td { style: "{td}", input { style: "{fld}", r#type:"number", value: "{tpart}", oninput: { let mut a=t2; move |ev:Event<FormData>| { if let Ok(v)=ev.data().value().parse() { a[i].partitions=v; on_topics.call(a.clone()); } } } }}
                                    td { style: "{td}", button { style: "background:none;border:none;color:#5d6a76;cursor:pointer;font:inherit;",
                                        onclick: { let mut a=t_del; move |_| { a.remove(i); on_topics.call(a.clone()); } }, "×"
                                    }}
                                }
                            }
                        }
                    }
                }
                button { style: "{add}",
                    onclick: { let mut a=topics; move |_| { a.push(Topic{name:"new.topic".into(),retention:"24h".into(),partitions:1}); on_topics.call(a.clone()); } },
                    "+ ADD TOPIC"
                }
                span { style: "{sec}", "PRODUCERS / CONSUMERS" }
                if evs.is_empty() {
                    div { style: "{note}", "NO EVENT LINKS. RIGHT-CLICK A SERVICE ▸ PUB / SUB." }
                }
                for ev_tr in evs {
                    {
                        let is_sub  = ev_tr.from == nid;
                        let other   = nodes.iter().find(|n| n.id == if is_sub { ev_tr.to.clone() } else { ev_tr.from.clone() }).map(|n| n.name.to_uppercase()).unwrap_or_default();
                        let tp_name = ev_tr.topic.clone().unwrap_or_default();
                        let tid     = ev_tr.id.clone();
                        let tag     = if is_sub { "SUB" } else { "PUB" };
                        rsx! {
                            div { style: "display:flex;align-items:center;gap:9px;padding:7px 2px;border-bottom:1px solid rgba(255,255,255,.04);font-size:10px;",
                                span { style: "color:#58a6ff;font-size:8px;letter-spacing:.12em;", "{tag}" }
                                span { "{other}" }
                                span { style: "margin-left:auto;color:#5d6a76;font-size:9px;", "{tp_name}" }
                                button { style: "background:none;border:none;color:#5d6a76;cursor:pointer;font:inherit;",
                                    onclick: move |_| on_rm_trace.call(tid.clone()), "×"
                                }
                            }
                        }
                    }
                }
                div { style: "{note}", "TOPICS PERSIST AS CLOUDEVENTS. CONSUMERS TRACK PER-DESTINATION CURSORS." }
            }
        }
        NodeKind::Blueprint => {
            let listed: Vec<ClNode> = nodes.iter().filter(|n| n.id != node.id).cloned().collect();
            let ttl = node.ttl.clone();
            rsx! {
                span { style: "{sec}", "SERVICE REGISTRY" }
                if listed.is_empty() {
                    div { style: "{note}", "NO SERVICES ON CANVAS. ADD NODES TO POPULATE THE REGISTRY." }
                }
                for svc in listed {
                    {
                        let led = if svc.online { "background:#3ddc97;box-shadow:0 0 5px #3ddc97;" } else { "background:#e5534b;box-shadow:0 0 5px #e5534b;" };
                        let addr = if svc.host.is_empty() { format!("{}.cluster.local", svc.name) } else { svc.host.clone() };
                        rsx! {
                            div { style: "display:flex;align-items:center;gap:9px;padding:7px 2px;border-bottom:1px solid rgba(255,255,255,.04);font-size:10px;",
                                span { style: "width:5px;height:5px;border-radius:50%;display:inline-block;{led}" }
                                span { "{svc.name.to_uppercase()}" }
                                span { style: "margin-left:auto;color:#5d6a76;font-size:9px;", "{svc.kind.sym()} · {addr}" }
                            }
                        }
                    }
                }
                span { style: "{sec}", "HEALTH CHECKS" }
                div { style: "{kv}", label { style: "{lbl}", "LEASE TTL" } input { style: "{fld}", value: "{ttl}", oninput: move |ev:Event<FormData>| on_ttl.call(ev.data().value()) } }
            }
        }
        NodeKind::Service => {
            let (host, port, proto) = (node.host.clone(), node.port, node.protocol.clone());
            rsx! {
                span { style: "{sec}", "ENDPOINT" }
                div { style: "{kv}", label { style: "{lbl}", "HOST" }
                    input { style: "{fld}", value: "{host}",
                        oninput: { let p=port; let pr=proto.clone(); move |ev:Event<FormData>| on_endpoint.call((ev.data().value(), p, pr.clone())) }
                    }
                }
                div { style: "{kv}", label { style: "{lbl}", "PORT" }
                    input { style: "{fld}", r#type:"number", value: "{port}",
                        oninput: { let h=host.clone(); let pr=proto.clone(); move |ev:Event<FormData>| { if let Ok(p)=ev.data().value().parse() { on_endpoint.call((h.clone(),p,pr.clone())); } } }
                    }
                }
                div { style: "{kv}", label { style: "{lbl}", "PROTOCOL" }
                    select { style: "{fld}", value: "{proto}",
                        onchange: { let h=host.clone(); let p=port; move |ev:Event<FormData>| on_endpoint.call((h.clone(),p,ev.data().value())) },
                        for opt in ["grpc","grpc-web","http"] { option { value:"{opt}", selected: proto==opt, "{opt}" } }
                    }
                }
                div { style: "{note}", "ON DEPLOY, THIS SERVICE SELF-REGISTERS WITH BLUEPRINT AND BECOMES ROUTABLE THROUGH FUSE." }
            }
        }
    };

    rsx! {
        div {
            style: "position:absolute;top:0;right:0;bottom:0;width:330px;z-index:60;\
                    background:rgba(12,17,22,.97);border-left:1px solid rgba(255,255,255,.07);\
                    display:flex;flex-direction:column;pointer-events:auto;",
            // Header
            div { style: "padding:14px 16px;border-bottom:1px solid rgba(255,255,255,.07);display:flex;align-items:center;gap:10px;",
                span { style: "color:{col};font-weight:700;", "{sym}" }
                div {
                    div { style: "font-size:11px;letter-spacing:.14em;", "{name}" }
                    div { style: "font-size:8px;letter-spacing:.2em;color:#5d6a76;margin-top:3px;", "{role}" }
                }
                button {
                    style: "margin-left:auto;background:none;border:1px solid rgba(255,255,255,.07);color:#5d6a76;width:22px;height:22px;cursor:pointer;font:inherit;",
                    onclick: move |_| on_close.call(()),
                    "×"
                }
            }
            // Body (scrollable)
            div { style: "flex:1;overflow-y:auto;padding:14px 16px;", {body} }
        }
    }
}
