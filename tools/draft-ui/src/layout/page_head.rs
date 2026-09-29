use dioxus::prelude::*;

use crate::ui::cx;

/// Eyebrow label, title, description and actions (`d-page-head`). `inline` is the compact variant
/// for data-heavy pages: title, summary label and controls on one row.
#[component]
pub fn PageHead(
    title: String,
    eyebrow: Option<String>,
    description: Option<String>,
    #[props(default)] inline: bool,
    /// A metadata line under the title (`d-meta`).
    meta: Option<Element>,
    actions: Option<Element>,
    /// Extra content beside the title in the inline variant (summary label, segmented controls).
    children: Element,
) -> Element {
    let class = cx(&["d-page-head", if inline { "d-page-head--inline" } else { "" }]);
    rsx! {
        div { class: "{class}",
            div {
                if let Some(e) = eyebrow {
                    span { class: "d-label", "{e}" }
                }
                h1 { "{title}" }
                if let Some(d) = description {
                    p { "{d}" }
                }
                if let Some(m) = meta {
                    div { class: "d-meta", {m} }
                }
            }
            if inline {
                {children}
            }
            if let Some(a) = actions {
                div { class: "d-actions", {a} }
            }
        }
    }
}

/// A row of controls above a table or board (`d-toolbar`).
#[component]
pub fn Toolbar(children: Element) -> Element {
    rsx! {
        div { class: "d-toolbar", {children} }
    }
}

/// A labelled divider (`d-section-title`).
#[component]
pub fn SectionTitle(children: Element) -> Element {
    rsx! {
        div { class: "d-section-title",
            span { class: "d-label", {children} }
        }
    }
}
