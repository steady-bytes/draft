use dioxus::prelude::*;

use crate::ui::{Btn, BtnSize, BtnVariant};

/// The detail panel of a [`Split`](super::Split) (`d-drawer`): a label row with a close button, a
/// title and subtitle, then blocks.
#[component]
pub fn Drawer(
    /// The accessible name, and the small label at the top (`Entry`, `Route · host`).
    label: String,
    on_close: Option<EventHandler<()>>,
    title: Option<String>,
    subtitle: Option<String>,
    /// Extra content in the label row, left of the close button (a status, a timestamp).
    lead: Option<Element>,
    children: Element,
) -> Element {
    rsx! {
        aside { class: "d-drawer", aria_label: "{label}",
            div { class: "d-drawer-head",
                if let Some(l) = lead {
                    {l}
                } else {
                    span { class: "d-label", "{label}" }
                }
                span { class: "d-spacer" }
                if let Some(close) = on_close {
                    Btn {
                        variant: BtnVariant::Ghost,
                        size: BtnSize::Sm,
                        icon: true,
                        aria_label: "Close".to_string(),
                        onclick: move |_| close.call(()),
                        "✕"
                    }
                }
            }
            if let Some(t) = title {
                h2 { class: "d-drawer-title", "{t}" }
            }
            if let Some(s) = subtitle {
                span { class: "d-label", "{s}" }
            }
            {children}
        }
    }
}

/// A titled section inside a drawer (Attributes, Logs, Match, Upstream …).
#[component]
pub fn DrawerBlock(title: String, children: Element) -> Element {
    rsx! {
        div { class: "d-block",
            div { class: "d-section-title",
                span { class: "d-label", "{title}" }
            }
            {children}
        }
    }
}
