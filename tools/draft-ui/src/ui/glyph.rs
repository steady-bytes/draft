use dioxus::prelude::*;

use super::cx;
use crate::kinds::{AppKind, Tone};

/// The two-letter kind chip (`d-glyph`).
#[component]
pub fn Glyph(code: String, #[props(default)] tone: Tone, #[props(default)] large: bool) -> Element {
    let class = cx(&["d-glyph", tone.glyph_class(), if large { "d-glyph--lg" } else { "" }]);
    rsx! {
        span { class: "{class}", aria_hidden: "true", "{code}" }
    }
}

/// The glyph for a Draft app (`Bp`, `Bc`, `Bn`, …) in its identity colour.
#[component]
pub fn AppGlyph(app: AppKind, #[props(default)] large: bool) -> Element {
    rsx! {
        Glyph { code: app.code().to_string(), tone: app.tone(), large }
    }
}
