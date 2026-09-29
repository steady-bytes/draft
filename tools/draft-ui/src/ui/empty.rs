use dioxus::prelude::*;

/// Something was asked for and nothing came back (`d-empty`).
#[component]
pub fn Empty(title: String, #[props(default)] compact: bool, children: Element) -> Element {
    let class = if compact { "d-empty d-empty--compact" } else { "d-empty" };
    rsx! {
        div { class: "{class}",
            b { "{title}" }
            span { {children} }
        }
    }
}

/// The sound-wave loader (`d-loading`), ported from Blueprint's `WaveLoader`.
#[component]
pub fn Loading() -> Element {
    rsx! {
        div { class: "d-loading", role: "status", aria_label: "Loading",
            span { class: "d-wave",
                for i in 0..7u32 {
                    i { key: "{i}", style: "--i:{i}" }
                }
            }
        }
    }
}

/// A placeholder line while a value loads.
#[component]
pub fn Skeleton(#[props(default = 120)] width: u32) -> Element {
    rsx! {
        span { class: "d-skel", style: "width:{width}px" }
    }
}

/// The 404 page, shared by every client (previously three near-identical copies).
#[component]
pub fn NotFound(route: Vec<String>) -> Element {
    let path = route.join("/");
    rsx! {
        div { class: "d-empty",
            b { "404" }
            span { "No page at /{path}" }
        }
    }
}
