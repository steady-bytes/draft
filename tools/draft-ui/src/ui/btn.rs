use dioxus::prelude::*;

use super::cx;

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum BtnVariant {
    #[default]
    Default,
    Primary,
    Ghost,
    Danger,
}

impl BtnVariant {
    const fn class(self) -> &'static str {
        match self {
            BtnVariant::Default => "",
            BtnVariant::Primary => "d-btn--primary",
            BtnVariant::Ghost => "d-btn--ghost",
            BtnVariant::Danger => "d-btn--danger",
        }
    }
}

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum BtnSize {
    #[default]
    Md,
    Sm,
}

/// A button (`d-btn`).
///
/// * default: a `<button type="button">`;
/// * `to`: an in-app router link;
/// * `href`: a plain anchor (external; opens in a new tab when `external`).
#[component]
pub fn Btn(
    #[props(default)] variant: BtnVariant,
    #[props(default)] size: BtnSize,
    #[props(default)] icon: bool,
    #[props(default)] disabled: bool,
    to: Option<String>,
    href: Option<String>,
    #[props(default)] external: bool,
    title: Option<String>,
    aria_label: Option<String>,
    onclick: Option<EventHandler<MouseEvent>>,
    children: Element,
) -> Element {
    let class = cx(&[
        "d-btn",
        variant.class(),
        if size == BtnSize::Sm { "d-btn--sm" } else { "" },
        if icon { "d-btn--icon" } else { "" },
    ]);
    let click = move |e: MouseEvent| {
        if let Some(h) = &onclick {
            h.call(e);
        }
    };

    if let Some(to) = to {
        return rsx! {
            super::RouteLink { to: to, class: class, title: title, aria_label: aria_label, onclick: click, {children} }
        };
    }
    if let Some(href) = href {
        let (target, rel) = if external { ("_blank", "noopener noreferrer") } else { ("_self", "") };
        return rsx! {
            a { class: "{class}", href: "{href}", target: "{target}", rel: "{rel}", title: title, aria_label: aria_label, onclick: click, {children} }
        };
    }
    rsx! {
        button { class: "{class}", r#type: "button", disabled, title: title, aria_label: aria_label, onclick: click, {children} }
    }
}
