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
    // Details drawer for a clicked row. `selected` is left holding the last-clicked event even
    // after the drawer closes (only `drawer_open` toggles) so the close transition slides an
    // empty-content flash-free panel away rather than the content vanishing the instant the
    // backdrop is clicked.
    let mut selected: Signal<Option<CloudEvent>> = use_signal(|| None);
    let mut drawer_open = use_signal(|| false);

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
            SortDir::Asc => OrderDirection::Asc as i32,
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
    let all_events: &Vec<CloudEvent> = if streaming() {
        &stream_events
    } else {
        &static_events
    };
    let filter = expression.read();

    let filtered: Vec<&CloudEvent> = all_events
        .iter()
        .filter(|e| matches_filter(&filter, e))
        .collect();

    // Precomputed (not inlined into the rsx! string below, per this codebase's own
    // established gotcha: rsx! format-string positions only accept simple
    // `{identifier}` interpolation, not `{if ... }` expressions) transition classes for
    // the details drawer -- always mounted (see `selected`'s own comment for why),
    // toggled purely by drawer_open.
    // Invisible (no dimming of the main page behind it) -- exists purely to catch a
    // click outside the panel and close it; pointer-events-none while closed so it
    // never blocks the page underneath.
    let backdrop_class = format!(
        "fixed inset-0 z-40 {}",
        if drawer_open() { "" } else { "pointer-events-none" }
    );
    let panel_class = format!(
        "fixed top-0 right-0 h-full w-full max-w-md bg-base-100 shadow-2xl z-50 \
         flex flex-col transition-transform duration-300 {}",
        if drawer_open() { "translate-x-0" } else { "translate-x-full" }
    );

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
                // QueryBuilder builds its own already-prefixed ("AND "/"OR ", per its own
                // connector toggle) fragment before calling this -- appending here is just
                // concatenation, no prefix decision to make.
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
                                // event is &&CloudEvent (filtered: Vec<&CloudEvent>, .iter() adds
                                // another layer) -- a bare `.clone()` would resolve to the
                                // reference's own Clone impl and just copy the pointer, not the
                                // event. Double-deref first to reach the owned CloudEvent.
                                let ev_click: CloudEvent = (**event).clone();
                                rsx! {
                                    tr {
                                        class: "hover:bg-base-300 cursor-pointer",
                                        onclick: move |_| {
                                            selected.set(Some(ev_click.clone()));
                                            drawer_open.set(true);
                                        },
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

            // ── Event details drawer ────────────────────────────────────────
            div { class: "{backdrop_class}", onclick: move |_| drawer_open.set(false) }
            div {
                class: "{panel_class}",
                onclick: move |ev| ev.stop_propagation(),
                if let Some(ev) = selected() {
                    EventDetailsDrawer {
                        event: ev,
                        on_close: move |_| drawer_open.set(false),
                        // Unlike QueryBuilder (which has its own AND/OR toggle and passes an
                        // already-prefixed fragment), a body-value click has no UI for that
                        // choice -- default to AND, narrowing the current query, since "find
                        // more like this specific event" is the click's whole premise.
                        on_add_filter: move |raw_fragment: String| {
                            let current = expression.read().clone();
                            let new_expr = if current.trim().is_empty() {
                                raw_fragment
                            } else {
                                format!("{current} AND {raw_fragment}")
                            };
                            expression.set(new_expr);
                        },
                    }
                }
            }
        }
    }
}

// ─── Event details drawer ───────────────────────────────────────────────────────

#[component]
fn EventDetailsDrawer(
    event: CloudEvent,
    on_close: EventHandler<()>,
    on_add_filter: EventHandler<String>,
) -> Element {
    let etype = event.r#type.clone();
    let time = event_time(&event);
    let forwarded_at = event_forwarded_at(&event);
    let subject = event_subject(&event);
    let body = event_text_data(&event).to_string();
    // Parsed once: if it's a JSON object/array, the interactive JsonTree below renders
    // it (with clickable leaves that build a body.<path> filter); anything else --
    // invalid JSON, or a bare JSON scalar/string with nothing to click into -- falls
    // back to the plain pretty-printed (or verbatim, if unparseable) text view.
    let parsed_body = serde_json::from_str::<serde_json::Value>(&body).ok();
    let body_tree = parsed_body.clone().filter(|v| v.is_object() || v.is_array());
    let pretty_body = parsed_body
        .and_then(|v| serde_json::to_string_pretty(&v).ok())
        .unwrap_or(body);

    // Every other attribute beyond the three already shown as their own fields above --
    // the table only surfaces time/forwarded_at/subject, but a CloudEvent can carry
    // arbitrary extension attributes worth seeing in the full-detail view.
    let mut extra_attrs: Vec<(String, String)> = event
        .attributes
        .iter()
        .filter(|(k, _)| !matches!(k.as_str(), "time" | "forwarded_at" | "subject"))
        .map(|(k, v)| (k.clone(), format_attr_value(&v.attr)))
        .collect();
    extra_attrs.sort_by(|a, b| a.0.cmp(&b.0));

    rsx! {
        div { class: "flex items-center justify-between p-4 border-b border-base-300 shrink-0",
            TypeBadge { event_type: etype }
            button {
                class: "btn btn-sm btn-circle btn-ghost",
                onclick: move |_| on_close.call(()),
                "✕"
            }
        }
        div { class: "flex-1 overflow-auto p-4 flex flex-col gap-4",
            DetailRow { label: "ID", value: event.id.clone() }
            DetailRow { label: "SOURCE", value: event.source.clone() }
            DetailRow { label: "SPEC VERSION", value: event.spec_version.clone() }
            DetailRow { label: "TIME (UTC)", value: time }
            DetailRow { label: "FORWARDED AT", value: forwarded_at }
            DetailRow { label: "SUBJECT", value: subject }

            if !extra_attrs.is_empty() {
                div { class: "divider text-[10px] tracking-wider text-base-content/40", "ATTRIBUTES" }
                for (key, value) in extra_attrs {
                    DetailRow { label: key.to_uppercase(), value: value }
                }
            }

            div { class: "divider text-[10px] tracking-wider text-base-content/40", "BODY" }
            if let Some(root) = body_tree {
                div { class: "text-[10px] text-base-content/40 mb-1",
                    "Click a value to add it to the query"
                }
                JsonTree { value: root, path: String::new(), addressable: true, on_add: on_add_filter }
            } else {
                pre {
                    class: "bg-base-200 rounded p-3 text-xs overflow-auto whitespace-pre-wrap break-all font-mono",
                    "{pretty_body}"
                }
            }
        }
    }
}

// Recursively renders a JSON object/array as key/value rows. Every scalar leaf reached
// through object keys only is clickable (there's a real body.<path> filter it can
// build, per json_get_path). `addressable` tracks that: it starts true at the root and
// latches false forever the moment recursion passes through an array, since dot-paths
// don't address array elements -- without this latch, an object nested *inside* an
// array (eg. one of a WideEvent's `logs` entries) would resume looking addressable to
// its own descendants, building an incorrect path that skips the array entirely (a real
// bug caught while writing this: clearing `path` at the array boundary alone isn't
// enough, since a later object key would just rebuild a path from that empty point as
// if it were still the root). Array contents still display, just as plain inert text.
#[component]
fn JsonTree(
    value: serde_json::Value,
    path: String,
    addressable: bool,
    on_add: EventHandler<String>,
) -> Element {
    match value {
        serde_json::Value::Object(map) => {
            let mut entries: Vec<(String, serde_json::Value)> = map.into_iter().collect();
            entries.sort_by(|a, b| a.0.cmp(&b.0));
            rsx! {
                div { class: "flex flex-col gap-1.5 pl-3 border-l border-base-300",
                    for (key, val) in entries {
                        {
                            let child_path = if path.is_empty() { key.clone() } else { format!("{path}.{key}") };
                            rsx! {
                                div {
                                    div { class: "text-[10px] text-base-content/40 font-mono", "{key}" }
                                    JsonTree { value: val, path: child_path, addressable, on_add }
                                }
                            }
                        }
                    }
                }
            }
        }
        serde_json::Value::Array(items) => {
            rsx! {
                div { class: "flex flex-col gap-1.5 pl-3 border-l border-base-300",
                    for (i, val) in items.into_iter().enumerate() {
                        div {
                            div { class: "text-[10px] text-base-content/40 font-mono", "[{i}]" }
                            JsonTree { value: val, path: String::new(), addressable: false, on_add }
                        }
                    }
                }
            }
        }
        leaf => {
            let display = json_leaf_display(&leaf);
            if !addressable || path.is_empty() || matches!(leaf, serde_json::Value::Null) {
                // Inside an array, a root-level scalar body, or null: nothing
                // addressable to click into.
                rsx! { span { class: "font-mono text-xs text-base-content/60 break-all", "{display}" } }
            } else {
                let frag = format!("body.{path} = '{display}'");
                rsx! {
                    button {
                        class: "font-mono text-xs text-left break-all hover:text-primary hover:underline",
                        title: "Add \"{frag}\" to query",
                        onclick: move |_| on_add.call(frag.clone()),
                        "{display}"
                    }
                }
            }
        }
    }
}

fn json_leaf_display(v: &serde_json::Value) -> String {
    match v {
        serde_json::Value::String(s) => s.clone(),
        serde_json::Value::Bool(b) => b.to_string(),
        serde_json::Value::Number(n) => n.to_string(),
        serde_json::Value::Null => "null".to_string(),
        // Object/Array never reach here -- matched separately in JsonTree.
        other => other.to_string(),
    }
}

#[component]
fn DetailRow(label: String, value: String) -> Element {
    rsx! {
        div {
            div { class: "text-[10px] tracking-wider text-base-content/40", "{label}" }
            div { class: "font-mono text-xs break-all", "{value}" }
        }
    }
}

// Stringifies any CloudEvent extension-attribute variant for display -- mirrors
// event_time/event_forwarded_at's own timestamp formatting for CeTimestamp so a
// custom timestamp-valued attribute renders the same way the built-in ones do.
fn format_attr_value(attr: &Option<cloud_event_attribute_value::Attr>) -> String {
    match attr {
        Some(cloud_event_attribute_value::Attr::CeBoolean(b)) => b.to_string(),
        Some(cloud_event_attribute_value::Attr::CeInteger(i)) => i.to_string(),
        Some(cloud_event_attribute_value::Attr::CeString(s)) => s.clone(),
        Some(cloud_event_attribute_value::Attr::CeBytes(b)) => format!("<{} bytes>", b.len()),
        Some(cloud_event_attribute_value::Attr::CeUri(s)) => s.clone(),
        Some(cloud_event_attribute_value::Attr::CeUriRef(s)) => s.clone(),
        Some(cloud_event_attribute_value::Attr::CeTimestamp(ts)) => {
            chrono::DateTime::from_timestamp(ts.seconds, ts.nanos as u32)
                .map(|dt| dt.format("%Y-%m-%d %H:%M:%S").to_string())
                .unwrap_or_else(|| "—".to_string())
        }
        None => "—".to_string(),
    }
}

// ─── Filter ───────────────────────────────────────────────────────────────────

// Evaluates a CESQL-subset expression against a CloudEvent.
// Supported forms:
//   field = 'value'              exact match (case-insensitive)
//   field LIKE '%pattern%'       contains (% wildcards stripped)
//   body.field = 'value'                    JSON body field exact match
//   body.field LIKE '%pattern%'             JSON body field contains
//   body.nested.field = 'value'             dot-path into a nested JSON object
//   body.nested.field LIKE '%pattern%'      same, LIKE form
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
        return matches_filter(&filter[..pos], event) || matches_filter(&filter[pos + 4..], event);
    }
    if let Some(pos) = lower.find(" and ") {
        return matches_filter(&filter[..pos], event) && matches_filter(&filter[pos + 5..], event);
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
        return event_subject(event)
            .to_lowercase()
            .contains(&val.to_lowercase());
    }
    // body.field = 'value' (field may be a dot-path into nested objects, eg.
    // "businessAttributes.callerService" -- see json_get_path)
    if let Some((field, val)) = extract_body_eq(filter) {
        let body: serde_json::Value =
            serde_json::from_str(event_text_data(event)).unwrap_or_default();
        return json_get_path(&body, &field)
            .map(|v| match v {
                serde_json::Value::String(s) => s.eq_ignore_ascii_case(&val),
                other => other.to_string().eq_ignore_ascii_case(&val),
            })
            .unwrap_or(false);
    }
    // body.field LIKE '%pattern%' (dot-path supported, same as the eq form above)
    if let Some((field, pat)) = extract_body_like(filter) {
        let body: serde_json::Value =
            serde_json::from_str(event_text_data(event)).unwrap_or_default();
        return json_get_path(&body, &field)
            .map(|v| match v {
                serde_json::Value::String(s) => s.to_lowercase().contains(&pat.to_lowercase()),
                other => other
                    .to_string()
                    .to_lowercase()
                    .contains(&pat.to_lowercase()),
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
    if stripped.is_empty() {
        None
    } else {
        Some(stripped.to_string())
    }
}

// Parses `body.field = 'value'`, returns (field_name, value).
// Field names from protojson are camelCase (e.g. "modelName", "userId").
fn extract_body_eq(filter: &str) -> Option<(String, String)> {
    let lower = filter.to_lowercase();
    if !lower.starts_with("body.") {
        return None;
    }
    let rest = &filter[5..]; // after "body."
    let rest_lower = rest.to_lowercase();
    let eq_pos = rest_lower.find('=')?;
    // Reject if this is actually a LIKE expression.
    if rest_lower[..eq_pos].contains("like") {
        return None;
    }
    let field_name = rest[..eq_pos].trim().to_string();
    if field_name.is_empty() {
        return None;
    }
    let after_eq = rest[eq_pos + 1..].trim();
    let val = extract_quoted(after_eq)?.to_string();
    Some((field_name, val))
}

// Parses `body.field LIKE '%pattern%'`, returns (field_name, pattern_without_%).
fn extract_body_like(filter: &str) -> Option<(String, String)> {
    let lower = filter.to_lowercase();
    if !lower.starts_with("body.") {
        return None;
    }
    let rest = &filter[5..];
    let rest_lower = rest.to_lowercase();
    let like_pos = rest_lower.find(" like ")?;
    let field_name = rest[..like_pos].trim().to_string();
    if field_name.is_empty() {
        return None;
    }
    let after_like = rest[like_pos + 6..].trim();
    let inner = extract_quoted(after_like)?;
    let stripped = inner.trim_matches('%');
    if stripped.is_empty() {
        return None;
    }
    Some((field_name, stripped.to_string()))
}

// Walks a dot-path into a JSON object, eg. "businessAttributes.callerService" ->
// body["businessAttributes"]["callerService"]. A single-segment path (no dots) is just
// a plain top-level `.get`, so this is a superset of the old body.field behavior, not a
// separate case. Array indexing isn't supported (`serde_json::Value::get` only takes a
// `&str` key for objects, never a numeric index parsed from a path segment) -- not
// needed for the body shapes this is used against (WideEvent's attribute maps), and
// keeps this from having to distinguish "field" from "field[0]" syntax.
fn json_get_path<'a>(value: &'a serde_json::Value, path: &str) -> Option<&'a serde_json::Value> {
    path.split('.').try_fold(value, |v, key| v.get(key))
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
