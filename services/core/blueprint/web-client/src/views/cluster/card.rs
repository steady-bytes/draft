#![allow(unused_imports)]
use super::*;

/// One node on the canvas: a panel with corner brackets in its kind's colour, a glyph, its name and
/// role, live status, and how many wires and event flows touch it. Drag it by the body; pull a wire
/// from a pad; the corner buttons (shown on hover or focus) open its inspector or remove it.
#[component]
pub(super) fn NodeCard(
    node: ClNode,
    wire_count: usize,
    event_count: usize,
    selected: bool,
    on_node_down: EventHandler<(f64, f64)>,
    on_context: EventHandler<(f64, f64)>,
    on_config: EventHandler<()>,
    on_delete: EventHandler<()>,
    on_pad_down: EventHandler<(i32, f64, f64)>,
    on_hover: EventHandler<bool>,
) -> Element {
    let col = node.kind.color();
    let tone = node.kind.tone();
    let name = node.name.clone();
    let role = node.kind.role();
    let manual = node.origin == NodeOrigin::Manual;
    let links = {
        let mut parts = Vec::new();
        if wire_count > 0 {
            parts.push(format!("{wire_count} link{}", if wire_count > 1 { "s" } else { "" }));
        }
        if event_count > 0 {
            parts.push(format!("{event_count} evt"));
        }
        if parts.is_empty() {
            "no links".to_string()
        } else {
            parts.join(" · ")
        }
    };
    // Manual nodes are dashed, so it is obvious which nodes are real and which are annotations.
    let class = format!("cv-node d-panel d-brackets{}{}", if manual { " is-manual" } else { "" }, if selected { " is-selected" } else { "" });
    let (status_kind, status_text) = if node.online { (StatusKind::Ok, "Online") } else { (StatusKind::Err, "Offline") };
    let label = format!("{name}, {role}. Press the menu button to inspect.");

    rsx! {
        div {
            class: "{class}",
            style: "left:{node.x}px; top:{node.y}px; --c:{col}; --bk:{col}",
            "aria-label": "{label}",
            onpointerdown: move |ev| {
                if ev.data().trigger_button() != Some(MouseButton::Primary) {
                    return;
                }
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

            div { class: "cv-head",
                Glyph { code: node.kind.sym().to_string(), tone, large: true }
                div { class: "cv-title",
                    div { class: "cv-name", title: "{name}", "{name}" }
                    span { class: "d-label", "{role}" }
                }
            }
            div { class: "cv-meta",
                Status { kind: status_kind, "{status_text}" }
                span { class: "cv-links", "{links}" }
            }

            // Corner controls: revealed by hover or keyboard focus.
            div { class: "cv-ctl",
                Btn {
                    size: BtnSize::Sm,
                    icon: true,
                    aria_label: format!("Inspect {name}"),
                    onclick: move |_| on_config.call(()),
                    "≡"
                }
                Btn {
                    size: BtnSize::Sm,
                    icon: true,
                    aria_label: format!("Remove {name} from the canvas"),
                    onclick: move |_| on_delete.call(()),
                    "×"
                }
            }

            // Wire pads.
            div {
                class: "cv-pad is-left",
                onpointerdown: move |ev| {
                    ev.stop_propagation();
                    let c = ev.data().client_coordinates();
                    on_pad_down.call((-1, c.x, c.y));
                },
            }
            div {
                class: "cv-pad is-right",
                onpointerdown: move |ev| {
                    ev.stop_propagation();
                    let c = ev.data().client_coordinates();
                    on_pad_down.call((1, c.x, c.y));
                },
            }
        }
    }
}
