#![allow(unused_imports)]
use super::*;

// ─────────────────────────────────────────────────────────────────────────────
// SVG trace layer
// ─────────────────────────────────────────────────────────────────────────────

#[component]
pub(super) fn TraceSvg(
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
                            let gc = mix(p, 0.18);
                            let lc = mix(p, 0.90);
                            rsx! {
                                path { d: "{d}", stroke: "{gc}", stroke_width: "4.5", fill: "none", stroke_linecap: "round", stroke_linejoin: "round" }
                                path { d: "{d}", stroke: "{lc}", stroke_width: "1.5", fill: "none", stroke_linecap: "round", stroke_linejoin: "round" }
                                WirePads { pts: pts.clone(), color: lc.clone() }
                                ViaDots  { pts: pts.clone(), color: p.clone() }
                            }
                        }
                        TraceKind::Event => {
                            let col  = "var(--ca)";
                            let gc   = mix(col, 0.13);
                            let lc   = mix(col, 0.80);
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
                                    rect { x: "{mid.0-30.0:.1}", y: "{mid.1-7.0:.1}", width: "60", height: "13", fill: "var(--bg)" }
                                    rect { x: "{mid.0-29.5:.1}", y: "{mid.1-6.5:.1}", width: "59", height: "12", fill: "none", stroke: "{mix(col,0.4)}", stroke_width: "1" }
                                    text {
                                        x: "{mid.0:.1}", y: "{mid.1+1.0:.1}",
                                        text_anchor: "middle", dominant_baseline: "middle",
                                        fill: "{mix(col,0.95)}",
                                        font_family: "var(--font-mono)",
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
                let ghost = mix(&primary, 0.55);
                rsx! { path { d: "{d}", stroke: "{ghost}", stroke_width: "1.5", fill: "none", stroke_dasharray: "5 4", stroke_linecap: "round" } }
            }
        }
    }
}

#[component]
pub(super) fn WirePads(pts: Vec<(f64, f64)>, color: String) -> Element {
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
pub(super) fn ViaDots(pts: Vec<(f64, f64)>, color: String) -> Element {
    let outer = mix(&color, 0.95);
    if pts.len() < 3 {
        return rsx! {};
    }
    rsx! {
        for &(vx,vy) in pts[1..pts.len()-1].iter() {
            circle { cx: "{vx:.1}", cy: "{vy:.1}", r: "2.6", fill: "{outer}" }
            circle { cx: "{vx:.1}", cy: "{vy:.1}", r: "1.1", fill: "var(--bg)" }
        }
    }
}

#[component]
pub(super) fn Chevrons(pts: Vec<(f64, f64)>, color: String) -> Element {
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
