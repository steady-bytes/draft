use dioxus::prelude::*;

use super::chrome::{BarItem, Chrome, ChromeSlot};
use super::nav::{NavItem, NavSection};
use crate::kinds::AppKind;
use crate::theme::{ThemeProvider, ThemeToggle};
use crate::ui::{Glyph, Kbd, RouteLink, Status};

/// The app frame: brand, topbar (breadcrumb, status pill, theme), rail, main and status bar.
///
/// * `sections` are the app's own rail sections; `apps` becomes the shared **Apps** block of
///   links to the other Draft apps (build it with `use_app_links` behind the `discovery` feature).
/// * `current` is the route path, used to highlight the active rail item.
/// * Views set their breadcrumb, status pill and status bar with
///   [`use_page_chrome`](super::use_page_chrome).
/// * Below 900 px the rail collapses behind a hamburger (CSS-only checkbox toggle).
#[component]
pub fn AppShell(
    app: AppKind,
    sections: Vec<NavSection>,
    current: String,
    #[props(default)] apps: Vec<NavItem>,
    /// Full-bleed main area (the cluster canvas).
    #[props(default)]
    flush: bool,
    children: Element,
) -> Element {
    rsx! {
        ThemeProvider { Frame { app, sections, current, apps, flush, {children} } }
    }
}

#[component]
fn Frame(app: AppKind, sections: Vec<NavSection>, current: String, apps: Vec<NavItem>, flush: bool, children: Element) -> Element {
    let chrome = use_context_provider(|| ChromeSlot(Signal::new(Chrome::default())));
    let mut rail_open = use_signal(|| false);
    let chrome = chrome.0.read().clone();
    let main_class = if flush { "d-main d-main--flush" } else { "d-main" };

    rsx! {
        div { class: "d-app",
            input {
                class: "d-rail-toggle",
                id: "d-rail-toggle",
                r#type: "checkbox",
                aria_label: "Toggle navigation",
                checked: rail_open(),
                onchange: move |e| rail_open.set(e.checked()),
            }

            div { class: "d-brand", Wordmark { app } }

            header { class: "d-topbar",
                label {
                    class: "d-btn d-btn--ghost d-btn--icon d-hamburger",
                    r#for: "d-rail-toggle",
                    aria_label: "Menu",
                    "☰"
                }
                span { class: "d-topbar-brand", Wordmark { app } }
                nav { class: "d-crumbs", aria_label: "Breadcrumb",
                    for (i , c) in chrome.crumbs.iter().enumerate() {
                        Fragment { key: "{i}",
                            if i > 0 {
                                i { aria_hidden: "true", "/" }
                            }
                            span { "{c}" }
                        }
                    }
                }
                span { class: "d-spacer" }
                if let Some(s) = chrome.status.clone() {
                    Status { kind: s.kind, live: s.live, "{s.text}" }
                }
                // The command palette has no design yet: shown, but inert.
                button {
                    class: "d-btn d-btn--ghost d-btn--sm d-cmd",
                    r#type: "button",
                    aria_label: "Command palette (not available yet)",
                    aria_disabled: "true",
                    Kbd { "⌘" }
                    Kbd { "K" }
                }
                ThemeToggle {}
            }

            aside { class: "d-rail", aria_label: "Navigation",
                for section in sections {
                    if !section.items.is_empty() {
                        Section { key: "{section.label}", section: section.clone(), current: current.clone(), on_navigate: move |_| rail_open.set(false) }
                    }
                }
                if !apps.is_empty() {
                    Section {
                        section: NavSection::new("Apps", apps.clone()),
                        current: current.clone(),
                        on_navigate: move |_| rail_open.set(false),
                    }
                }
            }

            main { class: "{main_class}", {children} }

            footer { class: "d-statusbar",
                for (i , item) in chrome.left.iter().enumerate() {
                    BarEntry { key: "l{i}", item: item.clone() }
                }
                span { class: "d-spacer" }
                for (i , item) in chrome.right.iter().enumerate() {
                    BarEntry { key: "r{i}", item: item.clone() }
                }
            }

            label { class: "d-rail-scrim", r#for: "d-rail-toggle" }
        }
    }
}

/// `{draft}` with the app's name beside it.
#[component]
fn Wordmark(app: AppKind) -> Element {
    // The brand cell shows the app name in small caps, as the mockups do for every app except
    // Blueprint (the control plane is just `{draft}`).
    let name = if app == AppKind::Blueprint { String::new() } else { app.name().to_string() };
    rsx! {
        span { class: "d-wordmark",
            b { "{{" }
            "draft"
            b { "}}" }
            if !name.is_empty() {
                small { "{name}" }
            }
        }
    }
}

#[component]
fn Section(section: NavSection, current: String, on_navigate: EventHandler<()>) -> Element {
    rsx! {
        span { class: "d-label", "{section.label}" }
        for item in section.items {
            NavLink { key: "{item.path}", item: item.clone(), current: current.clone(), on_navigate }
        }
    }
}

#[component]
fn NavLink(item: NavItem, current: String, on_navigate: EventHandler<()>) -> Element {
    let is_current = item.is_current(&current);
    let mut class = String::from("d-nav-item");
    if item.external {
        class.push_str(" d-nav-item--ext");
    }
    if item.action {
        class.push_str(" d-nav-item--action");
    }
    let aria = "page";
    let count_class = if item.count_err { "d-count d-count--err" } else { "d-count" };

    let body = rsx! {
        if let Some((code, tone)) = item.glyph.clone() {
            Glyph { code, tone }
        }
        span { class: "d-nav-text", "{item.label}" }
        if let Some(n) = item.count {
            span { class: "{count_class}", "{n}" }
        }
    };

    if item.external {
        rsx! {
            a {
                class: "{class}",
                href: "{item.path}",
                target: "_blank",
                rel: "noopener noreferrer",
                {body}
            }
        }
    } else {
        let aria = is_current.then(|| aria.to_string());
        rsx! {
            RouteLink { to: item.path.clone(), class: class, aria_current: aria, onclick: move |_| on_navigate.call(()), {body} }
        }
    }
}

#[component]
fn BarEntry(item: BarItem) -> Element {
    match item {
        BarItem::Kv(label, value) => rsx! {
            span { "{label} " b { "{value}" } }
        },
        BarItem::Hint(key, text) => rsx! {
            span { Kbd { "{key}" } " {text}" }
        },
        BarItem::Text(t) => rsx! {
            span { "{t}" }
        },
    }
}
