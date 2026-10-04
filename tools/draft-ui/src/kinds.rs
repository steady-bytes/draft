//! Meaning colours and app identities. Pure data — no rendering — so it is shared by the Dioxus
//! components, the tests and (by name) the Go templates.

/// A meaning colour. Maps 1:1 onto the `d-tag--*` / `d-glyph--*` modifiers and the `--ca`,
/// `--bp`, … custom properties.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub enum Tone {
    /// Neutral: quiet border, `--dim` text.
    #[default]
    Quiet,
    Primary,
    /// Catalyst blue; also "info".
    Ca,
    /// Blueprint violet; also agents.
    Bp,
    /// Fuse amber.
    Fs,
    /// Generic service slate.
    Sv,
    Err,
    Warn,
}

impl Tone {
    /// The `d-tag--*` modifier.
    pub const fn tag_class(self) -> &'static str {
        match self {
            Tone::Quiet => "d-tag--quiet",
            Tone::Primary => "d-tag--primary",
            Tone::Ca => "d-tag--ca",
            Tone::Bp => "d-tag--bp",
            Tone::Fs => "d-tag--fs",
            Tone::Sv => "d-tag--sv",
            Tone::Err => "d-tag--err",
            Tone::Warn => "d-tag--warn",
        }
    }

    /// The `d-glyph--*` modifier ("" for the default slate glyph).
    pub const fn glyph_class(self) -> &'static str {
        match self {
            Tone::Quiet | Tone::Sv => "",
            Tone::Primary => "d-glyph--primary",
            Tone::Ca => "d-glyph--ca",
            Tone::Bp => "d-glyph--bp",
            Tone::Fs => "d-glyph--fs",
            Tone::Err => "d-glyph--err",
            Tone::Warn => "d-glyph--warn",
        }
    }

    /// The custom property, for `style: "--c:{var}"` and SVG `stroke`/`fill`.
    pub const fn css_var(self) -> &'static str {
        match self {
            Tone::Quiet => "var(--dim)",
            Tone::Primary => "var(--primary)",
            Tone::Ca => "var(--ca)",
            Tone::Bp => "var(--bp)",
            Tone::Fs => "var(--fs)",
            Tone::Sv => "var(--sv)",
            Tone::Err => "var(--err)",
            Tone::Warn => "var(--warn)",
        }
    }

    /// A stable colour for an arbitrary name (service, series). Known Draft components keep their
    /// identity colour; anything else is hashed into the palette so the same name always renders
    /// the same colour without a lookup table.
    pub fn for_name(name: &str) -> Tone {
        match name.to_ascii_lowercase().as_str() {
            "blueprint" => return Tone::Bp,
            "catalyst" => return Tone::Ca,
            "fuse" => return Tone::Fs,
            _ => {}
        }
        const PALETTE: [Tone; 5] = [Tone::Ca, Tone::Bp, Tone::Fs, Tone::Primary, Tone::Sv];
        let hash = name
            .bytes()
            .fold(2_166_136_261u32, |acc, b| (acc ^ u32::from(b)).wrapping_mul(16_777_619));
        PALETTE[(hash % PALETTE.len() as u32) as usize]
    }

    /// The nth colour of the series palette (line charts). Cycles.
    pub const fn series(n: usize) -> Tone {
        const PALETTE: [Tone; 5] = [Tone::Bp, Tone::Ca, Tone::Primary, Tone::Fs, Tone::Sv];
        PALETTE[n % PALETTE.len()]
    }
}

/// A status: a dot and a label. Healthy stays quiet; only problems get loud.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum StatusKind {
    /// Healthy / passed.
    #[default]
    Ok,
    /// In flight / informational (blue).
    Info,
    Warn,
    Err,
    /// Offline / never run.
    Idle,
}

impl StatusKind {
    /// The `d-status--*` modifier ("" for the default).
    pub const fn class(self) -> &'static str {
        match self {
            StatusKind::Ok => "",
            StatusKind::Info => "d-status--info",
            StatusKind::Warn => "d-status--warn",
            StatusKind::Err => "d-status--err",
            StatusKind::Idle => "d-status--idle",
        }
    }
}

/// A Draft application. Drives the two-letter glyph and its colour in the rail's Apps block.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub enum AppKind {
    Blueprint,
    Catalyst,
    Fuse,
    Beacon,
    Bench,
    Foundry,
    Lineman,
    Relay,
    Allele,
    /// Any other service.
    Service,
}

impl AppKind {
    pub const fn code(self) -> &'static str {
        match self {
            AppKind::Blueprint => "Bp",
            AppKind::Catalyst => "Ca",
            AppKind::Fuse => "Fs",
            AppKind::Beacon => "Bc",
            AppKind::Bench => "Bn",
            AppKind::Foundry => "Fd",
            AppKind::Lineman => "Lm",
            AppKind::Relay => "Rl",
            AppKind::Allele => "Al",
            AppKind::Service => "Sv",
        }
    }

    pub const fn name(self) -> &'static str {
        match self {
            AppKind::Blueprint => "blueprint",
            AppKind::Catalyst => "catalyst",
            AppKind::Fuse => "fuse",
            AppKind::Beacon => "beacon",
            AppKind::Bench => "bench",
            AppKind::Foundry => "foundry",
            AppKind::Lineman => "lineman",
            AppKind::Relay => "relay",
            AppKind::Allele => "allele",
            AppKind::Service => "service",
        }
    }

    /// Glyph colour, as in the mockups: Blueprint violet, Catalyst blue, Fuse amber, Beacon blue,
    /// Lineman violet, Bench and Foundry slate. Relay has no dedicated hue of its own in the
    /// mockups (unlike the others, every existing `Tone` variant is already claimed by one of
    /// them) -- `Primary` is the theme's own accent colour, a reasonable default for a new app
    /// rather than overloading `Err`/`Warn`'s error/warning semantics just because Relay's own
    /// Record page happens to use `--err` for its record button. Allele is infrastructure/tooling
    /// in the same sense Bench/Foundry already are (a git server, not a user-facing colored app),
    /// so it joins their shared slate `Sv` rather than claiming yet another dedicated hue.
    pub const fn tone(self) -> Tone {
        match self {
            AppKind::Blueprint | AppKind::Lineman => Tone::Bp,
            AppKind::Catalyst | AppKind::Beacon => Tone::Ca,
            AppKind::Fuse => Tone::Fs,
            AppKind::Relay => Tone::Primary,
            AppKind::Bench | AppKind::Foundry | AppKind::Allele | AppKind::Service => Tone::Sv,
        }
    }

    /// Resolves the app for a name or host label ("beacon", "beacon.draft.localhost", "Beacon").
    pub fn from_name(name: &str) -> AppKind {
        let lower = name.split('.').next().unwrap_or(name).to_ascii_lowercase();
        let mut first = lower.as_str();
        for prefix in ["core-", "tooling-", "examples-"] {
            first = first.strip_prefix(prefix).unwrap_or(first);
        }
        first = first.strip_suffix("-ui").unwrap_or(first);
        match first {
            "blueprint" => AppKind::Blueprint,
            "catalyst" => AppKind::Catalyst,
            "fuse" => AppKind::Fuse,
            "beacon" => AppKind::Beacon,
            "bench" => AppKind::Bench,
            "foundry" => AppKind::Foundry,
            "lineman" => AppKind::Lineman,
            "relay" => AppKind::Relay,
            "allele" => AppKind::Allele,
            _ => AppKind::Service,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn known_components_keep_their_identity_colour() {
        assert_eq!(Tone::for_name("fuse"), Tone::Fs);
        assert_eq!(Tone::for_name("Blueprint"), Tone::Bp);
        assert_eq!(Tone::for_name("catalyst"), Tone::Ca);
    }

    #[test]
    fn hashed_colour_is_stable() {
        assert_eq!(Tone::for_name("crud"), Tone::for_name("crud"));
        assert_ne!(Tone::for_name("crud"), Tone::Quiet);
    }

    #[test]
    fn series_palette_cycles() {
        assert_eq!(Tone::series(0), Tone::series(5));
    }

    #[test]
    fn app_from_host_or_name() {
        assert_eq!(AppKind::from_name("beacon.draft.localhost"), AppKind::Beacon);
        assert_eq!(AppKind::from_name("core-blueprint-ui"), AppKind::Blueprint);
        assert_eq!(AppKind::from_name("tooling-bench-ui"), AppKind::Bench);
        assert_eq!(AppKind::from_name("examples-crud"), AppKind::Service);
        assert_eq!(AppKind::from_name("allele.draft.localhost"), AppKind::Allele);
    }

    #[test]
    fn tone_classes_exist_in_the_stylesheet() {
        let css = std::fs::read_to_string(concat!(env!("CARGO_MANIFEST_DIR"), "/dist/draft.css")).unwrap();
        for t in [Tone::Quiet, Tone::Primary, Tone::Ca, Tone::Bp, Tone::Fs, Tone::Sv, Tone::Err, Tone::Warn] {
            assert!(css.contains(&format!(".{}", t.tag_class())), "{}", t.tag_class());
            let g = t.glyph_class();
            if !g.is_empty() {
                assert!(css.contains(&format!(".{g}")), "{g}");
            }
        }
        for s in [StatusKind::Info, StatusKind::Warn, StatusKind::Err, StatusKind::Idle] {
            assert!(css.contains(&format!(".{}", s.class())), "{}", s.class());
        }
    }
}
