//! Events: Catalyst's CloudEvents, as a stored snapshot (`Query`) or a live stream
//! (`QueryStream`, which replays from the start of the window and then goes live). The CESQL bar
//! and the type facets narrow what was fetched. See `cesql.rs` for why filtering happens here
//! rather than in the request.

use std::collections::HashSet;

use chrono::{Duration, Utc};
use dioxus::prelude::*;
use dioxus_core::Task;
use draft_api::proto::core_message_broker_actors_v1::{query_client::QueryClient, CloudEvent, OrderDirection, QueryRequest};
use draft_ui::data::{CodeBlock, CodeLang, Kv, KvItem};
use draft_ui::layout::{Drawer, DrawerBlock, PageHead, Split};
use draft_ui::query::{string_literal, Connector, GrammarId, LiteralStyle, QueryBar, QueryFilters};
use draft_ui::shell::{use_page_chrome, BarItem, Chrome, ChromeStatus};
use draft_ui::ui::{Chip, Dot, Empty, Loading, Seg, Toggle};
use draft_ui::util::{short_type_name, truncate_middle};
use draft_ui::{StatusKind, Tone};
use tonic_web_wasm_client::Client as WasmClient;

use crate::cesql::Filter;
use crate::events::{attr, attr_text, event_time, forward_delay_ms, full_time, json_leaf, row_time, text_data, type_facets};

/// Events kept in the table, so a long stream does not grow without bound.
const MAX_EVENTS: usize = 1_000;

/// The time-range presets, in minutes.
const PRESETS: [i64; 5] = [15, 60, 180, 1440, 10_080];

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum Order {
    Newest,
    Oldest,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum StreamStatus {
    Connecting,
    Connected,
    /// A stored snapshot: nothing is arriving.
    Snapshot,
    Disconnected,
}

fn range_label(minutes: i64) -> String {
    match minutes {
        m if m < 60 => format!("{m}m"),
        m if m < 1440 => format!("{}h", m / 60),
        m => format!("{}d", m / 1440),
    }
}

/// A type's colour: stable per name, the same mapping services use elsewhere.
fn type_tone(event_type: &str) -> Tone {
    Tone::for_name(event_type)
}

fn client() -> QueryClient<WasmClient> {
    QueryClient::new(WasmClient::new(crate::CATALYST_DOMAIN.clone()))
}

#[component]
pub fn Store() -> Element {
    // `draft` is what is typed; `applied` is what filters the table (set on Run, chip or drawer
    // click), so half-typed text never flashes a parse error.
    let mut draft = use_signal(String::new);
    let mut applied = use_signal(String::new);
    let mut streaming = use_signal(|| false);
    let mut order = use_signal(|| Order::Newest);
    let mut range = use_signal(|| 180i64);
    let mut types: Signal<HashSet<String>> = use_signal(HashSet::new);

    let mut streamed: Signal<Vec<CloudEvent>> = use_signal(Vec::new);
    let mut stored: Signal<Vec<CloudEvent>> = use_signal(Vec::new);
    let mut status = use_signal(|| StreamStatus::Snapshot);
    let mut querying = use_signal(|| true);
    let mut fetch_error: Signal<Option<String>> = use_signal(|| None);
    let mut stream_task: Signal<Option<Task>> = use_signal(|| None);
    let mut selected: Signal<Option<CloudEvent>> = use_signal(|| None);

    let after = move || (Utc::now() - Duration::minutes(*range.peek())).to_rfc3339();

    // Opens the live stream (cancelling any previous one): historical replay from the start of
    // the window, then live events, newest first.
    let start_stream = use_callback(move |_: ()| {
        if let Some(t) = stream_task.write().take() {
            t.cancel();
        }
        streamed.set(Vec::new());
        fetch_error.set(None);
        status.set(StreamStatus::Connecting);
        let after = after();
        let task = spawn(async move {
            let response = client()
                .query_stream(QueryRequest { expression: None, limit: 0, after, order_by: OrderDirection::Asc as i32 })
                .await;
            let stream = match response {
                Ok(r) => r,
                Err(e) => {
                    status.set(StreamStatus::Disconnected);
                    fetch_error.set(Some(e.message().to_string()));
                    return;
                }
            };
            status.set(StreamStatus::Connected);
            let mut stream = stream.into_inner();
            loop {
                match stream.message().await {
                    Ok(Some(msg)) => {
                        if let Some(event) = msg.event {
                            let mut buf = streamed.write();
                            if buf.len() >= MAX_EVENTS {
                                buf.pop();
                            }
                            buf.insert(0, event);
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
        fetch_error.set(None);
        status.set(StreamStatus::Snapshot);
        let order_by = match *order.peek() {
            Order::Newest => OrderDirection::Desc as i32,
            Order::Oldest => OrderDirection::Asc as i32,
        };
        let after = after();
        spawn(async move {
            match client().query(QueryRequest { expression: None, limit: 0, after, order_by }).await {
                Ok(resp) => stored.set(resp.into_inner().events),
                Err(e) => fetch_error.set(Some(e.message().to_string())),
            }
            querying.set(false);
        });
    });

    // First render: pick up a filter handed off from another view (Topology's "Query these
    // events"), then load the default snapshot.
    use_effect(move || {
        if let Some(query) = crate::PENDING_EVENT_QUERY.write().take() {
            draft.set(query.clone());
            applied.set(query);
        }
        run_query.call(());
    });

    // Re-fetch under whichever mode is active.
    let refresh = use_callback(move |_: ()| {
        if streaming() {
            start_stream.call(());
        } else {
            run_query.call(());
        }
    });

    let apply = use_callback(move |_: ()| applied.set(draft.peek().clone()));

    // Derived ---------------------------------------------------------------------------------
    let source: Vec<CloudEvent> = if streaming() { streamed() } else { stored() };
    let (filter, filter_error) = match Filter::parse(&applied()) {
        Ok(f) => (f, None),
        Err(e) => (Filter::All, Some(e)),
    };
    let matched: Vec<&CloudEvent> = source.iter().filter(|e| filter.matches(*e)).collect();
    let facets = type_facets(matched.iter().copied());
    let chosen = types();
    let shown: Vec<&CloudEvent> = matched.iter().copied().filter(|e| chosen.is_empty() || chosen.contains(&e.r#type)).collect();
    let now = Utc::now();
    let selected_id = selected().map(|e| e.id);
    let summary = format!("{} matched · last {}", shown.len(), range_label(range()));
    let waiting = source.is_empty() && ((!streaming() && querying()) || (streaming() && status() == StreamStatus::Connecting));

    use_page_chrome(move || {
        let live = match (streaming(), status()) {
            (true, StreamStatus::Connected) => ChromeStatus::live("Streaming"),
            (true, StreamStatus::Connecting) => ChromeStatus::new(StatusKind::Warn, "Connecting"),
            (true, StreamStatus::Disconnected) => ChromeStatus::new(StatusKind::Err, "Catalyst disconnected"),
            _ => ChromeStatus::new(StatusKind::Idle, "Snapshot"),
        };
        Chrome {
            crumbs: vec!["Blueprint".into(), "Events".into(), "Query".into()],
            status: Some(live),
            left: vec![BarItem::kv("Source", "catalyst")],
            right: vec![BarItem::kv("Streaming", if streaming() { "on" } else { "off" })],
        }
    });

    let drawer = selected().map(|event| {
        rsx! {
            EventDrawer {
                key: "{event.id}",
                event,
                on_close: move |_| selected.set(None),
                on_add_filter: move |fragment: String| {
                    let next = GrammarId::Cesql.grammar().combine(&applied.peek(), Connector::And, &fragment);
                    draft.set(next.clone());
                    applied.set(next);
                },
            }
        }
    });

    rsx! {
        Split { flush: true, drawer,
            PageHead { inline: true, title: "Events".to_string(),
                span { class: "d-label", "{summary}" }
                span { class: "d-spacer" }
                Toggle {
                    checked: streaming(),
                    on_change: move |on| {
                        streaming.set(on);
                        if on {
                            stored.set(Vec::new());
                            start_stream.call(());
                        } else {
                            if let Some(t) = stream_task.write().take() {
                                t.cancel();
                            }
                            streamed.set(Vec::new());
                            run_query.call(());
                        }
                    },
                    "Stream"
                }
                Seg::<Order> {
                    options: vec![(Order::Newest, "Newest first".to_string()), (Order::Oldest, "Oldest first".to_string())],
                    value: order(),
                    // A stream always arrives newest first; the order applies to snapshots.
                    on_change: move |o| {
                        order.set(o);
                        if !streaming() {
                            run_query.call(());
                        }
                    },
                    label: "Order".to_string(),
                }
                Seg::<i64> {
                    options: PRESETS.iter().map(|&m| (m, range_label(m))).collect::<Vec<_>>(),
                    value: range(),
                    on_change: move |m| {
                        range.set(m);
                        refresh.call(());
                    },
                    label: "Time range".to_string(),
                }
            }

            QueryBar {
                grammar: GrammarId::Cesql,
                value: draft,
                on_run: move |_| {
                    apply.call(());
                    refresh.call(());
                },
                on_clear: move |_| {
                    draft.set(String::new());
                    applied.set(String::new());
                },
                error: filter_error,
            }
            QueryFilters { grammar: GrammarId::Cesql, expression: draft, on_run: move |_| apply.call(()) }

            if !facets.is_empty() {
                div { class: "eq-facets", role: "group", aria_label: "Event types",
                    span { class: "d-label", "Types" }
                    for (name , count) in facets.into_iter().take(12) {
                        {
                            let pressed = chosen.contains(&name);
                            let toggle_name = name.clone();
                            let label = short_type_name(&name).to_string();
                            rsx! {
                                Chip {
                                    key: "{name}",
                                    pressed,
                                    count: count as u32,
                                    dot: type_tone(&name),
                                    onclick: move |_| {
                                        let mut set = types.write();
                                        if !set.remove(&toggle_name) {
                                            set.insert(toggle_name.clone());
                                        }
                                    },
                                    "{label}"
                                }
                            }
                        }
                    }
                }
            }

            if let Some(err) = fetch_error() {
                div { style: "margin-top:12px",
                    draft_ui::ui::Alert { kind: StatusKind::Err, "Could not reach Catalyst: {err}" }
                }
            }

            div { style: "margin-top:16px",
                if waiting {
                    Loading {}
                } else if shown.is_empty() {
                    Empty { title: "No events".to_string(),
                        if source.is_empty() { "Nothing in this window yet. Events appear here as services publish them." } else { "No event matches the filters." }
                    }
                } else {
                    div { class: "d-panel d-table-wrap",
                        table { class: "d-table eq-table",
                            thead {
                                tr {
                                    th { if streaming() || order() == Order::Newest { "Time (UTC) ↓" } else { "Time (UTC) ↑" } }
                                    th { "Type" }
                                    th { "Source" }
                                    th { "Subject" }
                                    th { "ID" }
                                    th { "Data" }
                                }
                            }
                            tbody {
                                for e in shown {
                                    {
                                        let event = e.clone();
                                        let event_for_key = e.clone();
                                        let is_selected = selected_id.as_deref() == Some(e.id.as_str());
                                        let time = row_time(&event_time(e), now);
                                        let subject = attr(e, "subject").unwrap_or_else(|| "—".to_string());
                                        let id = truncate_middle(&e.id, 8, 0);
                                        let data = text_data(e).to_string();
                                        let tone = type_tone(&e.r#type);
                                        rsx! {
                                            tr {
                                                key: "{e.id}",
                                                aria_selected: "{is_selected}",
                                                tabindex: "0",
                                                onclick: move |_| selected.set(Some(event.clone())),
                                                onkeydown: move |k| {
                                                    if k.key() == Key::Enter {
                                                        selected.set(Some(event_for_key.clone()));
                                                    }
                                                },
                                                td { class: "t", "{time}" }
                                                td { class: "type", span { class: "d-row", style: "gap:8px", Dot { tone } "{e.r#type}" } }
                                                td { class: "src", "{e.source}" }
                                                td { "{subject}" }
                                                td { class: "id", title: "{e.id}", "{id}" }
                                                td { class: "body", span { class: "d-trunc", title: "{data}", "{data}" } }
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

/// The event drawer: envelope, extension attributes and the payload — as a tree whose values add
/// a filter when clicked, when it is a JSON object.
#[component]
fn EventDrawer(event: CloudEvent, on_close: EventHandler<()>, on_add_filter: EventHandler<String>) -> Element {
    let time = full_time(&event_time(&event));
    let delay = forward_delay_ms(&event).map(|ms| format!(" · forwarded +{ms} ms")).unwrap_or_default();
    let subtitle = format!("{time} UTC{delay}");
    let subject = attr(&event, "subject");
    let mut extra: Vec<(String, String)> = event
        .attributes
        .iter()
        .filter(|(k, _)| !matches!(k.as_str(), "time" | "forwarded_at" | "subject"))
        .filter_map(|(k, v)| attr_text(v).map(|t| (k.clone(), t)))
        .collect();
    extra.sort();

    let body = text_data(&event).to_string();
    let parsed = serde_json::from_str::<serde_json::Value>(&body).ok();
    let tree = parsed.clone().filter(|v| v.is_object() || v.is_array());
    let pretty = parsed.and_then(|v| serde_json::to_string_pretty(&v).ok()).unwrap_or(body);
    let lead = rsx! {
        span { class: "d-label", "CloudEvent · {event.spec_version}" }
    };

    rsx! {
        Drawer { label: "Event detail".to_string(), on_close: move |_| on_close.call(()), lead, title: event.r#type.clone(), subtitle,
            DrawerBlock { title: "Envelope".to_string(),
                Kv {
                    KvItem { label: "id".to_string(), "{event.id}" }
                    KvItem { label: "source".to_string(), "{event.source}" }
                    if let Some(s) = subject {
                        KvItem { label: "subject".to_string(), "{s}" }
                    }
                    for (k , v) in extra {
                        KvItem { key: "{k}", label: k.clone(), "{v}" }
                    }
                }
            }
            DrawerBlock { title: "Data".to_string(),
                if let Some(root) = tree {
                    p { class: "d-hint", style: "margin:0 0 8px", "Click a value to add it to the query." }
                    JsonTree { value: root, path: String::new(), addressable: true, on_add: on_add_filter }
                } else if pretty.is_empty() {
                    span { class: "d-muted", "No data" }
                } else {
                    CodeBlock { text: pretty, lang: CodeLang::Plain, wrap: true }
                }
            }
        }
    }
}

/// A JSON value as nested key / value rows. A scalar reached through object keys only is
/// clickable — there is a real `body.<path>` filter for it. `addressable` latches false for good
/// once the walk passes through an array, since dot paths cannot address array elements: without
/// the latch an object nested inside an array would look addressable again to its own descendants
/// and build a path that skips the array entirely. Array contents still show, as plain text.
#[component]
fn JsonTree(value: serde_json::Value, path: String, addressable: bool, on_add: EventHandler<String>) -> Element {
    match value {
        serde_json::Value::Object(map) => {
            let mut entries: Vec<(String, serde_json::Value)> = map.into_iter().collect();
            entries.sort_by(|a, b| a.0.cmp(&b.0));
            rsx! {
                div { class: "json-tree",
                    for (key , val) in entries {
                        {
                            let child = if path.is_empty() { key.clone() } else { format!("{path}.{key}") };
                            rsx! {
                                div { key: "{key}",
                                    div { class: "json-key", "{key}" }
                                    JsonTree { value: val, path: child, addressable, on_add }
                                }
                            }
                        }
                    }
                }
            }
        }
        serde_json::Value::Array(items) => rsx! {
            div { class: "json-tree",
                for (i , val) in items.into_iter().enumerate() {
                    div { key: "{i}",
                        div { class: "json-key", "[{i}]" }
                        JsonTree { value: val, path: String::new(), addressable: false, on_add }
                    }
                }
            }
        },
        leaf => {
            let display = json_leaf(&leaf);
            if !addressable || path.is_empty() || leaf.is_null() {
                rsx! { span { class: "json-leaf", "{display}" } }
            } else {
                let fragment = format!("body.{path} = {}", string_literal(LiteralStyle::SingleQuote, &display));
                let title = format!("Add {fragment} to the query");
                rsx! {
                    button { class: "json-leaf is-action", r#type: "button", title: "{title}", onclick: move |_| on_add.call(fragment.clone()), "{display}" }
                }
            }
        }
    }
}
