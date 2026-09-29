//! Small pieces the four signal views share.

use dioxus::prelude::*;
use draft_ui::data::{Kv, KvItem};
use draft_ui::layout::DrawerBlock;
use draft_ui::viz::{Histogram, Legend};
use draft_ui::ui::{Status, Tag};
use draft_ui::Tone;

use crate::data::{axis_labels, bucketize, Severity, SpanStatus, HISTO_BUCKETS};
use crate::Route;

/// The `key=value` cells of a row's attributes: keys quiet, values in ink, the whole run
/// truncated by the cell's width.
#[component]
pub fn Attrs(attrs: Vec<(String, String)>) -> Element {
    rsx! {
        span { class: "d-trunc",
            for (k , v) in attrs {
                span { class: "d-attr-k", "{k}=" }
                "{v} "
            }
        }
    }
}

/// A named block of key/value attributes in a drawer (`Kv`/`KvItem` under a `DrawerBlock` title) —
/// shared by the Traces and Wide events drawers, both of which show a span's plain OTel
/// `attributes` alongside its WideEvent's `business_attributes`/`runtime_attributes` (see
/// docs/website/content/docs/architecture/wide-events.md's Data Model for why those are three
/// separate maps, not one). Renders nothing when `attrs` is empty, so an unused block (a span with
/// no WideEvent recorded, or one with attributes but no business context) doesn't leave a
/// title over nothing.
#[component]
pub fn AttrBlock(title: String, attrs: Vec<(String, String)>) -> Element {
    if attrs.is_empty() {
        return rsx! {};
    }
    rsx! {
        DrawerBlock { title,
            Kv {
                for (k , v) in attrs {
                    KvItem { key: "{k}", label: k.clone(), "{v}" }
                }
            }
        }
    }
}

/// A log's severity as a tag.
#[component]
pub fn SeverityTag(text: String) -> Element {
    let tone = Severity::of(&text).tone();
    let label = crate::data::severity_label(&text);
    rsx! {
        Tag { tone, "{label}" }
    }
}

/// A span's or wide event's status.
#[component]
pub fn StatusText(code: String) -> Element {
    let status = SpanStatus::of(&code);
    rsx! {
        Status { kind: status.kind(), "{status.label()}" }
    }
}

/// The stacked volume histogram in its panel: a title, a legend with a total per class, the bars
/// and a time axis. `points` are `(time in ns, class)`; `names` and `tones` are per class, bottom
/// of the stack first. Renders nothing when there is no time span to draw.
#[component]
pub fn VolumePanel(title: String, label: String, points: Vec<(i64, usize)>, names: Vec<&'static str>, tones: Vec<Tone>) -> Element {
    let Some(b) = bucketize(&points, &names, HISTO_BUCKETS) else {
        return rsx! {};
    };
    let axis = axis_labels(b.start_ns, b.end_ns);
    // Legend reads top of the stack first, and skips classes with no rows.
    let legend: Vec<(Tone, String)> = (0..names.len())
        .rev()
        .filter(|&i| b.totals[i] > 0)
        .map(|i| (tones[i], format!("{} {}", capitalise(names[i]), b.totals[i])))
        .collect();
    rsx! {
        div { class: "d-panel bc-histo",
            div { class: "d-panel-head",
                span { class: "d-label", "{title}" }
                span { class: "d-spacer" }
                Legend { items: legend }
            }
            div { class: "d-panel-body",
                Histogram { buckets: b.buckets, tones, axis, label }
            }
        }
    }
}

fn capitalise(s: &str) -> String {
    let mut c = s.chars();
    c.next().map(|f| f.to_uppercase().collect::<String>() + c.as_str()).unwrap_or_default()
}

/// Opens a trace in the Traces view: leaves the id for it to pick up, then navigates.
pub fn use_open_trace() -> Callback<String> {
    let nav = use_navigator();
    use_callback(move |trace_id: String| {
        *crate::PENDING_TRACE_ID.write() = Some(trace_id);
        nav.push(Route::Traces {});
    })
}

/// Opens the Logs view with a BeaconQL filter already applied.
pub fn use_open_logs() -> Callback<String> {
    let nav = use_navigator();
    use_callback(move |filter: String| {
        *crate::PENDING_LOG_FILTER.write() = Some(filter);
        nav.push(Route::Stream {});
    })
}
