use dioxus::prelude::*;

/// An in-app link: a real `<a href>` (so it can be opened in a new tab or copied) that navigates
/// through the router on a plain click.
///
/// This is deliberately not dioxus-router's `Link`: `Link` decides `aria-current` itself using a
/// prefix match (so `/` would be "current" on every page) and would emit the attribute twice next
/// to ours. The caller decides what is current and passes `aria_current`.
#[component]
pub fn RouteLink(
    to: String,
    class: String,
    title: Option<String>,
    aria_label: Option<String>,
    /// `"page"` for the current page, otherwise omitted.
    aria_current: Option<String>,
    onclick: Option<EventHandler<MouseEvent>>,
    children: Element,
) -> Element {
    let nav = use_navigator();
    let target = to.clone();
    rsx! {
        a {
            class: "{class}",
            href: "{to}",
            title: title,
            aria_label: aria_label,
            "aria-current": aria_current,
            onclick: move |e| {
                if let Some(h) = &onclick {
                    h.call(e.clone());
                }
                // Let the browser handle ⌘/ctrl/shift/alt-clicks (new tab, new window, download).
                let m = e.modifiers();
                if m.meta() || m.ctrl() || m.shift() || m.alt() {
                    return;
                }
                e.prevent_default();
                nav.push(target.clone());
            },
            {children}
        }
    }
}
