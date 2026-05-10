use dioxus::prelude::*;
use tonic_web_wasm_client::Client as WasmClient;

use draft_api::proto::core_message_broker_actors_v1::{
    cloud_event::cloud_event_attribute_value,
    query_client::QueryClient,
    CloudEvent, QueryRequest,
};

use crate::components::{CesqlBar, FilterChips, TypeBadge};

// Cap stored events so the table doesn't grow without bound.
const MAX_EVENTS: usize = 1_000;

#[derive(Clone, PartialEq)]
enum StreamStatus {
    Connecting,
    Connected,
    Disconnected,
}

#[component]
pub fn Store() -> Element {
    let mut expression = use_signal(String::new);
    // `applied` is only updated when the user clicks Run, presses Enter, or
    // selects a chip — keeping draft input separate from the active filter.
    let mut applied = use_signal(String::new);
    let mut events: Signal<Vec<CloudEvent>> = use_signal(Vec::new);
    let mut status: Signal<StreamStatus> = use_signal(|| StreamStatus::Connecting);

    // Long-running coroutine: opens QueryStream once and appends every arriving
    // CloudEvent to `events`. The expression filter is applied in the render
    // pass below so no restart is needed when the user changes the query.
    use_coroutine(move |_rx: UnboundedReceiver<()>| {
        let host = crate::CATALYST_DOMAIN.clone();
        async move {
            let mut client = QueryClient::new(WasmClient::new(host));
            let stream_response = client
                .query_stream(QueryRequest {
                    expression: None,
                    limit: 0,
                    after: String::new(),
                })
                .await;
            let Ok(response) = stream_response else {
                status.set(StreamStatus::Disconnected);
                return;
            };
            status.set(StreamStatus::Connected);
            let mut stream = response.into_inner();
            loop {
                match stream.message().await {
                    Ok(Some(msg)) => {
                        if let Some(event) = msg.event {
                            let mut ev = events.write();
                            if ev.len() >= MAX_EVENTS {
                                ev.pop();
                            }
                            ev.insert(0, event);
                        }
                    }
                    Ok(None) | Err(_) => {
                        status.set(StreamStatus::Disconnected);
                        break;
                    }
                }
            }
        }
    });

    let all_events = events.read();
    let filter = applied.read();

    let filtered: Vec<&CloudEvent> = all_events
        .iter()
        .filter(|e| matches_filter(&filter, e))
        .collect();

    let total = all_events.len();
    let matched = filtered.len();

    rsx! {
        div { class: "p-4 flex flex-col gap-3",

            CesqlBar {
                expression,
                on_run: move |_| applied.set(expression.read().clone()),
                on_clear: move |_| {
                    expression.set(String::new());
                    applied.set(String::new());
                },
            }

            FilterChips {
                on_select: move |expr: String| {
                    expression.set(expr.clone());
                    applied.set(expr);
                },
            }

            div { class: "flex items-center gap-2",
                p { class: "text-sm text-base-content/50",
                    "{matched} of {total} events"
                }
                match status() {
                    StreamStatus::Disconnected => rsx! {
                        span {
                            class: "tooltip tooltip-right",
                            "data-tip": "Catalyst server disconnected",
                            svg {
                                class: "w-4 h-4 text-error",
                                xmlns: "http://www.w3.org/2000/svg",
                                view_box: "0 0 20 20",
                                fill: "currentColor",
                                path {
                                    fill_rule: "evenodd",
                                    clip_rule: "evenodd",
                                    d: "M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z",
                                }
                            }
                        }
                    },
                    _ => rsx! {},
                }
            }

            div { class: "overflow-x-auto",
                table { class: "table table-xs",
                    thead {
                        tr {
                            th { "TIME (UTC)" }
                            th { "TYPE" }
                            th { "SOURCE" }
                            th { "SUBJECT" }
                            th { "ID" }
                        }
                    }
                    tbody {
                        if filtered.is_empty() {
                            tr {
                                td {
                                    colspan: "5",
                                    class: "text-center text-base-content/40 py-6",
                                    if all_events.is_empty() { "Waiting for events…" } else { "No events match the filter." }
                                }
                            }
                        }
                        for event in filtered.iter() {
                            {
                                let time    = event_time(event);
                                let subject = event_subject(event);
                                let etype   = event.r#type.clone();
                                let source  = event.source.clone();
                                let id      = event.id.clone();
                                rsx! {
                                    tr { class: "hover:bg-base-300",
                                        td { class: "font-mono text-xs text-base-content/60", "{time}" }
                                        td { TypeBadge { event_type: etype } }
                                        td { class: "text-xs", "{source}" }
                                        td { class: "font-mono text-xs", "{subject}" }
                                        td { class: "font-mono text-xs text-base-content/40", "{id}" }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

// ─── Filter ───────────────────────────────────────────────────────────────────

// Evaluates a CESQL-subset expression against a CloudEvent.
// Supported forms:
//   field = 'value'          exact match (case-insensitive)
//   field LIKE '%pattern%'   contains (% wildcards stripped)
//   A OR B                   either branch matches
//   <text>                   fallback: substring across type/source/id/subject
fn matches_filter(filter: &str, event: &CloudEvent) -> bool {
    let filter = filter.trim();
    if filter.is_empty() {
        return true;
    }
    // OR: split on the first case-insensitive " OR " and recurse.
    let lower = filter.to_lowercase();
    if let Some(pos) = lower.find(" or ") {
        return matches_filter(&filter[..pos], event)
            || matches_filter(&filter[pos + 4..], event);
    }
    matches_single(filter, event)
}

fn matches_single(filter: &str, event: &CloudEvent) -> bool {
    if let Some(val) = extract_eq(filter, "type") {
        return event.r#type.eq_ignore_ascii_case(&val);
    }
    if let Some(val) = extract_eq(filter, "source") {
        return event.source.eq_ignore_ascii_case(&val);
    }
    if let Some(val) = extract_eq(filter, "id") {
        return event.id.eq_ignore_ascii_case(&val);
    }
    if let Some(val) = extract_like(filter, "type") {
        return event.r#type.to_lowercase().contains(&val.to_lowercase());
    }
    if let Some(val) = extract_like(filter, "source") {
        return event.source.to_lowercase().contains(&val.to_lowercase());
    }
    if let Some(val) = extract_like(filter, "subject") {
        return event_subject(event).to_lowercase().contains(&val.to_lowercase());
    }
    // Fallback: substring across all visible fields.
    let lf = filter.to_lowercase();
    event.r#type.to_lowercase().contains(&lf)
        || event.source.to_lowercase().contains(&lf)
        || event.id.to_lowercase().contains(&lf)
        || event_subject(event).to_lowercase().contains(&lf)
}

// Parses `field = 'value'` or `field = "value"`, returns the inner value.
fn extract_eq(filter: &str, field: &str) -> Option<String> {
    let lower = filter.to_lowercase();
    let field_lower = field.to_lowercase();
    let offset = if let Some(s) = lower.strip_prefix(&format!("{field_lower} =")) {
        filter.len() - s.len()
    } else if let Some(s) = lower.strip_prefix(&format!("{field_lower}=")) {
        filter.len() - s.len()
    } else {
        return None;
    };
    extract_quoted(&filter[offset..]).map(str::to_string)
}

// Parses `field LIKE '%pattern%'`, returns the inner pattern with % stripped.
fn extract_like(filter: &str, field: &str) -> Option<String> {
    let lower = filter.to_lowercase();
    let prefix = format!("{} like ", field.to_lowercase());
    if !lower.starts_with(&prefix) {
        return None;
    }
    let rest = filter[prefix.len()..].trim();
    let inner = extract_quoted(rest)?;
    let stripped = inner.trim_matches('%');
    if stripped.is_empty() { None } else { Some(stripped.to_string()) }
}

// Returns the content inside the outermost single or double quotes.
fn extract_quoted(s: &str) -> Option<&str> {
    let s = s.trim();
    if s.len() >= 2 {
        if s.starts_with('\'') && s.ends_with('\'') {
            return Some(&s[1..s.len() - 1]);
        }
        if s.starts_with('"') && s.ends_with('"') {
            return Some(&s[1..s.len() - 1]);
        }
    }
    None
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

fn event_time(event: &CloudEvent) -> String {
    if let Some(attr) = event.attributes.get("time") {
        if let Some(cloud_event_attribute_value::Attr::CeTimestamp(ts)) = &attr.attr {
            let dt = chrono::DateTime::from_timestamp(ts.seconds, ts.nanos as u32);
            if let Some(dt) = dt {
                return dt.format("%H:%M:%S%.3f").to_string();
            }
        }
        // fall back to the string representation if it was stored as CeString
        if let Some(cloud_event_attribute_value::Attr::CeString(s)) = &attr.attr {
            return s.chars().take(12).collect();
        }
    }
    "—".to_string()
}

fn event_subject(event: &CloudEvent) -> String {
    if let Some(attr) = event.attributes.get("subject") {
        if let Some(cloud_event_attribute_value::Attr::CeString(s)) = &attr.attr {
            return s.clone();
        }
    }
    "—".to_string()
}
