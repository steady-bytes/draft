use draft_api::proto::core_observability_metrics_v1::TimeSeries;
use dioxus::prelude::*;

// Fixed layout constants for the SVG coordinate space, mirroring
// flame_graph.rs's fixed-viewBox-scales-to-container approach: every point is
// computed as a fraction of CHART_WIDTH/CHART_HEIGHT, so the <svg> renders
// correctly at whatever pixel size the caller places it.
const CHART_WIDTH: f64 = 1000.0;
const CHART_HEIGHT: f64 = 260.0;
const PADDING_LEFT: f64 = 48.0;
const PADDING_BOTTOM: f64 = 24.0;
const PADDING_TOP: f64 = 12.0;

/// TimeSeriesChart is Beacon's Metrics drill-down view (Phase 9): a
/// multi-series SVG line chart plotting every sample of every `TimeSeries` a
/// `QueryMetrics` call returned, one polyline per distinct label set, colored
/// deterministically the same way FlameGraph colors spans by service_name.
/// Pure presentation — `series` comes straight from a QueryMetricsResponse.
#[component]
pub fn TimeSeriesChart(series: Vec<TimeSeries>) -> Element {
    let non_empty: Vec<&TimeSeries> = series.iter().filter(|s| !s.samples.is_empty()).collect();
    if non_empty.is_empty() {
        return rsx! {
            div { class: "text-center text-base-content/40 py-12 text-sm", "No data points in range" }
        };
    }

    let mut min_ts = i64::MAX;
    let mut max_ts = i64::MIN;
    let mut min_val = f64::INFINITY;
    let mut max_val = f64::NEG_INFINITY;
    for s in &non_empty {
        for sample in &s.samples {
            if let Some(ts) = &sample.timestamp {
                min_ts = min_ts.min(ts.seconds);
                max_ts = max_ts.max(ts.seconds);
            }
            min_val = min_val.min(sample.value);
            max_val = max_val.max(sample.value);
        }
    }
    // A flat/constant series would otherwise divide by zero when normalizing
    // — widen the value range symmetrically so a single-value line still
    // renders (as a flat mid-height line) rather than collapsing to NaN.
    if (max_val - min_val).abs() < f64::EPSILON {
        min_val -= 1.0;
        max_val += 1.0;
    }
    let ts_range = ((max_ts - min_ts).max(1)) as f64;
    let val_range = max_val - min_val;

    let plot_w = CHART_WIDTH - PADDING_LEFT;
    let plot_h = CHART_HEIGHT - PADDING_TOP - PADDING_BOTTOM;

    let x_for = move |seconds: i64| -> f64 {
        PADDING_LEFT + (seconds - min_ts) as f64 / ts_range * plot_w
    };
    let y_for = move |value: f64| -> f64 {
        PADDING_TOP + plot_h - (value - min_val) / val_range * plot_h
    };

    let view_box = format!("0 0 {CHART_WIDTH} {CHART_HEIGHT}");

    rsx! {
        div { class: "flex flex-col gap-2",
            svg {
                class: "w-full bg-base-200 rounded",
                "viewBox": "{view_box}",
                preserve_aspect_ratio: "none",
                // Y-axis gridlines/labels at min, mid, max.
                for (frac , label) in [(0.0, max_val), (0.5, (max_val + min_val) / 2.0), (1.0, min_val)] {
                    {
                        let y = PADDING_TOP + plot_h * frac;
                        rsx! {
                            line {
                                x1: "{PADDING_LEFT}",
                                x2: "{CHART_WIDTH}",
                                y1: "{y}",
                                y2: "{y}",
                                stroke: "currentColor",
                                class: "text-base-content/10",
                                "stroke-width": "1",
                            }
                            text {
                                x: "4",
                                y: "{y + 4.0}",
                                "font-size": "10",
                                fill: "currentColor",
                                class: "text-base-content/50",
                                "{format_axis_value(label)}"
                            }
                        }
                    }
                }
                for s in non_empty.iter() {
                    {
                        let color = series_color(&label_signature(&s.metric_name, &s.labels));
                        let points = s
                            .samples
                            .iter()
                            .filter_map(|sample| {
                                let ts = sample.timestamp.as_ref()?;
                                Some(format!("{:.1},{:.1}", x_for(ts.seconds), y_for(sample.value)))
                            })
                            .collect::<Vec<_>>()
                            .join(" ");
                        rsx! {
                            polyline {
                                points: "{points}",
                                stroke: "{color}",
                                "stroke-width": "2",
                                fill: "none",
                                stroke_linecap: "round",
                                stroke_linejoin: "round",
                            }
                        }
                    }
                }
            }
            div { class: "flex flex-wrap gap-3 px-1",
                for s in non_empty.iter() {
                    {
                        let sig = label_signature(&s.metric_name, &s.labels);
                        let color = series_color(&sig);
                        rsx! {
                            div { class: "flex items-center gap-1.5 text-xs text-base-content/70",
                                span {
                                    class: "inline-block w-2.5 h-2.5 rounded-full",
                                    style: "background-color: {color}",
                                }
                                span { class: "font-mono", "{sig}" }
                            }
                        }
                    }
                }
            }
        }
    }
}

/// label_signature builds a deterministic, human-readable identity for one
/// series — the metric name (when present, i.e. not aggregated away) plus
/// its sorted label pairs — used both for the legend text and as the input
/// to series_color, so the same series always gets the same color across
/// renders.
pub fn label_signature(metric_name: &str, labels: &std::collections::HashMap<String, String>) -> String {
    let mut pairs: Vec<String> = labels.iter().map(|(k, v)| format!("{k}={v}")).collect();
    pairs.sort();
    if metric_name.is_empty() {
        pairs.join(", ")
    } else if pairs.is_empty() {
        metric_name.to_string()
    } else {
        format!("{metric_name}{{{}}}", pairs.join(", "))
    }
}

/// series_color deterministically maps a series signature to an HSL color —
/// same FNV-1a-to-hue technique as flame_graph.rs's service_color, so the
/// same series renders the same color every time without a hardcoded table.
fn series_color(signature: &str) -> String {
    let hash: u32 = signature
        .bytes()
        .fold(2166136261u32, |acc, b| (acc ^ b as u32).wrapping_mul(16777619));
    let hue = hash % 360;
    format!("hsl({hue}, 70%, 55%)")
}

fn format_axis_value(v: f64) -> String {
    if v.abs() >= 1_000_000.0 {
        format!("{:.1}M", v / 1_000_000.0)
    } else if v.abs() >= 1_000.0 {
        format!("{:.1}k", v / 1_000.0)
    } else if v.abs() >= 10.0 {
        format!("{v:.0}")
    } else {
        format!("{v:.2}")
    }
}
