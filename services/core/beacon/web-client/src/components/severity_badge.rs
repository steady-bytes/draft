/// severity_stripe_color maps an OTel severity_text (case-insensitive) to the
/// color of a subtle left-edge row stripe in the Logs table — replaces the
/// louder colored-background badge this used to render as. Returns a CSS
/// `var(--color-*)` reference into daisyUI's own theme tokens (not a
/// hardcoded hex) so it always matches the active theme; debug/trace/empty
/// map to "transparent" so only rows worth noticing draw the eye.
///
/// Returned as a CSS value for an inline `style`, not a Tailwind utility
/// class: log rows are inserted continuously as the stream runs, and this
/// app loads Tailwind via the `@tailwindcss/browser` CDN runtime rather than
/// a precompiled stylesheet (see time_range.rs's doc comment on the same
/// issue) — an inline style applies immediately with no dependency on the
/// JIT scanner ever having seen a given class before.
pub fn severity_stripe_color(severity: &str) -> &'static str {
    match severity.to_ascii_uppercase().as_str() {
        "FATAL" | "ERROR" => "var(--color-error)",
        "WARN" | "WARNING" => "var(--color-warning)",
        "INFO" => "var(--color-info)",
        _ => "transparent",
    }
}

/// severity_label is the plain-text severity column value — "—" for an empty
/// severity_text rather than a blank cell.
pub fn severity_label(severity: &str) -> &str {
    if severity.is_empty() {
        "—"
    } else {
        severity
    }
}
