use dioxus::prelude::*;
use draft_api::proto::tooling_lineman_v1::AgentKind;
use draft_ui::ui::Tag;

use crate::rail::RailSnapshot;
use crate::state::priority_tag;

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

pub fn agent_kind_label(kind: i32) -> &'static str {
    match AgentKind::try_from(kind).unwrap_or(AgentKind::Unspecified) {
        AgentKind::Human => "Human",
        AgentKind::Script => "Script",
        AgentKind::Ai => "AI",
        AgentKind::Unspecified => "Unknown",
    }
}

/// The priority tag (`High` red, `Medium` amber, `Low` quiet) — the design system's `Tag` with
/// Lineman's priority mapping, replacing the daisyUI `priority_badge`.
#[component]
pub fn PriorityTag(priority: i32) -> Element {
    let (tone, label) = priority_tag(priority);
    rsx! {
        Tag { tone, "{label}" }
    }
}

/// The priorities a create form offers, as `(proto name, label)` pairs for a `Select`.
pub fn priority_options() -> Vec<(String, String)> {
    vec![
        ("PRIORITY_LOW".to_string(), "Low".to_string()),
        ("PRIORITY_MEDIUM".to_string(), "Medium".to_string()),
        ("PRIORITY_HIGH".to_string(), "High".to_string()),
    ]
}

/// Objectives to attach a scheduled task or loop to: an empty value means standalone.
pub fn objective_options(snapshot: &RailSnapshot) -> Vec<(String, String)> {
    let mut options = vec![(String::new(), "Standalone (no objective)".to_string())];
    options.extend(snapshot.objectives.iter().map(|o| (o.id.clone(), o.name.clone())));
    options
}

/// An objective's name for a table cell, or `—` for a standalone item.
pub fn objective_name(snapshot: &RailSnapshot, id: &str) -> String {
    if id.is_empty() {
        return "—".to_string();
    }
    snapshot.objectives.iter().find(|o| o.id == id).map(|o| o.name.clone()).unwrap_or_else(|| id.to_string())
}

/// `Sep 21, 2026`, or `—` when unset.
pub fn format_date(seconds: Option<i64>) -> String {
    seconds
        .and_then(|s| chrono::DateTime::from_timestamp(s, 0))
        .map(|d| d.format("%b %-d, %Y").to_string())
        .unwrap_or_else(|| "—".to_string())
}

/// `Sep 28 · 14:30 UTC`, or `—` when unset.
pub fn format_when(seconds: Option<i64>) -> String {
    seconds
        .and_then(|s| chrono::DateTime::from_timestamp(s, 0))
        .map(|d| d.format("%b %-d · %H:%M UTC").to_string())
        .unwrap_or_else(|| "—".to_string())
}

/// `daily at 09:00`, `every 3 days at 09:00`, `weekly at 09:00` from a loop's recurrence.
pub fn recurrence_label(kind: &str, interval_days: i32, at: &str) -> String {
    let when = if at.is_empty() { String::new() } else { format!(" at {at}") };
    match kind {
        "daily" => format!("daily{when}"),
        "weekly" => format!("weekly{when}"),
        "every_n_days" => format!("every {interval_days} days{when}"),
        other => format!("{other}{when}"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn datetime_local_is_read_as_utc() {
        assert_eq!(parse_datetime_local("2026-09-28T14:30").unwrap().seconds, 1_790_605_800);
        assert!(parse_datetime_local("not a date").is_none());
    }

    #[test]
    fn dates_and_times() {
        assert_eq!(format_date(Some(1_790_605_800)), "Sep 28, 2026");
        assert_eq!(format_when(Some(1_790_605_800)), "Sep 28 · 14:30 UTC");
        assert_eq!(format_date(None), "—");
    }

    #[test]
    fn recurrence_reads_naturally() {
        assert_eq!(recurrence_label("daily", 1, "09:00"), "daily at 09:00");
        assert_eq!(recurrence_label("every_n_days", 3, "09:00"), "every 3 days at 09:00");
        assert_eq!(recurrence_label("weekly", 1, ""), "weekly");
    }
}
