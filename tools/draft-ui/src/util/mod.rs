//! Small pure helpers shared by the clients (previously copied into each of them).

mod format;

pub use format::{format_bytes, format_clock, format_count, format_duration_ns, relative_time, short_type_name, truncate_middle, truncate_preview};

/// Resolves a service's API origin: the build-time `API_DOMAIN` when set and non-empty, else the
/// page origin. `option_env!` expands in the *app* crate, so the app passes it in:
///
/// ```ignore
/// pub static API_DOMAIN: Lazy<String> =
///     Lazy::new(|| draft_ui::util::resolve_domain(option_env!("API_DOMAIN"), draft_ui::util::page_origin));
/// ```
pub fn resolve_domain(build_time: Option<&str>, page_origin: impl FnOnce() -> String) -> String {
    match build_time {
        Some(d) if !d.is_empty() => d.to_string(),
        _ => page_origin(),
    }
}

/// `window.location.origin`, or an empty string outside a browser.
#[cfg(feature = "dioxus")]
pub fn page_origin() -> String {
    web_sys::window().and_then(|w| w.location().origin().ok()).unwrap_or_default()
}

/// Builds a sibling app's URL from the current page: same protocol and port, different host, so it
/// works whether Fuse listens on :10000 locally or :80/:443 in a deployment.
pub fn sibling_url(protocol: &str, port: &str, host: &str) -> String {
    if port.is_empty() {
        format!("{protocol}//{host}/")
    } else {
        format!("{protocol}//{host}:{port}/")
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn build_time_domain_wins_when_non_empty() {
        assert_eq!(resolve_domain(Some("http://x:1"), || "origin".into()), "http://x:1");
        assert_eq!(resolve_domain(Some(""), || "origin".into()), "origin");
        assert_eq!(resolve_domain(None, || "origin".into()), "origin");
    }

    #[test]
    fn sibling_url_keeps_port() {
        assert_eq!(sibling_url("http:", "10000", "beacon.draft.localhost"), "http://beacon.draft.localhost:10000/");
        assert_eq!(sibling_url("https:", "", "beacon.example.com"), "https://beacon.example.com/");
    }
}
