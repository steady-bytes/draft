use dioxus::prelude::*;

use super::{Count, Dot};
use crate::kinds::Tone;

/// A preset / facet chip (`d-chip`). With `pressed` it is a toggle (event-type facets); `count`
/// shows the number of matches and `dot` a colour key.
#[component]
pub fn Chip(
    pressed: Option<bool>,
    count: Option<u32>,
    dot: Option<Tone>,
    onclick: EventHandler<MouseEvent>,
    children: Element,
) -> Element {
    let aria = pressed.map(|p| if p { "true" } else { "false" });
    rsx! {
        button {
            class: "d-chip",
            r#type: "button",
            aria_pressed: aria,
            onclick: move |e| onclick.call(e),
            if let Some(tone) = dot {
                Dot { tone }
            }
            {children}
            if let Some(n) = count {
                Count { "{n}" }
            }
        }
    }
}
