use dioxus::prelude::*;
use dioxus_core::Task;
use tonic_web_wasm_client::Client as WasmClient;

use draft_api::proto::core_message_broker_actors_v1::{
    cloud_event::{cloud_event_attribute_value, Data as CloudEventData},
    query_client::QueryClient,
    CloudEvent, OrderDirection, QueryRequest,
};

use crate::components::{CesqlBar, QueryBuilder, SortDir, TypeBadge, WaveLoader};

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
    let mut streaming = use_signal(|| false);
    let mut events: Signal<Vec<CloudEvent>> = use_signal(Vec::new);
    let mut query_results: Signal<Vec<CloudEvent>> = use_signal(Vec::new);
    let mut sort_dir: Signal<SortDir> = use_signal(|| SortDir::Desc);
    let mut status: Signal<StreamStatus> = use_signal(|| StreamStatus::Disconnected);
    let mut querying = use_signal(|| true);
    // Holds the active stream task so it can be cancelled when the toggle turns off.
    let mut stream_task: Signal<Option<Task>> = use_signal(|| None);

    // Spawns a QueryStream task and stores the handle. Cancels any prior task first.
    let start_stream = use_callback(move |_: ()| {
        if let Some(t) = stream_task.write().take() {
            t.cancel();
        }
        events.set(Vec::new());
        status.set(StreamStatus::Connecting);
        let host = crate::CATALYST_DOMAIN.clone();
        let task = spawn(async move {
            let mut client = QueryClient::new(WasmClient::new(host));
            let stream_response = client
                .query_stream(QueryRequest {
                    expression: None,
                    limit: 0,
                    after: String::new(),
                    order_by: OrderDirection::Asc as i32,
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
        });
        stream_task.set(Some(task));
    });

    let run_query = use_callback(move |_: ()| {
        querying.set(true);
        let host = crate::CATALYST_DOMAIN.clone();
        // peek() reads the current value without creating a reactive subscription,
        // preventing use_effect from re-firing whenever sort_dir changes.
        let order_by = match sort_dir.peek().clone() {
            SortDir::Asc  => OrderDirection::Asc  as i32,
            SortDir::Desc => OrderDirection::Desc as i32,
        };
        spawn(async move {
            let mut client = QueryClient::new(WasmClient::new(host));
            if let Ok(resp) = client
                .query(QueryRequest {
                    expression: None,
                    limit: 0,
                    after: String::new(),
                    order_by,
                })
                .await
            {
                query_results.set(resp.into_inner().events);
            }
            querying.set(false);
        });
    });

    // On first render: default is query mode, fetch historical events immediately.
    use_effect(move || {
        run_query.call(());
    });

    let stream_events = events.read();
    let static_events = query_results.read();
    let all_events: &Vec<CloudEvent> = if streaming() { &stream_events } else { &static_events };
    let filter = expression.read();

    let filtered: Vec<&CloudEvent> = all_events
        .iter()
        .filter(|e| matches_filter(&filter, e))
        .collect();

    rsx! {
        div { class: "p-4 flex flex-col gap-3 h-screen",

            div { class: "flex items-center gap-2",
                label { class: "flex items-center gap-2 cursor-pointer select-none shrink-0",
                    input {
                        r#type: "checkbox",
                        class: "toggle toggle-xs toggle-primary",
                        checked: streaming(),
                        onchange: move |_| {
                            let was_streaming = streaming();
                            streaming.toggle();
                            if was_streaming {
                                // turned off — cancel the stream task and fetch a snapshot
                                if let Some(t) = stream_task.write().take() {
                                    t.cancel();
                                }
                                events.set(Vec::new());
                                run_query.call(());
                            } else {
                                // turned on — discard static snapshot and open the stream
                                query_results.set(Vec::new());
                                start_stream.call(());
                            }
                        },
                    }
                    span { class: "text-xs text-base-content/50", "Stream" }
                }
                CesqlBar {
                    expression,
                    on_run: move |_| {
                        if !streaming() { run_query.call(()); }
                    },
                    on_clear: move |_| {
                        expression.set(String::new());
                        query_results.set(Vec::new());
                    },
                }
            }

            QueryBuilder {
                has_expression: !expression.read().is_empty(),
                sort_dir: sort_dir(),
                on_sort_change: move |dir: SortDir| {
                    sort_dir.set(dir);
                    if !streaming() { run_query.call(()); }
                },
                on_add: move |fragment: String| {
                    let current = expression.read().clone();
                    let new_expr = if current.trim().is_empty() {
                        fragment
                    } else {
                        format!("{current} {fragment}")
                    };
                    expression.set(new_expr);
                },
            }

            div { class: "flex items-center gap-2",
                match (streaming(), status()) {
                    (true, StreamStatus::Disconnected) => rsx! {
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

            div { class: "flex-1 overflow-auto min-h-0",
                if all_events.is_empty() && (!streaming() && querying() || streaming() && status() == StreamStatus::Connecting) {
                    div { style: "position:fixed;top:50%;left:0;right:0;width:fit-content;margin-inline:auto;",
                        WaveLoader { width: 80, height: 28 }
                    }
                } else {
                table { class: "table table-xs",
                    thead {
                        tr {
                            th { "TIME (UTC)" }
                            th { "FORWARDED AT" }
                            th { "TYPE" }
                            th { "SOURCE" }
                            th { "SUBJECT" }
                            th { "ID" }
                            th { "BODY" }
                        }
                    }
                    tbody {
                        if filtered.is_empty() {
                            tr {
                                td {
                                    colspan: "7",
                                    class: "text-center text-base-content/40 py-6",
                                    if all_events.is_empty() { "Waiting for events…" } else { "No events match the filter." }
                                }
                            }
                        }
                        for event in filtered.iter() {
                            {
                                let time         = event_time(event);
                                let forwarded_at = event_forwarded_at(event);
                                let subject      = event_subject(event);
                                let etype        = event.r#type.clone();
                                let source       = event.source.clone();
                                let id           = event.id.clone();
                                let body         = event_text_data(event).to_string();
                                rsx! {
                                    tr { class: "hover:bg-base-300",
                                        td { class: "font-mono text-xs text-base-content/60 whitespace-nowrap", "{time}" }
                                        td { class: "font-mono text-xs text-base-content/60 whitespace-nowrap", "{forwarded_at}" }
                                        td { TypeBadge { event_type: etype } }
                                        td { class: "text-xs", "{source}" }
                                        td { class: "font-mono text-xs", "{subject}" }
                                        td { class: "font-mono text-xs text-base-content/40", "{id}" }
                                        td {
                                            class: "font-mono text-xs text-base-content/60",
                                            style: "max-width:280px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;",
                                            title: "{body}",
                                            "{body}"
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
    }
}

// ─── Filter ───────────────────────────────────────────────────────────────────

// Evaluates a CESQL-subset expression against a CloudEvent.
// Supported forms:
//   field = 'value'              exact match (case-insensitive)
//   field LIKE '%pattern%'       contains (% wildcards stripped)
//   body.field = 'value'         JSON body field exact match
//   body.field LIKE '%pattern%'  JSON body field contains
//   A OR B                       either branch matches
//   A AND B                      both branches must match
//   <text>                       fallback: substring across all fields including body
fn matches_filter(filter: &str, event: &CloudEvent) -> bool {
    let filter = filter.trim();
    if filter.is_empty() {
        return true;
    }
    let lower = filter.to_lowercase();
    // OR has lower precedence — split first so AND binds tighter.
    if let Some(pos) = lower.find(" or ") {
        return matches_filter(&filter[..pos], event)
            || matches_filter(&filter[pos + 4..], event);
    }
    if let Some(pos) = lower.find(" and ") {
        return matches_filter(&filter[..pos], event)
            && matches_filter(&filter[pos + 5..], event);
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
    // body.field = 'value'
    if let Some((field, val)) = extract_body_eq(filter) {
        let body: serde_json::Value =
            serde_json::from_str(event_text_data(event)).unwrap_or_default();
        return body
            .get(&field)
            .map(|v| match v {
                serde_json::Value::String(s) => s.eq_ignore_ascii_case(&val),
                other => other.to_string().eq_ignore_ascii_case(&val),
            })
            .unwrap_or(false);
    }
    // body.field LIKE '%pattern%'
    if let Some((field, pat)) = extract_body_like(filter) {
        let body: serde_json::Value =
            serde_json::from_str(event_text_data(event)).unwrap_or_default();
        return body
            .get(&field)
            .map(|v| match v {
                serde_json::Value::String(s) => s.to_lowercase().contains(&pat.to_lowercase()),
                other => other.to_string().to_lowercase().contains(&pat.to_lowercase()),
            })
            .unwrap_or(false);
    }
    // Fallback: substring across all visible fields including the JSON body.
    let lf = filter.to_lowercase();
    event.r#type.to_lowercase().contains(&lf)
        || event.source.to_lowercase().contains(&lf)
        || event.id.to_lowercase().contains(&lf)
        || event_subject(event).to_lowercase().contains(&lf)
        || event_text_data(event).to_lowercase().contains(&lf)
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

// Parses `body.field = 'value'`, returns (field_name, value).
// Field names from protojson are camelCase (e.g. "modelName", "userId").
fn extract_body_eq(filter: &str) -> Option<(String, String)> {
    let lower = filter.to_lowercase();
    if !lower.starts_with("body.") { return None; }
    let rest = &filter[5..]; // after "body."
    let rest_lower = rest.to_lowercase();
    let eq_pos = rest_lower.find('=')?;
    // Reject if this is actually a LIKE expression.
    if rest_lower[..eq_pos].contains("like") { return None; }
    let field_name = rest[..eq_pos].trim().to_string();
    if field_name.is_empty() { return None; }
    let after_eq = rest[eq_pos + 1..].trim();
    let val = extract_quoted(after_eq)?.to_string();
    Some((field_name, val))
}

// Parses `body.field LIKE '%pattern%'`, returns (field_name, pattern_without_%).
fn extract_body_like(filter: &str) -> Option<(String, String)> {
    let lower = filter.to_lowercase();
    if !lower.starts_with("body.") { return None; }
    let rest = &filter[5..];
    let rest_lower = rest.to_lowercase();
    let like_pos = rest_lower.find(" like ")?;
    let field_name = rest[..like_pos].trim().to_string();
    if field_name.is_empty() { return None; }
    let after_like = rest[like_pos + 6..].trim();
    let inner = extract_quoted(after_like)?;
    let stripped = inner.trim_matches('%');
    if stripped.is_empty() { return None; }
    Some((field_name, stripped.to_string()))
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
                return dt.format("%Y-%m-%d %H:%M:%S").to_string();
            }
        }
        if let Some(cloud_event_attribute_value::Attr::CeString(s)) = &attr.attr {
            return s.clone();
        }
    }
    "—".to_string()
}

fn event_forwarded_at(event: &CloudEvent) -> String {
    if let Some(attr) = event.attributes.get("forwarded_at") {
        if let Some(cloud_event_attribute_value::Attr::CeTimestamp(ts)) = &attr.attr {
            let dt = chrono::DateTime::from_timestamp(ts.seconds, ts.nanos as u32);
            if let Some(dt) = dt {
                return dt.format("%Y-%m-%d %H:%M:%S").to_string();
            }
        }
    }
    "—".to_string()
}

fn event_text_data(event: &CloudEvent) -> &str {
    match &event.data {
        Some(CloudEventData::TextData(s)) => s.as_str(),
        _ => "",
    }
}

fn event_subject(event: &CloudEvent) -> String {
    if let Some(attr) = event.attributes.get("subject") {
        if let Some(cloud_event_attribute_value::Attr::CeString(s)) = &attr.attr {
            return s.clone();
        }
    }
    "—".to_string()
}
