use chrono::Utc;
use dioxus::prelude::*;
use dioxus_core::Task;
use tonic_web_wasm_client::Client as WasmClient;

use draft_api::hook::core_observability_logs_v1::use_logs_service_service;
use draft_api::proto::core_observability_logs_v1::{
    logs_service_client::LogsServiceClient, LogRecord, QueryLogsRequest, StreamLogsRequest,
};

use crate::components::{
    severity_label, severity_stripe_color, LogDetailDrawer, QueryBar, QueryGrammar,
    SeverityHistogram, TimeRange, TimeRangePicker, TracePill,
};

// Cap stored lines so the table doesn't grow without bound during a long tail.
const MAX_LINES: usize = 1_000;

#[derive(Clone, PartialEq)]
enum StreamStatus {
    Connecting,
    Connected,
    // A fixed (non-"now"-trailing) TimeRange::Custom range is a one-shot
    // QueryLogs call, not a live tail — see the Phase 13 design doc section.
    Historical,
    Disconnected,
}

/// Stream is Beacon's Logs view (Phase 3, extended by Phase 13 with a time
/// range picker): a live log tail backed by `StreamLogs` when `TimeRange` is
/// `Live`, or a one-shot bounded `QueryLogs` call when it's `Custom`. A
/// `QueryBar` (BeaconQL grammar) sits above the table — running a new filter
/// re-issues the active query (continuing from the last-seen cursor in Live
/// mode; re-run against the same fixed window in Custom mode) rather than
/// tearing the connection down and rescanning from the beginning.
#[component]
pub fn Stream() -> Element {
    let mut expression = use_signal(String::new);
    let mut lines: Signal<Vec<LogRecord>> = use_signal(Vec::new);
    let mut cursor: Signal<String> = use_signal(String::new);
    let mut status: Signal<StreamStatus> = use_signal(|| StreamStatus::Disconnected);
    let mut error: Signal<Option<String>> = use_signal(|| None);
    // Holds the active stream task so it can be cancelled when a new filter is run.
    let mut stream_task: Signal<Option<Task>> = use_signal(|| None);
    let mut time_range: Signal<TimeRange> = use_signal(TimeRange::default);
    // The row a LogDetailDrawer (Phase 15) is currently open for, if any.
    let mut selected: Signal<Option<LogRecord>> = use_signal(|| None);

    // Custom-range one-shot query plumbing — same use_resource-backed hook
    // pattern traces.rs uses for SearchTraces. The resource also fires once,
    // wastefully, on mount with the zero-value QueryLogsRequest; the effect
    // below only acts on a result while TimeRange::Custom is actually active,
    // exactly mirroring traces.rs's GetTrace/selected_trace_id guard.
    let logs_service = use_logs_service_service();
    let mut query_req: Signal<QueryLogsRequest> = use_signal(QueryLogsRequest::default);
    let query_result = logs_service.query_logs(query_req);

    // Opens (or re-opens) the active query with the current filter. In
    // TimeRange::Live mode this is StreamLogs, continuing from `cursor` — the
    // last-received row's timestamp — so switching filters doesn't force a
    // full historical rescan; `reset` additionally reseeds `cursor` from
    // `now - duration` (initial load, the "clear" action, or a new time range
    // selection). In TimeRange::Custom mode this is a one-shot QueryLogs
    // bounded by the picker's fixed start/end — `reset` doesn't apply, it
    // always re-runs against the same window with the current filter.
    let start_stream = use_callback(move |reset: bool| {
        if let Some(t) = stream_task.write().take() {
            t.cancel();
        }
        lines.set(Vec::new());
        error.set(None);

        match time_range.peek().clone() {
            TimeRange::Live(duration) => {
                if reset {
                    cursor.set(String::new());
                }
                status.set(StreamStatus::Connecting);

                let host = crate::API_DOMAIN.clone();
                let filter = expression.peek().clone();
                let after = if reset {
                    (Utc::now() - duration).to_rfc3339()
                } else {
                    cursor.peek().clone()
                };

                let task = spawn(async move {
                    let mut client = LogsServiceClient::new(WasmClient::new(host));
                    let resp = client
                        .stream_logs(StreamLogsRequest {
                            filter,
                            limit: 200,
                            after,
                        })
                        .await;
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
                                if let Some(record) = msg.record {
                                    if let Some(ts) = &record.timestamp {
                                        if let Some(dt) = chrono::DateTime::from_timestamp(
                                            ts.seconds,
                                            ts.nanos as u32,
                                        ) {
                                            cursor.set(dt.to_rfc3339());
                                        }
                                    }
                                    let mut buf = lines.write();
                                    // Re-issuing StreamLogs with a new `after` cursor can, in rare
                                    // cases, replay the single most-recently-seen row again — the
                                    // cursor round-trips through an RFC3339 string on both the
                                    // client and server, and ClickHouse's DateTime64(9) vs. Go's
                                    // time.Time can disagree by a sub-nanosecond formatting rounding
                                    // that turns the server's strict `timestamp > ?` back into a
                                    // match for the exact row already displayed. Guard against the
                                    // visible symptom (an adjacent duplicate line) rather than
                                    // chasing sub-nanosecond precision across three languages.
                                    let is_duplicate = buf.last().is_some_and(|last| {
                                        last.timestamp == record.timestamp && last.body == record.body
                                    });
                                    if !is_duplicate {
                                        if buf.len() >= MAX_LINES {
                                            buf.remove(0);
                                        }
                                        buf.push(record);
                                    }
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
            }
            TimeRange::Custom { start, end } => {
                status.set(StreamStatus::Historical);
                query_req.set(QueryLogsRequest {
                    filter: expression.peek().clone(),
                    limit: 0,
                    after: start.to_rfc3339(),
                    before: end.to_rfc3339(),
                    ascending: false,
                });
            }
        }
    });

    // Seed `lines` from the latest QueryLogs result while a Custom range is
    // active. QueryLogs returns descending (most-recent-first); reversed here
    // so `lines` is always oldest-first internally regardless of which path
    // populated it, matching the render's `.rev()` below.
    use_effect(move || {
        if !matches!(time_range(), TimeRange::Custom { .. }) {
            return;
        }
        match &*query_result.read() {
            Some(Ok(resp)) => {
                let mut records = resp.records.clone();
                records.reverse();
                lines.set(records);
                status.set(StreamStatus::Historical);
                error.set(None);
            }
            Some(Err(e)) => {
                status.set(StreamStatus::Disconnected);
                error.set(Some(e.message().to_string()));
            }
            None => {}
        }
    });

    // Start tailing (or run the initial historical query) on first render.
    use_effect(move || {
        start_stream.call(true);
    });

    let displayed = lines.read();
    let status_val = status();

    rsx! {
        div { class: "p-4 flex flex-col gap-3 h-screen",
            div { class: "flex items-center gap-2",
                h1 { class: "text-lg font-bold shrink-0", "Logs" }
                div { class: "flex-1 min-w-0",
                    QueryBar {
                        grammar: QueryGrammar::BeaconQl,
                        expression,
                        on_run: move |_| start_stream.call(false),
                        on_clear: move |_| {
                            expression.set(String::new());
                            start_stream.call(true);
                        },
                    }
                }
                TimeRangePicker {
                    value: time_range(),
                    on_change: move |tr: TimeRange| {
                        time_range.set(tr);
                        start_stream.call(true);
                    },
                }
            }

            div { class: "flex items-center gap-2 text-xs",
                match status_val {
                    StreamStatus::Connecting => rsx! {
                        span { class: "badge badge-ghost badge-sm", "connecting…" }
                    },
                    StreamStatus::Connected => rsx! {
                        span { class: "badge badge-success badge-sm", "live" }
                    },
                    StreamStatus::Historical => rsx! {
                        span { class: "badge badge-neutral badge-sm", "historical" }
                    },
                    StreamStatus::Disconnected => rsx! {
                        span { class: "badge badge-error badge-sm", "disconnected" }
                    },
                }
                if let Some(err) = error() {
                    span { class: "text-error font-mono", "{err}" }
                }
            }

            SeverityHistogram { lines: displayed.clone() }

            div { class: "flex-1 min-h-0 flex",
                div { class: "flex-1 overflow-auto min-h-0",
                    table { class: "table table-xs",
                        thead {
                            tr {
                                th { "TIME (UTC)" }
                                th { "SEVERITY" }
                                th { "SERVICE" }
                                th { "TRACE" }
                                th { "BODY" }
                                th { "ATTRIBUTES" }
                            }
                        }
                        tbody {
                            if displayed.is_empty() {
                                tr {
                                    td {
                                        colspan: "6",
                                        class: "text-center text-base-content/40 py-6",
                                        "Waiting for logs…"
                                    }
                                }
                            }
                            for record in displayed.iter().rev() {
                                {
                                    let time = record_time(record);
                                    let severity = record.severity_text.clone();
                                    let stripe = severity_stripe_color(&severity);
                                    let severity_text = severity_label(&severity).to_string();
                                    let service = record.service_name.clone();
                                    let trace_id = record.trace_id.clone();
                                    let body = record.body.clone();
                                    let attrs = record_attrs(record);
                                    let record_for_click = record.clone();
                                    let is_selected = selected.peek().as_ref() == Some(record);
                                    let row_class = if is_selected {
                                        "hover:bg-base-300 cursor-pointer bg-base-300"
                                    } else {
                                        "hover:bg-base-300 cursor-pointer"
                                    };
                                    rsx! {
                                        tr {
                                            class: "{row_class}",
                                            onclick: move |_| selected.set(Some(record_for_click.clone())),
                                            td {
                                                class: "font-mono text-xs text-base-content/60 whitespace-nowrap",
                                                style: "position: relative;",
                                                // A short, vertically-centered bar rather than a
                                                // full-height border — reads as a quiet marker, not
                                                // a wall of color running the length of the row.
                                                // opacity mutes the (already theme-derived) color
                                                // further so it doesn't compete with the text.
                                                div {
                                                    style: "position:absolute; left:0; top:50%; transform:translateY(-50%); width:2px; height:12px; border-radius:1px; background:{stripe}; opacity:0.55;",
                                                }
                                                "{time}"
                                            }
                                            td { class: "text-xs text-base-content/70", "{severity_text}" }
                                            td { class: "text-xs", "{service}" }
                                            td { TracePill { trace_id } }
                                            td {
                                                class: "font-mono text-xs text-base-content/80",
                                                style: "max-width:480px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;",
                                                title: "{body}",
                                                "{body}"
                                            }
                                            td { class: "font-mono text-xs text-base-content/40", "{attrs}" }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }

                if let Some(record) = selected() {
                    LogDetailDrawer {
                        key: "{detail_key(&record)}",
                        record: record.clone(),
                        on_close: move |_| selected.set(None),
                        on_filter: move |clause: String| {
                            let current = expression.peek().clone();
                            expression.set(
                                if current.trim().is_empty() {
                                    clause
                                } else {
                                    // Parenthesize the existing filter: BeaconQL's AND binds
                                    // tighter than OR, so appending a bare `AND <clause>` to a
                                    // filter that already has a top-level OR would silently
                                    // scope the new clause to only the OR's last operand
                                    // instead of the whole existing filter.
                                    format!("({current}) AND {clause}")
                                },
                            );
                            start_stream.call(false);
                        },
                    }
                }
            }
        }
    }
}

/// detail_key gives a LogDetailDrawer instance a stable identity per row.
/// LogRecord has no dedicated id (see the design doc's "Copy/share permalink"
/// gap-table row — deliberately out of scope), so timestamp+body is used the
/// same way stream.rs's own live-tail duplicate detection already does.
/// Keying the drawer this way makes Dioxus remount it fresh on every new row
/// selection, resetting its tab/wrap/Context state without hand-rolled
/// prop-change tracking — see LogDetailDrawer's doc comment.
fn detail_key(record: &LogRecord) -> String {
    match &record.timestamp {
        Some(ts) => format!("{}.{}-{}", ts.seconds, ts.nanos, record.body),
        None => record.body.clone(),
    }
}

fn record_time(record: &LogRecord) -> String {
    match &record.timestamp {
        Some(ts) => match chrono::DateTime::from_timestamp(ts.seconds, ts.nanos as u32) {
            Some(dt) => dt.format("%Y-%m-%d %H:%M:%S%.3f").to_string(),
            None => "—".to_string(),
        },
        None => "—".to_string(),
    }
}

fn record_attrs(record: &LogRecord) -> String {
    if record.attributes.is_empty() {
        return String::new();
    }
    let mut pairs: Vec<String> = record
        .attributes
        .iter()
        .map(|(k, v)| format!("{k}={v}"))
        .collect();
    pairs.sort();
    pairs.join(" ")
}
