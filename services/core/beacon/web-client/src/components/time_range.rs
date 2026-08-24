use chrono::{DateTime, Duration, NaiveDateTime, Utc};
use dioxus::prelude::*;

/// TimeRange is the Logs view's query window (Phase 13): either a relative
/// window trailing "now" (the default, live-tailed via `StreamLogs`) or a
/// fixed historical range (a one-shot `QueryLogs` call, no live tail — see
/// the design doc's Phase 13 "How it works").
#[derive(Clone, PartialEq)]
pub enum TimeRange {
    Live(Duration),
    Custom { start: DateTime<Utc>, end: DateTime<Utc> },
}

impl Default for TimeRange {
    fn default() -> Self {
        TimeRange::Live(Duration::minutes(15))
    }
}

impl TimeRange {
    /// label is the pill's display text.
    pub fn label(&self) -> String {
        match self {
            TimeRange::Live(d) => format!("Last {}", format_duration(*d)),
            TimeRange::Custom { start, end } => {
                format!(
                    "{} → {} UTC",
                    start.format("%Y-%m-%d %H:%M"),
                    end.format("%Y-%m-%d %H:%M")
                )
            }
        }
    }
}

fn format_duration(d: Duration) -> String {
    let mins = d.num_minutes();
    if mins < 60 {
        format!("{mins}m")
    } else if mins < 60 * 24 {
        format!("{}h", mins / 60)
    } else {
        format!("{}d", mins / (60 * 24))
    }
}

// PRESETS mirrors the wireframe's relative-range list (see
// assets/beacon-logs-redesign.html's "Time range picker" wireframe).
const PRESETS_MINUTES: &[i64] = &[5, 15, 60, 60 * 6, 60 * 24, 60 * 24 * 7];

#[component]
pub fn TimeRangePicker(value: TimeRange, on_change: EventHandler<TimeRange>) -> Element {
    let mut open = use_signal(|| false);
    let mut draft_start = use_signal(String::new);
    let mut draft_end = use_signal(String::new);
    let mut draft_error: Signal<Option<String>> = use_signal(|| None);

    let label = value.label();

    rsx! {
        div { style: "position:relative;",
            button {
                class: "btn btn-sm btn-ghost font-mono",
                onclick: move |_| open.set(!open()),
                "{label}"
                span { class: "ml-1 opacity-60", "▾" }
            }

            if open() {
                // Click-outside-to-close: an invisible full-screen backdrop
                // beneath the panel, above everything else.
                //
                // Positioning here is inline `style`, not Tailwind utility
                // classes: this subtree only enters the DOM the first time the
                // picker is opened, and this app loads Tailwind via the
                // `@tailwindcss/browser` CDN runtime, which generates utility
                // CSS by scanning/observing the DOM rather than from a
                // precompiled stylesheet. A class that has never appeared
                // before (e.g. `right-0` on a panel that didn't exist until
                // now) can render one frame (or, in practice, indefinitely
                // once the panel's own layout depends on it) before the
                // runtime catches up — which is exactly what caused the panel
                // to render past the right edge of the viewport instead of
                // anchored to the button. Inline styles apply immediately,
                // with no JIT step, so the panel is correctly positioned from
                // its very first render.
                div {
                    style: "position:fixed; inset:0; z-index:10;",
                    onclick: move |_| open.set(false),
                }
                div {
                    style: "position:absolute; right:0; top:100%; margin-top:0.25rem; z-index:20; width:18rem;",
                    class: "bg-base-200 border border-base-300 rounded-box shadow-lg p-1",
                    ul { class: "menu menu-sm w-full",
                        for mins in PRESETS_MINUTES.iter().copied() {
                            {
                                let d = Duration::minutes(mins);
                                let is_selected = matches!(&value, TimeRange::Live(cur) if *cur == d);
                                rsx! {
                                    li {
                                        a {
                                            class: if is_selected { "active" } else { "" },
                                            onclick: move |_| {
                                                open.set(false);
                                                on_change.call(TimeRange::Live(d));
                                            },
                                            "Last {format_duration(d)}"
                                        }
                                    }
                                }
                            }
                        }
                    }

                    div { class: "divider my-1 text-xs text-base-content/40", "custom range (UTC)" }

                    div { class: "flex flex-col gap-2 p-2",
                        input {
                            r#type: "datetime-local",
                            class: "input input-bordered input-sm w-full font-mono",
                            value: "{draft_start}",
                            oninput: move |e| draft_start.set(e.value()),
                        }
                        input {
                            r#type: "datetime-local",
                            class: "input input-bordered input-sm w-full font-mono",
                            value: "{draft_end}",
                            oninput: move |e| draft_end.set(e.value()),
                        }
                        if let Some(err) = draft_error() {
                            span { class: "text-error text-xs", "{err}" }
                        }
                        button {
                            class: "btn btn-sm btn-neutral self-end",
                            onclick: move |_| {
                                match (
                                    parse_local_datetime(&draft_start()),
                                    parse_local_datetime(&draft_end()),
                                ) {
                                    (Some(start), Some(end)) if start < end => {
                                        draft_error.set(None);
                                        open.set(false);
                                        on_change.call(TimeRange::Custom { start, end });
                                    }
                                    (Some(_), Some(_)) => {
                                        draft_error.set(Some("start must be before end".to_string()));
                                    }
                                    _ => {
                                        draft_error.set(Some("enter both a start and end".to_string()));
                                    }
                                }
                            },
                            "Apply"
                        }
                    }
                }
            }
        }
    }
}

/// parse_local_datetime parses an `<input type="datetime-local">` value
/// (`YYYY-MM-DDTHH:MM`, no timezone) as UTC — the picker's custom-range
/// inputs are labeled "(UTC)" rather than converting through the browser's
/// local timezone, matching the rest of the Logs view (the table's TIME
/// column is already UTC).
fn parse_local_datetime(s: &str) -> Option<DateTime<Utc>> {
    NaiveDateTime::parse_from_str(s, "%Y-%m-%dT%H:%M")
        .ok()
        .map(|naive| naive.and_utc())
}
