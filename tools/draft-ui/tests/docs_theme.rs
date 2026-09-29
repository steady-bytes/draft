//! Guard for the docs-site theme (`dist/docs/_draft.scss`, from scss/docs.scss).
//!
//! The docs colour file is compiled to literal values for Hugo's libsass, so unlike the app CSS it
//! cannot be checked through the token maps: these tests read what it actually wrote.

mod common;
use common::*;

const AA: f64 = 4.5;

/// The `--name: value;` declarations of the block that opens with `header` (`html:root {`).
fn block(scss: &str, header: &str) -> std::collections::HashMap<String, String> {
    let start = scss.find(header).unwrap_or_else(|| panic!("no `{header}` block in dist/docs/_draft.scss"));
    let body = &scss[start + header.len()..];
    let body = &body[..body.find('}').expect("unterminated block")];
    body.lines()
        .filter_map(|l| {
            let l = l.trim().strip_suffix(';')?;
            let (k, v) = l.split_once(':')?;
            Some((k.trim().trim_start_matches("--").to_string(), v.trim().to_string()))
        })
        .collect()
}

fn modes() -> Vec<(&'static str, std::collections::HashMap<String, String>)> {
    let scss = dist("docs/_draft.scss");
    vec![
        ("light", block(&scss, "html:root {")),
        ("dark", block(&scss, "html[data-dark-mode] {")),
    ]
}

fn color(vars: &std::collections::HashMap<String, String>, name: &str) -> Rgb {
    Rgb::hex(vars.get(name).unwrap_or_else(|| panic!("--{name} is not set")))
}

#[test]
fn docs_text_clears_aa_on_the_page_and_on_code() {
    let mut failures = vec![];
    for (mode, v) in modes() {
        for surface in ["body-bg", "prism-code-bg", "sidebar-bg"] {
            for text in ["text-default", "text-muted", "primary", "link-color", "sidebar-text-color", "sidebar-primary"] {
                let r = contrast(color(&v, text), color(&v, surface));
                if r < AA {
                    failures.push(format!("{mode}: --{text} on --{surface} = {r:.2}"));
                }
            }
        }
        for token in ["draft-code-ink", "draft-code-dim", "draft-code-key", "draft-code-str", "draft-code-num", "draft-code-fn", "draft-code-err"] {
            let r = contrast(color(&v, token), color(&v, "prism-code-bg"));
            if r < AA {
                failures.push(format!("{mode}: --{token} on code background = {r:.2}"));
            }
        }
    }
    assert!(failures.is_empty(), "below 4.5:1:\n  {}", failures.join("\n  "));
}

#[test]
fn docs_palette_is_the_apps_palette() {
    // The docs surfaces are the same swatches the apps paint, not near-misses.
    let css = dist("draft.css");
    let (dark, light) = (theme_block(&css, ":root,\n[data-theme=draft] {"), theme_block(&css, "[data-theme=draft-light] {"));
    for (mode, v) in modes() {
        let tokens = if mode == "dark" { &dark } else { &light };
        for (docs, app) in [("body-bg", "bg"), ("text-default", "ink"), ("text-muted", "dim"), ("prism-code-bg", "panel-2"), ("sidebar-border-color", "rule")] {
            assert_eq!(v[docs], tokens[app], "{mode}: --{docs} should be the app's --{app}");
        }
    }
}

#[test]
fn docs_ship_their_fonts() {
    let scss = dist("docs/_draft.scss");
    for font in ["inter-latin.woff2", "jetbrains-mono-latin.woff2"] {
        assert!(scss.contains(font), "docs theme does not load {font}");
        assert!(std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("dist/fonts").join(font).exists(), "dist/fonts/{font} is missing");
    }
}
