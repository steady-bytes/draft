use chrono::DateTime;
use dioxus::prelude::*;

use draft_api::proto::core_observability_wide_events_v1::WideEvent;

// Same shape as components/severity_histogram.rs (see that file's own doc
// comment for the full rationale — client-side bucketing of whatever
// WideEvents are currently loaded, since BeaconQL has no aggregate
// functions), broken down by status_code rather than log severity: the
// closest direct analog (a small, fixed set of outcome categories) rather
// than an unbounded dimension like service_name/span_name, which would need
// a dynamic color palette instead of SeverityHistogram's hardcoded ones.
const BUCKETS: usize = 120;
const CHART_HEIGHT_PX: u32 = 71;

#[derive(Clone, Copy, Default)]
struct BucketCounts {
    error: u32,
    ok: u32,
    other: u32,
}

impl BucketCounts {
    fn total(&self) -> u32 {
        self.error + self.ok + self.other
    }
}

/// WideEventHistogram is a quiet, always-current volume-by-status_code chart
/// above the WideEvents table — the WideEvents equivalent of SeverityHistogram
/// above the Logs table. See that component's doc comment for why this
/// buckets client-side rather than issuing a separate aggregation query.
#[component]
pub fn WideEventHistogram(events: Vec<WideEvent>) -> Element {
    let Some((min_ns, max_ns)) = time_span(&events) else {
        return rsx! {};
    };

    let buckets = bucket(&events, min_ns, max_ns);
    let max_total = buckets.iter().map(BucketCounts::total).max().unwrap_or(0);
    if max_total == 0 {
        return rsx! {};
    }

    let start_label = format_axis_time(min_ns);
    let end_label = format_axis_time(max_ns);

    rsx! {
        div { class: "bg-base-200 border border-base-300 rounded-lg px-3 py-2.5 flex flex-col gap-2",
            div { class: "flex items-center justify-between",
                span {
                    class: "text-[10px] uppercase tracking-wide text-base-content/40 font-semibold",
                    "Volume by status"
                }
                div { class: "flex items-center gap-3",
                    LegendItem { color: "var(--color-error)", label: "error" }
                    LegendItem { color: "var(--color-success)", label: "ok" }
                    LegendItem { color: "var(--color-base-content)", label: "other", opacity: "0.4" }
                }
            }

            div {
                style: "display:flex; align-items:flex-end; gap:1px; height:{CHART_HEIGHT_PX}px;",
                for b in buckets.iter() {
                    {
                        let scale = |n: u32| (n * CHART_HEIGHT_PX) / max_total;
                        let title = format!("error {} · ok {} · other {}", b.error, b.ok, b.other);
                        rsx! {
                            div {
                                title: "{title}",
                                style: "flex:1; min-width:1px; display:flex; flex-direction:column-reverse; height:100%;",
                                if b.error > 0 {
                                    div { style: "height:{scale(b.error)}px; background:var(--color-error); opacity:0.65;" }
                                }
                                if b.ok > 0 {
                                    div { style: "height:{scale(b.ok)}px; background:var(--color-success); opacity:0.65;" }
                                }
                                if b.other > 0 {
                                    div { style: "height:{scale(b.other)}px; background:var(--color-base-content); opacity:0.15;" }
                                }
                            }
                        }
                    }
                }
            }

            div { class: "flex items-center justify-between text-[10px] font-mono text-base-content/35 pt-0.5 border-t border-base-300",
                span { "{start_label}" }
                span { "UTC" }
                span { "{end_label}" }
            }
        }
    }
}

#[component]
fn LegendItem(
    color: String,
    label: String,
    #[props(default = "0.65".to_string())] opacity: String,
) -> Element {
    rsx! {
        div { class: "flex items-center gap-1",
            span {
                style: "width:6px; height:6px; border-radius:1px; background:{color}; opacity:{opacity};",
            }
            span { class: "text-[10px] text-base-content/40", "{label}" }
        }
    }
}

fn timestamp_nanos(event: &WideEvent) -> Option<i64> {
    let ts = event.start_time.as_ref()?;
    Some(ts.seconds * 1_000_000_000 + ts.nanos as i64)
}

/// time_span returns (min, max) unix-nanos across `events`, or None if there
/// aren't at least two distinct timestamps to bucket between.
fn time_span(events: &[WideEvent]) -> Option<(i64, i64)> {
    let mut min_ns = i64::MAX;
    let mut max_ns = i64::MIN;
    for e in events {
        if let Some(ns) = timestamp_nanos(e) {
            min_ns = min_ns.min(ns);
            max_ns = max_ns.max(ns);
        }
    }
    if min_ns >= max_ns {
        return None;
    }
    Some((min_ns, max_ns))
}

fn bucket(events: &[WideEvent], min_ns: i64, max_ns: i64) -> Vec<BucketCounts> {
    let span = (max_ns - min_ns) as f64;
    let mut buckets = vec![BucketCounts::default(); BUCKETS];
    for e in events {
        let Some(ns) = timestamp_nanos(e) else {
            continue;
        };
        let frac = (ns - min_ns) as f64 / span;
        let idx = ((frac * BUCKETS as f64) as usize).min(BUCKETS - 1);
        let b = &mut buckets[idx];
        match e.status_code.to_ascii_uppercase().as_str() {
            "ERROR" | "STATUS_CODE_ERROR" => b.error += 1,
            "OK" | "STATUS_CODE_OK" => b.ok += 1,
            _ => b.other += 1,
        }
    }
    buckets
}

/// format_axis_time renders a bucket-boundary timestamp for the axis labels.
fn format_axis_time(unix_nanos: i64) -> String {
    let seconds = unix_nanos.div_euclid(1_000_000_000);
    let nanos = unix_nanos.rem_euclid(1_000_000_000) as u32;
    match DateTime::from_timestamp(seconds, nanos) {
        Some(dt) => dt.format("%H:%M:%S").to_string(),
        None => "—".to_string(),
    }
}
