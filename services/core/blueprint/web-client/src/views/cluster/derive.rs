#![allow(unused_imports)]
use super::*;

// ── Live data → graph derivation ────────────────────────────────────────────
// See docs/architecture/cluster-live-topology-implementation-plan.md for the
// full rationale behind the identity join and edge derivation below.

/// Recovers a logical service name from a CloudEvent `source` string (e.g.
/// `/services/bench` -> `bench`), the convention documented in
/// docs/architecture/wide-events.md and used by every real producer in this
/// repo. Falls back to the raw source when it doesn't match the convention,
/// so an unresolved topology node is still shown rather than dropped.
pub(super) fn service_name_from_source(source: &str) -> &str {
    source
        .strip_prefix("/services/")
        .or_else(|| source.strip_prefix("/plugins/"))
        .unwrap_or(source)
}

pub(super) fn kind_for_name(name: &str) -> NodeKind {
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
pub(super) fn auto_position(kind: &NodeKind, service_index: usize) -> (f64, f64) {
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
pub(super) fn derive_nodes(
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
pub(super) fn derive_traces(
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
pub(super) fn topology_from_response(resp: GetTopologyResponse) -> TopologyData {
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
pub(super) fn recompute(
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
