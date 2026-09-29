//! Traces: a search list of trace roots (`SearchTraces`), the selected trace as a waterfall
//! (`GetTrace`) and a drawer for the selected span. Both RPCs are unary — there is no
//! `StreamTraces` — so the generated hooks are used directly.

use chrono::Utc;
use dioxus::prelude::*;
use draft_api::hook::core_observability_traces_v1::{use_traces_service_service, GetTraceRequest, SearchTracesRequest};
use draft_api::hook::core_observability_wide_events_v1::use_wide_events_service_service;
use draft_api::proto::core_observability_traces_v1::{Span, TraceRoot};
use draft_api::proto::core_observability_wide_events_v1::{GetWideEventRequest, WideEvent};
use draft_ui::data::{DurationCell, Kv, KvItem};
use draft_ui::layout::{Drawer, DrawerBlock, PageHead, Split};
use draft_ui::query::{GrammarId, QueryBar, QueryFilters};
use draft_ui::shell::{use_page_chrome, BarItem, Chrome, ChromeStatus};
use draft_ui::ui::{Alert, Btn, BtnVariant, Empty, Loading, Status, Svc, Toggle};
use draft_ui::util::{format_duration_ns, truncate_middle};
use draft_ui::viz::{Legend, Waterfall};
use draft_ui::{StatusKind, Tone};

use crate::components::{use_open_logs, AttrBlock, StatusText};
use crate::data::{beaconql_string, full_time, row_time, sorted_attrs, ts_nanos, waterfall, window_clause, with_window, SpanLike, SpanStatus};
use crate::range::{TimeRange, TimeRangePicker};

/// The time-range presets, in minutes.
const PRESETS: [i64; 4] = [15, 60, 360, 1440];

fn request(expression: &str, range: &TimeRange) -> SearchTracesRequest {
    let (start, end) = range.bounds(Utc::now());
    SearchTracesRequest { filter: with_window(expression, &window_clause(start, end)), limit: 0, before: String::new() }
}

fn to_span(s: &Span) -> SpanLike {
    SpanLike {
        id: s.span_id.clone(),
        parent: s.parent_span_id.clone(),
        service: s.service_name.clone(),
        name: s.span_name.clone(),
        start_ns: i128::from(ts_nanos(&s.start_time).unwrap_or(0)),
        duration_ns: s.duration_ns,
        err: SpanStatus::of(&s.status_code) == SpanStatus::Error,
    }
}

#[component]
pub fn Traces() -> Element {
    let mut expression = use_signal(String::new);
    let mut range = use_signal(|| TimeRange::minutes(60));
    let mut errors_only = use_signal(|| false);
    let mut search_req = use_signal(|| request("", &TimeRange::minutes(60)));

    let mut selected_trace: Signal<Option<String>> = use_signal(|| None);
    let mut get_req = use_signal(GetTraceRequest::default);
    let mut selected_span: Signal<Option<String>> = use_signal(|| None);
    let mut wide_event_req = use_signal(GetWideEventRequest::default);

    let service = use_traces_service_service();
    let search_result = service.search_traces(search_req);
    let get_result = service.get_trace(get_req);
    let wide_event_service = use_wide_events_service_service();
    let wide_event_result = wide_event_service.get_wide_event(wide_event_req);

    let mut traces: Signal<Vec<TraceRoot>> = use_signal(Vec::new);
    let mut search_error: Signal<Option<String>> = use_signal(|| None);
    let mut spans: Signal<Vec<Span>> = use_signal(Vec::new);
    let mut get_error: Signal<Option<String>> = use_signal(|| None);
    // The selected span's WideEvent, if one was recorded for it — see the field's own doc comment
    // on why this is often None even for a span that clearly ran (WideEvent production is
    // fire-and-forget and best-effort, unlike the trace/span data itself).
    let mut wide_event: Signal<Option<WideEvent>> = use_signal(|| None);
    let open_logs = use_open_logs();

    // Selecting a span clears any previous span's WideEvent before asking for the new one's, so a
    // stale "business attributes" panel never flashes under a freshly selected, different span
    // while its own (possibly empty) result is still in flight — the same clear-then-refetch order
    // select_trace below already uses for the trace's own spans.
    let select_span = use_callback(move |span_id: String| {
        wide_event.set(None);
        wide_event_req.set(GetWideEventRequest { span_id: span_id.clone() });
        selected_span.set(Some(span_id));
    });

    let select_trace = use_callback(move |trace_id: String| {
        selected_span.set(None);
        wide_event.set(None);
        spans.set(Vec::new());
        selected_trace.set(Some(trace_id.clone()));
        get_req.set(GetTraceRequest { trace_id });
    });

    // A trace handed off from Logs or Wide events: filter the list to it and open its waterfall
    // in one step, then clear the hand-off so it fires once rather than on every later visit. The
    // list search skips the time window: the trace may be older than the default one.
    use_effect(move || {
        let Some(trace_id) = crate::PENDING_TRACE_ID.read().clone() else {
            return;
        };
        *crate::PENDING_TRACE_ID.write() = None;
        let filter = format!("trace_id = {}", beaconql_string(&trace_id));
        expression.set(filter.clone());
        search_req.set(SearchTracesRequest { filter, limit: 0, before: String::new() });
        select_trace.call(trace_id);
    });

    // Seed the list from the latest search.
    use_effect(move || match &*search_result.read() {
        Some(Ok(resp)) => {
            traces.set(resp.traces.clone());
            search_error.set(None);
        }
        Some(Err(e)) => search_error.set(Some(e.message().to_string())),
        None => {}
    });

    // Seed the spans, but only once a trace is selected: the resource fires once on mount with an
    // empty trace id, which the server rejects as InvalidArgument — expected, and not shown.
    use_effect(move || {
        let result = get_result.read();
        if selected_trace.read().is_none() {
            return;
        }
        match &*result {
            Some(Ok(resp)) => {
                spans.set(resp.spans.clone());
                get_error.set(None);
            }
            Some(Err(e)) => get_error.set(Some(e.message().to_string())),
            None => {}
        }
    });

    // Seed the selected span's WideEvent. A miss (no WideEvent was ever recorded for this span_id)
    // comes back as `Ok` with an empty/default `event` rather than an error (see
    // services/core/beacon/store/clickhouse.go's GetWideEvent: a query that matches no row is not
    // itself a failure) — sorted_attrs on its empty maps then yields empty vecs, and AttrBlock
    // renders nothing for them, so a plain miss draws no error and no empty section, just the
    // ordinary attributes the span itself always has.
    use_effect(move || {
        if let Some(Ok(resp)) = &*wide_event_result.read() {
            wide_event.set(resp.event.clone());
        }
    });

    let run = use_callback(move |_: ()| search_req.set(request(&expression.peek(), &range.peek())));

    // Derived ---------------------------------------------------------------------------------
    let all = traces();
    let loading = search_result.read().is_none();
    let with_errors = all.iter().filter(|t| SpanStatus::of(&t.status_code) == SpanStatus::Error).count();
    let shown: Vec<TraceRoot> = all
        .iter()
        .filter(|t| !errors_only() || SpanStatus::of(&t.status_code) == SpanStatus::Error)
        .cloned()
        .collect();
    let summary = format!("{} traces · {} with errors · {}", all.len(), with_errors, range().label());
    let max_duration = shown.iter().map(|t| t.duration_ns).max().unwrap_or(1).max(1);
    let now = Utc::now();
    let current = selected_trace();

    use_page_chrome(move || {
        let n = traces.read().len();
        Chrome {
            crumbs: vec!["Beacon".into(), "Signals".into(), "Traces".into()],
            status: Some(ChromeStatus::new(StatusKind::Ok, format!("{n} traces loaded"))),
            left: vec![BarItem::kv("Source", "clickhouse")],
            right: vec![BarItem::Text("Search only".into())],
        }
    });

    // The selected trace as a waterfall.
    let trace_spans = spans();
    let like: Vec<SpanLike> = trace_spans.iter().map(to_span).collect();
    let layout = waterfall(&like);
    let span_total = like.len();
    let by_id: std::collections::HashMap<&str, &Span> = trace_spans.iter().map(|s| (s.span_id.as_str(), s)).collect();
    let selected_id = selected_span();
    let position = selected_id.as_deref().and_then(|id| layout.rows.iter().position(|r| r.id == id));
    let mut services: Vec<String> = trace_spans.iter().map(|s| s.service_name.clone()).collect();
    services.sort();
    services.dedup();
    let mut legend: Vec<(Tone, String)> = services.iter().map(|s| (Tone::for_name(s), s.clone())).collect();
    if like.iter().any(|s| s.err) {
        legend.push((Tone::Err, "error".to_string()));
    }
    let trace_label = current.as_deref().map(|id| truncate_middle(id, 8, 4)).unwrap_or_default();
    let head = format!("Waterfall · trace {trace_label} · {span_total} spans · {}", format_duration_ns(layout.total_ns));

    // The selected span's WideEvent business/runtime attributes, when one was recorded for this
    // exact span (matched by span_id, not just "the most recently fetched one" — a stale response
    // for a since-deselected or since-reselected span never gets here: wide_event is cleared by
    // select_span/select_trace before the new fetch is even issued).
    let we = wide_event();
    let (business, runtime) = we
        .as_ref()
        .filter(|e| Some(e.span_id.as_str()) == selected_id.as_deref())
        .map(|e| (sorted_attrs(&e.business_attributes), sorted_attrs(&e.runtime_attributes)))
        .unwrap_or_default();

    let drawer = selected_id.as_deref().and_then(|id| by_id.get(id).copied()).map(|span| {
        let offset = layout.offsets.get(span.span_id.as_str()).copied().unwrap_or(0);
        let parent = by_id.get(span.parent_span_id.as_str()).map(|p| format!("{} · {}", p.span_name, p.span_id));
        let trace_id = span.trace_id.clone();
        rsx! {
            SpanDrawer {
                key: "{span.span_id}",
                span: span.clone(),
                position: position.map(|p| p + 1).unwrap_or(0),
                total: span_total,
                offset_ns: offset,
                parent,
                business: business.clone(),
                runtime: runtime.clone(),
                on_close: move |_| selected_span.set(None),
                on_open_logs: move |_| open_logs.call(format!("trace_id = {}", beaconql_string(&trace_id))),
            }
        }
    });

    rsx! {
        Split { flush: true, drawer,
            PageHead { inline: true, title: "Traces".to_string(),
                span { class: "d-label", "{summary}" }
                span { class: "d-spacer" }
                Toggle { checked: errors_only(), on_change: move |on| errors_only.set(on), "Errors only" }
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
                grammar: GrammarId::BeaconTraces,
                value: expression,
                on_run: move |_| run.call(()),
                on_clear: move |_| {
                    expression.set(String::new());
                    run.call(());
                },
                error: search_error(),
            }
            QueryFilters { grammar: GrammarId::BeaconTraces, expression, on_run: move |_| run.call(()) }

            div { style: "margin-top:16px",
                if loading && all.is_empty() {
                    Loading {}
                } else if shown.is_empty() {
                    Empty { title: "No traces".to_string(), "Nothing matched this query in the selected range." }
                } else {
                    div { class: "d-panel d-table-wrap bc-scroll",
                        table { class: "d-table bc-table",
                            thead {
                                tr {
                                    th { "Root span" }
                                    th { "Service" }
                                    th { "Duration" }
                                    th { "Status" }
                                    th { class: "is-right", "Started" }
                                }
                            }
                            tbody {
                                for t in shown.iter() {
                                    {
                                        let id = t.trace_id.clone();
                                        let id_for_key = t.trace_id.clone();
                                        let is_selected = current.as_deref() == Some(t.trace_id.as_str());
                                        let err = SpanStatus::of(&t.status_code) == SpanStatus::Error;
                                        let duration = format_duration_ns(t.duration_ns);
                                        let ratio = t.duration_ns as f64 / max_duration as f64;
                                        let tone = if err { Some(Tone::Err) } else { None };
                                        let started = row_time(&t.start_time, now);
                                        rsx! {
                                            tr {
                                                key: "{t.trace_id}",
                                                class: if err { "is-error" } else { "" },
                                                aria_selected: "{is_selected}",
                                                tabindex: "0",
                                                onclick: move |_| select_trace.call(id.clone()),
                                                onkeydown: move |k| {
                                                    if k.key() == Key::Enter || k.key() == Key::Character(" ".to_string()) {
                                                        k.prevent_default();
                                                        select_trace.call(id_for_key.clone());
                                                    }
                                                },
                                                td { class: "bc-body", span { class: "d-trunc", title: "{t.trace_id}", "{t.span_name}" } }
                                                td { class: "bc-fit", Svc { name: t.service_name.clone() } }
                                                td { class: "bc-fit", DurationCell { text: duration, ratio, tone } }
                                                td { class: "bc-fit", StatusText { code: t.status_code.clone() } }
                                                td { class: "bc-time is-right", "{started}" }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }

            if current.is_some() {
                div { class: "d-panel", style: "margin-top:16px",
                    div { class: "d-panel-head",
                        span { class: "d-label", "{head}" }
                        span { class: "d-spacer" }
                        Legend { items: legend }
                    }
                    if let Some(err) = get_error() {
                        div { class: "d-panel-body", Alert { kind: StatusKind::Err, "{err}" } }
                    } else if like.is_empty() {
                        div { class: "d-panel-body", Loading {} }
                    } else {
                        Waterfall {
                            rows: layout.rows,
                            ticks: layout.ticks,
                            selected: selected_id.clone(),
                            on_select: move |id: String| select_span.call(id),
                        }
                    }
                }
            } else if !shown.is_empty() {
                p { class: "d-muted", style: "margin-top:16px", "Select a trace to see its waterfall." }
            }
        }
    }
}

/// The span drawer: identity, attributes, and a way to the trace's logs. `business`/`runtime` are
/// the matching WideEvent's own attribute maps (see the Traces component's `wide_event` signal) —
/// empty whenever no WideEvent was recorded for this span, which `AttrBlock` renders as no block at
/// all rather than an empty one.
#[component]
fn SpanDrawer(
    span: Span,
    position: usize,
    total: usize,
    offset_ns: u64,
    parent: Option<String>,
    business: Vec<(String, String)>,
    runtime: Vec<(String, String)>,
    on_close: EventHandler<()>,
    on_open_logs: EventHandler<()>,
) -> Element {
    let status = SpanStatus::of(&span.status_code);
    let attrs = sorted_attrs(&span.attributes);
    let when = full_time(&span.start_time);
    let subtitle = format!("{} · {} · starts at +{}", span.service_name, format_duration_ns(span.duration_ns), format_duration_ns(offset_ns));
    let lead = rsx! {
        Status { kind: status.kind(), "{status.label()}" }
        span { class: "d-label", "Span {position} of {total}" }
    };
    rsx! {
        Drawer {
            label: "Span detail".to_string(),
            on_close: move |_| on_close.call(()),
            lead,
            title: span.span_name.clone(),
            subtitle,
            DrawerBlock { title: "Identity".to_string(),
                Kv {
                    KvItem { label: "span_id".to_string(), "{span.span_id}" }
                    if let Some(p) = parent {
                        KvItem { label: "parent".to_string(), "{p}" }
                    }
                    if !span.kind.is_empty() {
                        KvItem { label: "kind".to_string(), "{span.kind}" }
                    }
                    KvItem { label: "started".to_string(), "{when} UTC" }
                }
            }
            DrawerBlock { title: "Attributes".to_string(),
                if attrs.is_empty() {
                    span { class: "d-muted", "No attributes" }
                } else {
                    Kv {
                        for (k , v) in attrs {
                            KvItem { key: "{k}", label: k.clone(), "{v}" }
                        }
                    }
                }
            }
            AttrBlock { title: "Business attributes".to_string(), attrs: business }
            AttrBlock { title: "Runtime attributes".to_string(), attrs: runtime }
            div { class: "bc-actions",
                Btn { variant: BtnVariant::Primary, onclick: move |_| on_open_logs.call(()), "Logs for this trace" }
            }
        }
    }
}
