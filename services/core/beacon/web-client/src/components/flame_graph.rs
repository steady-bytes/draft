use dioxus::prelude::*;
use draft_ui::util::format_duration_ns;
use draft_ui::Tone;

use crate::data::{waterfall, SpanLike};

const ROW_HEIGHT: f64 = 26.0;
const ROW_STEP: f64 = 28.0;

/// One trace as nested horizontal bars: width is duration, depth is parent/child nesting and the
/// colour is the service's. Pure presentation — the caller supplies the spans and hears which bar
/// was picked. The Traces view uses the waterfall for this job; the flame graph stays as the
/// wide-event view's alternative shape.
#[component]
pub fn FlameGraph(spans: Vec<SpanLike>, selected: Option<String>, on_select: EventHandler<String>) -> Element {
    if spans.is_empty() {
        return rsx! {
            div { class: "d-empty d-empty--compact", b { "No spans to display" } }
        };
    }
    let layout = waterfall(&spans);
    let max_depth = layout.rows.iter().map(|r| r.depth).max().unwrap_or(0);
    let height = f64::from(max_depth + 1) * ROW_STEP;
    let durations: std::collections::HashMap<&str, u64> = spans.iter().map(|s| (s.id.as_str(), s.duration_ns)).collect();

    rsx! {
        div { class: "bc-flame", style: "height:{height}px",
            for row in layout.rows {
                {
                    let id = row.id.clone();
                    let top = f64::from(row.depth) * ROW_STEP;
                    let text = format!("{} · {}", row.name, format_duration_ns(durations.get(row.id.as_str()).copied().unwrap_or(0)));
                    let tone = if row.err { Tone::Err } else { row.tone };
                    let style = format!("--c:{}; left:{:.3}%; width:{:.3}%; top:{top}px; height:{ROW_HEIGHT}px", tone.css_var(), row.left * 100.0, row.width * 100.0);
                    let class = if row.err { "bc-flame-bar is-err" } else { "bc-flame-bar" };
                    let is_selected = selected.as_deref() == Some(row.id.as_str());
                    rsx! {
                        button {
                            key: "{row.id}",
                            class: "{class}",
                            style: "{style}",
                            r#type: "button",
                            title: "{text} — {row.service}",
                            aria_selected: "{is_selected}",
                            onclick: move |_| on_select.call(id.clone()),
                            "{text}"
                        }
                    }
                }
            }
        }
    }
}
