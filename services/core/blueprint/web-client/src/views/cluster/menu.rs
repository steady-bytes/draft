#![allow(unused_imports)]
use super::*;
use draft_ui::ui::{Menu as UiMenu, MenuItem};

/// The canvas and node context menus, built on the shared `Menu`. Each item's id names the action;
/// [`CtxMenu`] decodes it back into the matching callback.
#[component]
pub(super) fn CtxMenu(
    m: Menu,
    nodes: Vec<ClNode>,
    traces: Vec<ClTrace>,
    snap_on: bool,
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
    let (heading, items) = items_for(&m, &nodes, snap_on);
    let for_ = m.for_.clone();
    let uid = uid_val();

    rsx! {
        UiMenu {
            heading: heading,
            x: m.x,
            y: m.y,
            items,
            on_close: move |_| on_close.call(()),
            on_select: move |id: String| {
                let mut parts = id.splitn(3, '|');
                let action = parts.next().unwrap_or_default();
                match (&for_, action) {
                    (MenuFor::Canvas { wx, wy }, "add") => {
                        let (kind, name) = match parts.next().unwrap_or_default() {
                            "catalyst" => (NodeKind::Catalyst, format!("catalyst-{uid:02}")),
                            "blueprint" => (NodeKind::Blueprint, format!("blueprint-{uid:02}")),
                            "fuse" => (NodeKind::Fuse, format!("fuse-{uid:02}")),
                            _ => (NodeKind::Service, format!("svc-{uid:02}")),
                        };
                        on_add.call((kind, name, *wx, *wy));
                    }
                    (MenuFor::Canvas { .. }, "snap") => on_snap.call(()),
                    (MenuFor::Canvas { .. }, "reset") => on_reset.call(()),
                    (MenuFor::Node(nid), "drawer") => on_drawer.call(nid.clone()),
                    (MenuFor::Node(nid), "online") => on_toggle_online.call(nid.clone()),
                    (MenuFor::Node(nid), "disconnect") => on_disconnect.call(nid.clone()),
                    (MenuFor::Node(nid), "remove") => on_delete.call(nid.clone()),
                    (MenuFor::Node(nid), "reg-bp") => on_reg_bp.call(nid.clone()),
                    (MenuFor::Node(nid), "pub") | (MenuFor::Node(nid), "sub") => {
                        let cat = parts.next().unwrap_or_default().to_string();
                        let topic = parts.next().unwrap_or_default().to_string();
                        if action == "pub" {
                            on_pub.call((nid.clone(), cat, topic));
                        } else {
                            on_sub.call((nid.clone(), cat, topic));
                        }
                    }
                    _ => {}
                }
            },
        }
    }
}

/// The heading and items for a menu: what can be done on the canvas, or to one node.
fn items_for(m: &Menu, nodes: &[ClNode], snap_on: bool) -> (String, Vec<MenuItem>) {
    match &m.for_ {
        MenuFor::Canvas { .. } => (
            "Canvas".to_string(),
            vec![
                MenuItem::new("add|catalyst", "Add Catalyst"),
                MenuItem::new("add|blueprint", "Add Blueprint"),
                MenuItem::new("add|fuse", "Add Fuse"),
                MenuItem::new("add|service", "Add service"),
                MenuItem::new("snap", "Snap to grid").checked(snap_on).separated(),
                MenuItem::new("reset", "Reset view"),
            ],
        ),
        MenuFor::Node(nid) => {
            let Some(node) = nodes.iter().find(|n| &n.id == nid) else {
                return (String::new(), Vec::new());
            };
            let mut items: Vec<MenuItem> = Vec::new();
            match node.kind {
                NodeKind::Fuse => items.push(MenuItem::new("drawer", format!("Routing rules… ({})", node.rules.len()))),
                NodeKind::Catalyst => items.push(MenuItem::new("drawer", format!("Topics… ({})", node.topics.len()))),
                NodeKind::Blueprint => items.push(MenuItem::new("drawer", "Service registry…")),
                NodeKind::Service => {
                    items.push(MenuItem::new("drawer", "Endpoint config…"));
                    let cats: Vec<&ClNode> = nodes.iter().filter(|n| n.kind == NodeKind::Catalyst).collect();
                    for c in &cats {
                        for t in &c.topics {
                            items.push(MenuItem::new(format!("pub|{}|{}", c.id, t.name), format!("Publish · {}", t.name)));
                        }
                    }
                    for c in &cats {
                        for t in &c.topics {
                            items.push(MenuItem::new(format!("sub|{}|{}", c.id, t.name), format!("Subscribe · {}", t.name)));
                        }
                    }
                    if nodes.iter().any(|n| n.kind == NodeKind::Blueprint) {
                        items.push(MenuItem::new("reg-bp", "Register with Blueprint"));
                    }
                }
            }
            // A live node reflects the registry: its existence and status are not the canvas's to
            // change. Only hand-added nodes can be toggled, disconnected or removed.
            let heading = if node.origin == NodeOrigin::Live {
                format!("{} · live", node.name)
            } else {
                items.push(MenuItem::new("online", if node.online { "Mark offline" } else { "Mark online" }).separated());
                items.push(MenuItem::new("disconnect", "Disconnect all"));
                items.push(MenuItem::new("remove", "Remove node").danger().separated());
                node.name.clone()
            };
            (heading, items)
        }
    }
}
