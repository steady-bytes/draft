use dioxus::prelude::*;
use crate::components::topology::{TopologyData, event_color};

const SVG_H: f32 = 640.0;
const NODE_W: f32 = 220.0;
const NODE_H: f32 = 40.0;
const NODE_RX: f32 = 6.0;
const PROD_CX: f32 = 200.0;
const CONS_CX: f32 = 760.0;
const VERT_GAP: f32 = 24.0;
const EDGE_X1: f32 = 313.0;
const EDGE_X2: f32 = 647.0;

fn col_ys(count: usize) -> Vec<f32> {
    let total_h = count as f32 * NODE_H + (count.saturating_sub(1)) as f32 * VERT_GAP;
    let start_y = (SVG_H - total_h) / 2.0;
    (0..count)
        .map(|i| start_y + i as f32 * (NODE_H + VERT_GAP) + NODE_H / 2.0)
        .collect()
}

fn bezier(x1: f32, y1: f32, x2: f32, y2: f32) -> String {
    let cp = (x2 - x1) * 0.35;
    format!("M {x1:.1} {y1:.1} C {:.1} {y1:.1} {:.1} {y2:.1} {x2:.1} {y2:.1}",
        x1 + cp, x2 - cp)
}

struct EdgeInfo {
    key: String,
    pid: String,
    cid: String,
    etype: String,
    path: String,
    mid_x: f32,
    mid_y: f32,
    color: &'static str,
}

struct NodeInfo {
    id: String,
    name: String,
    cy: f32,
    dot_color: &'static str,
    glow_color: &'static str,
    sel_stroke: &'static str,
}

#[component]
pub fn ArcSpine(data: TopologyData) -> Element {
    let mut selected: Signal<Option<String>> = use_signal(|| None);

    let prod_ys = col_ys(data.producers.len());
    let cons_ys = col_ys(data.consumers.len());

    let prod_cy: std::collections::HashMap<String, f32> = data.producers.iter().zip(prod_ys.iter())
        .map(|(n, &y)| (n.id.clone(), y))
        .collect();
    let cons_cy: std::collections::HashMap<String, f32> = data.consumers.iter().zip(cons_ys.iter())
        .map(|(n, &y)| (n.id.clone(), y))
        .collect();

    // Bundle offset: group edges by (pid, cid), assign slot per-edge
    let mut group_totals: std::collections::HashMap<(String, String), usize> = Default::default();
    for e in &data.edges {
        *group_totals.entry((e.producer_id.clone(), e.consumer_id.clone())).or_insert(0) += 1;
    }
    let mut group_pos: std::collections::HashMap<(String, String), usize> = Default::default();

    let edges: Vec<EdgeInfo> = data.edges.iter().map(|e| {
        let py = prod_cy.get(&e.producer_id).copied().unwrap_or(SVG_H / 2.0);
        let cy = cons_cy.get(&e.consumer_id).copied().unwrap_or(SVG_H / 2.0);
        let key = (e.producer_id.clone(), e.consumer_id.clone());
        let n = *group_totals.get(&key).unwrap_or(&1);
        let pos = {
            let slot = group_pos.entry(key).or_insert(0);
            let v = *slot;
            *slot += 1;
            v
        };
        let offset = (pos as f32 - (n as f32 - 1.0) / 2.0) * 13.0;
        let y1 = py + offset;
        let y2 = cy + offset;
        EdgeInfo {
            key: format!("{}-{}-{}", e.producer_id, e.consumer_id, e.event_type),
            pid: e.producer_id.clone(),
            cid: e.consumer_id.clone(),
            etype: e.event_type.clone(),
            path: bezier(EDGE_X1, y1, EDGE_X2, y2),
            mid_x: (EDGE_X1 + EDGE_X2) / 2.0,
            mid_y: (y1 + y2) / 2.0,
            color: event_color(&e.event_type),
        }
    }).collect();

    let prod_nodes: Vec<NodeInfo> = data.producers.iter().zip(prod_ys.iter()).map(|(n, &y)| {
        NodeInfo {
            id: n.id.clone(),
            name: n.name.clone(),
            cy: y,
            dot_color: "#3b82f6",
            glow_color: "#3b82f666",
            sel_stroke: "#3b82f6",
        }
    }).collect();

    let cons_nodes: Vec<NodeInfo> = data.consumers.iter().zip(cons_ys.iter()).map(|(n, &y)| {
        NodeInfo {
            id: n.id.clone(),
            name: n.name.clone(),
            cy: y,
            dot_color: "#22c55e",
            glow_color: "#22c55e66",
            sel_stroke: "#22c55e",
        }
    }).collect();

    let event_types: Vec<(String, &'static str)> = data.unique_event_types().iter()
        .map(|s| (s.to_string(), event_color(s)))
        .collect();

    rsx! {
        div { class: "w-full overflow-x-auto",
            svg {
                view_box: "0 0 960 640",
                width: "960",
                height: "640",
                style: "max-width:100%; display:block; margin:0 auto;",

                rect { x: "0", y: "0", width: "960", height: "640", fill: "var(--color-base-100)" }

                defs {
                    pattern {
                        id: "as-grid",
                        width: "24", height: "24",
                        pattern_units: "userSpaceOnUse",
                        circle { cx: "1", cy: "1", r: "1", fill: "#ffffff0d" }
                    }
                }
                rect { x: "0", y: "0", width: "960", height: "640", fill: "url(#as-grid)" }

                line {
                    x1: "480", y1: "48", x2: "480", y2: "592",
                    stroke: "#1e293b", stroke_width: "1",
                }

                text {
                    x: "200", y: "38",
                    text_anchor: "middle",
                    fill: "#475569",
                    font_size: "10",
                    font_family: "monospace",
                    letter_spacing: "2",
                    "PRODUCERS"
                }
                text {
                    x: "760", y: "38",
                    text_anchor: "middle",
                    fill: "#475569",
                    font_size: "10",
                    font_family: "monospace",
                    letter_spacing: "2",
                    "CONSUMERS"
                }

                for ed in edges.iter() {
                    {
                        let pid = ed.pid.clone();
                        let cid = ed.cid.clone();
                        let etype = ed.etype.clone();
                        let path = ed.path.clone();
                        let mid_x = ed.mid_x;
                        let mid_y = ed.mid_y;
                        let color = ed.color;
                        let ekey = ed.key.clone();
                        let sel = selected.read().clone();
                        let active = sel.as_ref().map(|id| id == &pid || id == &cid).unwrap_or(true);
                        rsx! {
                            g { key: "{ekey}",
                                path {
                                    d: "{path}",
                                    fill: "none",
                                    stroke: "{color}",
                                    stroke_width: "1.5",
                                    opacity: if active { "1" } else { "0.1" },
                                }
                                if active {
                                    rect {
                                        x: "{mid_x - 30.0}", y: "{mid_y - 8.5}",
                                        width: "60", height: "17",
                                        rx: "8.5",
                                        fill: "{color}1a",
                                        stroke: "{color}",
                                        stroke_width: "0.5",
                                    }
                                    text {
                                        x: "{mid_x}", y: "{mid_y + 4.5}",
                                        text_anchor: "middle",
                                        fill: "{color}",
                                        font_size: "8.5",
                                        font_family: "monospace",
                                        "{etype}"
                                    }
                                }
                            }
                        }
                    }
                }

                for node in prod_nodes.iter() {
                    {
                        let nid = node.id.clone();
                        let nname = node.name.clone();
                        let cy = node.cy;
                        let dot_color = node.dot_color;
                        let glow_color = node.glow_color;
                        let sel_stroke = node.sel_stroke;
                        let nx = PROD_CX - NODE_W / 2.0;
                        let ny = cy - NODE_H / 2.0;
                        let sel = selected.read().clone();
                        let is_sel = sel.as_deref() == Some(nid.as_str());
                        let is_dim = sel.as_ref().map(|s| s != &nid).unwrap_or(false);
                        let nid_click = nid.clone();
                        rsx! {
                            g {
                                key: "p-{nid}",
                                style: if is_sel {
                                    format!("filter:drop-shadow(0 0 8px {glow_color}); cursor:pointer;")
                                } else {
                                    "cursor:pointer;".to_string()
                                },
                                opacity: if is_dim { "0.2" } else { "1" },
                                onclick: move |_| {
                                    if selected.read().as_deref() == Some(nid_click.as_str()) {
                                        selected.set(None);
                                    } else {
                                        selected.set(Some(nid_click.clone()));
                                    }
                                },
                                rect {
                                    x: "{nx}", y: "{ny}",
                                    width: "{NODE_W}", height: "{NODE_H}",
                                    rx: "{NODE_RX}",
                                    fill: "#1e293b",
                                    stroke: if is_sel { sel_stroke } else { "#334155" },
                                    stroke_width: if is_sel { "2" } else { "1" },
                                }
                                circle {
                                    cx: "{nx + 16.0}", cy: "{cy}",
                                    r: "4", fill: "{dot_color}",
                                }
                                text {
                                    x: "{nx + 30.0}", y: "{cy + 5.0}",
                                    fill: "#e2e8f0",
                                    font_size: "13",
                                    font_family: "ui-sans-serif, system-ui, sans-serif",
                                    "{nname}"
                                }
                            }
                        }
                    }
                }

                for node in cons_nodes.iter() {
                    {
                        let nid = node.id.clone();
                        let nname = node.name.clone();
                        let cy = node.cy;
                        let dot_color = node.dot_color;
                        let glow_color = node.glow_color;
                        let sel_stroke = node.sel_stroke;
                        let nx = CONS_CX - NODE_W / 2.0;
                        let ny = cy - NODE_H / 2.0;
                        let sel = selected.read().clone();
                        let is_sel = sel.as_deref() == Some(nid.as_str());
                        let is_dim = sel.as_ref().map(|s| s != &nid).unwrap_or(false);
                        let nid_click = nid.clone();
                        rsx! {
                            g {
                                key: "c-{nid}",
                                style: if is_sel {
                                    format!("filter:drop-shadow(0 0 8px {glow_color}); cursor:pointer;")
                                } else {
                                    "cursor:pointer;".to_string()
                                },
                                opacity: if is_dim { "0.2" } else { "1" },
                                onclick: move |_| {
                                    if selected.read().as_deref() == Some(nid_click.as_str()) {
                                        selected.set(None);
                                    } else {
                                        selected.set(Some(nid_click.clone()));
                                    }
                                },
                                rect {
                                    x: "{nx}", y: "{ny}",
                                    width: "{NODE_W}", height: "{NODE_H}",
                                    rx: "{NODE_RX}",
                                    fill: "#1e293b",
                                    stroke: if is_sel { sel_stroke } else { "#334155" },
                                    stroke_width: if is_sel { "2" } else { "1" },
                                }
                                circle {
                                    cx: "{nx + 16.0}", cy: "{cy}",
                                    r: "4", fill: "{dot_color}",
                                }
                                text {
                                    x: "{nx + 30.0}", y: "{cy + 5.0}",
                                    fill: "#e2e8f0",
                                    font_size: "13",
                                    font_family: "ui-sans-serif, system-ui, sans-serif",
                                    "{nname}"
                                }
                            }
                        }
                    }
                }

                // Legend row
                for (idx, (etype, color)) in event_types.iter().enumerate() {
                    {
                        let lx = 48.0 + idx as f32 * 118.0;
                        let ly = SVG_H - 18.0;
                        let etype = etype.clone();
                        let color = *color;
                        rsx! {
                            g { key: "leg-{etype}",
                                rect {
                                    x: "{lx}", y: "{ly - 8.0}",
                                    width: "108", height: "16",
                                    rx: "8",
                                    fill: "{color}18",
                                    stroke: "{color}",
                                    stroke_width: "0.75",
                                }
                                text {
                                    x: "{lx + 54.0}", y: "{ly + 4.5}",
                                    text_anchor: "middle",
                                    fill: "{color}",
                                    font_size: "9",
                                    font_family: "monospace",
                                    "{etype}"
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
