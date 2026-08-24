use chrono::DateTime;
use dioxus::prelude::*;

use draft_api::proto::core_observability_logs_v1::LogRecord;

// BUCKETS/CHART_HEIGHT_PX size the strip to read as a real chart — tall
// enough to show shape at a glance, not a hairline afterthought. BUCKETS is
// deliberately higher than the bar container is ever likely to have pixels
// for one-per-bucket at a comfortable width — each bar is `flex:1`, so more
// buckets means narrower bars packed into the same card, trading individual
// bar width for finer time resolution.
const BUCKETS: usize = 120;
const CHART_HEIGHT_PX: u32 = 71;

#[derive(Clone, Copy, Default)]
struct BucketCounts {
    error: u32,
    warn: u32,
    info: u32,
    other: u32,
}

impl BucketCounts {
    fn total(&self) -> u32 {
        self.error + self.warn + self.info + self.other
    }
}

/// SeverityHistogram is a quiet, always-current volume-by-severity chart
/// above the Logs table (Phase 17 of the gap-analysis follow-up). It buckets
/// whatever rows are *currently displayed* client-side rather than issuing a
/// new aggregation query — BeaconQL has no aggregate functions (see the gap
/// table's "Aggregation / Group By" row, still low-priority/unstarted), and
/// the already-loaded `lines` buffer (capped at MAX_LINES in stream.rs) is
/// exactly the data the table itself is showing, so the histogram and the
/// rows underneath it never disagree about what's "on screen." Being a plain
/// function of the `lines` prop, it re-buckets and re-renders for free on
/// every new row the live tail receives — no separate polling or state.
///
/// Framed as a labeled card (title, severity legend, start/end time axis)
/// rather than a bare strip of bars — a chart with no key and no axis reads
/// as decoration; the labels are what make it legible as *data*.
#[component]
pub fn SeverityHistogram(lines: Vec<LogRecord>) -> Element {
    let Some((min_ns, max_ns)) = time_span(&lines) else {
        return rsx! {};
    };

    let buckets = bucket(&lines, min_ns, max_ns);
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
                    "Volume by severity"
                }
                div { class: "flex items-center gap-3",
                    LegendItem { color: "var(--color-error)", label: "error" }
                    LegendItem { color: "var(--color-warning)", label: "warn" }
                    LegendItem { color: "var(--color-info)", label: "info" }
                    LegendItem { color: "var(--color-base-content)", label: "other", opacity: "0.4" }
                }
            }

            div {
                style: "display:flex; align-items:flex-end; gap:1px; height:{CHART_HEIGHT_PX}px;",
                for b in buckets.iter() {
                    {
                        let scale = |n: u32| (n * CHART_HEIGHT_PX) / max_total;
                        let title = format!(
                            "error {} · warn {} · info {} · other {}",
                            b.error, b.warn, b.info, b.other,
                        );
                        rsx! {
                            div {
                                title: "{title}",
                                style: "flex:1; min-width:1px; display:flex; flex-direction:column-reverse; height:100%;",
                                if b.error > 0 {
                                    div { style: "height:{scale(b.error)}px; background:var(--color-error); opacity:0.65;" }
                                }
                                if b.warn > 0 {
                                    div { style: "height:{scale(b.warn)}px; background:var(--color-warning); opacity:0.65;" }
                                }
                                if b.info > 0 {
                                    div { style: "height:{scale(b.info)}px; background:var(--color-info); opacity:0.65;" }
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
fn LegendItem(color: String, label: String, #[props(default = "0.65".to_string())] opacity: String) -> Element {
    rsx! {
        div { class: "flex items-center gap-1",
            span {
                style: "width:6px; height:6px; border-radius:1px; background:{color}; opacity:{opacity};",
            }
            span { class: "text-[10px] text-base-content/40", "{label}" }
        }
    }
}

fn timestamp_nanos(record: &LogRecord) -> Option<i64> {
    let ts = record.timestamp.as_ref()?;
    Some(ts.seconds * 1_000_000_000 + ts.nanos as i64)
}

/// time_span returns (min, max) unix-nanos across `lines`, or None if there
/// aren't at least two distinct timestamps to bucket between.
fn time_span(lines: &[LogRecord]) -> Option<(i64, i64)> {
    let mut min_ns = i64::MAX;
    let mut max_ns = i64::MIN;
    for r in lines {
        if let Some(ns) = timestamp_nanos(r) {
            min_ns = min_ns.min(ns);
            max_ns = max_ns.max(ns);
        }
    }
    if min_ns >= max_ns {
        return None;
    }
    Some((min_ns, max_ns))
}

fn bucket(lines: &[LogRecord], min_ns: i64, max_ns: i64) -> Vec<BucketCounts> {
    let span = (max_ns - min_ns) as f64;
    let mut buckets = vec![BucketCounts::default(); BUCKETS];
    for r in lines {
        let Some(ns) = timestamp_nanos(r) else { continue };
        let frac = (ns - min_ns) as f64 / span;
        let idx = ((frac * BUCKETS as f64) as usize).min(BUCKETS - 1);
        let b = &mut buckets[idx];
        match r.severity_text.to_ascii_uppercase().as_str() {
            "FATAL" | "ERROR" => b.error += 1,
            "WARN" | "WARNING" => b.warn += 1,
            "INFO" => b.info += 1,
            _ => b.other += 1,
        }
    }
    buckets
}

/// format_axis_time renders a bucket-boundary timestamp for the axis labels.
/// The chart's own span is usually well under a day (the live buffer caps at
/// MAX_LINES rows), so HH:MM:SS carries the useful precision; a date would
/// mostly just be visual noise here.
fn format_axis_time(unix_nanos: i64) -> String {
    let seconds = unix_nanos.div_euclid(1_000_000_000);
    let nanos = unix_nanos.rem_euclid(1_000_000_000) as u32;
    match DateTime::from_timestamp(seconds, nanos) {
        Some(dt) => dt.format("%H:%M:%S").to_string(),
        None => "—".to_string(),
    }
}
