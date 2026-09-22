use dioxus::prelude::*;
use draft_api::proto::tooling_lineman_v1::{AgentKind, Priority};

/// Parses an HTML `<input type="datetime-local">` value ("YYYY-MM-DDTHH:MM")
/// into a Timestamp. Treated as UTC -- a documented simplification, not
/// timezone-aware; good enough for a local-dev framework with no real
/// timezone requirement yet.
pub fn parse_datetime_local(value: &str) -> Option<prost_types::Timestamp> {
    let naive = chrono::NaiveDateTime::parse_from_str(value, "%Y-%m-%dT%H:%M").ok()?;
    let utc = naive.and_utc();
    Some(prost_types::Timestamp {
        seconds: utc.timestamp(),
        nanos: 0,
    })
}

/// A daisyUI priority badge, matching the design brief's PriorityBadge
/// component (badge-error/warning/ghost).
pub fn priority_badge(priority: i32) -> Element {
    let (label, class) = match Priority::try_from(priority).unwrap_or(Priority::Unspecified) {
        Priority::High => ("High", "badge-error"),
        Priority::Medium => ("Medium", "badge-warning"),
        Priority::Low => ("Low", "badge-ghost"),
        Priority::Unspecified => ("—", "badge-ghost"),
    };
    rsx! {
        span { class: "badge {class} badge-sm", "{label}" }
    }
}

/// The pulsing agent-identity line used on Task Board cards and Task Detail
/// -- animate-pulse for "actively working," a static dot for anything else
/// (matches the design brief's distinction between In Flight and Needs
/// Input agent dots).
pub fn agent_badge(agent_id: &str, pulsing: bool) -> Element {
    if agent_id.is_empty() {
        return rsx! {};
    }
    let dot_class = if pulsing {
        "w-1.5 h-1.5 rounded-full bg-info animate-pulse"
    } else {
        "w-1.5 h-1.5 rounded-full bg-warning"
    };
    let agent_id = agent_id.to_string();
    rsx! {
        div { class: "flex items-center gap-1.5 text-xs text-base-content/60",
            span { class: "{dot_class}" }
            "🤖 {agent_id}"
        }
    }
}

pub fn agent_kind_label(kind: i32) -> &'static str {
    match AgentKind::try_from(kind).unwrap_or(AgentKind::Unspecified) {
        AgentKind::Human => "Human",
        AgentKind::Script => "Script",
        AgentKind::Ai => "AI",
        AgentKind::Unspecified => "Unknown",
    }
}

/// A transient daisyUI toast, anchored bottom-left (`toast-start toast-bottom`
/// -- daisyUI's own corner-positioning modifiers). Purely presentational;
/// callers own the signal that decides whether/what to show and how long it
/// stays up (see task_detail.rs for the show-then-auto-clear pattern).
pub fn toast(message: &str) -> Element {
    rsx! {
        div { class: "toast toast-start toast-bottom z-50",
            div { class: "alert alert-success shadow-lg",
                span { "{message}" }
            }
        }
    }
}
