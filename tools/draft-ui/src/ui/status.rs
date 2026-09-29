use dioxus::prelude::*;

use super::cx;
use crate::kinds::{StatusKind, Tone};

/// Dot + label (`d-status`). Healthy is the quietest state on screen.
#[component]
pub fn Status(#[props(default)] kind: StatusKind, #[props(default)] live: bool, children: Element) -> Element {
    let class = cx(&["d-status", kind.class(), if live { "d-status--live" } else { "" }]);
    rsx! {
        span { class: "{class}", {children} }
    }
}

/// A coloured dot (`d-dot`): facets, series keys, column heads.
#[component]
pub fn Dot(#[props(default)] tone: Tone) -> Element {
    let style = format!("--c:{}", tone.css_var());
    rsx! {
        span { class: "d-dot", style: "{style}", aria_hidden: "true" }
    }
}

/// A small bordered number (`d-count`). `err` colours it as a problem count.
#[component]
pub fn Count(#[props(default)] err: bool, children: Element) -> Element {
    let class = cx(&["d-count", if err { "d-count--err" } else { "" }]);
    rsx! {
        span { class: "{class}", {children} }
    }
}

/// A keyboard key hint (`d-kbd`).
#[component]
pub fn Kbd(children: Element) -> Element {
    rsx! {
        span { class: "d-kbd", {children} }
    }
}
