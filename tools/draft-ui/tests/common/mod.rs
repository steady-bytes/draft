//! Shared helpers: a tiny colour library (sRGB, oklab mixing, WCAG contrast) and a parser for the
//! theme blocks in `dist/draft.css`.
#![allow(dead_code)]

use std::collections::HashMap;

pub fn dist(file: &str) -> String {
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("dist").join(file);
    std::fs::read_to_string(&path).unwrap_or_else(|e| panic!("read {}: {e}", path.display()))
}

#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Rgb(pub f64, pub f64, pub f64);

impl Rgb {
    pub fn hex(s: &str) -> Rgb {
        let s = s.trim().trim_start_matches('#');
        let v = |i: usize| u8::from_str_radix(&s[i..i + 2], 16).unwrap() as f64 / 255.0;
        Rgb(v(0), v(2), v(4))
    }

    fn lin(c: f64) -> f64 {
        if c <= 0.04045 { c / 12.92 } else { ((c + 0.055) / 1.055).powf(2.4) }
    }

    fn gam(c: f64) -> f64 {
        let c = c.clamp(0.0, 1.0);
        if c <= 0.0031308 { 12.92 * c } else { 1.055 * c.powf(1.0 / 2.4) - 0.055 }
    }

    pub fn luminance(self) -> f64 {
        0.2126 * Self::lin(self.0) + 0.7152 * Self::lin(self.1) + 0.0722 * Self::lin(self.2)
    }

    /// `color-mix(in oklab, self p%, black)`.
    pub fn mix_black(self, p: f64) -> Rgb {
        let (r, g, b) = (Self::lin(self.0), Self::lin(self.1), Self::lin(self.2));
        let l = (0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b).cbrt();
        let m = (0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b).cbrt();
        let s = (0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b).cbrt();
        let (ll, a, bb) = (
            (0.2104542553 * l + 0.7936177850 * m - 0.0040720468 * s) * p,
            (1.9779984951 * l - 2.4285922050 * m + 0.4505937099 * s) * p,
            (0.0259040371 * l + 0.7827717662 * m - 0.8086757660 * s) * p,
        );
        let l_ = ll + 0.3963377774 * a + 0.2158037573 * bb;
        let m_ = ll - 0.1055613458 * a - 0.0638541728 * bb;
        let s_ = ll - 0.0894841775 * a - 1.2914855480 * bb;
        let (l3, m3, s3) = (l_.powi(3), m_.powi(3), s_.powi(3));
        Rgb(
            Self::gam(4.0767416621 * l3 - 3.3077115913 * m3 + 0.2309699292 * s3),
            Self::gam(-1.2684380046 * l3 + 2.6097574011 * m3 - 0.3413193965 * s3),
            Self::gam(-0.0041960863 * l3 - 0.7034186147 * m3 + 1.7076147010 * s3),
        )
    }

    /// `self` at `alpha` composited over `under` (in gamma space, as browsers do).
    pub fn over(self, alpha: f64, under: Rgb) -> Rgb {
        Rgb(
            self.0 * alpha + under.0 * (1.0 - alpha),
            self.1 * alpha + under.1 * (1.0 - alpha),
            self.2 * alpha + under.2 * (1.0 - alpha),
        )
    }
}

/// WCAG 2.x contrast ratio.
pub fn contrast(a: Rgb, b: Rgb) -> f64 {
    let (la, lb) = (a.luminance(), b.luminance());
    let (hi, lo) = if la > lb { (la, lb) } else { (lb, la) };
    (hi + 0.05) / (lo + 0.05)
}

/// Custom properties declared in the CSS block that starts at `header`.
pub fn theme_block(css: &str, header: &str) -> HashMap<String, String> {
    let start = css.find(header).unwrap_or_else(|| panic!("no block `{header}`"));
    let open = css[start..].find('{').unwrap() + start;
    let close = css[open..].find('}').unwrap() + open;
    css[open + 1..close]
        .lines()
        .filter_map(|l| {
            let l = l.trim().trim_end_matches(';');
            let l = l.strip_prefix("--")?;
            let (k, v) = l.split_once(':')?;
            Some((k.trim().to_string(), v.trim().to_string()))
        })
        .collect()
}

/// Resolved theme colours for `theme` ("dark" | "light") with `swatch` as `--primary-base`.
pub struct Theme {
    pub name: &'static str,
    pub vars: HashMap<String, String>,
    pub primary: Rgb,
}

pub const SWATCHES: [&str; 5] = ["#4db380", "#3ddc97", "#4fb3d9", "#b48cff", "#ffb454"];

pub fn themes(swatch: &str) -> [Theme; 2] {
    let css = dist("draft.css");
    let dark = theme_block(&css, ":root,\n[data-theme=draft] {");
    let light = theme_block(&css, "[data-theme=draft-light] {");
    let base = Rgb::hex(swatch);

    // Light derives its primary as `color-mix(in oklab, <base> N%, #000)`.
    let mix = light.get("primary").expect("light --primary");
    let pct: f64 = mix
        .split_whitespace()
        .map(|t| t.trim_end_matches(','))
        .find(|t| t.ends_with('%'))
        .and_then(|t| t.trim_end_matches('%').parse().ok())
        .expect("mix percentage in light --primary");
    let mut light_vars = dark.clone();
    light_vars.extend(light.clone());

    [
        Theme { name: "dark", vars: dark, primary: base },
        Theme { name: "light", vars: light_vars, primary: base.mix_black(pct / 100.0) },
    ]
}

impl Theme {
    pub fn color(&self, token: &str) -> Rgb {
        if token == "primary" {
            return self.primary;
        }
        let v = self.vars.get(token).unwrap_or_else(|| panic!("--{token} missing in {}", self.name));
        assert!(v.starts_with('#'), "--{token} in {} is not a hex colour: {v}", self.name);
        Rgb::hex(v)
    }
}
