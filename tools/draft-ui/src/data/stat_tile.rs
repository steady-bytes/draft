use dioxus::prelude::*;

use crate::kinds::Tone;
use crate::viz::Sparkline;

/// The change line under a stat's value. Colour follows whether the change is *good* or *bad*,
/// never its direction (a falling error rate is good).
#[derive(Clone, Debug, PartialEq)]
pub struct Delta {
    pub text: String,
    /// `Some(true)` good, `Some(false)` bad, `None` neutral.
    pub good: Option<bool>,
}

impl Delta {
    pub fn neutral(text: impl Into<String>) -> Self {
        Self { text: text.into(), good: None }
    }

    pub fn good(text: impl Into<String>) -> Self {
        Self { text: text.into(), good: Some(true) }
    }

    pub fn bad(text: impl Into<String>) -> Self {
        Self { text: text.into(), good: Some(false) }
    }
}

/// A stat tile (`d-stat`): label, big value with an optional unit, a delta and a sparkline.
/// Replaces the two near-identical `MetricCard`s and daisyUI's `stat`.
#[component]
pub fn StatTile(
    label: String,
    value: String,
    unit: Option<String>,
    delta: Option<Delta>,
    spark: Option<Vec<f64>>,
    #[props(default = Tone::Primary)] spark_tone: Tone,
    /// Colours the value (`Tone::Err` for a failing count).
    value_tone: Option<Tone>,
) -> Element {
    let value_style = value_tone.map(|t| format!("color:{}", t.css_var())).unwrap_or_default();
    let delta_class = match delta.as_ref().and_then(|d| d.good) {
        Some(true) => "d-delta is-good",
        Some(false) => "d-delta is-bad",
        None => "d-delta",
    };
    rsx! {
        div { class: "d-panel d-stat",
            span { class: "d-label", "{label}" }
            div { class: "d-stat-value", style: "{value_style}",
                "{value}"
                if let Some(u) = unit {
                    small { "{u}" }
                }
            }
            if delta.is_some() || spark.is_some() {
                div { class: "d-stat-foot",
                    if let Some(d) = delta {
                        span { class: "{delta_class}", "{d.text}" }
                    } else {
                        span {}
                    }
                    if let Some(values) = spark {
                        Sparkline { values, tone: spark_tone }
                    }
                }
            }
        }
    }
}
