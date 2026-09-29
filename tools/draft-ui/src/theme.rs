//! Theme and primary colour: state, persistence, and the controls.

use dioxus::prelude::*;

use crate::prefs;
use crate::ui::{Btn, BtnSize};

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Theme {
    #[default]
    Dark,
    Light,
}

impl Theme {
    /// The `data-theme` attribute value.
    pub const fn attr(self) -> &'static str {
        match self {
            Theme::Dark => "draft",
            Theme::Light => "draft-light",
        }
    }

    pub fn from_attr(v: &str) -> Option<Theme> {
        match v {
            "draft" => Some(Theme::Dark),
            "draft-light" => Some(Theme::Light),
            _ => None,
        }
    }

    pub const fn toggled(self) -> Theme {
        match self {
            Theme::Dark => Theme::Light,
            Theme::Light => Theme::Dark,
        }
    }
}

/// The five primary-colour swatches offered by the picker. The first is the default.
pub const PRIMARY_SWATCHES: [&str; 5] = ["#4db380", "#3ddc97", "#4fb3d9", "#b48cff", "#ffb454"];

/// Theme and primary as reactive state. Provided by [`ThemeProvider`] (and so by `AppShell`).
#[derive(Clone, Copy, PartialEq)]
pub struct ThemeState {
    pub theme: Signal<Theme>,
    pub primary: Signal<String>,
}

/// Mount once near the root; keeps `<html data-theme>` and `--primary-base` in sync with the state
/// and persists changes. `dist/boot.js` (inlined in `index.html`) restores them before first paint.
pub fn use_theme_provider() -> ThemeState {
    let state = use_context_provider(|| {
        let theme = prefs::get(prefs::THEME_KEY).and_then(|v| Theme::from_attr(&v)).unwrap_or_default();
        let primary = prefs::get(prefs::PRIMARY_KEY)
            .filter(|v| prefs::is_hex_colour(v))
            .unwrap_or_else(|| PRIMARY_SWATCHES[0].to_string());
        ThemeState { theme: Signal::new(theme), primary: Signal::new(primary) }
    });

    use_effect(move || {
        let theme = *state.theme.read();
        let primary = state.primary.read().clone();
        apply(theme, &primary);
    });
    state
}

/// The theme state provided by an ancestor [`ThemeProvider`].
pub fn use_theme() -> ThemeState {
    use_context::<ThemeState>()
}

#[cfg(target_arch = "wasm32")]
fn apply(theme: Theme, primary: &str) {
    use wasm_bindgen::JsCast;
    let Some(root) = web_sys::window().and_then(|w| w.document()).and_then(|d| d.document_element()) else {
        return;
    };
    let _ = root.set_attribute("data-theme", theme.attr());
    if let Some(el) = root.dyn_ref::<web_sys::HtmlElement>() {
        let _ = el.style().set_property("--primary-base", primary);
    }
}

#[cfg(not(target_arch = "wasm32"))]
fn apply(_theme: Theme, _primary: &str) {}

#[component]
pub fn ThemeProvider(children: Element) -> Element {
    use_theme_provider();
    rsx! { {children} }
}

/// Flips dark ⇄ light and remembers the choice.
#[component]
pub fn ThemeToggle() -> Element {
    let state = use_theme();
    let mut theme = state.theme;
    let label = if *theme.read() == Theme::Light { "Dark" } else { "Light" };
    let aria = if *theme.read() == Theme::Light { "Switch to dark theme" } else { "Switch to light theme" };
    rsx! {
        Btn {
            size: BtnSize::Sm,
            aria_label: aria.to_string(),
            onclick: move |_| {
                let next = theme.peek().toggled();
                theme.set(next);
                prefs::set(prefs::THEME_KEY, next.attr());
            },
            "{label}"
        }
    }
}

/// The five primary-colour swatch buttons.
#[component]
pub fn PrimarySwatches() -> Element {
    let state = use_theme();
    let mut primary = state.primary;
    let current = primary.read().to_ascii_lowercase();
    rsx! {
        div { class: "d-row", style: "gap:6px", role: "group", aria_label: "Primary colour",
            for swatch in PRIMARY_SWATCHES {
                {
                    let pressed = if current == swatch { "true" } else { "false" };
                    let style = format!("width:18px;height:18px;padding:0;border:1px solid var(--rule-strong);border-radius:var(--radius);background:{swatch};cursor:pointer");
                    rsx! {
                        button {
                            key: "{swatch}",
                            r#type: "button",
                            style: "{style}",
                            title: "{swatch}",
                            aria_label: "{swatch}",
                            aria_pressed: "{pressed}",
                            onclick: move |_| {
                                primary.set(swatch.to_string());
                                prefs::set(prefs::PRIMARY_KEY, swatch);
                            },
                        }
                    }
                }
            }
        }
    }
}
