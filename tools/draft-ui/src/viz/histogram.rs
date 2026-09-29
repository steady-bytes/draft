use dioxus::prelude::*;

use super::geom::stack_heights;
use crate::kinds::Tone;

/// One time bucket of a stacked histogram: a title for the hover and one value per class,
/// bottom to top.
#[derive(Clone, Debug, PartialEq)]
pub struct HistoBucket {
    pub title: String,
    pub values: Vec<f64>,
}

/// A stacked histogram (`d-histo`): log volume by severity, event volume by status. Replaces the
/// two bucketing-and-drawing copies in Beacon. `tones[i]` colours class `i`; the "ok" class
/// (primary) is drawn quieter so failures stand out, and every non-zero segment keeps `min_px`
/// so a single error stays visible as a cap.
#[component]
pub fn Histogram(
    buckets: Vec<HistoBucket>,
    tones: Vec<Tone>,
    #[props(default = 72.0)] height: f64,
    #[props(default = 3.0)] min_px: f64,
    /// Time-axis labels, spread evenly under the bars.
    #[props(default)]
    axis: Vec<String>,
    label: Option<String>,
) -> Element {
    let max_total = buckets.iter().map(|b| b.values.iter().sum::<f64>()).fold(0.0, f64::max).max(1.0);
    rsx! {
        div { class: "d-histo", style: "height:{height}px", role: "img", aria_label: label,
            for (i , b) in buckets.into_iter().enumerate() {
                {
                    let heights = stack_heights(&b.values, max_total, height, min_px);
                    rsx! {
                        div { key: "{i}", class: "d-histo-bar", title: "{b.title}",
                            for (j , h) in heights.into_iter().enumerate() {
                                if h > 0.0 {
                                    {
                                        let tone = tones.get(j).copied().unwrap_or_default();
                                        let opacity = if tone == Tone::Primary { "0.55" } else { "1" };
                                        let style = format!("height:{h:.1}px;background:{};opacity:{opacity}", tone.css_var());
                                        rsx! { span { key: "{j}", style: "{style}" } }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
        if !axis.is_empty() {
            div { class: "d-axis",
                for (i , a) in axis.into_iter().enumerate() {
                    span { key: "{i}", "{a}" }
                }
            }
        }
    }
}

/// A legend (`d-legend`): coloured squares, or line swatches for line charts.
#[component]
pub fn Legend(items: Vec<(Tone, String)>, #[props(default)] line: bool) -> Element {
    let class = if line { "d-legend d-legend--line" } else { "d-legend" };
    rsx! {
        div { class: "{class}",
            for (i , (tone , text)) in items.into_iter().enumerate() {
                span { key: "{i}", style: "--c:{tone.css_var()}", "{text}" }
            }
        }
    }
}
