//! The Logs drawer: Overview (body, attributes with filter-in / filter-out, trace), JSON, and
//! Context (the rows just before and after, ignoring the active filter).

use dioxus::prelude::*;
use draft_api::hook::core_observability_logs_v1::use_logs_service_service;
use draft_api::proto::core_observability_logs_v1::{LogRecord, QueryLogsRequest};
use draft_ui::data::{CodeBlock, CodeLang};
use draft_ui::layout::{Drawer, DrawerBlock};
use draft_ui::ui::{Btn, BtnSize, BtnVariant, Seg, Toggle, TraceLink};
use draft_ui::util::truncate_preview;

use crate::components::SeverityTag;
use crate::data::{beaconql_string, full_time, json_object, json_string, rfc3339, row_time, severity_label, sorted_attrs};

/// Rows fetched per side of the Context tab, and per "load more".
const CONTEXT_PAGE: i32 = 10;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum Tab {
    Overview,
    Json,
    Context,
}

/// Give this a `key` derived from the record (see `stream.rs`): a new selection then remounts it,
/// which resets the tab, the wrap switch and the Context rows without any prop-change tracking.
///
/// `on_filter` is the click-to-filter hand-off: an attribute's `+` / `−` calls it with a ready
/// BeaconQL clause (`attributes["route"] = "/api"`) that the view combines into its filter.
#[component]
pub fn LogDrawer(
    record: LogRecord,
    on_close: EventHandler<()>,
    on_filter: EventHandler<String>,
    on_open_trace: EventHandler<String>,
) -> Element {
    let mut tab = use_signal(|| Tab::Overview);
    let wrap = use_signal(|| true);
    let ts = rfc3339(&record.timestamp);

    // Context: rows immediately before and after the selected one, ignoring the active filter.
    // "Before" is QueryLogs{before, descending}, "after" is QueryLogs{after, ascending}. Seeded once
    // on mount (the drawer is keyed per record); "load more" repeats the request from the newly
    // extreme row.
    let logs = use_logs_service_service();
    let mut before_req: Signal<QueryLogsRequest> = use_signal(QueryLogsRequest::default);
    let mut after_req: Signal<QueryLogsRequest> = use_signal(QueryLogsRequest::default);
    let before_result = logs.query_logs(before_req);
    let after_result = logs.query_logs(after_req);
    let mut before_rows: Signal<Vec<LogRecord>> = use_signal(Vec::new);
    let mut after_rows: Signal<Vec<LogRecord>> = use_signal(Vec::new);
    let mut before_more = use_signal(|| true);
    let mut after_more = use_signal(|| true);

    use_effect({
        let ts = ts.clone();
        move || {
            if let Some(ts) = ts.clone() {
                before_req.set(QueryLogsRequest { before: ts.clone(), limit: CONTEXT_PAGE, ascending: false, ..Default::default() });
                after_req.set(QueryLogsRequest { after: ts, limit: CONTEXT_PAGE, ascending: true, ..Default::default() });
            }
        }
    });
    // `before` arrives newest first; reversed so the tab reads oldest to newest.
    use_effect(move || {
        if let Some(Ok(resp)) = &*before_result.read() {
            let mut recs = resp.records.clone();
            before_more.set(recs.len() as i32 >= CONTEXT_PAGE);
            recs.reverse();
            recs.extend(before_rows.peek().iter().cloned());
            before_rows.set(recs);
        }
    });
    use_effect(move || {
        if let Some(Ok(resp)) = &*after_result.read() {
            let recs = resp.records.clone();
            after_more.set(recs.len() as i32 >= CONTEXT_PAGE);
            let mut merged = after_rows.peek().clone();
            merged.extend(recs);
            after_rows.set(merged);
        }
    });

    let when = full_time(&record.timestamp);
    let title = truncate_preview(&record.body, 140);
    let subtitle = if record.service_name.is_empty() { String::new() } else { record.service_name.clone() };
    let severity = record.severity_text.clone();
    let trace_id = record.trace_id.clone();
    let lead = rsx! {
        SeverityTag { text: severity }
        span { class: "d-label", "{when} UTC" }
    };

    rsx! {
        Drawer {
            label: "Log detail".to_string(),
            on_close: move |_| on_close.call(()),
            lead,
            title,
            subtitle,

            div { style: "margin-top:14px",
                Seg::<Tab> {
                    options: vec![(Tab::Overview, "Overview".to_string()), (Tab::Json, "JSON".to_string()), (Tab::Context, "Context".to_string())],
                    value: tab(),
                    on_change: move |t| tab.set(t),
                    label: "Detail view".to_string(),
                }
            }

            match tab() {
                Tab::Overview => rsx! {
                    Overview { record: record.clone(), wrap, on_filter }
                    if !trace_id.is_empty() {
                        DrawerBlock { title: "Trace".to_string(),
                            TraceLink { trace_id: trace_id.clone(), onclick: move |_| on_open_trace.call(trace_id.clone()) }
                        }
                    }
                },
                Tab::Json => rsx! {
                    div { class: "bc-block",
                        CodeBlock { text: record_json(&record), lang: CodeLang::Json, wrap: true }
                    }
                },
                Tab::Context => rsx! {
                    Context {
                        record: record.clone(),
                        before: before_rows(),
                        after: after_rows(),
                        before_more: before_more(),
                        after_more: after_more(),
                        on_load_before: move |_| {
                            if let Some(ts) = before_rows.peek().first().and_then(|r| rfc3339(&r.timestamp)) {
                                before_req.set(QueryLogsRequest { before: ts, limit: CONTEXT_PAGE, ascending: false, ..Default::default() });
                            }
                        },
                        on_load_after: move |_| {
                            if let Some(ts) = after_rows.peek().last().and_then(|r| rfc3339(&r.timestamp)) {
                                after_req.set(QueryLogsRequest { after: ts, limit: CONTEXT_PAGE, ascending: true, ..Default::default() });
                            }
                        },
                    }
                },
            }
        }
    }
}

#[component]
fn Overview(record: LogRecord, wrap: Signal<bool>, on_filter: EventHandler<String>) -> Element {
    let mut wrap = wrap;
    rsx! {
        div { class: "bc-block",
            div { class: "d-row", style: "margin-bottom:8px",
                span { class: "d-label", "Body" }
                span { class: "d-spacer" }
                Toggle { checked: wrap(), on_change: move |on| wrap.set(on), "Wrap" }
            }
            CodeBlock { text: record.body.clone(), wrap: wrap() }
        }
        AttrTable { title: "Attributes".to_string(), namespace: "attributes", attrs: sorted_attrs(&record.attributes), on_filter }
        AttrTable { title: "Resource attributes".to_string(), namespace: "resource_attributes", attrs: sorted_attrs(&record.resource_attributes), on_filter }
    }
}

/// Attribute rows with `+` (filter for this value) and `−` (filter it out).
#[component]
fn AttrTable(title: String, namespace: &'static str, attrs: Vec<(String, String)>, on_filter: EventHandler<String>) -> Element {
    if attrs.is_empty() {
        return rsx! {};
    }
    rsx! {
        DrawerBlock { title,
            for (k , v) in attrs {
                {
                    let include = format!("{namespace}[{}] = {}", beaconql_string(&k), beaconql_string(&v));
                    let exclude = format!("{namespace}[{}] != {}", beaconql_string(&k), beaconql_string(&v));
                    let label_in = format!("Filter for {k} = {v}");
                    let label_out = format!("Filter out {k} = {v}");
                    rsx! {
                        div { key: "{k}", class: "bc-attr",
                            span { "{k}" }
                            span { "{v}" }
                            span { class: "bc-attr-actions",
                                Btn {
                                    variant: BtnVariant::Ghost,
                                    size: BtnSize::Sm,
                                    icon: true,
                                    aria_label: label_in,
                                    onclick: move |_| on_filter.call(include.clone()),
                                    "+"
                                }
                                Btn {
                                    variant: BtnVariant::Ghost,
                                    size: BtnSize::Sm,
                                    icon: true,
                                    aria_label: label_out,
                                    onclick: move |_| on_filter.call(exclude.clone()),
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

#[component]
fn Context(
    record: LogRecord,
    before: Vec<LogRecord>,
    after: Vec<LogRecord>,
    before_more: bool,
    after_more: bool,
    on_load_before: EventHandler<()>,
    on_load_after: EventHandler<()>,
) -> Element {
    rsx! {
        div { class: "bc-block",
            div { class: "bc-ctx",
                if before_more {
                    Btn { variant: BtnVariant::Ghost, size: BtnSize::Sm, onclick: move |_| on_load_before.call(()), "Load 10 more before" }
                }
                for (i , r) in before.iter().enumerate() {
                    ContextRow { key: "b{i}", record: r.clone(), focus: false }
                }
                ContextRow { record, focus: true }
                for (i , r) in after.iter().enumerate() {
                    ContextRow { key: "a{i}", record: r.clone(), focus: false }
                }
                if after_more {
                    Btn { variant: BtnVariant::Ghost, size: BtnSize::Sm, onclick: move |_| on_load_after.call(()), "Load 10 more after" }
                }
            }
        }
    }
}

#[component]
fn ContextRow(record: LogRecord, focus: bool) -> Element {
    let time = row_time(&record.timestamp, chrono::Utc::now());
    let severity = severity_label(&record.severity_text);
    let class = if focus { "bc-ctx-row is-focus" } else { "bc-ctx-row" };
    rsx! {
        div { class: "{class}",
            span { "{time}" }
            span { "{severity} · {record.body}" }
        }
    }
}

/// The record as pretty-printed JSON.
fn record_json(record: &LogRecord) -> String {
    let timestamp = rfc3339(&record.timestamp).unwrap_or_default();
    format!(
        "{{\n  \"timestamp\": {},\n  \"severity_text\": {},\n  \"severity_number\": {},\n  \"service_name\": {},\n  \"trace_id\": {},\n  \"span_id\": {},\n  \"body\": {},\n  \"attributes\": {},\n  \"resource_attributes\": {}\n}}",
        json_string(&timestamp),
        json_string(&record.severity_text),
        record.severity_number,
        json_string(&record.service_name),
        json_string(&record.trace_id),
        json_string(&record.span_id),
        json_string(&record.body),
        json_object(&sorted_attrs(&record.attributes)),
        json_object(&sorted_attrs(&record.resource_attributes)),
    )
}
