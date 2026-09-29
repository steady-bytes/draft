use dioxus::prelude::*;

/// The compiled Draft stylesheet, fingerprinted and bundled by `dx` (manganis `asset!`).
pub const DRAFT_CSS: Asset = asset!("/dist/draft.css");

/// Self-hosted fonts (latin subset, variable weights). `dx` does not follow `url()` references
/// inside CSS, so they are bundled here and the `@font-face` rules are emitted with their
/// resolved URLs — the same rules `dist/fonts.css` serves the Go services with relative paths.
const INTER: Asset = asset!("/dist/fonts/inter-latin.woff2");
const JETBRAINS_MONO: Asset = asset!("/dist/fonts/jetbrains-mono-latin.woff2");

const LATIN: &str = "U+0000-00FF, U+0131, U+0152-0153, U+02BB-02BC, U+02C6, U+02DA, U+02DC, U+0304, U+0308, U+0329, U+2000-206F, U+20AC, U+2122, U+2191, U+2193, U+2212, U+2215, U+FEFF, U+FFFD";

fn font_face(family: &str, weights: &str, url: &Asset) -> String {
    format!(
        "@font-face{{font-family:\"{family}\";font-style:normal;font-weight:{weights};font-display:swap;src:url({url}) format(\"woff2\");unicode-range:{LATIN}}}"
    )
}

/// Mount once at the root of an app to load the design system (stylesheet and fonts).
///
/// Also put `dist/boot.js` inline in `index.html`'s `<head>` so the saved theme is applied before
/// first paint.
#[component]
pub fn DraftStyles() -> Element {
    let fonts = format!(
        "{}{}",
        font_face("Inter", "400 600", &INTER),
        font_face("JetBrains Mono", "400 700", &JETBRAINS_MONO)
    );
    rsx! {
        document::Style { "{fonts}" }
        document::Stylesheet { href: DRAFT_CSS }
    }
}
