use std::collections::HashMap;

use dioxus::prelude::*;

use draft_api::proto::core_observability_traces_v1::Span;

// Fixed layout constants for the SVG coordinate space — the viewBox is always
// `0 0 CHART_WIDTH svg_height`, so every span's x/width is computed as a
// fraction of CHART_WIDTH and scales with whatever pixel width the caller
// renders the <svg> at.
const CHART_WIDTH: f64 = 1000.0;
const ROW_HEIGHT: f64 = 26.0;
const ROW_GAP: f64 = 2.0;
const MIN_BAR_WIDTH: f64 = 2.0;

/// FlameGraph renders one trace's spans as nested horizontal bars — width
/// proportional to duration, depth by parent/child nesting, color-coded by
/// `service_name` — per the design doc's Traces view description. It is pure
/// presentation: `spans` comes from a `GetTrace` response (views/traces.rs),
/// and clicking a bar reports the clicked `Span` via `on_select` so the
/// caller can render an attribute/status detail panel.
#[component]
pub fn FlameGraph(spans: Vec<Span>, on_select: EventHandler<Span>) -> Element {
    if spans.is_empty() {
        return rsx! {
            div { class: "text-center text-base-content/40 py-8 text-sm", "No spans to display" }
        };
    }

    let by_id: HashMap<&str, &Span> = spans.iter().map(|s| (s.span_id.as_str(), s)).collect();
    let mut depth_cache: HashMap<&str, usize> = HashMap::new();
    let positioned: Vec<PositionedSpan> = spans
        .iter()
        .map(|s| PositionedSpan {
            span: s.clone(),
            depth: compute_depth(&s.span_id, &by_id, &mut depth_cache),
            start_ns: timestamp_to_nanos(&s.start_time),
        })
        .collect();

    let trace_start_ns = positioned.iter().map(|p| p.start_ns).min().unwrap_or(0);
    let trace_end_ns = positioned
        .iter()
        .map(|p| p.start_ns + p.span.duration_ns as i128)
        .max()
        .unwrap_or(trace_start_ns + 1);
    let total_ns = (trace_end_ns - trace_start_ns).max(1) as f64;

    let max_depth = positioned.iter().map(|p| p.depth).max().unwrap_or(0);
    let svg_height = (max_depth + 1) as f64 * ROW_HEIGHT + 8.0;
    let view_box = format!("0 0 {CHART_WIDTH} {svg_height}");
    let height_attr = format!("{svg_height}");

    rsx! {
        svg {
            class: "w-full",
            "viewBox": "{view_box}",
            height: "{height_attr}",
            preserve_aspect_ratio: "none",
            for p in positioned {
                {
                    let left = (p.start_ns - trace_start_ns) as f64 / total_ns * CHART_WIDTH;
                    let width = (p.span.duration_ns as f64 / total_ns * CHART_WIDTH).max(MIN_BAR_WIDTH);
                    let y = p.depth as f64 * ROW_HEIGHT;
                    let bar_height = ROW_HEIGHT - ROW_GAP;
                    let fill = service_color(&p.span.service_name);
                    let is_error = p.span.status_code == "STATUS_CODE_ERROR";
                    let stroke = if is_error { "#f43f5e" } else { "none" };
                    let stroke_width = if is_error { "1.5" } else { "0" };
                    let label = span_label(&p.span);
                    let text_x = left + 4.0;
                    let text_y = y + bar_height / 2.0 + 4.0;
                    let span_for_click = p.span.clone();
                    let tooltip = format!(
                        "{} ({}) — {} — {}",
                        p.span.span_name,
                        p.span.service_name,
                        format_duration_ns(p.span.duration_ns),
                        p.span.status_code,
                    );
                    rsx! {
                        g {
                            class: "cursor-pointer",
                            onclick: move |_| on_select.call(span_for_click.clone()),
                            title { "{tooltip}" }
                            rect {
                                x: "{left}",
                                y: "{y}",
                                width: "{width}",
                                height: "{bar_height}",
                                rx: "2",
                                fill: "{fill}",
                                stroke: "{stroke}",
                                "stroke-width": "{stroke_width}",
                                class: "hover:opacity-80",
                            }
                            text {
                                x: "{text_x}",
                                y: "{text_y}",
                                "font-size": "10",
                                fill: "white",
                                class: "pointer-events-none select-none",
                                "{label}"
                            }
                        }
                    }
                }
            }
        }
    }
}

struct PositionedSpan {
    span: Span,
    depth: usize,
    start_ns: i128,
}

/// compute_depth walks parent_span_id links to find how deeply nested `span_id`
/// is within the current span set, memoizing as it goes. A span whose parent
/// isn't present in `by_id` (the trace's own root, or a truncated/partial
/// export) is treated as depth 0 rather than panicking or looping forever on
/// malformed data (e.g. a self-referential parent_span_id).
fn compute_depth<'a>(
    span_id: &'a str,
    by_id: &HashMap<&'a str, &'a Span>,
    cache: &mut HashMap<&'a str, usize>,
) -> usize {
    if let Some(d) = cache.get(span_id) {
        return *d;
    }
    let depth = match by_id.get(span_id) {
        None => 0,
        Some(span) => {
            let parent_id = span.parent_span_id.as_str();
            if parent_id.is_empty() || parent_id == span_id || !by_id.contains_key(parent_id) {
                0
            } else {
                compute_depth(parent_id, by_id, cache) + 1
            }
        }
    };
    cache.insert(span_id, depth);
    depth
}

fn timestamp_to_nanos(ts: &Option<prost_types::Timestamp>) -> i128 {
    match ts {
        Some(t) => t.seconds as i128 * 1_000_000_000 + t.nanos as i128,
        None => 0,
    }
}

fn span_label(span: &Span) -> String {
    format!("{} · {}", span.span_name, format_duration_ns(span.duration_ns))
}

pub fn format_duration_ns(ns: u64) -> String {
    if ns >= 1_000_000_000 {
        format!("{:.2}s", ns as f64 / 1_000_000_000.0)
    } else if ns >= 1_000_000 {
        format!("{:.2}ms", ns as f64 / 1_000_000.0)
    } else if ns >= 1_000 {
        format!("{:.2}µs", ns as f64 / 1_000.0)
    } else {
        format!("{ns}ns")
    }
}

/// service_color deterministically maps a service_name to an HSL color so the
/// same service always renders with the same color across traces/renders,
/// without maintaining a hardcoded service→color table. FNV-1a keeps the hash
/// dependency-free.
fn service_color(service_name: &str) -> String {
    let hash: u32 = service_name
        .bytes()
        .fold(2166136261u32, |acc, b| (acc ^ b as u32).wrapping_mul(16777619));
    let hue = hash % 360;
    format!("hsl({hue}, 65%, 45%)")
}
