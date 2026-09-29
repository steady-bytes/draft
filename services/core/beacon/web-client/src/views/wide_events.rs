//! Wide events: one row per span, searched with BeaconQL, with a status histogram above the
//! table and a detail drawer for the selected row.
//!
//! Search only: `WideEventsService` has no streaming RPC, so running a query re-issues a one-shot
//! `SearchWideEvents`. Selecting a row also loads every event that shares its `trace_id` (a second
//! `SearchWideEvents`, filtered to that trace) — the drawer draws it as a compact waterfall, and
//! the Flame graph view draws it full width.

use chrono::{Duration, Utc};
use dioxus::prelude::*;
use draft_api::hook::core_observability_wide_events_v1::use_wide_events_service_service;
use draft_api::proto::core_observability_wide_events_v1::{LogLine, SearchWideEventsRequest, WideEvent};
use draft_ui::data::{DurationCell, Kv, KvItem};
use draft_ui::layout::{Drawer, DrawerBlock, PageHead, Split};
use draft_ui::query::{GrammarId, QueryBar, QueryFilters};
use draft_ui::shell::{use_page_chrome, BarItem, Chrome, ChromeStatus};
use draft_ui::ui::{Alert, Btn, BtnVariant, Empty, Loading, Seg, Status, Svc};
use draft_ui::util::format_duration_ns;
use draft_ui::viz::CompactWaterfall;
use draft_ui::{StatusKind, Tone};

use crate::components::{use_open_logs, use_open_trace, AttrBlock, Attrs, FlameGraph, StatusText, VolumePanel};
use crate::data::{
    beaconql_string, compact_rows, full_time, row_time, sorted_attrs, ts_nanos, window_clause, with_window, Severity, SpanLike,
    SpanStatus,
};
use crate::range::{TimeRange, TimeRangePicker};

/// The time-range presets, in minutes.
const PRESETS: [i64; 4] = [15, 60, 180, 1440];

/// How many attributes a row's cell shows before the drawer takes over.
const ROW_ATTRS: usize = 4;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum ViewMode {
    List,
    Flame,
}

fn event_key(e: &WideEvent) -> String {
    format!("{}:{}", e.trace_id, e.span_id)
}

fn to_span(e: &WideEvent) -> SpanLike {
    SpanLike {
        id: e.span_id.clone(),
        parent: e.parent_span_id.clone(),
        service: e.service_name.clone(),
        name: e.span_name.clone(),
        start_ns: i128::from(ts_nanos(&e.start_time).unwrap_or(0)),
        duration_ns: e.duration_ns,
        err: SpanStatus::of(&e.status_code) == SpanStatus::Error,
    }
}

/// The trace's root: the span with no parent, else the earliest — a truncated fetch may not
/// include the true root, and the earliest known span is still the most useful header.
fn root_of(events: &[WideEvent]) -> Option<&WideEvent> {
    events
        .iter()
        .find(|e| e.parent_span_id.is_empty())
        .or_else(|| events.iter().min_by_key(|e| ts_nanos(&e.start_time).unwrap_or(i64::MAX)))
}

fn request(expression: &str, range: &TimeRange) -> SearchWideEventsRequest {
    let (start, end) = range.bounds(Utc::now());
    SearchWideEventsRequest { filter: with_window(expression, &window_clause(start, end)), limit: 0, before: String::new() }
}

#[component]
pub fn WideEvents() -> Element {
    let mut view = use_signal(|| ViewMode::List);
    let mut expression = use_signal(String::new);
    let mut range = use_signal(|| TimeRange::Last(Duration::hours(3)));
    let mut search_req = use_signal(|| request("", &TimeRange::Last(Duration::hours(3))));
    let mut selected: Signal<Option<WideEvent>> = use_signal(|| None);

    let service = use_wide_events_service_service();
    let search_result = service.search_wide_events(search_req);
    let mut events: Signal<Vec<WideEvent>> = use_signal(Vec::new);
    let mut search_error: Signal<Option<String>> = use_signal(|| None);

    // Seed `events` from the latest result. A new `search_req` re-runs the hook's resource, which
    // lands here again.
    use_effect(move || match &*search_result.read() {
        Some(Ok(resp)) => {
            events.set(resp.events.clone());
            search_error.set(None);
        }
        Some(Err(e)) => search_error.set(Some(e.message().to_string())),
        None => {}
    });

    // The selected event's whole trace: a second search scoped to its trace id. The resource also
    // fires once on mount with an empty filter; `selected_trace` gates the effect so that result
    // is ignored.
    let mut selected_trace: Signal<Option<String>> = use_signal(|| None);
    let mut trace_req = use_signal(SearchWideEventsRequest::default);
    let trace_result = service.search_wide_events(trace_req);
    let mut trace_events: Signal<Vec<WideEvent>> = use_signal(Vec::new);
    let mut trace_error: Signal<Option<String>> = use_signal(|| None);
    use_effect(move || {
        if selected_trace.read().is_none() {
            return;
        }
        match &*trace_result.read() {
            Some(Ok(resp)) => {
                trace_events.set(resp.events.clone());
                trace_error.set(None);
            }
            Some(Err(e)) => trace_error.set(Some(e.message().to_string())),
            None => {}
        }
    });

    let select = use_callback(move |event: WideEvent| {
        let trace_id = event.trace_id.clone();
        selected.set(Some(event));
        if selected_trace.peek().as_deref() != Some(trace_id.as_str()) {
            trace_events.set(Vec::new());
            selected_trace.set(Some(trace_id.clone()));
            trace_req.set(SearchWideEventsRequest {
                filter: format!("trace_id = {}", beaconql_string(&trace_id)),
                limit: 0,
                before: String::new(),
            });
        }
    });

    let run = use_callback(move |_: ()| search_req.set(request(&expression.peek(), &range.peek())));
    let open_trace = use_open_trace();
    let open_logs = use_open_logs();

    // Derived ---------------------------------------------------------------------------------
    let list = events();
    let loading = search_result.read().is_none();
    let errors = list.iter().filter(|e| SpanStatus::of(&e.status_code) == SpanStatus::Error).count();
    let summary = format!("{} events · {} errors · {}", list.len(), errors, range().label());

    use_page_chrome(move || {
        let n = events.read().len();
        Chrome {
            crumbs: vec!["Beacon".into(), "Signals".into(), "Wide events".into()],
            status: Some(ChromeStatus::new(StatusKind::Ok, format!("{n} events loaded"))),
            left: vec![BarItem::kv("Source", "clickhouse")],
            right: vec![BarItem::Text("Search only".into())],
        }
    });

    let points: Vec<(i64, usize)> = list
        .iter()
        .filter_map(|e| {
            let class = match SpanStatus::of(&e.status_code) {
                SpanStatus::Other => 0,
                SpanStatus::Ok => 1,
                SpanStatus::Error => 2,
            };
            ts_nanos(&e.start_time).map(|ns| (ns, class))
        })
        .collect();
    let now = Utc::now();
    let max_duration = list.iter().map(|e| e.duration_ns).max().unwrap_or(1).max(1);
    let selected_key = selected().as_ref().map(event_key);

    let drawer = selected().map(|event| {
        let trace: Vec<WideEvent> = if selected_trace.read().as_deref() == Some(event.trace_id.as_str()) {
            trace_events.read().clone()
        } else {
            Vec::new()
        };
        rsx! {
            EventDrawer {
                key: "{event_key(&event)}",
                event: event.clone(),
                trace,
                on_close: move |_| selected.set(None),
                on_open_trace: move |id: String| open_trace.call(id),
                on_open_logs: move |id: String| open_logs.call(format!("trace_id = {}", beaconql_string(&id))),
                on_similar: move |q: String| {
                    expression.set(q);
                    run.call(());
                },
            }
        }
    });

    rsx! {
        Split { flush: true, drawer,
            PageHead { inline: true, title: "Wide events".to_string(),
                span { class: "d-label", "{summary}" }
                span { class: "d-spacer" }
                Seg::<ViewMode> {
                    options: vec![(ViewMode::List, "List".to_string()), (ViewMode::Flame, "Flame graph".to_string())],
                    value: view(),
                    on_change: move |v| view.set(v),
                    label: "View".to_string(),
                }
                TimeRangePicker {
                    value: range(),
                    presets: PRESETS.to_vec(),
                    on_change: move |r| {
                        range.set(r);
                        run.call(());
                    },
                }
            }

            QueryBar {
                grammar: GrammarId::BeaconWideEvents,
                value: expression,
                on_run: move |_| run.call(()),
                on_clear: move |_| {
                    expression.set(String::new());
                    run.call(());
                },
                error: search_error(),
            }
            QueryFilters { grammar: GrammarId::BeaconWideEvents, expression, on_run: move |_| run.call(()) }

            VolumePanel {
                title: "Volume by status".to_string(),
                label: "Wide events over time, stacked by status".to_string(),
                points,
                names: vec!["unset", "ok", "error"],
                tones: vec![Tone::Quiet, Tone::Primary, Tone::Err],
            }

            if view() == ViewMode::List {
                if loading && list.is_empty() {
                    Loading {}
                } else if list.is_empty() {
                    Empty { title: "No wide events".to_string(), "Nothing matched this query in the selected range." }
                } else {
                    div { class: "d-panel d-table-wrap",
                        table { class: "d-table bc-table",
                            thead {
                                tr {
                                    th { "Time (UTC)" }
                                    th { "Service" }
                                    th { "Span" }
                                    th { "Duration" }
                                    th { "Status" }
                                    th { "Attributes" }
                                }
                            }
                            tbody {
                                for e in list.iter() {
                                    {
                                        let event = e.clone();
                                        let event_for_key = e.clone();
                                        let key = event_key(e);
                                        let is_selected = selected_key.as_deref() == Some(key.as_str());
                                        let err = SpanStatus::of(&e.status_code) == SpanStatus::Error;
                                        let time = row_time(&e.start_time, now);
                                        let duration = format_duration_ns(e.duration_ns);
                                        let ratio = e.duration_ns as f64 / max_duration as f64;
                                        let tone = if err { Some(Tone::Err) } else { None };
                                        let attrs: Vec<(String, String)> = sorted_attrs(&e.attributes).into_iter().take(ROW_ATTRS).collect();
                                        rsx! {
                                            tr {
                                                key: "{key}",
                                                class: if err { "is-error" } else { "" },
                                                aria_selected: "{is_selected}",
                                                tabindex: "0",
                                                onclick: move |_| select.call(event.clone()),
                                                onkeydown: move |k| {
                                                    if k.key() == Key::Enter || k.key() == Key::Character(" ".to_string()) {
                                                        k.prevent_default();
                                                        select.call(event_for_key.clone());
                                                    }
                                                },
                                                td { class: "bc-time", "{time}" }
                                                td { class: "bc-fit", Svc { name: e.service_name.clone() } }
                                                td { class: "bc-body", span { class: "d-trunc", title: "{e.span_name}", "{e.span_name}" } }
                                                td { class: "bc-fit", DurationCell { text: duration, ratio, tone } }
                                                td { class: "bc-fit", StatusText { code: e.status_code.clone() } }
                                                td { class: "bc-attrs", Attrs { attrs } }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            } else if selected().is_none() {
                Empty { title: "No event selected".to_string(),
                    "Select a wide event in the List view first, then switch to Flame graph."
                }
            } else if let Some(err) = trace_error() {
                Alert { kind: StatusKind::Err, "{err}" }
            } else {
                FlamePage {
                    events: trace_events(),
                    selected: selected_key.clone(),
                    on_select: move |id: String| {
                        if let Some(e) = trace_events.read().iter().find(|e| e.span_id == id) {
                            selected.set(Some(e.clone()));
                        }
                    },
                }
            }
        }
    }
}

/// The full-width flame graph of one trace, under a summary of its root span.
#[component]
fn FlamePage(events: Vec<WideEvent>, selected: Option<String>, on_select: EventHandler<String>) -> Element {
    let Some(root) = root_of(&events) else {
        return rsx! {
            Empty { title: "No spans".to_string(), "This trace has no spans to display." }
        };
    };
    let spans: Vec<SpanLike> = events.iter().map(to_span).collect();
    let errors = spans.iter().filter(|s| s.err).count();
    let selected_span = selected.as_deref().and_then(|k| k.split_once(':')).map(|(_, span)| span.to_string());
    let when = full_time(&root.start_time);
    let duration = format_duration_ns(root.duration_ns);
    let counts = format!("{} spans · {} errors", spans.len(), errors);

    rsx! {
        div { class: "d-panel d-panel-body",
            div { class: "bc-flame-summary",
                Svc { name: root.service_name.clone() }
                span { "{root.span_name}" }
                span { class: "d-muted", "{duration}" }
                span { class: "d-muted", "{when} UTC" }
                StatusText { code: root.status_code.clone() }
                span { class: "d-spacer" }
                span { class: "d-label", "{counts}" }
            }
            FlameGraph { spans, selected: selected_span, on_select }
        }
    }
}

/// The detail drawer: status and time, the spans of this event's trace, its attributes and logs.
#[component]
fn EventDrawer(
    event: WideEvent,
    trace: Vec<WideEvent>,
    on_close: EventHandler<()>,
    on_open_trace: EventHandler<String>,
    on_open_logs: EventHandler<String>,
    on_similar: EventHandler<String>,
) -> Element {
    let status = SpanStatus::of(&event.status_code);
    let spans: Vec<SpanLike> = trace.iter().map(to_span).collect();
    let rows = compact_rows(&spans);
    let attributes = sorted_attrs(&event.attributes);
    let business = sorted_attrs(&event.business_attributes);
    let runtime = sorted_attrs(&event.runtime_attributes);
    let when = full_time(&event.start_time);
    let subtitle = format!(
        "{} · {} · {} spans · {} logs",
        event.service_name,
        format_duration_ns(event.duration_ns),
        spans.len().max(1),
        event.logs.len()
    );
    let lead = rsx! {
        Status { kind: status.kind(), "{status.label()}" }
        span { class: "d-label", "{when} UTC" }
    };
    let trace_id = event.trace_id.clone();
    let for_logs = event.trace_id.clone();
    // "Similar events" narrows to the same service and span name — for an RPC that is the route.
    let similar = format!(
        "service_name = {} AND span_name = {}",
        beaconql_string(&event.service_name),
        beaconql_string(&event.span_name)
    );

    rsx! {
        Drawer {
            label: "Wide event detail".to_string(),
            on_close: move |_| on_close.call(()),
            lead,
            title: event.span_name.clone(),
            subtitle,

            if !rows.is_empty() {
                DrawerBlock { title: "Spans".to_string(), CompactWaterfall { rows } }
            }
            DrawerBlock { title: "Identity".to_string(),
                Kv {
                    KvItem { label: "trace_id".to_string(), tone: Tone::Ca, "{event.trace_id}" }
                    KvItem { label: "span_id".to_string(), "{event.span_id}" }
                    if !event.parent_span_id.is_empty() {
                        KvItem { label: "parent_span_id".to_string(), "{event.parent_span_id}" }
                    }
                }
            }
            AttrBlock { title: "Attributes".to_string(), attrs: attributes }
            AttrBlock { title: "Business attributes".to_string(), attrs: business }
            AttrBlock { title: "Runtime attributes".to_string(), attrs: runtime }
            DrawerBlock { title: format!("Logs · {}", event.logs.len()),
                if event.logs.is_empty() {
                    span { class: "d-muted", "No log lines" }
                } else {
                    Kv {
                        for (i , line) in event.logs.iter().enumerate() {
                            LogRow { key: "{i}", line: line.clone() }
                        }
                    }
                }
            }
            div { class: "bc-actions",
                Btn { variant: BtnVariant::Primary, onclick: move |_| on_open_trace.call(trace_id.clone()), "Open trace" }
                Btn { onclick: move |_| on_open_logs.call(for_logs.clone()), "Logs for trace" }
                Btn { onclick: move |_| on_similar.call(similar.clone()), "Similar events" }
            }
        }
    }
}

/// One log line of the event, its key coloured by severity.
#[component]
fn LogRow(line: LogLine) -> Element {
    let tone = match Severity::of(&line.severity) {
        Severity::Other => None,
        s => Some(s.tone()),
    };
    let label = line.severity.to_uppercase();
    rsx! {
        KvItem { label, label_tone: tone, "{line.body}" }
    }
}
