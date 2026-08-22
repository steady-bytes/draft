use std::f32::consts::PI;
use dioxus::prelude::*;
use crate::components::topology::TopologyData;

const CX0: f32 = 340.0;
const CY0: f32 = 340.0;
const R: f32 = 210.0;
const OR: f32 = 308.0;
const ARC_W: f32 = 7.0;
const ARC_SPAN: f32 = 4.58; // ≈ 0.08 radians
const CP_R: f32 = 55.0;
const LABEL_R: f32 = R + ARC_W + 18.0;

fn circ(deg: f32, radius: f32) -> (f32, f32) {
    let rad = (deg - 90.0) * PI / 180.0;
    (CX0 + radius * rad.cos(), CY0 + radius * rad.sin())
}

fn evenly_spaced(start: f32, end: f32, n: usize) -> Vec<f32> {
    match n {
        0 => vec![],
        1 => vec![(start + end) / 2.0],
        _ => (0..n).map(|i| start + i as f32 * (end - start) / (n - 1) as f32).collect(),
    }
}

fn node_arc(deg: f32) -> String {
    let (x1, y1) = circ(deg - ARC_SPAN, R);
    let (x2, y2) = circ(deg + ARC_SPAN, R);
    format!("M {x1:.2} {y1:.2} A {R} {R} 0 0 1 {x2:.2} {y2:.2}")
}

fn bezier(pa: f32, ca: f32) -> String {
    let (px, py) = circ(pa, R);
    let (ex, ey) = circ(ca, R);
    let (c1x, c1y) = circ(pa, CP_R);
    let (c2x, c2y) = circ(ca, CP_R);
    format!("M {px:.2} {py:.2} C {c1x:.2} {c1y:.2} {c2x:.2} {c2y:.2} {ex:.2} {ey:.2}")
}

fn thread_lw(vol: u32, max: u32) -> f32 {
    0.5 + (vol as f32 / max as f32) * 7.5
}

fn thread_alpha(vol: u32, max: u32) -> f32 {
    0.12 + 0.55 * (vol as f32 / max as f32)
}

static BG_DOTS: &[(f32, f32)] = &[
    (72.0, 44.0), (145.0, 90.0), (312.0, 28.0), (498.0, 61.0), (620.0, 44.0),
    (51.0, 200.0), (190.0, 155.0), (580.0, 130.0), (660.0, 210.0), (30.0, 380.0),
    (640.0, 340.0), (85.0, 520.0), (600.0, 490.0), (180.0, 610.0), (480.0, 640.0),
    (620.0, 580.0), (350.0, 665.0), (120.0, 660.0), (230.0, 50.0), (560.0, 330.0),
];

#[derive(Clone)]
struct Node {
    id: String,
    label: String,
    is_prod: bool,
    deg: f32,
}

#[derive(Clone)]
struct Edge {
    pid: String,
    cid: String,
    ev: String,
    vol: u32,
    path: String,
}

#[derive(Clone)]
struct ConnRow {
    ev: String,
    partner: String,
    vol: u32,
    bar_w: u32,
}

fn has_conn(sel_id: &str, nid: &str, edges: &[Edge]) -> bool {
    nid == sel_id
        || edges.iter().any(|e| {
            (e.pid == sel_id || e.cid == sel_id) && (e.pid == nid || e.cid == nid)
        })
}

#[component]
pub fn FullCircle(data: TopologyData) -> Element {
    let mut sel: Signal<Option<String>> = use_signal(|| None);
    let mut hovered: Signal<Option<String>> = use_signal(|| None);
    let mut hover_pos: Signal<(f64, f64)> = use_signal(|| (0.0, 0.0));
    let mut show_lbl = use_signal(|| true);
    let mut show_vol = use_signal(|| true);

    // Producers: upper 160° arc, centred on top   (−80° → +80°)
    // Consumers: lower 160° arc, centred on bottom (100° → 260°)
    // 20° gap on each side keeps producers and consumers fully separated.
    let prod_degs = evenly_spaced(-80.0, 80.0, data.producers.len());
    let cons_degs = evenly_spaced(100.0, 260.0, data.consumers.len());

    let nodes: Vec<Node> = data.producers.iter().zip(&prod_degs)
        .map(|(n, &d)| Node { id: n.id.clone(), label: n.name.clone(), is_prod: true, deg: d })
        .chain(data.consumers.iter().zip(&cons_degs)
            .map(|(n, &d)| Node { id: n.id.clone(), label: n.name.clone(), is_prod: false, deg: d }))
        .collect();

    let max_vol = data.edges.iter().map(|e| e.vol).max().unwrap_or(1);

    let edges: Vec<Edge> = data.edges.iter().map(|e| {
        let pa = nodes.iter().find(|n| n.id == e.producer_id).map(|n| n.deg).unwrap_or(0.0);
        let ca = nodes.iter().find(|n| n.id == e.consumer_id).map(|n| n.deg).unwrap_or(180.0);
        Edge {
            pid: e.producer_id.clone(),
            cid: e.consumer_id.clone(),
            ev: e.event_type.clone(),
            vol: e.vol,
            path: bezier(pa, ca),
        }
    }).collect();

    let sel_val = sel();
    let hov_val = hovered();
    let (hx, hy) = hover_pos();
    let lbl = show_lbl();
    let vol_mode = show_vol();

    // 36 evenly-spaced outer-ring tick marks, skipping positions near a node (< 4°)
    let ticks: Vec<[f32; 4]> = (0..36u32).filter_map(|i| {
        let tick_deg = (i as f32 / 36.0) * 360.0 + 90.0;
        let near = nodes.iter().any(|nd| {
            let d = ((nd.deg - tick_deg + 540.0) % 360.0) - 180.0;
            d.abs() < 4.01
        });
        if near { return None; }
        let (x1, y1) = circ(tick_deg, OR - 2.0);
        let (x2, y2) = circ(tick_deg, OR + 2.0);
        Some([x1, y1, x2, y2])
    }).collect();

    // Volume-scale bar descriptors: (stroke-width, x-start)
    let vol_bars: [(f32, f32); 4] = [(0.5, 128.0), (2.0, 150.0), (5.0, 172.0), (8.0, 194.0)];

    // Pre-compute tooltip data from hovered node
    let tooltip: Option<(String, String, Vec<ConnRow>, u32)> =
        hov_val.as_ref().and_then(|hid| {
            nodes.iter().find(|n| &n.id == hid).map(|nd| {
                let mut rows: Vec<ConnRow> = edges.iter()
                    .filter(|e| &e.pid == hid || &e.cid == hid)
                    .map(|e| {
                        let partner_id = if &e.pid == hid { &e.cid } else { &e.pid };
                        let partner = nodes.iter().find(|n| &n.id == partner_id)
                            .map(|n| n.label.clone())
                            .unwrap_or_else(|| partner_id.clone());
                        ConnRow {
                            ev: e.ev.clone(),
                            partner,
                            vol: e.vol,
                            bar_w: ((e.vol as f32 / max_vol as f32) * 60.0).round() as u32,
                        }
                    })
                    .collect();
                rows.sort_by(|a, b| b.vol.cmp(&a.vol));
                let total: u32 = rows.iter().map(|r| r.vol).sum();
                let ntype = if nd.is_prod { "producer" } else { "consumer" };
                (nd.label.clone(), ntype.to_string(), rows, total)
            })
        });

    // Pre-compute center-panel data from selected node (or mesh summary)
    let (sel_center, n_nodes, n_edges) = {
        let nn = nodes.len();
        let ne = edges.len();
        let info = sel_val.as_ref().and_then(|sid| {
            nodes.iter().find(|n| &n.id == sid).map(|nd| {
                let conns: Vec<_> = edges.iter()
                    .filter(|e| &e.pid == sid || &e.cid == sid)
                    .collect();
                let total: u32 = conns.iter().map(|e| e.vol).sum();
                let ntype = if nd.is_prod { "producer" } else { "consumer" };
                (nd.label.clone(), ntype.to_string(), total, conns.len())
            })
        });
        (info, nn, ne)
    };

    // Button pill styles
    let pill = |on: bool| -> String {
        format!(
            "font-size:10px;padding:2px 9px;border-radius:99px;border:0.5px solid {};background:transparent;color:{};cursor:pointer;font-family:inherit;",
            if on { "rgba(255,255,255,0.28)" } else { "rgba(255,255,255,0.08)" },
            if on { "rgba(255,255,255,0.65)" } else { "rgba(255,255,255,0.2)" },
        )
    };
    let lbl_pill = pill(lbl);
    let vol_pill = pill(vol_mode);

    rsx! {
        div {
            style: "position:relative;width:100%;height:100%;overflow:hidden;background:#080808;font-family:ui-sans-serif,system-ui,sans-serif;",

            svg {
                view_box: "-38 -38 756 756",
                width: "100%",
                height: "100%",
                style: "display:block;",
                onmousemove: move |ev| {
                    let c = ev.element_coordinates();
                    hover_pos.set((c.x, c.y));
                },
                onmouseleave: move |_| { hovered.set(None); },

                // ── Background ──────────────────────────────────────────────
                rect { x: "0", y: "0", width: "680", height: "680", fill: "#080808" }
                for dot in BG_DOTS {
                    circle { cx: "{dot.0}", cy: "{dot.1}", r: "0.7", fill: "rgba(255,255,255,0.18)" }
                }

                // ── Outer decorative ring ────────────────────────────────────
                circle {
                    cx: "{CX0}", cy: "{CY0}", r: "{OR}",
                    fill: "none", stroke: "rgba(255,255,255,0.18)", stroke_width: "0.5"
                }
                for t in ticks.iter() {
                    line {
                        x1: "{t[0]:.2}", y1: "{t[1]:.2}", x2: "{t[2]:.2}", y2: "{t[3]:.2}",
                        stroke: "rgba(255,255,255,0.09)", stroke_width: "0.5"
                    }
                }

                // ── Base node ring ───────────────────────────────────────────
                circle {
                    cx: "{CX0}", cy: "{CY0}", r: "{R}",
                    fill: "none", stroke: "rgba(255,255,255,0.05)", stroke_width: "1"
                }

                // ── Dimmed threads ───────────────────────────────────────────
                for e in edges.iter() {
                    if sel_val.as_ref().map_or(false, |sid| &e.pid != sid && &e.cid != sid) {
                        path {
                            d: "{e.path}",
                            fill: "none",
                            stroke: "rgba(255,255,255,0.04)",
                            stroke_width: "0.5"
                        }
                    }
                }

                // ── Active threads ───────────────────────────────────────────
                for e in edges.iter() {
                    if sel_val.as_ref().map_or(true, |sid| &e.pid == sid || &e.cid == sid) {
                        {
                            let sw = if vol_mode { thread_lw(e.vol, max_vol) } else { 1.5_f32 };
                            let alpha = if vol_mode { thread_alpha(e.vol, max_vol) } else { 0.45_f32 };
                            let stroke = format!("rgba(255,255,255,{alpha:.3})");
                            let path = e.path.clone();
                            rsx! {
                                path {
                                    d: "{path}",
                                    fill: "none",
                                    stroke: "{stroke}",
                                    stroke_width: "{sw:.2}",
                                    stroke_linecap: "round",
                                    stroke_linejoin: "round"
                                }
                            }
                        }
                    }
                }

                // ── Nodes ────────────────────────────────────────────────────
                for nd in nodes.iter() {
                    {
                        let nid = nd.id.clone();
                        let nid2 = nd.id.clone();

                        let dim = sel_val.as_ref().map_or(false, |s| !has_conn(s, &nd.id, &edges));
                        let is_sel = sel_val.as_deref() == Some(nd.id.as_str());

                        // Arc segment
                        let arc = node_arc(nd.deg);
                        let arc_stroke = if is_sel { "rgba(255,255,255,0.95)" }
                            else if nd.is_prod { "rgba(255,200,80,0.65)" }
                            else { "rgba(80,160,255,0.5)" };
                        let arc_sw = if is_sel { ARC_W } else { ARC_W * 0.6 };

                        // Inward tick
                        let (tx, ty) = circ(nd.deg, R);
                        let (tick_x, tick_y) = circ(nd.deg, R - 10.0);
                        let tick_clr = if is_sel { "rgba(255,255,255,0.5)" } else { "rgba(255,255,255,0.18)" };

                        // Outer ring marker
                        let (ox1, oy1) = circ(nd.deg, OR - 3.0);
                        let (ox2, oy2) = circ(nd.deg, OR + 3.0);
                        let outer_clr = if is_sel { "rgba(255,255,255,0.7)" }
                            else if nd.is_prod { "rgba(255,255,255,0.35)" }
                            else { "rgba(255,255,255,0.2)" };
                        let outer_sw = if is_sel { "1.5" } else { "0.5" };

                        // Label – placed in a local coordinate group so that the
                        // indicator (line / dot) can be drawn without additional
                        // trigonometry. `canvas_deg` is the standard SVG rotation
                        // angle (CW from right); the "flip" condition prevents text
                        // on the left hemisphere from rendering upside-down.
                        let (lx, ly) = circ(nd.deg, LABEL_R);
                        let canvas_deg = nd.deg - 90.0;
                        let norm = (canvas_deg + 360.0) % 360.0;
                        let flip = norm > 90.0 && norm < 270.0;
                        let rot = if flip { canvas_deg + 180.0 } else { canvas_deg };
                        let gtransform = format!("translate({lx:.2},{ly:.2}) rotate({rot:.1})");
                        let txt_x: f32 = if flip { -4.0 } else { 4.0 };
                        let txt_anchor = if flip { "end" } else { "start" };
                        let ind_x1: f32 = if flip { -14.0 } else { -4.0 };
                        let ind_x2: f32 = if flip { -4.0 } else { 4.0 };
                        let dot_cx: f32 = if flip { -9.0 } else { 0.0 };
                        let lbl_fill = if is_sel { "rgba(255,255,255,0.95)" }
                            else if nd.is_prod { "rgba(255,255,255,0.65)" }
                            else { "rgba(255,255,255,0.4)" };
                        let lbl_w = if is_sel { "500" } else { "400" };
                        let ind_clr = if is_sel { "rgba(255,255,255,0.35)" } else { "rgba(255,255,255,0.15)" };

                        let opacity = if dim { "0.1" } else { "1" };
                        let label = nd.label.clone();
                        let is_prod = nd.is_prod;

                        rsx! {
                            g {
                                opacity: "{opacity}",
                                style: "cursor:pointer;",
                                onmouseenter: move |_| hovered.set(Some(nid.clone())),
                                onmouseleave: move |_| hovered.set(None),
                                onclick: move |_| {
                                    let cur = sel();
                                    sel.set(
                                        if cur.as_deref() == Some(nid2.as_str()) { None }
                                        else { Some(nid2.clone()) }
                                    );
                                },

                                // Arc segment marker
                                path {
                                    d: "{arc}",
                                    fill: "none",
                                    stroke: "{arc_stroke}",
                                    stroke_width: "{arc_sw:.1}",
                                    stroke_linecap: "round"
                                }
                                // Inward tick
                                line {
                                    x1: "{tx:.2}", y1: "{ty:.2}",
                                    x2: "{tick_x:.2}", y2: "{tick_y:.2}",
                                    stroke: "{tick_clr}", stroke_width: "0.5"
                                }
                                // Outer ring tick
                                line {
                                    x1: "{ox1:.2}", y1: "{oy1:.2}",
                                    x2: "{ox2:.2}", y2: "{oy2:.2}",
                                    stroke: "{outer_clr}", stroke_width: "{outer_sw}"
                                }
                                // Invisible hit-area circle
                                circle { cx: "{tx:.2}", cy: "{ty:.2}", r: "20", fill: "transparent" }

                                // Label + producer-line / consumer-dot indicator
                                if lbl {
                                    g { transform: "{gtransform}",
                                        text {
                                            x: "{txt_x}", y: "0",
                                            text_anchor: "{txt_anchor}",
                                            dominant_baseline: "middle",
                                            fill: "{lbl_fill}",
                                            font_size: "11",
                                            font_weight: "{lbl_w}",
                                            font_family: "ui-sans-serif,system-ui,sans-serif",
                                            "{label}"
                                        }
                                        if is_prod {
                                            line {
                                                x1: "{ind_x1}", y1: "12",
                                                x2: "{ind_x2}", y2: "12",
                                                stroke: "{ind_clr}", stroke_width: "1"
                                            }
                                        } else {
                                            circle { cx: "{dot_cx}", cy: "12", r: "1.5", fill: "{ind_clr}" }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }

                // ── Legend (producer line · "producer"  ●  "consumer") ───────
                if lbl {
                    line {
                        x1: "{680.0 - 18.0 - 60.0 - 8.0 - 56.0 - 14.0}",
                        y1: "{680.0 - 20.0}",
                        x2: "{680.0 - 18.0 - 60.0 - 8.0 - 56.0 - 4.0}",
                        y2: "{680.0 - 20.0}",
                        stroke: "rgba(255,200,80,0.5)", stroke_width: "1"
                    }
                    text {
                        x: "{680.0 - 18.0 - 60.0 - 8.0}", y: "{680.0 - 20.0}",
                        text_anchor: "end", dominant_baseline: "middle",
                        fill: "rgba(255,255,255,0.2)", font_size: "10",
                        font_family: "ui-sans-serif,sans-serif",
                        "producer"
                    }
                    circle {
                        cx: "{680.0 - 18.0 - 60.0}", cy: "{680.0 - 20.0}",
                        r: "1.5", fill: "rgba(80,160,255,0.5)"
                    }
                    text {
                        x: "{680.0 - 18.0}", y: "{680.0 - 20.0}",
                        text_anchor: "end", dominant_baseline: "middle",
                        fill: "rgba(255,255,255,0.2)", font_size: "10",
                        font_family: "ui-sans-serif,sans-serif",
                        "consumer"
                    }
                }

                // ── Volume-scale legend ───────────────────────────────────────
                if vol_mode {
                    text {
                        x: "16", y: "{680.0 - 20.0}",
                        dominant_baseline: "middle",
                        fill: "rgba(255,255,255,0.16)", font_size: "9",
                        font_family: "ui-sans-serif,sans-serif",
                        "thread = msgs/min"
                    }
                    for bar in vol_bars.iter() {
                        {
                            let sw = bar.0;
                            let bx = bar.1;
                            let alpha = 0.15 + sw * 0.06;
                            let s = format!("rgba(255,255,255,{alpha:.3})");
                            rsx! {
                                line {
                                    x1: "{bx}", y1: "{680.0 - 20.0}",
                                    x2: "{bx + 16.0}", y2: "{680.0 - 20.0}",
                                    stroke: "{s}", stroke_width: "{sw}",
                                    stroke_linecap: "round"
                                }
                            }
                        }
                    }
                }

                // ── Center panel ─────────────────────────────────────────────
                if let Some((c_lbl, c_type, c_vol, c_conn)) = sel_center {
                    {
                        let conn_s = if c_conn != 1 { "s" } else { "" };
                        rsx! {
                            text {
                                x: "{CX0}", y: "{CY0 - 22.0}",
                                text_anchor: "middle", dominant_baseline: "middle",
                                fill: "rgba(255,255,255,0.72)", font_size: "13",
                                font_weight: "500", font_family: "ui-sans-serif,sans-serif",
                                "{c_lbl}"
                            }
                            text {
                                x: "{CX0}", y: "{CY0 - 5.0}",
                                text_anchor: "middle", dominant_baseline: "middle",
                                fill: "rgba(255,255,255,0.28)", font_size: "10",
                                font_family: "ui-sans-serif,sans-serif",
                                "{c_type}"
                            }
                            text {
                                x: "{CX0}", y: "{CY0 + 11.0}",
                                text_anchor: "middle", dominant_baseline: "middle",
                                fill: "rgba(255,255,255,0.22)", font_size: "10",
                                font_family: "ui-sans-serif,sans-serif",
                                "{c_vol} msgs/min"
                            }
                            text {
                                x: "{CX0}", y: "{CY0 + 26.0}",
                                text_anchor: "middle", dominant_baseline: "middle",
                                fill: "rgba(255,255,255,0.14)", font_size: "10",
                                font_family: "ui-sans-serif,sans-serif",
                                "{c_conn} connection{conn_s}"
                            }
                        }
                    }
                } else {
                    text {
                        x: "{CX0}", y: "{CY0 - 8.0}",
                        text_anchor: "middle", dominant_baseline: "middle",
                        fill: "rgba(255,255,255,0.08)", font_size: "11",
                        font_weight: "500", font_family: "ui-sans-serif,sans-serif",
                        "mesh"
                    }
                    text {
                        x: "{CX0}", y: "{CY0 + 8.0}",
                        text_anchor: "middle", dominant_baseline: "middle",
                        fill: "rgba(255,255,255,0.05)", font_size: "10",
                        font_family: "ui-sans-serif,sans-serif",
                        "{n_nodes} nodes · {n_edges} edges"
                    }
                }
            }

            // ── View-toggle overlay ──────────────────────────────────────────
            div {
                style: "position:absolute;top:14px;left:16px;display:flex;gap:6px;align-items:center;",
                span {
                    style: "font-size:10px;color:rgba(255,255,255,0.22);letter-spacing:0.08em;text-transform:uppercase;margin-right:2px;",
                    "view"
                }
                button {
                    style: "{lbl_pill}",
                    onclick: move |_| show_lbl.toggle(),
                    "labels"
                }
                button {
                    style: "{vol_pill}",
                    onclick: move |_| show_vol.toggle(),
                    "volume"
                }
            }

            // ── Hover tooltip ────────────────────────────────────────────────
            if let Some((t_lbl, t_type, t_rows, t_total)) = tooltip {
                {
                    // Container height = viewport minus 4rem navbar.
                    let container_h = web_sys::window()
                        .and_then(|w| w.inner_height().ok())
                        .and_then(|v| v.as_f64())
                        .unwrap_or(800.0) - 64.0;

                    // Estimate rendered tooltip height: header block + per-row + footer.
                    let est_h = 80.0 + t_rows.len() as f64 * 42.0 + 36.0;

                    let tip_x = hx + 18.0;
                    let tip_y_raw = (hy - 10.0).max(4.0);
                    let tip_y = if tip_y_raw + est_h > container_h {
                        (container_h - est_h - 4.0).max(4.0)
                    } else {
                        tip_y_raw
                    };

                    // Hard cap: tooltip can never be taller than the space below tip_y.
                    let max_h = (container_h - tip_y - 4.0).max(80.0);

                    rsx! {
                        div {
                            style: "position:absolute;pointer-events:none;left:{tip_x}px;top:{tip_y}px;",
                            div {
                                style: "background:rgba(8,8,8,0.97);border:0.5px solid rgba(255,255,255,0.1);border-radius:8px;padding:10px 14px;min-width:200px;max-height:{max_h}px;overflow-y:auto;",
                                div {
                                    style: "font-size:11px;font-weight:500;color:rgba(255,255,255,0.85);margin-bottom:2px;",
                                    "{t_lbl}"
                                }
                                div {
                                    style: "font-size:9px;letter-spacing:0.08em;text-transform:uppercase;color:rgba(255,255,255,0.3);margin-bottom:8px;",
                                    "{t_type}"
                                }
                                for row in t_rows.iter() {
                                    {
                                        let ev = row.ev.clone();
                                        let partner = row.partner.clone();
                                        let vol = row.vol;
                                        let bw = row.bar_w;
                                        rsx! {
                                            div {
                                                style: "padding:3px 0;border-bottom:0.5px solid rgba(255,255,255,0.06);",
                                                div {
                                                    style: "display:flex;align-items:center;gap:16px;margin-bottom:3px;",
                                                    span {
                                                        style: "font-size:10px;color:rgba(255,255,255,0.5);flex:1;min-width:0;",
                                                        "{ev}"
                                                    }
                                                    span {
                                                        style: "font-size:10px;color:rgba(255,255,255,0.55);font-family:monospace;flex-shrink:0;",
                                                        "{vol}/min"
                                                    }
                                                }
                                                div {
                                                    style: "display:flex;align-items:center;gap:6px;",
                                                    div {
                                                        style: "width:{bw}px;height:1.5px;background:rgba(255,255,255,0.35);border-radius:1px;"
                                                    }
                                                    span {
                                                        style: "font-size:10px;color:rgba(255,255,255,0.3);",
                                                        "{partner}"
                                                    }
                                                }
                                            }
                                        }
                                    }
                                }
                                div {
                                    style: "display:flex;justify-content:space-between;padding-top:6px;margin-top:2px;",
                                    span {
                                        style: "font-size:10px;color:rgba(255,255,255,0.25);",
                                        "total"
                                    }
                                    span {
                                        style: "font-size:10px;color:rgba(255,255,255,0.55);font-family:monospace;",
                                        "{t_total}/min"
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
