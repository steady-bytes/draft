use dioxus::prelude::*;

use crate::kinds::Tone;

/// A single-value bar (`d-meter`): event volume, share of total. `percent` is 0–100.
#[component]
pub fn Meter(percent: f64, #[props(default = Tone::Primary)] tone: Tone, label: Option<String>) -> Element {
    let w = percent.clamp(0.0, 100.0);
    let style = format!("width:{w:.1}%;--c:{}", tone.css_var());
    rsx! {
        div { class: "d-meter", role: "img", aria_label: label,
            i { style: "{style}" }
        }
    }
}

#[derive(Clone, Debug, PartialEq)]
pub struct ProgressSegment {
    pub tone: Tone,
    /// Width, 0–100.
    pub percent: f64,
}

impl ProgressSegment {
    pub fn new(tone: Tone, percent: f64) -> Self {
        Self { tone, percent }
    }
}

/// Segmented progress (`d-progress`): a Lineman objective's done / in flight / queued split.
/// `Tone::Quiet` renders as the neutral track colour.
#[component]
pub fn Progress(segments: Vec<ProgressSegment>, #[props(default)] large: bool, label: Option<String>) -> Element {
    let class = if large { "d-progress d-progress--lg" } else { "d-progress" };
    rsx! {
        div { class: "{class}", role: "img", aria_label: label,
            for (i , s) in segments.into_iter().enumerate() {
                {
                    let c = if s.tone == Tone::Quiet { "var(--rule-strong)" } else { s.tone.css_var() };
                    let style = format!("--c:{c}; width:{:.2}%", s.percent.clamp(0.0, 100.0));
                    rsx! { span { key: "{i}", style: "{style}" } }
                }
            }
        }
    }
}

/// A duration and a proportional bar (`d-dur`). `ratio` is 0–1 of the longest in the set.
#[component]
pub fn DurationCell(text: String, ratio: f64, tone: Option<Tone>) -> Element {
    let w = (ratio.clamp(0.0, 1.0) * 100.0).max(0.5);
    let outer = tone.map(|t| format!("--c:{}", t.css_var())).unwrap_or_default();
    rsx! {
        span { class: "d-dur", style: "{outer}",
            span { "{text}" }
            i {
                b { style: "width:{w:.1}%" }
            }
        }
    }
}
