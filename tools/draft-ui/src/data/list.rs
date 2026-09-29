use dioxus::prelude::*;

/// A stack of rows (`d-list`): drawer lists, "used by", metric names.
#[component]
pub fn List(#[props(default)] flush: bool, children: Element) -> Element {
    let class = if flush { "d-list d-list--flush" } else { "d-list" };
    rsx! {
        div { class: "{class}", {children} }
    }
}

/// One row: an in-app link (`to`), a plain link (`href`) or static content.
#[component]
pub fn ListRow(to: Option<String>, href: Option<String>, #[props(default)] current: bool, children: Element) -> Element {
    let aria = if current { "true" } else { "false" };
    if let Some(to) = to {
        let cur = current.then(|| "true".to_string());
        return rsx! {
            crate::ui::RouteLink { to: to, class: "d-list-row".to_string(), aria_current: cur, {children} }
        };
    }
    if let Some(href) = href {
        return rsx! {
            a { class: "d-list-row", href: "{href}", aria_current: "{aria}", {children} }
        };
    }
    rsx! {
        div { class: "d-list-row", aria_current: "{aria}", {children} }
    }
}
