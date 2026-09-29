//! Theme and primary-colour persistence, compatible with `dist/boot.js`.
//!
//! Preferences live in a cookie shared by sibling subdomains (`bench.draft.localhost` ↔
//! `blueprint.draft.localhost`), with `localStorage` as the fallback when the browser rejects a
//! parent-domain cookie. Verified across `a.`/`b.draft.localhost` in Chrome (plan, Phase 0).
//! Outside a browser every function is a no-op.

pub const THEME_KEY: &str = "draft.theme";
pub const PRIMARY_KEY: &str = "draft.primary";
/// A user-dragged [`Split`](crate::layout::Split) drawer width, in CSS pixels as a plain integer
/// string (e.g. `"420"`). Shared across every drawer in every Draft app, the same way theme/primary
/// are — one dragged width is a reasonable default everywhere rather than something to track per
/// page.
pub const DRAWER_WIDTH_KEY: &str = "draft.drawer_width";

/// The parent domain to share a cookie with: the host minus its first label, when it has at
/// least three labels (`blueprint.draft.localhost` → `draft.localhost`).
pub fn cookie_domain(hostname: &str) -> Option<String> {
    let labels: Vec<&str> = hostname.split('.').collect();
    (labels.len() >= 3).then(|| labels[1..].join("."))
}

/// True for the `#rrggbb` values `boot.js` and the swatches use. Anything else is ignored, so a
/// tampered cookie cannot inject CSS.
pub fn is_hex_colour(v: &str) -> bool {
    v.len() == 7 && v.starts_with('#') && v[1..].chars().all(|c| c.is_ascii_hexdigit())
}

#[cfg(all(feature = "dioxus", target_arch = "wasm32"))]
mod web {
    use wasm_bindgen::JsCast;

    fn document() -> Option<web_sys::HtmlDocument> {
        web_sys::window()?.document()?.dyn_into::<web_sys::HtmlDocument>().ok()
    }

    pub fn get(key: &str) -> Option<String> {
        if let Some(doc) = document() {
            let cookies = doc.cookie().unwrap_or_default();
            for part in cookies.split("; ") {
                if let Some(v) = part.strip_prefix(&format!("{key}=")) {
                    return Some(js_sys_decode(v));
                }
            }
        }
        web_sys::window()?.local_storage().ok()??.get_item(key).ok()?
    }

    pub fn set(key: &str, value: &str) {
        if let Some(storage) = web_sys::window().and_then(|w| w.local_storage().ok().flatten()) {
            let _ = storage.set_item(key, value);
        }
        let Some(doc) = document() else { return };
        let base = format!("{key}={}; Path=/; Max-Age=31536000; SameSite=Lax", js_sys_encode(value));
        let host = web_sys::window().and_then(|w| w.location().hostname().ok()).unwrap_or_default();
        if let Some(domain) = super::cookie_domain(&host) {
            let _ = doc.set_cookie(&format!("{base}; Domain={domain}"));
            if doc.cookie().unwrap_or_default().contains(&format!("{key}=")) {
                return; // parent-domain cookie accepted
            }
        }
        let _ = doc.set_cookie(&base); // host-only fallback
    }

    // Values are `draft`, `draft-light` or `#rrggbb`: only `#` needs escaping.
    fn js_sys_encode(v: &str) -> String {
        v.replace('#', "%23")
    }

    fn js_sys_decode(v: &str) -> String {
        v.replace("%23", "#")
    }
}

/// Reads a saved preference.
pub fn get(key: &str) -> Option<String> {
    #[cfg(all(feature = "dioxus", target_arch = "wasm32"))]
    {
        web::get(key)
    }
    #[cfg(not(all(feature = "dioxus", target_arch = "wasm32")))]
    {
        let _ = key;
        None
    }
}

/// Saves a preference.
pub fn set(key: &str, value: &str) {
    #[cfg(all(feature = "dioxus", target_arch = "wasm32"))]
    web::set(key, value);
    #[cfg(not(all(feature = "dioxus", target_arch = "wasm32")))]
    let _ = (key, value);
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parent_domain_for_subdomains_only() {
        assert_eq!(cookie_domain("blueprint.draft.localhost").as_deref(), Some("draft.localhost"));
        assert_eq!(cookie_domain("bench.example.com").as_deref(), Some("example.com"));
        assert_eq!(cookie_domain("localhost"), None);
        assert_eq!(cookie_domain("draft.localhost"), None);
    }

    #[test]
    fn only_six_digit_hex_is_accepted() {
        assert!(is_hex_colour("#4db380"));
        assert!(!is_hex_colour("#4db38"));
        assert!(!is_hex_colour("red; background:url(x)"));
        assert!(!is_hex_colour("4db380"));
    }
}
