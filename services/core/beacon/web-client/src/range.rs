//! The time-range control shared by the Logs, Traces, Wide events and Metrics views: a segmented
//! control of presets plus a Custom range chosen in a dialog (both ends in UTC).

use chrono::{DateTime, Duration, NaiveDateTime, Utc};
use dioxus::prelude::*;
use draft_ui::ui::{Alert, Btn, BtnVariant, Field, Modal, Seg, TextInput};
use draft_ui::StatusKind;

use crate::data::short_duration;

/// The window a view queries: a relative window trailing "now", or a fixed historical range.
#[derive(Clone, Debug, PartialEq)]
pub enum TimeRange {
    Last(Duration),
    Custom { start: DateTime<Utc>, end: DateTime<Utc> },
}

impl TimeRange {
    pub fn minutes(minutes: i64) -> Self {
        TimeRange::Last(Duration::minutes(minutes))
    }

    /// The window as `(start, end)`. A trailing window has no end: it runs to now.
    pub fn bounds(&self, now: DateTime<Utc>) -> (DateTime<Utc>, Option<DateTime<Utc>>) {
        match self {
            TimeRange::Last(d) => (now - *d, None),
            TimeRange::Custom { start, end } => (*start, Some(*end)),
        }
    }

    /// `last 15m`, or `Sep 26 13:00 → 14:30 UTC` for a custom range — for the summary line.
    pub fn label(&self) -> String {
        match self {
            TimeRange::Last(d) => format!("last {}", short_duration(*d)),
            TimeRange::Custom { start, end } => {
                format!("{} → {} UTC", start.format("%b %-d %H:%M"), end.format("%b %-d %H:%M"))
            }
        }
    }

    /// A trailing window keeps running; a custom range is a fixed snapshot.
    pub fn is_trailing(&self) -> bool {
        matches!(self, TimeRange::Last(_))
    }
}

/// `YYYY-MM-DDTHH:MM` from a `datetime-local` input, read as UTC (the inputs are labelled UTC
/// rather than converted through the browser's zone, like every timestamp in Beacon).
fn parse_local(value: &str) -> Option<DateTime<Utc>> {
    NaiveDateTime::parse_from_str(value, "%Y-%m-%dT%H:%M").ok().map(|n| n.and_utc())
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum Choice {
    Preset(i64),
    Custom,
}

/// The segmented control. `presets` are minutes; Custom opens the range dialog.
#[component]
pub fn TimeRangePicker(value: TimeRange, presets: Vec<i64>, on_change: EventHandler<TimeRange>) -> Element {
    let mut open = use_signal(|| false);
    let mut start = use_signal(String::new);
    let mut end = use_signal(String::new);
    let mut error: Signal<Option<String>> = use_signal(|| None);

    let mut options: Vec<(Choice, String)> =
        presets.iter().map(|&m| (Choice::Preset(m), short_duration(Duration::minutes(m)))).collect();
    options.push((Choice::Custom, "Custom".to_string()));
    let current = match &value {
        TimeRange::Last(d) => Choice::Preset(d.num_minutes()),
        TimeRange::Custom { .. } => Choice::Custom,
    };

    let apply = move |_| match (parse_local(&start()), parse_local(&end())) {
        (Some(s), Some(e)) if s < e => {
            error.set(None);
            open.set(false);
            on_change.call(TimeRange::Custom { start: s, end: e });
        }
        (Some(_), Some(_)) => error.set(Some("The start must be before the end".to_string())),
        _ => error.set(Some("Enter both a start and an end".to_string())),
    };

    rsx! {
        Seg::<Choice> {
            options,
            value: current,
            label: "Time range".to_string(),
            on_change: move |c| match c {
                Choice::Preset(m) => on_change.call(TimeRange::minutes(m)),
                Choice::Custom => open.set(true),
            },
        }
        Modal {
            open: open(),
            title: "Custom range (UTC)".to_string(),
            on_close: move |_| open.set(false),
            footer: rsx! {
                Btn { variant: BtnVariant::Ghost, onclick: move |_| open.set(false), "Cancel" }
                Btn { variant: BtnVariant::Primary, onclick: apply, "Apply" }
            },
            if let Some(msg) = error() {
                Alert { kind: StatusKind::Err, "{msg}" }
            }
            Field { label: "Start".to_string(),
                TextInput { r#type: "datetime-local".to_string(), value: start(), oninput: move |v| start.set(v) }
            }
            Field { label: "End".to_string(),
                TextInput { r#type: "datetime-local".to_string(), value: end(), oninput: move |v| end.set(v) }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn at(s: &str) -> DateTime<Utc> {
        DateTime::parse_from_rfc3339(s).unwrap().with_timezone(&Utc)
    }

    #[test]
    fn a_trailing_window_runs_to_now() {
        let now = at("2026-09-26T12:00:00Z");
        assert_eq!(TimeRange::minutes(90).bounds(now), (at("2026-09-26T10:30:00Z"), None));
        assert_eq!(TimeRange::minutes(15).label(), "last 15m");
        assert!(TimeRange::minutes(15).is_trailing());
    }

    #[test]
    fn a_custom_range_is_fixed() {
        let r = TimeRange::Custom { start: at("2026-09-26T13:00:00Z"), end: at("2026-09-26T14:30:00Z") };
        assert_eq!(r.bounds(at("2030-01-01T00:00:00Z")), (at("2026-09-26T13:00:00Z"), Some(at("2026-09-26T14:30:00Z"))));
        assert_eq!(r.label(), "Sep 26 13:00 → Sep 26 14:30 UTC");
        assert!(!r.is_trailing());
    }

    #[test]
    fn datetime_local_is_read_as_utc() {
        assert_eq!(parse_local("2026-09-26T13:05"), Some(at("2026-09-26T13:05:00Z")));
        assert_eq!(parse_local("nope"), None);
    }
}
