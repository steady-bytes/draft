use chrono::{DateTime, Utc};
use dioxus::prelude::*;

use draft_api::hook::core_observability_logs_v1::use_logs_service_service;
use draft_api::proto::core_observability_logs_v1::{LogRecord, QueryLogsRequest};

use crate::components::{severity_label, severity_stripe_color, TracePill};

const CONTEXT_PAGE: i32 = 10;

#[derive(Clone, Copy, PartialEq)]
enum DetailTab {
    Overview,
    Json,
    Context,
}

/// LogDetailDrawer is the Logs view's log detail panel (Phase 15): a
/// right-side drawer with Overview / JSON / Context tabs, opened by clicking
/// a row. Give it a `key` derived from the selected record's identity when
/// rendering it (see stream.rs) — that makes Dioxus remount it fresh on every
/// new row selection, which is what resets `tab`/`wrap`/the Context
/// before-and-after state below without hand-rolled prop-change tracking.
///
/// `on_filter` is the click-to-filter hand-off: the Overview tab's attribute
/// tables call it with a ready-to-use BeaconQL clause (e.g.
/// `attributes["route"] = "/api/logs"`) when a value's `+`/`−` action is
/// clicked; stream.rs AND-combines it into the active filter and re-runs.
#[component]
pub fn LogDetailDrawer(record: LogRecord, on_close: EventHandler<()>, on_filter: EventHandler<String>) -> Element {
    let mut tab = use_signal(|| DetailTab::Overview);
    let wrap = use_signal(|| true);

    let ts = record_rfc3339(&record);

    // Context tab: 10 rows immediately before/after the selected record,
    // ignoring the active BeaconQL filter (matches SigNoz's Context tab) —
    // "before" via QueryLogs{before: ts, ascending: false} (Phase 12's
    // addition), "after" via QueryLogs{after: ts, ascending: true}, both
    // independent of Stream's own live tail. Seeded once on mount (this
    // component instance is keyed per-record — see the doc comment above),
    // "load more" re-runs the same request with the newly-extreme edge as
    // the new cursor.
    let logs_service = use_logs_service_service();
    let mut before_req: Signal<QueryLogsRequest> = use_signal(QueryLogsRequest::default);
    let mut after_req: Signal<QueryLogsRequest> = use_signal(QueryLogsRequest::default);
    let before_result = logs_service.query_logs(before_req);
    let after_result = logs_service.query_logs(after_req);

    let mut before_rows: Signal<Vec<LogRecord>> = use_signal(Vec::new);
    let mut after_rows: Signal<Vec<LogRecord>> = use_signal(Vec::new);
    let mut before_has_more = use_signal(|| true);
    let mut after_has_more = use_signal(|| true);

    use_effect({
        let ts = ts.clone();
        move || {
            if let Some(ts) = ts.clone() {
                before_req.set(QueryLogsRequest {
                    before: ts.clone(),
                    limit: CONTEXT_PAGE,
                    ascending: false,
                    ..Default::default()
                });
                after_req.set(QueryLogsRequest {
                    after: ts,
                    limit: CONTEXT_PAGE,
                    ascending: true,
                    ..Default::default()
                });
            }
        }
    });

    // QueryLogs returns descending for `before`; reverse so before_rows reads
    // oldest-first, matching the chronological top-to-bottom Context layout.
    use_effect(move || {
        if let Some(Ok(resp)) = &*before_result.read() {
            let mut recs = resp.records.clone();
            before_has_more.set(recs.len() as i32 >= CONTEXT_PAGE);
            recs.reverse();
            let mut merged = recs;
            merged.extend(before_rows.peek().iter().cloned());
            before_rows.set(merged);
        }
    });
    use_effect(move || {
        if let Some(Ok(resp)) = &*after_result.read() {
            let recs = resp.records.clone();
            after_has_more.set(recs.len() as i32 >= CONTEXT_PAGE);
            let mut merged = after_rows.peek().clone();
            merged.extend(recs);
            after_rows.set(merged);
        }
    });

    let severity = record.severity_text.clone();
    let stripe = severity_stripe_color(&severity);
    let severity_text = severity_label(&severity).to_string();
    let time_display = record_time(&record);
    let trace_id = record.trace_id.clone();

    rsx! {
        div { class: "w-[420px] shrink-0 border-l border-base-300 flex flex-col h-full overflow-hidden bg-base-100",
            div { class: "flex items-center justify-between px-3 py-2 border-b border-base-300",
                div { class: "flex items-center gap-2 min-w-0",
                    div {
                        style: "width:2px; height:12px; border-radius:1px; background:{stripe}; opacity:0.55; flex-shrink:0;",
                    }
                    span { class: "text-xs text-base-content/70", "{severity_text}" }
                    span { class: "font-mono text-xs text-base-content/40 whitespace-nowrap", "{time_display} UTC" }
                }
                button {
                    class: "btn btn-xs btn-ghost",
                    onclick: move |_| on_close.call(()),
                    "✕"
                }
            }

            div { class: "flex gap-1 px-3 pt-2 border-b border-base-300",
                TabButton { label: "Overview", active: tab() == DetailTab::Overview, onclick: move |_| tab.set(DetailTab::Overview) }
                TabButton { label: "JSON", active: tab() == DetailTab::Json, onclick: move |_| tab.set(DetailTab::Json) }
                TabButton { label: "Context", active: tab() == DetailTab::Context, onclick: move |_| tab.set(DetailTab::Context) }
            }

            div { class: "flex-1 overflow-auto p-3",
                match tab() {
                    DetailTab::Overview => rsx! {
                        OverviewTab { record: record.clone(), trace_id, wrap, on_filter }
                    },
                    DetailTab::Json => rsx! {
                        JsonTab { record: record.clone() }
                    },
                    DetailTab::Context => rsx! {
                        ContextTab {
                            record: record.clone(),
                            before_rows: before_rows(),
                            after_rows: after_rows(),
                            before_has_more: before_has_more(),
                            after_has_more: after_has_more(),
                            on_load_before: move |_| {
                                if let Some(oldest) = before_rows.peek().first() {
                                    if let Some(ts) = record_rfc3339(oldest) {
                                        before_req.set(QueryLogsRequest {
                                            before: ts,
                                            limit: CONTEXT_PAGE,
                                            ascending: false,
                                            ..Default::default()
                                        });
                                    }
                                }
                            },
                            on_load_after: move |_| {
                                if let Some(newest) = after_rows.peek().last() {
                                    if let Some(ts) = record_rfc3339(newest) {
                                        after_req.set(QueryLogsRequest {
                                            after: ts,
                                            limit: CONTEXT_PAGE,
                                            ascending: true,
                                            ..Default::default()
                                        });
                                    }
                                }
                            },
                        }
                    },
                }
            }
        }
    }
}

#[component]
fn TabButton(label: String, active: bool, onclick: EventHandler<()>) -> Element {
    let class = if active {
        "px-2 py-1 text-xs font-semibold border-b-2 border-primary text-base-content"
    } else {
        "px-2 py-1 text-xs text-base-content/50 hover:text-base-content border-b-2 border-transparent"
    };
    rsx! {
        button { class: "{class}", onclick: move |_| onclick.call(()), "{label}" }
    }
}

#[component]
fn OverviewTab(record: LogRecord, trace_id: String, wrap: Signal<bool>, on_filter: EventHandler<String>) -> Element {
    let body = record.body.clone();
    let body_class = if wrap() {
        "font-mono text-xs bg-base-200 border border-base-300 rounded p-2 whitespace-pre-wrap break-words"
    } else {
        "font-mono text-xs bg-base-200 border border-base-300 rounded p-2 whitespace-nowrap overflow-x-auto"
    };

    let mut attrs: Vec<(String, String)> = record.attributes.clone().into_iter().collect();
    attrs.sort();
    let mut resource_attrs: Vec<(String, String)> = record.resource_attributes.clone().into_iter().collect();
    resource_attrs.sort();

    rsx! {
        div { class: "flex flex-col gap-4",
            div {
                div { class: "flex items-center justify-between mb-1",
                    span { class: "text-[10px] uppercase tracking-wide text-base-content/40", "Body" }
                    button {
                        class: "text-[10px] text-base-content/40 hover:text-base-content",
                        onclick: move |_| wrap.set(!wrap()),
                        if wrap() { "wrap: on" } else { "wrap: off" }
                    }
                }
                div { class: "{body_class}", "{body}" }
            }

            AttributeTable { title: "Attributes", namespace: "attributes", attrs, on_filter }
            AttributeTable { title: "Resource Attributes", namespace: "resource_attributes", attrs: resource_attrs, on_filter }

            if !trace_id.is_empty() {
                div {
                    div { class: "text-[10px] uppercase tracking-wide text-base-content/40 mb-1", "Trace" }
                    div { class: "flex items-center justify-between",
                        TracePill { trace_id: trace_id.clone() }
                    }
                }
            }
        }
    }
}

#[component]
fn AttributeTable(
    title: String,
    namespace: &'static str,
    attrs: Vec<(String, String)>,
    on_filter: EventHandler<String>,
) -> Element {
    if attrs.is_empty() {
        return rsx! {};
    }
    rsx! {
        div {
            div { class: "text-[10px] uppercase tracking-wide text-base-content/40 mb-1", "{title}" }
            div { class: "border border-base-300 rounded overflow-hidden",
                for (k , v) in attrs {
                    {
                        let include_clause = format!("{namespace}[{}] = {}", beaconql_string(&k), beaconql_string(&v));
                        let exclude_clause = format!("{namespace}[{}] != {}", beaconql_string(&k), beaconql_string(&v));
                        rsx! {
                            div { class: "flex justify-between gap-2 px-2 py-1 text-xs font-mono border-b border-base-300 last:border-b-0 items-center",
                                span { class: "text-base-content/50 shrink-0", "{k}" }
                                span { class: "text-right break-all flex-1", "{v}" }
                                div { class: "flex gap-1 shrink-0",
                                    button {
                                        class: "text-[10px] text-base-content/30 hover:text-info leading-none",
                                        title: "Filter for this value",
                                        onclick: move |_| on_filter.call(include_clause.clone()),
                                        "+"
                                    }
                                    button {
                                        class: "text-[10px] text-base-content/30 hover:text-error leading-none",
                                        title: "Filter out this value",
                                        onclick: move |_| on_filter.call(exclude_clause.clone()),
                                        "−"
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

/// beaconql_string renders a BeaconQL string literal for an arbitrary Go map
/// key/value — backslash-then-quote escaping, matching what
/// query/beaconql.go's lexString unescapes on the way back in (`\` followed
/// by any character is that character literally).
fn beaconql_string(s: &str) -> String {
    let mut out = String::with_capacity(s.len() + 2);
    out.push('"');
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            c => out.push(c),
        }
    }
    out.push('"');
    out
}

#[component]
fn JsonTab(record: LogRecord) -> Element {
    let json = record_json(&record);
    rsx! {
        pre {
            class: "font-mono text-xs bg-base-200 border border-base-300 rounded p-2 whitespace-pre-wrap break-words",
            "{json}"
        }
    }
}

#[component]
fn ContextTab(
    record: LogRecord,
    before_rows: Vec<LogRecord>,
    after_rows: Vec<LogRecord>,
    before_has_more: bool,
    after_has_more: bool,
    on_load_before: EventHandler<()>,
    on_load_after: EventHandler<()>,
) -> Element {
    rsx! {
        div { class: "flex flex-col gap-1",
            if before_has_more {
                button {
                    class: "text-[10px] text-center text-base-content/40 hover:text-base-content border border-dashed border-base-300 rounded py-1",
                    onclick: move |_| on_load_before.call(()),
                    "load 10 more before"
                }
            }
            for r in before_rows.iter() {
                ContextRow { record: r.clone(), focus: false }
            }
            ContextRow { record, focus: true }
            for r in after_rows.iter() {
                ContextRow { record: r.clone(), focus: false }
            }
            if after_has_more {
                button {
                    class: "text-[10px] text-center text-base-content/40 hover:text-base-content border border-dashed border-base-300 rounded py-1",
                    onclick: move |_| on_load_after.call(()),
                    "load 10 more after"
                }
            }
        }
    }
}

#[component]
fn ContextRow(record: LogRecord, focus: bool) -> Element {
    let time = record_time(&record);
    let severity = severity_label(&record.severity_text).to_string();
    let body = record.body.clone();
    let class = if focus {
        "flex justify-between gap-2 px-2 py-1 text-xs font-mono rounded bg-primary/10 border border-primary/40"
    } else {
        "flex justify-between gap-2 px-2 py-1 text-xs font-mono text-base-content/50"
    };
    rsx! {
        div { class: "{class}",
            span { class: "shrink-0", "{time}" }
            span { class: "truncate", "{severity} · {body}" }
        }
    }
}

fn record_time(record: &LogRecord) -> String {
    match &record.timestamp {
        Some(ts) => match DateTime::from_timestamp(ts.seconds, ts.nanos as u32) {
            Some(dt) => dt.format("%Y-%m-%d %H:%M:%S%.3f").to_string(),
            None => "—".to_string(),
        },
        None => "—".to_string(),
    }
}

fn record_rfc3339(record: &LogRecord) -> Option<String> {
    let ts = record.timestamp.as_ref()?;
    let dt: DateTime<Utc> = DateTime::from_timestamp(ts.seconds, ts.nanos as u32)?;
    Some(dt.to_rfc3339())
}

/// record_json hand-formats the record as pretty-printed JSON — no `serde_json`
/// dependency purely for a debug view; `LogRecord`'s prost-generated struct
/// doesn't derive `Serialize`, and this is a small, fixed set of fields.
fn record_json(record: &LogRecord) -> String {
    let mut attrs: Vec<(String, String)> = record.attributes.clone().into_iter().collect();
    attrs.sort();
    let mut resource_attrs: Vec<(String, String)> = record.resource_attributes.clone().into_iter().collect();
    resource_attrs.sort();

    let timestamp = record_rfc3339(record).unwrap_or_default();
    let attrs_json = json_object(&attrs);
    let resource_attrs_json = json_object(&resource_attrs);

    format!(
        "{{\n  \"timestamp\": {},\n  \"severity_text\": {},\n  \"severity_number\": {},\n  \"service_name\": {},\n  \"trace_id\": {},\n  \"span_id\": {},\n  \"body\": {},\n  \"attributes\": {},\n  \"resource_attributes\": {}\n}}",
        json_string(&timestamp),
        json_string(&record.severity_text),
        record.severity_number,
        json_string(&record.service_name),
        json_string(&record.trace_id),
        json_string(&record.span_id),
        json_string(&record.body),
        attrs_json,
        resource_attrs_json,
    )
}

fn json_object(pairs: &[(String, String)]) -> String {
    if pairs.is_empty() {
        return "{}".to_string();
    }
    let body: Vec<String> = pairs
        .iter()
        .map(|(k, v)| format!("    {}: {}", json_string(k), json_string(v)))
        .collect();
    format!("{{\n{}\n  }}", body.join(",\n"))
}

fn json_string(s: &str) -> String {
    let mut out = String::with_capacity(s.len() + 2);
    out.push('"');
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if (c as u32) < 0x20 => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push(c),
        }
    }
    out.push('"');
    out
}
