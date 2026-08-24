use dioxus::prelude::*;
use dioxus_core::Task;
use tonic_web_wasm_client::Client as WasmClient;

use draft_api::proto::core_observability_logs_v1::{
    logs_service_client::LogsServiceClient, LogRecord, StreamLogsRequest,
};

use crate::components::{QueryBar, QueryGrammar, SeverityBadge};

// Cap stored lines so the table doesn't grow without bound during a long tail.
const MAX_LINES: usize = 1_000;

#[derive(Clone, PartialEq)]
enum StreamStatus {
    Connecting,
    Connected,
    Disconnected,
}

/// Stream is Beacon's first visible product surface (Phase 3): a live log tail
/// backed by `StreamLogs`. A `QueryBar` (BeaconQL grammar) sits above the
/// table — running a new filter re-issues the stream from the last-seen
/// cursor rather than tearing the connection down and rescanning from the
/// beginning, per the design doc's Stream view description.
#[component]
pub fn Stream() -> Element {
    let mut expression = use_signal(String::new);
    let mut lines: Signal<Vec<LogRecord>> = use_signal(Vec::new);
    let mut cursor: Signal<String> = use_signal(String::new);
    let mut status: Signal<StreamStatus> = use_signal(|| StreamStatus::Disconnected);
    let mut error: Signal<Option<String>> = use_signal(|| None);
    // Holds the active stream task so it can be cancelled when a new filter is run.
    let mut stream_task: Signal<Option<Task>> = use_signal(|| None);

    // Opens (or re-opens) StreamLogs with the current filter, continuing from
    // `cursor` — the last-received row's timestamp — so switching filters
    // doesn't force a full historical rescan. `reset` clears the displayed
    // lines and cursor first, for the initial load and the "clear" action.
    let start_stream = use_callback(move |reset: bool| {
        if let Some(t) = stream_task.write().take() {
            t.cancel();
        }
        // The displayed rows always reflect the *current* filter, so they're
        // cleared on every (re-)run — including a plain filter change, where
        // `reset` is false. Only the cursor is conditionally preserved: `reset`
        // additionally clears it (initial load, the "clear" action) so a fresh
        // filter still gets a bounded historical replay rather than rescanning
        // from the beginning, per the design doc's Stream view description.
        lines.set(Vec::new());
        if reset {
            cursor.set(String::new());
        }
        error.set(None);
        status.set(StreamStatus::Connecting);

        let host = crate::API_DOMAIN.clone();
        let filter = expression.peek().clone();
        let after = cursor.peek().clone();

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
                                if let Some(dt) =
                                    chrono::DateTime::from_timestamp(ts.seconds, ts.nanos as u32)
                                {
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
                            let is_duplicate = buf
                                .last()
                                .is_some_and(|last| last.timestamp == record.timestamp && last.body == record.body);
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
    });

    // Start tailing on first render.
    use_effect(move || {
        start_stream.call(true);
    });

    let displayed = lines.read();
    let status_val = status();

    rsx! {
        div { class: "p-4 flex flex-col gap-3 h-screen",
            div { class: "flex items-center gap-2",
                h1 { class: "text-lg font-bold shrink-0", "Stream" }
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

            div { class: "flex items-center gap-2 text-xs",
                match status_val {
                    StreamStatus::Connecting => rsx! {
                        span { class: "badge badge-ghost badge-sm", "connecting…" }
                    },
                    StreamStatus::Connected => rsx! {
                        span { class: "badge badge-success badge-sm", "live" }
                    },
                    StreamStatus::Disconnected => rsx! {
                        span { class: "badge badge-error badge-sm", "disconnected" }
                    },
                }
                if let Some(err) = error() {
                    span { class: "text-error font-mono", "{err}" }
                }
            }

            div { class: "flex-1 overflow-auto min-h-0",
                table { class: "table table-xs",
                    thead {
                        tr {
                            th { "TIME (UTC)" }
                            th { "SEVERITY" }
                            th { "SERVICE" }
                            th { "BODY" }
                            th { "ATTRIBUTES" }
                        }
                    }
                    tbody {
                        if displayed.is_empty() {
                            tr {
                                td {
                                    colspan: "5",
                                    class: "text-center text-base-content/40 py-6",
                                    "Waiting for logs…"
                                }
                            }
                        }
                        for record in displayed.iter().rev() {
                            {
                                let time = record_time(record);
                                let severity = record.severity_text.clone();
                                let service = record.service_name.clone();
                                let body = record.body.clone();
                                let attrs = record_attrs(record);
                                rsx! {
                                    tr { class: "hover:bg-base-300",
                                        td { class: "font-mono text-xs text-base-content/60 whitespace-nowrap", "{time}" }
                                        td { SeverityBadge { severity } }
                                        td { class: "text-xs", "{service}" }
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
        }
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
