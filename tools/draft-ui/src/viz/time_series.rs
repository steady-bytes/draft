use dioxus::prelude::*;

use super::geom::{compact, nearest, nice_ticks, scale, series_stats};
use crate::kinds::Tone;
use crate::util::format_clock;

// SVG coordinate space, as in the mockups. The svg scales to its container.
const W: f64 = 960.0;
const H: f64 = 260.0;
const L: f64 = 40.0;
const R: f64 = 16.0;
const T: f64 = 12.0;
const B: f64 = 26.0;
/// At most this many hover slots per chart, however many samples there are.
const MAX_SLOTS: usize = 240;

#[derive(Clone, Debug, PartialEq)]
pub struct ChartSeries {
    pub name: String,
    pub tone: Tone,
    /// `(unix seconds, value)`, ascending in time.
    pub points: Vec<(f64, f64)>,
    pub visible: bool,
}

impl ChartSeries {
    pub fn new(name: impl Into<String>, tone: Tone, points: Vec<(f64, f64)>) -> Self {
        Self { name: name.into(), tone, points, visible: true }
    }
}

/// A multi-series line chart with a crosshair and a tooltip (`d-chart`). Replaces Beacon's
/// `TimeSeriesChart` and the hand-rolled chart in Blueprint's Metrics view. Data-agnostic: the
/// caller maps its own samples to [`ChartSeries`].
#[component]
pub fn TimeSeriesChart(
    series: Vec<ChartSeries>,
    #[props(default)] unit: String,
    #[props(default = "Line chart".to_string())] label: String,
) -> Element {
    let mut hover: Signal<Option<usize>> = use_signal(|| None);
    let visible: Vec<&ChartSeries> = series.iter().filter(|s| s.visible && !s.points.is_empty()).collect();
    if visible.is_empty() {
        return rsx! {
            div { class: "d-empty d-empty--compact", b { "No data points in range" } }
        };
    }

    // Domains.
    let (mut x0, mut x1) = (f64::INFINITY, f64::NEG_INFINITY);
    let (mut y_min, mut y_max) = (f64::INFINITY, f64::NEG_INFINITY);
    for s in &visible {
        for &(x, y) in &s.points {
            x0 = x0.min(x);
            x1 = x1.max(x);
            y_min = y_min.min(y);
            y_max = y_max.max(y);
        }
    }
    // Counters and rates start at zero; anything else gets its own range.
    let base = if y_min >= 0.0 { 0.0 } else { y_min };
    let ticks = nice_ticks(base, y_max, 4);
    let (ya, yb) = (ticks.first().copied().unwrap_or(base), ticks.last().copied().unwrap_or(y_max));
    let px = |x: f64| scale(x, x0, x1, L, W - R);
    let py = |y: f64| scale(y, ya, yb, H - B, T);

    // Hover slots come from the longest visible series.
    let longest = visible.iter().max_by_key(|s| s.points.len()).copied().unwrap_or(visible[0]);
    let step = longest.points.len().div_ceil(MAX_SLOTS).max(1);
    let slots: Vec<f64> = longest.points.iter().step_by(step).map(|p| p.0).collect();
    let slot_w = ((W - L - R) / slots.len().max(1) as f64).max(1.0);

    let hover_x = hover().and_then(|i| slots.get(i).copied());

    rsx! {
        div { class: "d-chart",
            svg {
                view_box: "0 0 {W} {H}",
                role: "img",
                "aria-label": "{label}",
                onmouseleave: move |_| hover.set(None),
                g { class: "grid",
                    for (i , t) in ticks.iter().enumerate() {
                        line { key: "{i}", x1: "{L}", x2: "{W - R}", y1: "{py(*t)}", y2: "{py(*t)}" }
                    }
                }
                g { class: "axis",
                    for (i , t) in ticks.iter().enumerate() {
                        text { key: "{i}", x: "{L - 8.0}", y: "{py(*t) + 4.0}", text_anchor: "end", "{compact(*t)}" }
                    }
                    for j in 0..=6usize {
                        {
                            let x = x0 + (x1 - x0) * j as f64 / 6.0;
                            let anchor = if j == 0 { "start" } else if j == 6 { "end" } else { "middle" };
                            let text = if j == 6 { format!("{} UTC", format_clock(x as i64)) } else { format_clock(x as i64) };
                            rsx! { text { key: "{j}", x: "{px(x)}", y: "{H - 6.0}", text_anchor: "{anchor}", "{text}" } }
                        }
                    }
                }
                g { class: "series",
                    for (i , s) in visible.iter().enumerate() {
                        {
                            let d = path(&s.points, &px, &py);
                            let stroke = s.tone.css_var();
                            let last = s.points.last().copied().unwrap_or((x0, 0.0));
                            rsx! {
                                Fragment { key: "{i}",
                                    path { d: "{d}", stroke: "{stroke}" }
                                    circle { class: "end", cx: "{px(last.0)}", cy: "{py(last.1)}", r: "4", fill: "{stroke}" }
                                }
                            }
                        }
                    }
                }
                if let Some(x) = hover_x {
                    line { class: "xhair", x1: "{px(x)}", x2: "{px(x)}", y1: "{T}", y2: "{H - B}" }
                    for (i , s) in visible.iter().enumerate() {
                        {
                            let xs: Vec<f64> = s.points.iter().map(|p| p.0).collect();
                            let dot = nearest(&xs, x).map(|k| s.points[k]);
                            rsx! {
                                if let Some((dx , dy)) = dot {
                                    circle { key: "{i}", class: "hover-dot", cx: "{px(dx)}", cy: "{py(dy)}", r: "4", fill: "{s.tone.css_var()}" }
                                }
                            }
                        }
                    }
                }
                // Invisible hit slots: no pointer-coordinate maths, so it works at any size.
                for (i , x) in slots.iter().enumerate() {
                    rect {
                        key: "h{i}",
                        x: "{px(*x) - slot_w / 2.0}",
                        y: "{T}",
                        width: "{slot_w}",
                        height: "{H - B - T}",
                        fill: "transparent",
                        onmouseenter: move |_| hover.set(Some(i)),
                    }
                }
            }
            if let Some(x) = hover_x {
                {
                    let pct = (px(x) / W * 100.0).clamp(0.0, 100.0);
                    let side = if pct > 60.0 { format!("right:{:.1}%", 100.0 - pct + 1.0) } else { format!("left:{:.1}%", pct + 1.0) };
                    rsx! {
                        div { class: "d-chart-tip", style: "{side}",
                            b { "{format_clock(x as i64)} UTC" }
                            for (i , s) in visible.iter().enumerate() {
                                {
                                    let xs: Vec<f64> = s.points.iter().map(|p| p.0).collect();
                                    let v = nearest(&xs, x).map(|k| s.points[k].1).unwrap_or(0.0);
                                    rsx! {
                                        div { key: "{i}", style: "--c:{s.tone.css_var()}",
                                            i {}
                                            span { "{s.name}" }
                                            span { "{v:.2}{unit}" }
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
}

fn path(points: &[(f64, f64)], px: &impl Fn(f64) -> f64, py: &impl Fn(f64) -> f64) -> String {
    let mut d = String::with_capacity(points.len() * 14);
    for (i, &(x, y)) in points.iter().enumerate() {
        d.push_str(&format!("{}{:.1} {:.1} ", if i == 0 { "M" } else { "L" }, px(x), py(y)));
    }
    d.trim_end().to_string()
}

/// The table under a line chart: one row per series with a visibility checkbox, a colour key and
/// last / min / avg / max. Hidden series are dimmed.
#[component]
pub fn SeriesTable(series: Vec<ChartSeries>, on_toggle: EventHandler<usize>, #[props(default)] unit: String) -> Element {
    rsx! {
        div { class: "d-panel d-table-wrap",
            table { class: "d-table",
                thead {
                    tr {
                        th { "Series" }
                        th { class: "is-right", "Last" }
                        th { class: "is-right", "Min" }
                        th { class: "is-right", "Avg" }
                        th { class: "is-right", "Max" }
                    }
                }
                tbody {
                    for (i , s) in series.into_iter().enumerate() {
                        {
                            let values: Vec<f64> = s.points.iter().map(|p| p.1).collect();
                            let stats = series_stats(&values);
                            let row = if s.visible { "" } else { "is-off" };
                            let fmt = |v: f64| format!("{v:.2}{unit}");
                            rsx! {
                                tr { key: "{i}", class: "{row}",
                                    td {
                                        label { class: "d-row", style: "gap:10px;cursor:pointer",
                                            input {
                                                class: "d-check",
                                                r#type: "checkbox",
                                                checked: s.visible,
                                                onchange: move |_| on_toggle.call(i),
                                            }
                                            span { class: "d-sw", style: "--c:{s.tone.css_var()}" }
                                            "{s.name}"
                                        }
                                    }
                                    if let Some((last , min , avg , max)) = stats {
                                        td { class: "is-right", "{fmt(last)}" }
                                        td { class: "is-right is-dim", "{fmt(min)}" }
                                        td { class: "is-right is-dim", "{fmt(avg)}" }
                                        td { class: "is-right is-dim", "{fmt(max)}" }
                                    } else {
                                        td { class: "is-right is-dim", colspan: "4", "no samples" }
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
