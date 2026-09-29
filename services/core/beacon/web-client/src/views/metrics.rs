//! Metrics: a PromQL-subset query, four stat tiles derived from the result, a time-series chart
//! with a crosshair and toggleable series, and a per-series table. Nothing runs until the
//! operator names a metric: the grammar needs one (as in real PromQL there is no "list every
//! metric"), so there is nothing sensible to default to.

use chrono::Utc;
use dioxus::prelude::*;
use draft_api::hook::core_observability_metrics_v1::{use_metrics_service_service, QueryMetricsRequest};
use draft_api::proto::core_observability_metrics_v1::TimeSeries;
use draft_ui::data::{Delta, StatTile};
use draft_ui::layout::PageHead;
use draft_ui::query::{GrammarId, QueryBar, QueryFilters};
use draft_ui::shell::{use_page_chrome, BarItem, Chrome, ChromeStatus};
use draft_ui::ui::{Empty, Loading, Toggle};
use draft_ui::util::format_clock;
use draft_ui::viz::{ChartSeries, SeriesTable, TimeSeriesChart};
use draft_ui::{StatusKind, Tone};
use gloo_timers::future::TimeoutFuture;

use crate::data::{format_value, label_signature};
use crate::range::{TimeRange, TimeRangePicker};

/// The time-range presets, in minutes.
const PRESETS: [i64; 4] = [15, 60, 180, 1440];

/// How often the Live switch re-runs the query.
const LIVE_INTERVAL_MS: u32 = 15_000;

fn request(query: &str, range: &TimeRange) -> QueryMetricsRequest {
    let (start, end) = range.bounds(Utc::now());
    QueryMetricsRequest {
        query: query.to_string(),
        start: start.to_rfc3339(),
        end: end.map(|e| e.to_rfc3339()).unwrap_or_default(),
        step: String::new(),
    }
}

/// One `QueryMetrics` series as a chart series: samples as `(unix seconds, value)`.
fn chart_series(index: usize, s: &TimeSeries, visible: bool) -> ChartSeries {
    let points = s.samples.iter().filter_map(|p| p.timestamp.as_ref().map(|t| (t.seconds as f64, p.value))).collect();
    ChartSeries { name: label_signature(&s.metric_name, &s.labels), tone: Tone::series(index), points, visible }
}

#[component]
pub fn Metrics() -> Element {
    let mut expression = use_signal(String::new);
    let mut range = use_signal(|| TimeRange::minutes(180));
    let mut live = use_signal(|| false);
    let mut query_req = use_signal(QueryMetricsRequest::default);
    let mut has_run = use_signal(|| false);
    // Series switched off in the table, by index. Reset whenever a new query is run.
    let mut hidden: Signal<Vec<usize>> = use_signal(Vec::new);

    let service = use_metrics_service_service();
    let result = service.query_metrics(query_req);
    let mut series: Signal<Vec<TimeSeries>> = use_signal(Vec::new);
    let mut error: Signal<Option<String>> = use_signal(|| None);

    // Seed from the latest result, but only once a query has been run: the resource fires once on
    // mount with an empty query, which the server rejects ("query is empty") — expected, not shown.
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

    let run = use_callback(move |_: ()| {
        has_run.set(true);
        hidden.set(Vec::new());
        query_req.set(request(&expression.peek(), &range.peek()));
    });

    // Live: re-run on an interval while the switch is on and a query has been run. The request
    // carries a fresh window start each time, so the resource sees a new value and refetches; the
    // hidden-series choice is kept.
    use_future(move || async move {
        loop {
            TimeoutFuture::new(LIVE_INTERVAL_MS).await;
            if *live.peek() && *has_run.peek() && range.peek().is_trailing() {
                query_req.set(request(&expression.peek(), &range.peek()));
            }
        }
    });

    // Derived ---------------------------------------------------------------------------------
    let list = series();
    let loading = *has_run.read() && result.read().is_none() && list.is_empty();
    let hidden_now = hidden();
    let chart: Vec<ChartSeries> =
        list.iter().enumerate().map(|(i, s)| chart_series(i, s, !hidden_now.contains(&i))).collect();

    let samples: usize = list.iter().map(|s| s.samples.len()).sum();
    let steps = list.iter().map(|s| s.samples.len()).max().unwrap_or(0);
    let total_now: f64 = list.iter().filter_map(|s| s.samples.last()).map(|p| p.value).sum();
    let peak = list
        .iter()
        .flat_map(|s| s.samples.iter())
        .filter_map(|p| p.timestamp.as_ref().map(|t| (t.seconds, p.value)))
        .fold(None::<(i64, f64)>, |best, cur| match best {
            Some(b) if b.1 >= cur.1 => Some(b),
            _ => Some(cur),
        });
    let peak_text = peak.map(|p| format_value(p.1)).unwrap_or_else(|| "—".to_string());
    let peak_at = peak.map(|p| format!("at {} UTC", format_clock(p.0)));
    let summary = format!("{} series · {}", list.len(), range().label());

    use_page_chrome(move || {
        let status = if *has_run.read() { ChromeStatus::new(StatusKind::Ok, format!("{} series", series.read().len())) } else { ChromeStatus::new(StatusKind::Idle, "No query") };
        Chrome {
            crumbs: vec!["Beacon".into(), "Signals".into(), "Metrics".into()],
            status: Some(status),
            left: vec![BarItem::kv("Source", "clickhouse")],
            right: vec![BarItem::kv("Live", if live() { "on" } else { "off" })],
        }
    });

    rsx! {
        PageHead { inline: true, title: "Metrics".to_string(),
            span { class: "d-label", "{summary}" }
            span { class: "d-spacer" }
            Toggle { checked: live(), disabled: !range().is_trailing(), on_change: move |on| live.set(on), "Live" }
            TimeRangePicker {
                value: range(),
                presets: PRESETS.to_vec(),
                on_change: move |r| {
                    range.set(r);
                    if *has_run.peek() {
                        run.call(());
                    }
                },
            }
        }

        QueryBar {
            grammar: GrammarId::PromQl,
            value: expression,
            on_run: move |_| run.call(()),
            on_clear: move |_| {
                expression.set(String::new());
                has_run.set(false);
                series.set(Vec::new());
                error.set(None);
            },
            error: error(),
        }
        QueryFilters { grammar: GrammarId::PromQl, expression, on_run: move |_| run.call(()) }

        if !*has_run.read() {
            Empty { title: "Run a query".to_string(),
                "Name a metric — a bare selector such as my_metric_name, or rate(my_counter[5m]) — to see stat tiles and a chart."
            }
        } else if loading {
            Loading {}
        } else if list.is_empty() {
            Empty { title: "No time series matched".to_string(), "The query ran, but returned no series in this range." }
        } else {
            div { class: "d-stats", style: "margin-top:16px",
                StatTile { label: "Total now".to_string(), value: format_value(total_now) }
                StatTile {
                    label: "Peak".to_string(),
                    value: peak_text,
                    delta: peak_at.map(Delta::neutral),
                }
                StatTile { label: "Series".to_string(), value: list.len().to_string() }
                StatTile {
                    label: "Samples".to_string(),
                    value: samples.to_string(),
                    delta: Delta::neutral(format!("{steps} steps × {} series", list.len())),
                }
            }

            div { class: "d-panel", style: "margin-top:12px",
                div { class: "d-panel-head",
                    span { class: "d-label", "Time series" }
                    span { class: "d-spacer" }
                    span { class: "d-label", "Hover for values" }
                }
                div { class: "d-panel-body",
                    TimeSeriesChart { series: chart.clone(), label: "Line chart of the query's series over time".to_string() }
                }
            }

            div { style: "margin-top:12px",
                SeriesTable {
                    series: chart,
                    on_toggle: move |i: usize| {
                        let mut h = hidden.write();
                        if let Some(pos) = h.iter().position(|&x| x == i) {
                            h.remove(pos);
                        } else {
                            h.push(i);
                        }
                    },
                }
            }
        }
    }
}
