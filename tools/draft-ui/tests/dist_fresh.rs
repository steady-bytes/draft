//! `dist/` is committed (Go's `go:embed` cannot build CSS), so it must match what `scss/` compiles
//! to. Run `cargo run --features cli --bin draft-ui-css` to refresh.

use grass::{Options, OutputStyle};
use std::path::Path;

#[test]
fn dist_matches_scss() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR"));
    let targets = [
        ("scss/draft.scss", "dist/draft.css", OutputStyle::Expanded),
        ("scss/draft.scss", "dist/draft.min.css", OutputStyle::Compressed),
        ("scss/tokens.scss", "dist/tokens.css", OutputStyle::Expanded),
        ("scss/fonts.scss", "dist/fonts.css", OutputStyle::Expanded),
        ("scss/daisyui-theme.scss", "dist/draft-daisyui.theme.css", OutputStyle::Expanded),
    ];
    for (entry, out, style) in targets {
        let opts = Options::default().style(style).load_path(root.join("scss"));
        let css = grass::from_path(root.join(entry), &opts).unwrap_or_else(|e| panic!("{entry}: {e}"));
        let current = std::fs::read_to_string(root.join(out)).unwrap_or_default();
        assert_eq!(current, css, "{out} is stale — run `cargo run --features cli --bin draft-ui-css`");
    }

    let scripts: [(&str, &[&str]); 2] = [
        ("dist/boot.js", &["js/boot.js"]),
        ("dist/draft.js", &["js/boot.js", "js/behaviors.js"]),
    ];
    for (out, sources) in scripts {
        let mut js = String::new();
        for s in sources {
            js.push_str(&std::fs::read_to_string(root.join(s)).unwrap());
            if !js.ends_with('\n') {
                js.push('\n');
            }
        }
        let current = std::fs::read_to_string(root.join(out)).unwrap_or_default();
        assert_eq!(current, js, "{out} is stale — run `cargo run --features cli --bin draft-ui-css`");
    }
}
