//! Accessibility guard for the design tokens (WCAG 2.x): every text colour must clear 4.5 : 1 on
//! every surface it can appear on, in both themes, for all five primary swatches.
//!
//! These tests read the committed `dist/draft.css`, so they also fail if someone edits a token
//! without regenerating.

mod common;
use common::*;

const SURFACES: [&str; 4] = ["bg", "panel", "panel-2", "panel-3"];
const TEXT: [&str; 9] = ["ink", "dim", "ca", "bp", "fs", "sv", "err", "warn", "primary"];
const TINTED: [&str; 7] = ["ca", "bp", "fs", "sv", "err", "warn", "primary"];
const AA: f64 = 4.5;

#[test]
fn text_tokens_clear_aa_on_every_surface() {
    let mut failures = vec![];
    for swatch in SWATCHES {
        for theme in themes(swatch) {
            for t in TEXT {
                for s in SURFACES {
                    let r = contrast(theme.color(t), theme.color(s));
                    if r < AA {
                        failures.push(format!("{} / {swatch}: --{t} on --{s} = {r:.2}", theme.name));
                    }
                }
            }
        }
    }
    assert!(failures.is_empty(), "below 4.5:1:\n  {}", failures.join("\n  "));
}

#[test]
fn tag_text_clears_aa_on_its_own_tint() {
    // `.d-tag` and `.d-alert` paint their colour at 8–9 % over the surface; the text sits on that.
    let mut failures = vec![];
    for swatch in SWATCHES {
        for theme in themes(swatch) {
            for t in TINTED {
                for s in ["bg", "panel", "panel-2"] {
                    let c = theme.color(t);
                    let bg = c.over(0.09, theme.color(s));
                    let r = contrast(c, bg);
                    if r < AA {
                        failures.push(format!("{} / {swatch}: --{t} on 9% tint over --{s} = {r:.2}", theme.name));
                    }
                }
            }
        }
    }
    assert!(failures.is_empty(), "below 4.5:1:\n  {}", failures.join("\n  "));
}

#[test]
fn filled_primary_button_text_is_legible() {
    for swatch in SWATCHES {
        for theme in themes(swatch) {
            let r = contrast(theme.color("primary-ink"), theme.color("primary"));
            assert!(r >= AA, "{} / {swatch}: --primary-ink on --primary = {r:.2}", theme.name);
        }
    }
}

#[test]
fn solid_tags_keep_text_legible() {
    // `.d-tag--solid` paints text in --bg on a solid tone.
    for swatch in SWATCHES {
        for theme in themes(swatch) {
            for t in TINTED {
                let r = contrast(theme.color("bg"), theme.color(t));
                assert!(r >= AA, "{} / {swatch}: --bg on solid --{t} = {r:.2}", theme.name);
            }
        }
    }
}

/// `--dimmer` fails AA as text in both themes (3.1 : 1 dark, 2.4 : 1 light). It exists for rules,
/// decoration and disabled states, so every rule that colours *text* with it must be listed here on
/// purpose. Adding to this list is a review decision, not a convenience.
#[test]
fn dimmer_is_only_used_as_text_colour_where_decorative() {
    let css = dist("draft.css");
    let allowed = [
        ".d-crumbs i",               // "/" separators
        ".d-nav-item--ext::after",   // ↗ glyph
        ".d-table th[aria-sort]::after", // sort arrow
        ".d-secret-mask",            // •••• bullets
    ];
    let mut found = vec![];
    let mut sel = String::new();
    for line in css.lines() {
        let l = line.trim();
        if l.ends_with('{') {
            sel = l.trim_end_matches('{').trim().to_string();
        } else if l.starts_with("color: var(--dimmer)") {
            found.push(sel.clone());
        }
    }
    let unexpected: Vec<_> = found.iter().filter(|s| !allowed.iter().any(|a| s.contains(a))).collect();
    assert!(unexpected.is_empty(), "`color: var(--dimmer)` on text: {unexpected:?}");
}

#[test]
fn light_primary_derivation_keeps_every_swatch_readable_on_the_darkest_surface() {
    for swatch in SWATCHES {
        let light = &themes(swatch)[1];
        let r = contrast(light.color("primary"), light.color("panel-3"));
        assert!(r >= AA, "{swatch} derives to {r:.2} on --panel-3");
    }
}
