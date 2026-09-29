//! The Dioxus clients inline `js/boot.js` in their `index.html` (it must run before first paint, so
//! it cannot be a separate request). This keeps those copies from drifting from the source: a fix to
//! the boot script (the IP-host cookie bug, say) has to reach every client.
//!
//! It reads the same `DRAFT_UI_SCAN` directories as the class contract — each app's `src/`, with its
//! `index.html` one level up — and does nothing when the variable is unset.

use std::path::Path;

#[test]
fn inlined_boot_scripts_match_the_source() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR"));
    let source = std::fs::read_to_string(root.join("js/boot.js")).unwrap();
    let source = source.trim();
    let scan = std::env::var("DRAFT_UI_SCAN").unwrap_or_default();

    let mut stale = vec![];
    let mut checked = 0;
    for dir in scan.split(':').filter(|d| !d.is_empty()) {
        let Some(index) = Path::new(dir).parent().map(|p| p.join("index.html")) else { continue };
        let Ok(html) = std::fs::read_to_string(&index) else { continue };
        if !html.contains("draft boot") {
            continue;
        }
        checked += 1;
        if !html.contains(source) {
            stale.push(index.display().to_string());
        }
    }
    assert!(
        stale.is_empty(),
        "these index.html files inline an out-of-date js/boot.js (paste dist/boot.js between their <script> tags):\n  {}",
        stale.join("\n  ")
    );
    if !scan.is_empty() {
        assert!(checked > 0, "DRAFT_UI_SCAN is set but no scanned app inlines the boot script");
    }
}
