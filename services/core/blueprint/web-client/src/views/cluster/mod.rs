#![allow(unused_imports)]
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

use crate::topology::{TopologyData, TopologyEdge, TopologyNode};
use draft_ui::shell::{use_page_chrome, BarItem};
use draft_ui::ui::{Btn, BtnSize, BtnVariant, Glyph, Status, Toggle};
use draft_ui::StatusKind;

/// The KV key layout positions are persisted under — a plain JSON-encoded
/// `LayoutBlob`, using the same generic `Value{data: string}` shape
/// `leader`/`fuse_address` already use elsewhere in this store (see
/// docs/architecture/cluster-live-topology-implementation-plan.md's
/// "no new proto messages" non-goal).
pub(super) const LAYOUT_KV_KEY: &str = "cluster/layout";
pub(super) const LAYOUT_LOCAL_STORAGE_KEY: &str = "cluster_layout";
pub(super) const LAYOUT_VALUE_TYPE_URL: &str = "type.googleapis.com/core.registry.key_value.v1.Value";
/// How often Gateway routes are re-polled — ListRoutes has no Watch RPC, and
/// routes change far less often than process health or event volume.
pub(super) const GATEWAY_POLL_MS: u32 = 10_000;


mod card;
mod derive;
mod geometry;
mod inspector;
mod menu;
mod model;
mod persist;
mod traces;

use card::*;
use derive::*;
use geometry::*;
use inspector::*;
use menu::*;
use model::*;
use persist::*;
use traces::*;

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
    // Wires are drawn in the theme's primary colour, so they follow the theme and its accent.
    let pri = "var(--primary)".to_string();
    let (cx, cy) = coord();
    let zoom_pct = (zk * 100.0).round() as i32;

    // ── Grid background ───────────────────────────────────────────────────
    let cell = 16.0 * zk;
    let major = 80.0 * zk;
    let gx = ((px % cell) + cell) % cell;
    let gy = ((py % cell) + cell) % cell;
    let gmx = ((px % major) + major) % major;
    let gmy = ((py % major) + major) % major;
    let fc = mix("var(--ink)", 0.035); // minor lines
    let mc = mix("var(--ink)", 0.09); // major lines
    let grid_bg = format!(
        "background-color:var(--bg);\
         background-image:repeating-linear-gradient({mc} 0 1px,transparent 1px 100%),\
         repeating-linear-gradient(90deg,{mc} 0 1px,transparent 1px 100%),\
         repeating-linear-gradient({fc} 0 1px,transparent 1px 100%),\
         repeating-linear-gradient(90deg,{fc} 0 1px,transparent 1px 100%);\
         background-size:{major}px {major}px,{major}px {major}px,{cell}px {cell}px,{cell}px {cell}px;\
         background-position:{gmx}px {gmy}px,{gmx}px {gmy}px,{gx}px {gy}px,{gx}px {gy}px;"
    );

    // ── Helpers ───────────────────────────────────────────────────────────
    let to_world = move |sx: f64, sy: f64| -> (f64, f64) {
        // Read the origin now: the shell lays the stage out after this component first renders, so
        // a value measured on mount can be stale.
        let (ol, ot) = stage_origin().unwrap_or_else(|| stg_off());
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
        ev.prevent_default();
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
        let (ol, ot) = stage_origin().unwrap_or_else(|| stg_off());
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

    // Zoom about the middle of the stage, and fit every node in view.
    let zoom_by = use_callback(move |factor: f64| {
        let Some((w, h)) = stage_size() else { return };
        let nk = (zoom() * factor).clamp(0.25, 3.0);
        let (mx, my) = (w / 2.0, h / 2.0);
        let (wx, wy) = ((mx - pan_x()) / zoom(), (my - pan_y()) / zoom());
        pan_x.set(mx - wx * nk);
        pan_y.set(my - wy * nk);
        zoom.set(nk);
        save_view(nodes, pan_x, pan_y, zoom);
    });
    let fit = move |_| {
        let Some((w, h)) = stage_size() else { return };
        let ns = nodes.read();
        if ns.is_empty() {
            return;
        }
        let (min_x, min_y) = ns.iter().fold((f64::MAX, f64::MAX), |a, n| (a.0.min(n.x), a.1.min(n.y)));
        let (max_x, max_y) = ns.iter().fold((f64::MIN, f64::MIN), |a, n| (a.0.max(n.x + NW), a.1.max(n.y + NH)));
        const PAD: f64 = 48.0;
        let (bw, bh) = ((max_x - min_x + 2.0 * PAD).max(1.0), (max_y - min_y + 2.0 * PAD).max(1.0));
        let k = (w / bw).min(h / bh).clamp(0.25, 1.5);
        drop(ns);
        pan_x.set((w - (bw - 2.0 * PAD) * k) / 2.0 - min_x * k);
        pan_y.set((h - (bh - 2.0 * PAD) * k) / 2.0 - min_y * k);
        zoom.set(k);
        save_view(nodes, pan_x, pan_y, zoom);
    };

    let raft = crate::raft::use_raft();
    use_page_chrome(move || {
        let mut chrome = raft.chrome(&["Blueprint", "Control plane", "Cluster"]);
        chrome.left = vec![
            BarItem::kv("Nodes", nodes.read().len().to_string()),
            BarItem::kv("Links", traces.read().len().to_string()),
        ];
        chrome.right = vec![BarItem::hint("Right-click", "menu"), BarItem::hint("Scroll", "zoom")];
        chrome
    });

    rsx! {
        div { class: "cv-root",
            // ── Toolbar ─────────────────────────────────────────────────────
            div { class: "cv-toolbar",
                span { class: "cv-key",
                    svg { view_box: "0 0 26 8", path { d: "M0 4 H26", stroke: "var(--primary)", stroke_width: "2" } }
                    "Wire"
                }
                span { class: "cv-key",
                    svg { view_box: "0 0 26 8", path { d: "M0 4 H26", stroke: "var(--ca)", stroke_width: "2", stroke_dasharray: "5 4" } }
                    "Event flow"
                }
                span { class: "d-spacer" }
                Toggle { checked: snap_on, on_change: move |on| snap.set(on), "Snap" }
                Toggle { checked: flow_on, on_change: move |on| flow.set(on), "Flow" }
                span { class: "cv-count", "Zoom " b { "{zoom_pct}%" } }
                Btn { size: BtnSize::Sm, icon: true, aria_label: "Zoom out".to_string(), onclick: move |_| zoom_by.call(1.0 / 1.25), "−" }
                Btn { size: BtnSize::Sm, icon: true, aria_label: "Zoom in".to_string(), onclick: move |_| zoom_by.call(1.25), "+" }
                Btn { size: BtnSize::Sm, onclick: fit, "Fit" }
                Btn {
                    size: BtnSize::Sm,
                    onclick: move |_| {
                        pan_x.set(0.0);
                        pan_y.set(0.0);
                        zoom.set(1.0);
                        save_view(nodes, pan_x, pan_y, zoom);
                    },
                    "Reset"
                }
            }

            // ── Stage ───────────────────────────────────────────────────────
            div {
                id: "cv-stage",
                class: "cv-stage",
                style: "{grid_bg}",
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
                                    selected:    drawer() == Some(node.id.clone()),
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
                                Inspector {
                                    node:    node.clone(),
                                    nodes:   nodes(),
                                    traces:  traces(),
                                    routes:  routes(),
                                    topology: topology_data(),
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

                // Pointer position and the gestures, bottom-left.
                div { class: "cv-hud",
                    span { "X " b { "{cx:.0}" } " · Y " b { "{cy:.0}" } }
                    span { "Drag background · pan" }
                    span { "Drag a pad onto a node · connect" }
                }
            }
        }
    }
}

/// The stage's top-left corner in client coordinates, read from the DOM at the moment of use.
fn stage_origin() -> Option<(f64, f64)> {
    let el = web_sys::window()?.document()?.get_element_by_id("cv-stage")?;
    let r = el.get_bounding_client_rect();
    Some((r.left(), r.top()))
}

/// The stage's size in CSS pixels, read from the DOM (it is laid out by the shell, so it is not
/// known to this component).
fn stage_size() -> Option<(f64, f64)> {
    let el = web_sys::window()?.document()?.get_element_by_id("cv-stage")?;
    let r = el.get_bounding_client_rect();
    Some((r.width(), r.height()))
}

/// Saves the current pan and zoom (with node positions) — the same "settled gesture" persistence a
/// drag-end does, for the toolbar's zoom, fit and reset.
fn save_view(nodes: Signal<Vec<ClNode>>, pan_x: Signal<f64>, pan_y: Signal<f64>, zoom: Signal<f64>) {
    let positions: HashMap<String, (f64, f64)> = nodes.read().iter().map(|n| (n.id.clone(), (n.x, n.y))).collect();
    persist_layout(LayoutBlob { positions, pan_x: pan_x(), pan_y: pan_y(), zoom: zoom() });
}
