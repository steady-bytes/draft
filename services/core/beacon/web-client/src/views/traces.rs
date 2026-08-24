use dioxus::prelude::*;

use draft_api::hook::core_observability_traces_v1::{
    use_traces_service_service, GetTraceRequest, SearchTracesRequest,
};
use draft_api::proto::core_observability_traces_v1::{Span, TraceRoot};

use crate::components::{format_duration_ns, FlameGraph, QueryBar, QueryGrammar};

/// Traces is Beacon's second visible product surface (Phase 6): a search list
/// of trace roots (`SearchTraces`) plus a flame-graph detail view for the
/// selected trace (`GetTrace`) — the SigNoz-style "trace list + flame graph"
/// pattern described in the design doc's Traces view section. Both RPCs are
/// unary (no StreamTraces — see TraceReceiver's doc comment in
/// services/core/beacon/ingest/traces.go for why), so this view uses the
/// generated `use_traces_service_service` hooks directly rather than a hand
/// rolled streaming loop like Stream does.
#[component]
pub fn Traces() -> Element {
    let mut expression = use_signal(String::new);
    let mut search_req = use_signal(SearchTracesRequest::default);

    let mut selected_trace_id: Signal<Option<String>> = use_signal(|| None);
    let mut get_req = use_signal(GetTraceRequest::default);
    let mut selected_span: Signal<Option<Span>> = use_signal(|| None);

    let service = use_traces_service_service();
    let search_result = service.search_traces(search_req);
    let get_result = service.get_trace(get_req);

    let mut traces: Signal<Vec<TraceRoot>> = use_signal(Vec::new);
    let mut search_error: Signal<Option<String>> = use_signal(|| None);
    let mut spans: Signal<Vec<Span>> = use_signal(Vec::new);
    let mut get_error: Signal<Option<String>> = use_signal(|| None);

    // Consume a pending Trace-pill hand-off from the Logs view (Phase 14): if
    // set, immediately filter the search list AND fetch+select that trace's
    // flame graph — one click gets you the rendered flame graph, not just a
    // filtered list needing a second click. Clearing the signal afterward
    // means this only fires once per hand-off, not on every subsequent visit
    // to /traces (including a plain nav-link click, which leaves it None).
    use_effect(move || {
        let Some(trace_id) = crate::PENDING_TRACE_ID.read().clone() else {
            return;
        };
        *crate::PENDING_TRACE_ID.write() = None;

        let filter = format!("trace_id = \"{trace_id}\"");
        expression.set(filter.clone());
        search_req.set(SearchTracesRequest {
            filter,
            limit: 0,
            before: String::new(),
        });
        selected_span.set(None);
        selected_trace_id.set(Some(trace_id.clone()));
        get_req.set(GetTraceRequest { trace_id });
    });

    // Seed `traces` from the latest SearchTraces result. A new `search_req`
    // (run/clear) re-triggers the hook's underlying use_resource, which lands
    // here again.
    use_effect(move || {
        match &*search_result.read() {
            Some(Ok(resp)) => {
                traces.set(resp.traces.clone());
                search_error.set(None);
            }
            Some(Err(e)) => search_error.set(Some(e.message().to_string())),
            None => {}
        }
    });

    // Seed `spans` from the latest GetTrace result, but only once a trace has
    // actually been selected — the hook's resource fires once on mount with
    // GetTraceRequest::default() (empty trace_id), which the server rejects
    // as CodeInvalidArgument; that's expected and not shown to the user.
    use_effect(move || {
        let result = get_result.read();
        if selected_trace_id.read().is_none() {
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

    let trace_list = traces.read();
    let is_loading_search = search_result.read().is_none();
    let selected = selected_trace_id.read().clone();

    rsx! {
        div { class: "p-4 flex flex-col gap-3 h-screen",
            div { class: "flex items-center gap-2",
                h1 { class: "text-lg font-bold shrink-0", "Traces" }
                QueryBar {
                    grammar: QueryGrammar::BeaconQl,
                    expression,
                    on_run: move |_| {
                        search_req.set(SearchTracesRequest {
                            filter: expression.peek().clone(),
                            limit: 0,
                            before: String::new(),
                        });
                    },
                    on_clear: move |_| {
                        expression.set(String::new());
                        search_req.set(SearchTracesRequest::default());
                    },
                }
            }

            if let Some(err) = search_error() {
                div { class: "text-error font-mono text-xs", "{err}" }
            }

            div { class: "flex-1 min-h-0 flex gap-4",
                // Left: trace list (root span, service, duration, status).
                div { class: "w-[420px] shrink-0 overflow-auto border border-base-300 rounded",
                    table { class: "table table-xs",
                        thead {
                            tr {
                                th { "ROOT SPAN" }
                                th { "SERVICE" }
                                th { "DURATION" }
                                th { "STATUS" }
                            }
                        }
                        tbody {
                            if trace_list.is_empty() {
                                tr {
                                    td {
                                        colspan: "4",
                                        class: "text-center text-base-content/40 py-6",
                                        if is_loading_search { "Searching…" } else { "No traces found" }
                                    }
                                }
                            }
                            for t in trace_list.iter() {
                                {
                                    let trace_id = t.trace_id.clone();
                                    let trace_id_for_click = trace_id.clone();
                                    let is_selected = selected.as_deref() == Some(trace_id.as_str());
                                    let row_class = if is_selected {
                                        "hover:bg-base-300 cursor-pointer bg-base-300"
                                    } else {
                                        "hover:bg-base-300 cursor-pointer"
                                    };
                                    let span_name = t.span_name.clone();
                                    let service_name = t.service_name.clone();
                                    let duration = format_duration_ns(t.duration_ns);
                                    let status = t.status_code.clone();
                                    let status_class = status_badge_class(&status);
                                    rsx! {
                                        tr {
                                            class: "{row_class}",
                                            onclick: move |_| {
                                                selected_span.set(None);
                                                selected_trace_id.set(Some(trace_id_for_click.clone()));
                                                get_req
                                                    .set(GetTraceRequest {
                                                        trace_id: trace_id_for_click.clone(),
                                                    });
                                            },
                                            td {
                                                class: "font-mono text-xs",
                                                title: "{trace_id}",
                                                "{span_name}"
                                            }
                                            td { class: "text-xs", "{service_name}" }
                                            td { class: "font-mono text-xs", "{duration}" }
                                            td { span { class: "{status_class}", "{status}" } }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }

                // Right: flame graph + span detail for the selected trace.
                div { class: "flex-1 min-h-0 overflow-auto flex flex-col gap-3",
                    if selected.is_none() {
                        div { class: "text-center text-base-content/40 py-12 text-sm",
                            "Select a trace to view its flame graph"
                        }
                    } else {
                        if let Some(err) = get_error() {
                            div { class: "text-error font-mono text-xs", "{err}" }
                        }
                        div { class: "border border-base-300 rounded p-3 bg-base-200",
                            FlameGraph {
                                spans: spans.read().clone(),
                                on_select: move |s: Span| selected_span.set(Some(s)),
                            }
                        }
                        if let Some(s) = selected_span() {
                            SpanDetail { span: s }
                        }
                    }
                }
            }
        }
    }
}

#[component]
fn SpanDetail(span: Span) -> Element {
    let mut attrs: Vec<(String, String)> = span.attributes.clone().into_iter().collect();
    attrs.sort();
    let status_class = status_badge_class(&span.status_code);

    rsx! {
        div { class: "border border-base-300 rounded p-3 flex flex-col gap-2 bg-base-200",
            div { class: "flex items-center gap-2",
                span { class: "font-bold text-sm", "{span.span_name}" }
                span { class: "badge badge-ghost badge-sm", "{span.service_name}" }
                span { class: "{status_class}", "{span.status_code}" }
            }
            div { class: "text-xs text-base-content/60 font-mono", "span_id={span.span_id}" }
            div { class: "text-xs text-base-content/60 font-mono", "parent_span_id={span.parent_span_id}" }
            div { class: "text-xs text-base-content/60", "kind: {span.kind}" }
            if attrs.is_empty() {
                div { class: "text-xs text-base-content/40", "No attributes" }
            } else {
                div { class: "flex flex-col gap-1 mt-1",
                    for (k , v) in attrs {
                        div { class: "text-xs font-mono",
                            span { class: "text-base-content/50", "{k}=" }
                            span { "{v}" }
                        }
                    }
                }
            }
        }
    }
}

fn status_badge_class(status: &str) -> &'static str {
    match status {
        "STATUS_CODE_ERROR" => "badge badge-error badge-sm",
        "STATUS_CODE_OK" => "badge badge-success badge-sm",
        _ => "badge badge-ghost badge-sm",
    }
}
