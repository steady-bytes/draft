use dioxus::prelude::*;

use draft_api::hook::core_observability_metrics_v1::{use_metrics_service_service, QueryMetricsRequest};
use draft_api::proto::core_observability_metrics_v1::TimeSeries;

use crate::components::{label_signature, MetricCard, MetricIcon, QueryBar, QueryGrammar, TimeSeriesChart};

/// Metrics is Beacon's third visible product surface (Phase 9): a stat-tile
/// grid — one `MetricCard` per distinct time series a `QueryMetrics` call
/// returns — plus a `TimeSeriesChart` drill-down below it, per the design
/// doc's Metrics view description. Unlike Stream/Traces, this view does not
/// auto-run a query on mount: the PromQL-subset grammar requires a concrete
/// metric name (there is no "list every metric" wildcard query — the same is
/// true of real PromQL), so there is nothing sensible to default to until the
/// operator names one.
#[component]
pub fn Metrics() -> Element {
    let mut expression = use_signal(String::new);
    let mut query_req = use_signal(QueryMetricsRequest::default);
    let mut has_run = use_signal(|| false);

    let service = use_metrics_service_service();
    let result = service.query_metrics(query_req);

    let mut series: Signal<Vec<TimeSeries>> = use_signal(Vec::new);
    let mut error: Signal<Option<String>> = use_signal(|| None);

    // Seed `series`/`error` from the latest QueryMetrics result, but only
    // once the operator has actually run a query — the hook's underlying
    // use_resource fires once on mount with QueryMetricsRequest::default()
    // (an empty query string), which the server rejects as
    // CodeInvalidArgument ("query is empty"); that's expected and not shown
    // as an error. Mirrors views/traces.rs's `selected_trace_id.is_none()`
    // guard for the same reason.
    use_effect(move || {
        let r = result.read();
        if !*has_run.read() {
            return;
        }
        match &*r {
            Some(Ok(resp)) => {
                series.set(resp.series.clone());
                error.set(None);
            }
            Some(Err(e)) => error.set(Some(e.message().to_string())),
            None => {}
        }
    });

    let run_query = move |_| {
        has_run.set(true);
        query_req.set(QueryMetricsRequest {
            query: expression.peek().clone(),
            start: String::new(),
            end: String::new(),
            step: String::new(),
        });
    };

    let series_list = series.read();
    let is_loading = *has_run.read() && result.read().is_none();

    rsx! {
        div { class: "p-4 flex flex-col gap-4 h-screen overflow-auto",
            div { class: "flex items-center gap-2",
                h1 { class: "text-lg font-bold shrink-0", "Metrics" }
                QueryBar {
                    grammar: QueryGrammar::PromQl,
                    expression,
                    on_run: run_query,
                    on_clear: move |_| {
                        expression.set(String::new());
                        has_run.set(false);
                        series.set(Vec::new());
                        error.set(None);
                    },
                }
            }

            if let Some(err) = error() {
                div { class: "text-error font-mono text-xs", "{err}" }
            }

            if !*has_run.read() {
                div { class: "text-center text-base-content/40 py-12 text-sm max-w-lg mx-auto",
                    "Run a PromQL-subset query — e.g. a bare selector like "
                    span { class: "font-mono", "my_metric_name" }
                    " or "
                    span { class: "font-mono", "rate(my_counter[5m])" }
                    " — to see live stat tiles and a chart."
                }
            } else if is_loading {
                div { class: "text-center text-base-content/40 py-12 text-sm", "Querying…" }
            } else if series_list.is_empty() {
                div { class: "text-center text-base-content/40 py-12 text-sm", "No time series matched" }
            } else {
                div {
                    class: "grid gap-3",
                    style: "grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));",
                    for s in series_list.iter() {
                        MetricStatTile { series: s.clone() }
                    }
                }
                div { class: "flex flex-col gap-2",
                    h2 { class: "text-sm font-semibold text-base-content/60", "Time series" }
                    TimeSeriesChart { series: series_list.clone() }
                }
            }
        }
    }
}

/// MetricStatTile adapts one QueryMetrics TimeSeries into Blueprint's
/// MetricCard shape: current value = the series' most recent sample, trend =
/// delta from the sample before it, sparkline = every sample's value in
/// order.
#[component]
fn MetricStatTile(series: TimeSeries) -> Element {
    let label = label_signature(&series.metric_name, &series.labels);
    let samples = &series.samples;
    let last = samples.last();
    let prev = if samples.len() >= 2 {
        samples.get(samples.len() - 2)
    } else {
        None
    };

    let value = last.map(|s| format_value(s.value)).unwrap_or_else(|| "—".to_string());
    let (trend_up, trend_delta) = match (last, prev) {
        (Some(l), Some(p)) => {
            let delta = l.value - p.value;
            (delta >= 0.0, format_value(delta.abs()))
        }
        _ => (true, "—".to_string()),
    };
    let sparkline_data: Vec<f32> = samples.iter().map(|s| s.value as f32).collect();
    let icon = icon_for_metric(&series.metric_name);

    rsx! {
        MetricCard {
            icon,
            label,
            value,
            unit: None,
            trend_up,
            trend_good: trend_up,
            trend_delta,
            sparkline_data,
            sparkline_color: "#58a6ff".to_string(),
        }
    }
}

/// icon_for_metric picks a MetricCard icon from a lightweight keyword match
/// on the metric name — purely cosmetic, no query-language meaning — falling
/// back to a generic server icon for anything unrecognized.
fn icon_for_metric(metric_name: &str) -> MetricIcon {
    let name = metric_name.to_lowercase();
    if name.contains("error") || name.contains("warn") || name.contains("fail") {
        MetricIcon::Warning
    } else if name.contains("duration") || name.contains("latency") || name.contains("time") {
        MetricIcon::Clock
    } else if name.contains("request") || name.contains("message") || name.contains("event") {
        MetricIcon::Messages
    } else if name.contains("user") || name.contains("session") || name.contains("connection") {
        MetricIcon::Users
    } else if name.ends_with("_count") || name.ends_with("_total") {
        MetricIcon::ArrowUp
    } else {
        MetricIcon::Server
    }
}

fn format_value(v: f64) -> String {
    if v.abs() >= 1_000_000.0 {
        format!("{:.2}M", v / 1_000_000.0)
    } else if v.abs() >= 1_000.0 {
        format!("{:.2}k", v / 1_000.0)
    } else if v.abs() >= 100.0 {
        format!("{v:.1}")
    } else {
        format!("{v:.3}")
    }
}
