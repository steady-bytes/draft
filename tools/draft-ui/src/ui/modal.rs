use dioxus::prelude::*;

use super::Btn;
use super::BtnSize;
use super::BtnVariant;

/// A modal dialog (`d-scrim` + `d-modal`). Renders nothing while closed. Escape and a click on the
/// backdrop close it.
///
/// Follows the WAI-ARIA dialog pattern: focus moves into the dialog when it opens (the first field,
/// else the dialog itself), Tab and Shift+Tab stay inside it, and focus returns to whatever had it
/// when it closes.
#[component]
pub fn Modal(
    open: bool,
    title: String,
    on_close: EventHandler<()>,
    #[props(default)] wide: bool,
    footer: Option<Element>,
    children: Element,
) -> Element {
    if !open {
        return rsx! {};
    }
    rsx! {
        ModalDialog { title, on_close, wide, footer, {children} }
    }
}

/// The open dialog. Its own component so that mounting it (open) and dropping it (closed) are the
/// moments focus moves in and back out.
#[component]
fn ModalDialog(
    title: String,
    on_close: EventHandler<()>,
    wide: bool,
    footer: Option<Element>,
    children: Element,
) -> Element {
    let opener = use_hook(focus::active_element);
    use_effect(focus::move_into_dialog);
    use_drop(move || focus::restore(&opener));

    let class = if wide { "d-modal d-modal--wide d-brackets is-active" } else { "d-modal d-brackets is-active" };
    rsx! {
        div {
            class: "d-scrim",
            onkeydown: move |e| match e.key() {
                Key::Escape => on_close.call(()),
                Key::Tab => {
                    if focus::cycle(e.modifiers().shift()) {
                        e.prevent_default();
                    }
                }
                _ => {}
            },
            onclick: move |_| on_close.call(()),
            div {
                class: "{class}",
                role: "dialog",
                aria_modal: "true",
                aria_label: "{title}",
                tabindex: "-1",
                // A click inside the dialog must not reach the backdrop.
                onclick: move |e| e.stop_propagation(),
                header {
                    h2 { "{title}" }
                    span { class: "d-spacer" }
                    Btn {
                        variant: BtnVariant::Ghost,
                        size: BtnSize::Sm,
                        icon: true,
                        aria_label: "Close".to_string(),
                        onclick: move |_| on_close.call(()),
                        "✕"
                    }
                }
                div { class: "d-modal-body", {children} }
                if let Some(f) = footer {
                    footer { {f} }
                }
            }
        }
    }
}

/// Focus management. All of it is a no-op off the browser (server-side rendering, native tests).
#[cfg(target_arch = "wasm32")]
mod focus {
    use wasm_bindgen::JsCast;
    use web_sys::{Element, HtmlElement};

    const DIALOG: &str = ".d-modal";
    /// Everything Tab can land on inside the dialog.
    const FOCUSABLE: &str = "a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex='-1'])";
    /// Where focus goes first: the first field of the form, if there is one.
    const FIRST_FIELD: &str = ".d-modal-body input:not([disabled]), .d-modal-body select:not([disabled]), .d-modal-body textarea:not([disabled])";

    fn document() -> Option<web_sys::Document> {
        web_sys::window()?.document()
    }

    pub fn active_element() -> Option<HtmlElement> {
        document()?.active_element()?.dyn_into::<HtmlElement>().ok()
    }

    pub fn move_into_dialog() {
        let Some(doc) = document() else { return };
        let target = doc
            .query_selector(FIRST_FIELD)
            .ok()
            .flatten()
            .or_else(|| doc.query_selector(DIALOG).ok().flatten())
            .and_then(|el| el.dyn_into::<HtmlElement>().ok());
        if let Some(el) = target {
            let _ = el.focus();
        }
    }

    pub fn restore(opener: &Option<HtmlElement>) {
        if let Some(el) = opener {
            // The opener may have been removed while the dialog was open.
            if el.is_connected() {
                let _ = el.focus();
            }
        }
    }

    /// Keeps Tab inside the dialog. Returns true when it moved focus itself (so the browser's own
    /// Tab handling must be cancelled).
    pub fn cycle(shift: bool) -> bool {
        let Some(doc) = document() else { return false };
        let Some(dialog) = doc.query_selector(DIALOG).ok().flatten() else { return false };
        let Ok(list) = dialog.query_selector_all(FOCUSABLE) else { return false };
        let items: Vec<HtmlElement> =
            (0..list.length()).filter_map(|i| list.item(i)).filter_map(|n| n.dyn_into::<HtmlElement>().ok()).collect();
        let (Some(first), Some(last)) = (items.first(), items.last()) else { return false };
        let active: Option<Element> = doc.active_element();
        let on = |el: &HtmlElement| active.as_ref().is_some_and(|a| a == el.as_ref());
        // Focus on the dialog itself (nothing inside is focused) counts as before the first item.
        let on_dialog = active.as_ref().is_some_and(|a| *a == dialog);
        if shift && (on(first) || on_dialog) {
            let _ = last.focus();
            true
        } else if !shift && on(last) {
            let _ = first.focus();
            true
        } else {
            false
        }
    }
}

#[cfg(not(target_arch = "wasm32"))]
mod focus {
    pub fn active_element() -> Option<()> {
        None
    }
    pub fn move_into_dialog() {}
    pub fn restore(_: &Option<()>) {}
    pub fn cycle(_: bool) -> bool {
        false
    }
}
