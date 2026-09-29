//! Class contract: every `d-*`, `is-*` and `tk-*` class referenced by Rust, Go templates or the
//! preview gallery must be defined in `dist/draft.css`. Catches typos and drift between the
//! renderers and the stylesheet.

mod common;

use regex::Regex;
use std::collections::BTreeSet;
use std::path::Path;

fn walk(dir: &Path, exts: &[&str], out: &mut Vec<std::path::PathBuf>) {
    let Ok(rd) = std::fs::read_dir(dir) else { return };
    for e in rd.flatten() {
        let p = e.path();
        if p.is_dir() {
            walk(&p, exts, out);
        } else if p.extension().and_then(|x| x.to_str()).is_some_and(|x| exts.contains(&x)) {
            out.push(p);
        }
    }
}

/// Classes defined by a stylesheet.
fn classes_in(css: &str, set: &mut BTreeSet<String>) {
    let re = Regex::new(r"\.((?:d|is|tk)-[a-z0-9]+(?:-[a-z0-9]+)*(?:--[a-z0-9]+(?:-[a-z0-9]+)*)?)").unwrap();
    let re_state = Regex::new(r"\.((?:is)-[a-z0-9-]+)").unwrap();
    set.extend(re.captures_iter(css).map(|c| c[1].to_string()));
    set.extend(re_state.captures_iter(css).map(|c| c[1].to_string()));
}

/// The shared stylesheet, plus each scanned app's own (`<dir>/../assets/*.css`, e.g. a web-client's
/// `src/` next to its `assets/beacon.css`): apps keep page-local state classes such as `is-error`
/// there, and those are legitimately not part of the shared contract.
fn defined(extra_dirs: &[&Path]) -> BTreeSet<String> {
    let mut set = BTreeSet::new();
    classes_in(&common::dist("draft.css"), &mut set);
    for dir in extra_dirs {
        let mut css = vec![];
        if let Some(parent) = dir.parent() {
            walk(&parent.join("assets"), &["css"], &mut css);
        }
        for f in css {
            if let Ok(text) = std::fs::read_to_string(f) {
                classes_in(&text, &mut set);
            }
        }
    }
    set
}

/// Classes that legitimately appear in sources without a rule of their own (state hooks applied by
/// scripts, or documented as examples).
const ALLOW: &[&str] = &["d-app-shell"];

#[test]
fn every_referenced_class_is_defined() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR"));
    let mut files = vec![];
    for sub in ["src", "templates", "preview"] {
        walk(&root.join(sub), &["rs", "html", "tmpl", "gohtml"], &mut files);
    }
    let extra = std::env::var("DRAFT_UI_SCAN").unwrap_or_default();
    let extra_dirs: Vec<&Path> = extra.split(':').filter(|d| !d.is_empty()).map(Path::new).collect();
    for dir in &extra_dirs {
        walk(dir, &["rs", "html", "tmpl", "gohtml"], &mut files);
    }

    let defined = defined(&extra_dirs);
    let used = Regex::new(r#"(?:^|[\s"'`{(])((?:d|is|tk)-[a-z0-9]+(?:-[a-z0-9]+)*(?:--[a-z0-9]+(?:-[a-z0-9]+)*)?)(?:[\s"'`})]|$)"#).unwrap();

    let mut missing: BTreeSet<(String, String)> = BTreeSet::new();
    for f in &files {
        let Ok(text) = std::fs::read_to_string(f) else { continue };
        for cap in used.captures_iter(&text) {
            let c = &cap[1];
            if !defined.contains(c) && !ALLOW.contains(&c) {
                missing.insert((c.to_string(), f.strip_prefix(root).unwrap_or(f).display().to_string()));
            }
        }
    }
    assert!(
        missing.is_empty(),
        "classes used but not defined in dist/draft.css:\n{}",
        missing.iter().map(|(c, f)| format!("  {c}  ({f})")).collect::<Vec<_>>().join("\n")
    );
}
