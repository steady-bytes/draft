use dioxus::prelude::*;

/// Maps an OTel severity_text (case-insensitive) to a DaisyUI badge class so
/// the Stream table's severity column reads at a glance, same visual language
/// as Blueprint's `TypeBadge`.
fn badge_class(severity: &str) -> &'static str {
    match severity.to_ascii_uppercase().as_str() {
        "FATAL" | "ERROR" => "badge badge-error badge-sm",
        "WARN" | "WARNING" => "badge badge-warning badge-sm",
        "INFO" => "badge badge-info badge-sm",
        "DEBUG" | "TRACE" => "badge badge-ghost badge-sm",
        "" => "badge badge-neutral badge-sm",
        _ => "badge badge-neutral badge-sm",
    }
}

#[component]
pub fn SeverityBadge(severity: String) -> Element {
    let class = badge_class(&severity);
    let label = if severity.is_empty() { "—".to_string() } else { severity };
    rsx! {
        span { class: "{class}", "{label}" }
    }
}
