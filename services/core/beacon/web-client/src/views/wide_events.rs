use dioxus::prelude::*;

use draft_api::hook::core_observability_wide_events_v1::use_wide_events_service_service;
use draft_api::proto::core_observability_traces_v1::Span;
use draft_api::proto::core_observability_wide_events_v1::{
    LogLine, SearchWideEventsRequest, WideEvent,
};

use crate::components::{
    format_duration_ns, FlameGraph, QueryBar, QueryGrammar, WideEventHistogram,
    WideEventQueryBuilder,
};

/// ViewMode toggles between List (the searchable table + detail panel) and
/// FlameGraph (a full-width, single-trace visualization) — two ways to look
/// at the same underlying WideEvents, not two different data sets. Design
/// inspiration for FlameGraph mode: assets/flamgraph/Flamgraph_idea_1.png
/// (a trace-summary header, a time-axis ruler, span/error counts).
#[derive(Clone, Copy, PartialEq, Default)]
enum ViewMode {
    #[default]
    List,
    FlameGraph,
}

/// WideEvents is Beacon's search view for WideEvents: a `QueryBar` +
/// `WideEventQueryBuilder` (both BeaconQL, against wide_events' own schema)
/// drive a `SearchWideEvents` call, with a `WideEventHistogram` (volume by
/// status_code) above the results list and a detail panel for the selected
/// row. Search-only, like Traces — `WideEventsService` has no streaming RPC
/// (see docs/website/content/docs/architecture/wide-events.md), so unlike
/// Stream/Logs there's no live-tail toggle here; running a query re-issues
/// a one-shot `SearchWideEvents` call every time.
///
/// Selecting a row also loads every WideEvent sharing its trace_id (a
/// second `SearchWideEvents` call filtered on `trace_id = "…"`, not a new
/// RPC — WideEvent already carries the same trace_id/span_id/
/// parent_span_id/start_time/duration_ns shape Traces' own flame graph
/// needs), reusing the existing `FlameGraph` component via a thin
/// `WideEvent -> Span` conversion rather than teaching it a second shape.
/// `ViewMode` decides whether that trace renders as a small side detail or
/// as the dedicated FlameGraph page.
#[component]
pub fn WideEvents() -> Element {
    let mut view_mode = use_signal(ViewMode::default);
    let mut expression = use_signal(String::new);
    let mut search_req = use_signal(SearchWideEventsRequest::default);
    let mut selected: Signal<Option<WideEvent>> = use_signal(|| None);

    let service = use_wide_events_service_service();
    let search_result = service.search_wide_events(search_req);

    let mut events: Signal<Vec<WideEvent>> = use_signal(Vec::new);
    let mut search_error: Signal<Option<String>> = use_signal(|| None);

    // Seed `events` from the latest SearchWideEvents result. A new
    // `search_req` (run/clear/query-builder add) re-triggers the hook's
    // underlying use_resource, which lands here again — same pattern as
    // traces.rs's own SearchTraces effect.
    use_effect(move || match &*search_result.read() {
        Some(Ok(resp)) => {
            events.set(resp.events.clone());
            search_error.set(None);
        }
        Some(Err(e)) => search_error.set(Some(e.message().to_string())),
        None => {}
    });

    // Flame-graph plumbing — a second, independent SearchWideEvents call
    // scoped to one trace_id, mirroring traces.rs's own SearchTraces/GetTrace
    // split (one hook, two calls). `selected_trace_id` guards the seeding
    // effect below against the resource's default fire-on-mount with an
    // empty filter, same as traces.rs's own `selected_trace_id` guard.
    let mut selected_trace_id: Signal<Option<String>> = use_signal(|| None);
    let mut trace_req = use_signal(SearchWideEventsRequest::default);
    let trace_result = service.search_wide_events(trace_req);
    let mut trace_events: Signal<Vec<WideEvent>> = use_signal(Vec::new);
    let mut trace_error: Signal<Option<String>> = use_signal(|| None);

    use_effect(move || {
        if selected_trace_id.read().is_none() {
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

    // select opens (or re-opens) the flame graph for event's trace and shows
    // its own detail panel — called both from a results-list row click and
    // from a flame-graph node click (see WideEventFlame below).
    let mut select = move |event: WideEvent| {
        let trace_id = event.trace_id.clone();
        selected.set(Some(event));
        selected_trace_id.set(Some(trace_id.clone()));
        trace_req.set(SearchWideEventsRequest {
            filter: format!("trace_id = \"{trace_id}\""),
            limit: 0,
            before: String::new(),
        });
    };

    let mut run = move || {
        search_req.set(SearchWideEventsRequest {
            filter: expression.peek().clone(),
            limit: 0,
            before: String::new(),
        });
    };

    let event_list = events.read();
    let is_loading = search_result.read().is_none();
    let displayed = event_list.clone();

    rsx! {
        div { class: "p-4 flex flex-col gap-3 h-screen",
            div { class: "flex items-center gap-2",
                h1 { class: "text-lg font-bold shrink-0", "WideEvents" }
                div { class: "flex-1 min-w-0",
                    QueryBar {
                        grammar: QueryGrammar::BeaconQl,
                        expression,
                        on_run: move |_| run(),
                        on_clear: move |_| {
                            expression.set(String::new());
                            search_req.set(SearchWideEventsRequest::default());
                        },
                    }
                }
                div { class: "join shrink-0",
                    button {
                        class: if view_mode() == ViewMode::List { "btn btn-sm join-item btn-neutral" } else { "btn btn-sm join-item btn-ghost" },
                        onclick: move |_| view_mode.set(ViewMode::List),
                        "List"
                    }
                    button {
                        class: if view_mode() == ViewMode::FlameGraph { "btn btn-sm join-item btn-neutral" } else { "btn btn-sm join-item btn-ghost" },
                        onclick: move |_| view_mode.set(ViewMode::FlameGraph),
                        "Flame Graph"
                    }
                }
            }

            WideEventQueryBuilder {
                expression,
                on_add: move |next: String| {
                    expression.set(next);
                    run();
                },
            }

            if let Some(err) = search_error() {
                div { class: "text-xs text-error font-mono", "{err}" }
            }

            WideEventHistogram { events: displayed }

            if view_mode() == ViewMode::List {
                div { class: "flex-1 min-h-0 flex gap-4",
                    // Left: results list.
                    div { class: "flex-1 min-w-0 overflow-auto border border-base-300 rounded",
                        table { class: "table table-xs",
                            thead {
                                tr {
                                    th { "TIME (UTC)" }
                                    th { "SERVICE" }
                                    th { "SPAN" }
                                    th { "DURATION" }
                                    th { "STATUS" }
                                }
                            }
                            tbody {
                                if event_list.is_empty() {
                                    tr {
                                        td {
                                            colspan: "5",
                                            class: "text-center text-base-content/40 py-6",
                                            if is_loading { "Searching…" } else { "No WideEvents found" }
                                        }
                                    }
                                }
                                for e in event_list.iter() {
                                    {
                                        let event_for_click = e.clone();
                                        let is_selected = selected.peek().as_ref() == Some(e);
                                        let row_class = if is_selected {
                                            "hover:bg-base-300 cursor-pointer bg-base-300"
                                        } else {
                                            "hover:bg-base-300 cursor-pointer"
                                        };
                                        let time = event_time(e);
                                        let service_name = e.service_name.clone();
                                        let span_name = e.span_name.clone();
                                        let duration = format_duration_ns(e.duration_ns);
                                        let status = e.status_code.clone();
                                        let status_class = status_badge_class(&status);
                                        rsx! {
                                            tr {
                                                class: "{row_class}",
                                                onclick: move |_| select(event_for_click.clone()),
                                                td { class: "font-mono text-xs text-base-content/60 whitespace-nowrap", "{time}" }
                                                td { class: "text-xs", "{service_name}" }
                                                td {
                                                    class: "font-mono text-xs",
                                                    style: "max-width:320px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;",
                                                    title: "{span_name}",
                                                    "{span_name}"
                                                }
                                                td { class: "font-mono text-xs", "{duration}" }
                                                td { span { class: "{status_class}", "{status}" } }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }

                    // Right: detail panel for whichever WideEvent is
                    // currently selected. The trace's flame graph lives in
                    // its own ViewMode::FlameGraph page now, not inline here.
                    div { class: "w-[420px] shrink-0 overflow-auto",
                        if let Some(e) = selected() {
                            WideEventDetail { event: e }
                        } else {
                            div { class: "text-center text-base-content/40 py-12 text-sm",
                                "Select a WideEvent to view its logs and attributes"
                            }
                        }
                    }
                }
            } else {
                div { class: "flex-1 min-h-0 overflow-auto",
                    if selected_trace_id.read().is_none() {
                        div { class: "text-center text-base-content/40 py-12 text-sm",
                            "Select a WideEvent from the List view first, then switch to Flame Graph"
                        }
                    } else if let Some(err) = trace_error() {
                        div { class: "text-xs text-error font-mono", "{err}" }
                    } else {
                        WideEventFlamePage {
                            events: trace_events.read().clone(),
                            selected: selected(),
                            on_select: move |e: WideEvent| selected.set(Some(e)),
                        }
                    }
                }
            }
        }
    }
}

/// TIME_AXIS_TICKS is the number of evenly-spaced tick marks drawn above the
/// flame graph — matches Flamgraph_idea_1.png's own ruler cadence closely
/// enough to read as the same idiom without hardcoding to its exact count.
const TIME_AXIS_TICKS: usize = 6;

/// WideEventFlamePage is the dedicated, full-width flame-graph view
/// (ViewMode::FlameGraph) — a trace-summary header (root span's service/
/// name/duration/timestamp/status), a time-axis ruler, the flame graph
/// itself (span/error counts alongside its own section label), and a detail
/// panel for whichever span is currently selected. Modeled on
/// assets/flamgraph/Flamgraph_idea_1.png's own layout, simplified to what
/// WideEvent's schema actually has (no separate "Waterfall" section — the
/// flame graph and the List view's table already cover that ground between
/// them).
#[component]
fn WideEventFlamePage(
    events: Vec<WideEvent>,
    selected: Option<WideEvent>,
    on_select: EventHandler<WideEvent>,
) -> Element {
    let Some(root) = root_of(&events) else {
        return rsx! {
            div { class: "text-center text-base-content/40 py-12 text-sm", "No spans to display" }
        };
    };
    let span_count = events.len();
    let error_count = events
        .iter()
        .filter(|e| status_badge_class(&e.status_code) == "badge badge-error badge-sm")
        .count();
    let spans: Vec<Span> = events.iter().map(wide_event_to_span).collect();
    let total_ns = spans
        .iter()
        .map(|s| s.duration_ns)
        .max()
        .unwrap_or(1)
        .max(1);

    rsx! {
        div { class: "flex flex-col gap-3",
            // Trace summary header.
            div { class: "flex items-center gap-3 flex-wrap pb-2 border-b border-base-300",
                span { class: "font-bold", "{root.service_name}" }
                span { class: "text-base-content/40", "—" }
                span { class: "font-mono text-sm", "{root.span_name}" }
                span { class: "badge badge-ghost badge-sm font-mono", "{format_duration_ns(root.duration_ns)}" }
                span { class: "text-xs text-base-content/50 font-mono", "{event_time(&root)}" }
                span { class: "{status_badge_class(&root.status_code)}", "{root.status_code}" }
            }

            // Flame Graph section.
            div { class: "flex flex-col gap-2",
                div { class: "flex items-center justify-between",
                    span { class: "text-xs font-semibold uppercase tracking-wide text-base-content/50", "Flame Graph" }
                    span { class: "text-[10px] font-mono text-base-content/40",
                        "Spans: {span_count}   Errors: {error_count}"
                    }
                }
                div { class: "border border-base-300 rounded bg-base-200 p-3 flex flex-col gap-1",
                    TimeAxisRuler { total_ns }
                    FlameGraph {
                        spans,
                        on_select: move |s: Span| {
                            if let Some(e) = events.iter().find(|e| e.span_id == s.span_id) {
                                on_select.call(e.clone());
                            }
                        },
                    }
                }
            }

            if let Some(e) = selected {
                WideEventDetail { event: e }
            }
        }
    }
}

/// TimeAxisRuler renders TIME_AXIS_TICKS evenly-spaced labels above a
/// FlameGraph, from 0 to total_ns — plain percentage-based positioning
/// (matching FlameGraph's own left-offset-as-fraction-of-width layout)
/// rather than sharing an SVG coordinate space, so this stays a trivial,
/// independent component instead of a FlameGraph prop.
#[component]
fn TimeAxisRuler(total_ns: u64) -> Element {
    rsx! {
        div { class: "relative h-4 text-[10px] font-mono text-base-content/35",
            for i in 0..TIME_AXIS_TICKS {
                {
                    let frac = i as f64 / (TIME_AXIS_TICKS - 1) as f64;
                    let ns = (total_ns as f64 * frac) as u64;
                    let left_pct = frac * 100.0;
                    let (transform, justify) = if i == 0 {
                        ("translateX(0)", "left")
                    } else if i == TIME_AXIS_TICKS - 1 {
                        ("translateX(-100%)", "right")
                    } else {
                        ("translateX(-50%)", "center")
                    };
                    rsx! {
                        span {
                            style: "position:absolute; left:{left_pct}%; transform:{transform}; text-align:{justify};",
                            "{format_duration_ns(ns)}"
                        }
                    }
                }
            }
        }
    }
}

/// root_of returns the trace's root span (empty parent_span_id) if present,
/// falling back to the earliest start_time — a truncated/partial fetch
/// might not include the true root, but the earliest-known span is still
/// the most reasonable header to show.
fn root_of(events: &[WideEvent]) -> Option<WideEvent> {
    events
        .iter()
        .find(|e| e.parent_span_id.is_empty())
        .or_else(|| {
            events.iter().min_by_key(|e| {
                e.start_time
                    .as_ref()
                    .map(|ts| (ts.seconds, ts.nanos))
                    .unwrap_or((i64::MAX, 0))
            })
        })
        .cloned()
}

#[component]
fn WideEventDetail(event: WideEvent) -> Element {
    let status_class = status_badge_class(&event.status_code);
    let mut attrs: Vec<(String, String)> = event.attributes.clone().into_iter().collect();
    attrs.sort();
    let mut business_attrs: Vec<(String, String)> =
        event.business_attributes.clone().into_iter().collect();
    business_attrs.sort();
    let mut runtime_attrs: Vec<(String, String)> =
        event.runtime_attributes.clone().into_iter().collect();
    runtime_attrs.sort();

    rsx! {
        div { class: "border border-base-300 rounded p-3 flex flex-col gap-3 bg-base-200",
            div { class: "flex items-center gap-2 flex-wrap",
                span { class: "font-bold text-sm", "{event.span_name}" }
                span { class: "badge badge-ghost badge-sm", "{event.service_name}" }
                span { class: "{status_class}", "{event.status_code}" }
            }
            div { class: "text-xs text-base-content/60 font-mono", "trace_id={event.trace_id}" }
            div { class: "text-xs text-base-content/60 font-mono", "span_id={event.span_id}" }
            if !event.parent_span_id.is_empty() {
                div { class: "text-xs text-base-content/60 font-mono", "parent_span_id={event.parent_span_id}" }
            }
            div { class: "text-xs text-base-content/60", "duration: {format_duration_ns(event.duration_ns)}" }

            AttributeSection { title: "attributes", attrs: attrs }
            AttributeSection { title: "business_attributes", attrs: business_attrs }
            AttributeSection { title: "runtime_attributes", attrs: runtime_attrs }

            div { class: "flex flex-col gap-1",
                span { class: "text-[10px] uppercase tracking-wide text-base-content/40 font-semibold", "logs ({event.logs.len()})" }
                if event.logs.is_empty() {
                    div { class: "text-xs text-base-content/40", "No log lines" }
                } else {
                    div { class: "flex flex-col gap-1 max-h-64 overflow-auto",
                        for line in event.logs.iter() {
                            LogLineRow { line: line.clone() }
                        }
                    }
                }
            }
        }
    }
}

#[component]
fn AttributeSection(title: String, attrs: Vec<(String, String)>) -> Element {
    if attrs.is_empty() {
        return rsx! {};
    }
    rsx! {
        div { class: "flex flex-col gap-1",
            span { class: "text-[10px] uppercase tracking-wide text-base-content/40 font-semibold", "{title}" }
            for (k , v) in attrs {
                div { class: "text-xs font-mono",
                    span { class: "text-base-content/50", "{k}=" }
                    span { "{v}" }
                }
            }
        }
    }
}

#[component]
fn LogLineRow(line: LogLine) -> Element {
    let severity = line.severity.clone();
    rsx! {
        div { class: "text-xs font-mono flex gap-2 items-baseline",
            span { class: "text-base-content/40 uppercase shrink-0", "{severity}" }
            span { class: "text-base-content/80 truncate", title: "{line.body}", "{line.body}" }
        }
    }
}

fn status_badge_class(status: &str) -> &'static str {
    match status.to_ascii_uppercase().as_str() {
        "ERROR" | "STATUS_CODE_ERROR" => "badge badge-error badge-sm",
        "OK" | "STATUS_CODE_OK" => "badge badge-success badge-sm",
        _ => "badge badge-ghost badge-sm",
    }
}

/// wide_event_to_span converts a WideEvent to the Span shape FlameGraph
/// (built for Traces' GetTrace response) actually needs — every field maps
/// directly except `kind`, which WideEvent has no equivalent of and
/// FlameGraph's own layout doesn't use for anything but display.
fn wide_event_to_span(event: &WideEvent) -> Span {
    Span {
        trace_id: event.trace_id.clone(),
        span_id: event.span_id.clone(),
        parent_span_id: event.parent_span_id.clone(),
        service_name: event.service_name.clone(),
        span_name: event.span_name.clone(),
        kind: String::new(),
        start_time: event.start_time.clone(),
        duration_ns: event.duration_ns,
        status_code: event.status_code.clone(),
        attributes: event.attributes.clone(),
    }
}

fn event_time(event: &WideEvent) -> String {
    match &event.start_time {
        Some(ts) => match chrono::DateTime::from_timestamp(ts.seconds, ts.nanos as u32) {
            Some(dt) => dt.format("%Y-%m-%d %H:%M:%S%.3f").to_string(),
            None => "—".to_string(),
        },
        None => "—".to_string(),
    }
}
