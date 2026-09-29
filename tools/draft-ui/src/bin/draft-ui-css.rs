//! Compiles `scss/` into `dist/`.
//!
//! ```text
//! cargo run --features cli --bin draft-ui-css            # write dist/
//! cargo run --features cli --bin draft-ui-css -- --check # exit 1 if dist/ is stale (CI)
//! ```
//!
//! `dist/` is committed because Go's `go:embed` cannot build CSS; `--check` keeps it honest.

use std::path::{Path, PathBuf};
use std::process::ExitCode;

use grass::{Options, OutputStyle};

/// (entry, output, style)
const TARGETS: &[(&str, &str, OutputStyle)] = &[
    ("scss/draft.scss", "dist/draft.css", OutputStyle::Expanded),
    ("scss/draft.scss", "dist/draft.min.css", OutputStyle::Compressed),
    ("scss/tokens.scss", "dist/tokens.css", OutputStyle::Expanded),
    ("scss/fonts.scss", "dist/fonts.css", OutputStyle::Expanded),
    ("scss/daisyui-theme.scss", "dist/draft-daisyui.theme.css", OutputStyle::Expanded),
    // The docs site's colour file: it is Sass by name only (plain CSS is valid Sass), because Hugo
    // `@import`s it into the lotusdocs stylesheet. See scss/docs.scss.
    ("scss/docs.scss", "dist/docs/_draft.scss", OutputStyle::Expanded),
];

/// (output, concatenated sources). `boot.js` is the inline pre-paint snippet; `draft.js` is that plus
/// the progressive-enhancement behaviours, so the theme logic exists once.
const SCRIPTS: &[(&str, &[&str])] = &[
    ("dist/boot.js", &["js/boot.js"]),
    ("dist/draft.js", &["js/boot.js", "js/behaviors.js"]),
];

fn assemble(root: &Path, sources: &[&str]) -> Result<String, String> {
    let mut out = String::new();
    for s in sources {
        let text = std::fs::read_to_string(root.join(s)).map_err(|e| format!("{s}: {e}"))?;
        out.push_str(&text);
        if !out.ends_with('\n') {
            out.push('\n');
        }
    }
    Ok(out)
}

fn compile(root: &Path, entry: &str, style: OutputStyle) -> Result<String, String> {
    let options = Options::default().style(style).load_path(root.join("scss"));
    grass::from_path(root.join(entry), &options).map_err(|e| format!("{entry}: {e}"))
}

fn main() -> ExitCode {
    let root = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    let check = std::env::args().any(|a| a == "--check");
    let mut stale = false;

    for (entry, out, style) in TARGETS {
        let css = match compile(&root, entry, *style) {
            Ok(css) => css,
            Err(e) => {
                eprintln!("error: {e}");
                return ExitCode::FAILURE;
            }
        };
        let path = root.join(out);
        if check {
            let current = std::fs::read_to_string(&path).unwrap_or_default();
            if current != css {
                eprintln!("stale: {out} (run `cargo run --features cli --bin draft-ui-css`)");
                stale = true;
            }
        } else if let Err(e) = path
            .parent()
            .map_or(Ok(()), std::fs::create_dir_all)
            .and_then(|()| std::fs::write(&path, &css))
        {
            eprintln!("error: writing {out}: {e}");
            return ExitCode::FAILURE;
        } else {
            println!("wrote {out} ({} bytes)", css.len());
        }
    }

    for (out, sources) in SCRIPTS {
        let js = match assemble(&root, sources) {
            Ok(js) => js,
            Err(e) => {
                eprintln!("error: {e}");
                return ExitCode::FAILURE;
            }
        };
        let path = root.join(out);
        if check {
            if std::fs::read_to_string(&path).unwrap_or_default() != js {
                eprintln!("stale: {out} (run `cargo run --features cli --bin draft-ui-css`)");
                stale = true;
            }
        } else if let Err(e) = std::fs::write(&path, &js) {
            eprintln!("error: writing {out}: {e}");
            return ExitCode::FAILURE;
        } else {
            println!("wrote {out} ({} bytes)", js.len());
        }
    }

    if stale {
        ExitCode::FAILURE
    } else {
        ExitCode::SUCCESS
    }
}
