use dioxus::prelude::*;

use crate::kinds::Tone;

/// One span in a full waterfall.
#[derive(Clone, Debug, PartialEq)]
pub struct WaterfallRow {
    pub id: String,
    pub name: String,
    pub service: String,
    pub tone: Tone,
    /// Nesting depth (indents the name).
    pub depth: u32,
    /// Offset and width as fractions (0–1) of the whole trace; see `geom::span_fractions`.
    pub left: f64,
    pub width: f64,
    /// Duration text shown beside the bar.
    pub label: Option<String>,
    pub err: bool,
}

/// A trace as a span-per-row timeline (`d-wf`). Retries appear as separate bars on one timeline.
#[component]
pub fn Waterfall(
    rows: Vec<WaterfallRow>,
    /// `(fraction, label)` ticks along the top; see `geom::duration_ticks`.
    ticks: Vec<(f64, String)>,
    selected: Option<String>,
    on_select: EventHandler<String>,
) -> Element {
    rsx! {
        div { class: "d-wf",
            div { class: "d-wf-head",
                div { class: "d-label", "Span" }
                div {
                    div { class: "d-wf-ticks",
                        for (i , (f , t)) in ticks.into_iter().enumerate() {
                            span { key: "{i}", style: "left:{f * 100.0:.1}%", "{t}" }
                        }
                    }
                }
            }
            for row in rows {
                {
                    let id = row.id.clone();
                    let is_sel = selected.as_deref() == Some(row.id.as_str());
                    // The duration label goes on the side of the bar that has room: to the right
                    // by default, to the left when the bar ends near the lane's edge, and inside
                    // the bar when it spans nearly the whole lane.
                    let bar = format!("d-wf-bar{}{}", if row.err { " is-err" } else { "" }, label_side(row.left, row.width));
                    let row_style = format!("--c:{}; --d:{}", row.tone.css_var(), row.depth);
                    let bar_style = format!("left:{:.2}%; width:{:.2}%", row.left * 100.0, row.width * 100.0);
                    rsx! {
                        div {
                            key: "{row.id}",
                            class: "d-wf-row",
                            style: "{row_style}",
                            aria_selected: "{is_sel}",
                            onclick: move |_| on_select.call(id.clone()),
                            div { class: "d-wf-name",
                                span { class: "t", "{row.name}" }
                                span { class: "s", "{row.service}" }
                            }
                            div { class: "d-wf-lane",
                                span { class: "{bar}", style: "{bar_style}",
                                    if let Some(l) = row.label {
                                        em { "{l}" }
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

/// One line of the compact waterfall.
#[derive(Clone, Debug, PartialEq)]
pub struct CompactRow {
    pub name: String,
    pub tone: Tone,
    pub left: f64,
    pub width: f64,
    pub duration: String,
}

/// The three-column waterfall for a drawer (`d-wf--compact`): name, lane, duration.
#[component]
pub fn CompactWaterfall(rows: Vec<CompactRow>) -> Element {
    rsx! {
        div { class: "d-wf--compact",
            for (i , r) in rows.into_iter().enumerate() {
                div { key: "{i}", class: "d-wf-row", style: "--c:{r.tone.css_var()}",
                    span { "{r.name}" }
                    div { class: "d-wf-lane",
                        i { style: "left:{r.left * 100.0:.2}%; width:{r.width * 100.0:.2}%" }
                    }
                    span { "{r.duration}" }
                }
            }
        }
    }
}

/// How much of the lane a duration label needs beside a bar (`612 ms · refused` at 10.5px).
const LABEL_ROOM: f64 = 0.14;

/// The modifier class for where a bar's duration label sits.
fn label_side(left: f64, width: f64) -> &'static str {
    if 1.0 - (left + width) >= LABEL_ROOM {
        ""
    } else if left >= LABEL_ROOM {
        " is-right"
    } else {
        " is-inside"
    }
}

#[cfg(test)]
mod tests {
    use super::label_side;

    #[test]
    fn the_label_goes_where_there_is_room() {
        assert_eq!(label_side(0.0, 0.3), "");
        assert_eq!(label_side(0.6, 0.38), " is-right");
        assert_eq!(label_side(0.0, 1.0), " is-inside");
        assert_eq!(label_side(0.05, 0.9), " is-inside");
    }
}
