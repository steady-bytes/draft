use dioxus::prelude::*;

use super::cx;
use crate::kinds::Tone;

/// An outlined chip in a meaning colour (`d-tag`).
#[component]
pub fn Tag(
    #[props(default)] tone: Tone,
    #[props(default)] solid: bool,
    #[props(default)] large: bool,
    title: Option<String>,
    children: Element,
) -> Element {
    let class = cx(&[
        "d-tag",
        tone.tag_class(),
        if solid { "d-tag--solid" } else { "" },
        if large { "d-tag--lg" } else { "" },
    ]);
    rsx! {
        span { class: "{class}", title: title, {children} }
    }
}

/// A service label (`d-svc`): a small square in the service's colour, then its name. The colour
/// is stable per name ([`Tone::for_name`]) unless `tone` overrides it.
#[component]
pub fn Svc(name: String, tone: Option<Tone>) -> Element {
    let tone = tone.unwrap_or_else(|| Tone::for_name(&name));
    rsx! {
        span { class: "d-svc", style: "--c:{tone.css_var()}", "{name}" }
    }
}

/// A trace-id pill (`d-tracelink`): the id shortened to `head…tail`, the full id in the tooltip.
/// A `<button>` because opening a trace is an in-app hand-off, not a URL.
#[component]
pub fn TraceLink(trace_id: String, onclick: EventHandler<()>) -> Element {
    let short = crate::util::truncate_middle(&trace_id, 8, 4);
    rsx! {
        button {
            class: "d-tracelink",
            r#type: "button",
            title: "View trace {trace_id}",
            onclick: move |e| {
                // A table row around it also opens a detail drawer; this must not.
                e.stop_propagation();
                onclick.call(());
            },
            "{short}"
        }
    }
}
