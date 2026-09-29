//! Logs: a live tail backed by `StreamLogs` while the range trails "now" and streaming is on, or a
//! one-shot bounded `QueryLogs` snapshot when the range is fixed or the stream is paused. A
//! BeaconQL bar filters either. Running a new filter re-issues the active query, continuing from
//! the last-seen cursor in live mode rather than rescanning from the start.

use chrono::Utc;
use dioxus::prelude::*;
use dioxus_core::Task;
use draft_api::hook::core_observability_logs_v1::use_logs_service_service;
use draft_api::proto::core_observability_logs_v1::{
    logs_service_client::LogsServiceClient, LogRecord, QueryLogsRequest, StreamLogsRequest,
};
use draft_ui::layout::{PageHead, Split};
use draft_ui::query::{Connector, GrammarId, QueryBar, QueryFilters};
use draft_ui::shell::{use_page_chrome, BarItem, Chrome, ChromeStatus};
use draft_ui::ui::{Empty, Svc, Toggle, TraceLink};
use draft_ui::{StatusKind, Tone};
use tonic_web_wasm_client::Client as WasmClient;

use super::log_detail::LogDrawer;
use crate::components::{use_open_trace, Attrs, SeverityTag, VolumePanel};
use crate::data::{row_time, sorted_attrs, ts_nanos, Severity};
use crate::range::{TimeRange, TimeRangePicker};

/// Lines kept in the table, so a long tail does not grow without bound.
const MAX_LINES: usize = 1_000;

/// The time-range presets, in minutes.
const PRESETS: [i64; 4] = [15, 60, 360, 1440];

/// How many attributes a row's cell shows before the drawer takes over.
const ROW_ATTRS: usize = 4;

#[derive(Clone, Copy, PartialEq)]
enum StreamStatus {
    Connecting,
    Connected,
    /// A snapshot: a fixed range or a paused tail. Nothing is arriving.
    Snapshot,
    Disconnected,
}

impl StreamStatus {
    fn chrome(self) -> ChromeStatus {
        match self {
            StreamStatus::Connecting => ChromeStatus::new(StatusKind::Warn, "Connecting"),
            StreamStatus::Connected => ChromeStatus::live("Streaming"),
            StreamStatus::Snapshot => ChromeStatus::new(StatusKind::Idle, "Snapshot"),
            StreamStatus::Disconnected => ChromeStatus::new(StatusKind::Err, "Disconnected"),
        }
    }
}

#[component]
pub fn Stream() -> Element {
    let mut expression = use_signal(String::new);
    let mut lines: Signal<Vec<LogRecord>> = use_signal(Vec::new);
    let mut cursor: Signal<String> = use_signal(String::new);
    let mut status: Signal<StreamStatus> = use_signal(|| StreamStatus::Snapshot);
    let mut error: Signal<Option<String>> = use_signal(|| None);
    // The active stream task, held so a new filter can cancel it.
    let mut stream_task: Signal<Option<Task>> = use_signal(|| None);
    let mut range: Signal<TimeRange> = use_signal(|| TimeRange::minutes(15));
    let mut selected: Signal<Option<LogRecord>> = use_signal(|| None);
    // Pausing keeps the time window but freezes the tail on a one-shot snapshot of it. Only
    // meaningful for a trailing range; a custom range is already a fixed query.
    let mut streaming = use_signal(|| true);

    // One-shot snapshots go through QueryLogs via the generated hook. The resource also fires once,
    // needlessly, on mount with an empty request; the effect below only acts on it while the
    // active query is a snapshot.
    let logs_service = use_logs_service_service();
    let mut query_req: Signal<QueryLogsRequest> = use_signal(QueryLogsRequest::default);
    let query_result = logs_service.query_logs(query_req);

    // (Re)opens the active query with the current filter. `reset` reseeds the live cursor from the
    // start of the window (first load, clear, a new range); without it a live tail continues from
    // the last row seen.
    let start_stream = use_callback(move |reset: bool| {
        if let Some(t) = stream_task.write().take() {
            t.cancel();
        }
        lines.set(Vec::new());
        error.set(None);

        let now = Utc::now();
        let (window_start, window_end) = range.peek().bounds(now);
        let live = range.peek().is_trailing() && *streaming.peek();

        if !live {
            status.set(StreamStatus::Snapshot);
            query_req.set(QueryLogsRequest {
                filter: expression.peek().clone(),
                limit: 0,
                after: window_start.to_rfc3339(),
                before: window_end.unwrap_or(now).to_rfc3339(),
                ascending: false,
            });
            return;
        }

        if reset {
            cursor.set(String::new());
        }
        status.set(StreamStatus::Connecting);
        let host = crate::API_DOMAIN.clone();
        let filter = expression.peek().clone();
        let after = if reset { window_start.to_rfc3339() } else { cursor.peek().clone() };

        let task = spawn(async move {
            let mut client = LogsServiceClient::new(WasmClient::new(host));
            let resp = client.stream_logs(StreamLogsRequest { filter, limit: 200, after }).await;
            let mut stream = match resp {
                Ok(r) => {
                    status.set(StreamStatus::Connected);
                    r.into_inner()
                }
                Err(e) => {
                    status.set(StreamStatus::Disconnected);
                    error.set(Some(e.message().to_string()));
                    return;
                }
            };
            loop {
                match stream.message().await {
                    Ok(Some(msg)) => {
                        let Some(record) = msg.record else { continue };
                        if let Some(dt) = record.timestamp.as_ref().and_then(|ts| chrono::DateTime::from_timestamp(ts.seconds, ts.nanos as u32)) {
                            cursor.set(dt.to_rfc3339());
                        }
                        let mut buf = lines.write();
                        // Re-issuing StreamLogs with a new `after` cursor can replay the most
                        // recent row: the cursor round-trips through an RFC 3339 string on both
                        // ends and ClickHouse's DateTime64(9) can disagree with Go's time.Time by
                        // a rounding step, turning the server's strict `>` back into a match.
                        // Guard the visible symptom (an adjacent duplicate) rather than chase
                        // sub-nanosecond precision across three languages.
                        let duplicate = buf.last().is_some_and(|last| last.timestamp == record.timestamp && last.body == record.body);
                        if !duplicate {
                            if buf.len() >= MAX_LINES {
                                buf.remove(0);
                            }
                            buf.push(record);
                        }
                    }
                    Ok(None) => {
                        status.set(StreamStatus::Disconnected);
                        break;
                    }
                    Err(e) => {
                        status.set(StreamStatus::Disconnected);
                        error.set(Some(e.message().to_string()));
                        break;
                    }
                }
            }
        });
        stream_task.set(Some(task));
    });

    // Seed `lines` from a snapshot. QueryLogs returns newest first; `lines` is kept oldest first
    // whichever path filled it, and the table draws it reversed.
    use_effect(move || {
        if range.read().is_trailing() && streaming() {
            return;
        }
        match &*query_result.read() {
            Some(Ok(resp)) => {
                let mut records = resp.records.clone();
                records.reverse();
                lines.set(records);
                status.set(StreamStatus::Snapshot);
                error.set(None);
            }
            Some(Err(e)) => {
                status.set(StreamStatus::Disconnected);
                error.set(Some(e.message().to_string()));
            }
            None => {}
        }
    });

    // First render: pick up a filter handed off from another view (Wide events' "Logs for trace"),
    // then start tailing.
    use_effect(move || {
        if let Some(filter) = crate::PENDING_LOG_FILTER.write().take() {
            expression.set(filter);
        }
        start_stream.call(true);
    });

    let open_trace = use_open_trace();

    let displayed = lines();
    let now = Utc::now();
    use_page_chrome(move || {
        let n = lines.read().len();
        Chrome {
            crumbs: vec!["Beacon".into(), "Signals".into(), "Logs".into()],
            status: Some(status().chrome()),
            left: vec![BarItem::kv("Lines", n.to_string())],
            right: vec![BarItem::kv("Streaming", if streaming() { "on" } else { "off" })],
        }
    });

    let points: Vec<(i64, usize)> = displayed
        .iter()
        .filter_map(|r| {
            let class = match Severity::of(&r.severity_text) {
                Severity::Other => 0,
                Severity::Info => 1,
                Severity::Warn => 2,
                Severity::Error => 3,
            };
            ts_nanos(&r.timestamp).map(|ns| (ns, class))
        })
        .collect();
    let summary = format!("{} lines · {}", displayed.len(), range().label());

    let drawer = selected().map(|record| {
        rsx! {
            LogDrawer {
                key: "{detail_key(&record)}",
                record,
                on_close: move |_| selected.set(None),
                on_open_trace: move |id: String| open_trace.call(id),
                on_filter: move |clause: String| {
                    // Parenthesise the existing filter first: AND binds tighter than OR, so a bare
                    // `AND clause` would only scope the last OR operand.
                    let next = draft_ui::query::GrammarId::BeaconLogs.grammar().combine(&expression.peek(), Connector::And, &clause);
                    expression.set(next);
                    start_stream.call(false);
                },
            }
        }
    });

    rsx! {
        Split { flush: true, drawer,
            PageHead { inline: true, title: "Logs".to_string(),
                span { class: "d-label", "{summary}" }
                span { class: "d-spacer" }
                Toggle {
                    checked: streaming(),
                    disabled: !range().is_trailing(),
                    on_change: move |on| {
                        streaming.set(on);
                        start_stream.call(true);
                    },
                    "Stream"
                }
                TimeRangePicker {
                    value: range(),
                    presets: PRESETS.to_vec(),
                    on_change: move |r| {
                        range.set(r);
                        start_stream.call(true);
                    },
                }
            }

            QueryBar {
                grammar: GrammarId::BeaconLogs,
                value: expression,
                on_run: move |_| start_stream.call(false),
                on_clear: move |_| {
                    expression.set(String::new());
                    start_stream.call(true);
                },
                error: error(),
            }
            QueryFilters { grammar: GrammarId::BeaconLogs, expression, on_run: move |_| start_stream.call(false) }

            VolumePanel {
                title: "Volume by severity".to_string(),
                label: "Log volume over time, stacked by severity".to_string(),
                points,
                names: vec!["other", "info", "warn", "error"],
                tones: vec![Tone::Quiet, Tone::Ca, Tone::Warn, Tone::Err],
            }

            if displayed.is_empty() {
                Empty { title: "Waiting for logs".to_string(), "Lines appear here as they are written." }
            } else {
                div { class: "d-panel d-table-wrap",
                    table { class: "d-table bc-table",
                        thead {
                            tr {
                                th { "Time (UTC)" }
                                th { "Sev" }
                                th { "Service" }
                                th { "Trace" }
                                th { "Body" }
                                th { "Attributes" }
                            }
                        }
                        tbody {
                            for record in displayed.iter().rev() {
                                {
                                    let record_for_click = record.clone();
                                    let record_for_key = record.clone();
                                    let key = detail_key(record);
                                    let is_selected = selected.peek().as_ref().map(detail_key).as_deref() == Some(key.as_str());
                                    let err = Severity::of(&record.severity_text) == Severity::Error;
                                    let time = row_time(&record.timestamp, now);
                                    let trace_id = record.trace_id.clone();
                                    let attrs: Vec<(String, String)> = sorted_attrs(&record.attributes).into_iter().take(ROW_ATTRS).collect();
                                    rsx! {
                                        tr {
                                            key: "{key}",
                                            class: if err { "is-error" } else { "" },
                                            aria_selected: "{is_selected}",
                                            tabindex: "0",
                                            onclick: move |_| selected.set(Some(record_for_click.clone())),
                                            onkeydown: move |k| {
                                                if k.key() == Key::Enter || k.key() == Key::Character(" ".to_string()) {
                                                    k.prevent_default();
                                                    selected.set(Some(record_for_key.clone()));
                                                }
                                            },
                                            td { class: "bc-time", "{time}" }
                                            td { class: "bc-fit", SeverityTag { text: record.severity_text.clone() } }
                                            td { class: "bc-fit", Svc { name: record.service_name.clone() } }
                                            td { class: "bc-fit",
                                                if trace_id.is_empty() {
                                                    span { class: "d-muted", "—" }
                                                } else {
                                                    TraceLink { trace_id: trace_id.clone(), onclick: move |_| open_trace.call(trace_id.clone()) }
                                                }
                                            }
                                            td { class: "bc-body", span { class: "d-trunc", title: "{record.body}", "{record.body}" } }
                                            td { class: "bc-attrs", Attrs { attrs } }
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

/// A row's identity. `LogRecord` has no id, so timestamp and body stand in — the same pair the
/// live tail's duplicate guard uses. Keying the drawer and the rows this way remounts the drawer
/// on every new selection, which resets its tab and context state.
fn detail_key(record: &LogRecord) -> String {
    match &record.timestamp {
        Some(ts) => format!("{}.{}-{}", ts.seconds, ts.nanos, record.body),
        None => record.body.clone(),
    }
}
