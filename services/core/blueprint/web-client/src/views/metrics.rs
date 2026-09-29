//! Events metrics: Catalyst's throughput and latency for a window, its resource counters, and the
//! per-type breakdown. Polled every few seconds.
//!
//! Only measured values are shown. Catalyst has no time-series endpoint, so there is no trend to
//! draw: the old page's sparklines were hard-coded placeholder arrays and its error-rate tile was
//! always empty, and neither is carried over.

use std::collections::{HashMap, HashSet};

use chrono::Utc;
use dioxus::prelude::*;
use draft_api::proto::core_message_broker_actors_v1::{
    metrics_client::MetricsClient, resource_metrics_client::ResourceMetricsClient, topology_client::TopologyClient, EdgeVolume,
    GetMetricsRequest, GetResourceMetricsRequest, GetTopologyRequest, TopologyEdge,
};
use draft_ui::data::{Delta, Meter, StatTile};
use draft_ui::layout::{PageHead, SectionTitle};
use draft_ui::shell::{use_page_chrome, BarItem, Chrome, ChromeStatus};
use draft_ui::ui::{Dot, Empty, Loading, Seg};
use draft_ui::util::{format_count, relative_time};
use draft_ui::{StatusKind, Tone};
use gloo_timers::future::TimeoutFuture;
use tonic_web_wasm_client::Client as WasmClient;

const POLL_MS: u32 = 5_000;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum Range {
    Min15,
    Hour1,
    Hour3,
    Hour24,
    All,
}

impl Range {
    const ALL: [Range; 5] = [Range::Min15, Range::Hour1, Range::Hour3, Range::Hour24, Range::All];

    fn label(self) -> &'static str {
        match self {
            Range::Min15 => "15m",
            Range::Hour1 => "1h",
            Range::Hour3 => "3h",
            Range::Hour24 => "24h",
            Range::All => "All",
        }
    }

    fn description(self) -> &'static str {
        match self {
            Range::Min15 => "last 15 minutes",
            Range::Hour1 => "last hour",
            Range::Hour3 => "last 3 hours",
            Range::Hour24 => "last 24 hours",
            Range::All => "all time",
        }
    }

    /// The window in seconds; -1 means no time filter (every stored event).
    fn window_seconds(self) -> i32 {
        match self {
            Range::Min15 => 900,
            Range::Hour1 => 3_600,
            Range::Hour3 => 10_800,
            Range::Hour24 => 86_400,
            Range::All => -1,
        }
    }
}

#[derive(Clone, Default)]
struct Throughput {
    msgs_per_min: u32,
    median_ms: f64,
    p95_ms: f64,
    volumes: Vec<EdgeVolume>,
    edges: Vec<TopologyEdge>,
}

#[derive(Clone, Default)]
struct Resources {
    published: u64,
    dropped: u64,
    queue_depth: u32,
    queue_capacity: u32,
    consumers: u32,
    producers: u32,
    flush_p95_ms: f64,
    flush_count: u32,
}

/// One event type's row: how many events, from how many producers, to how many consumers.
#[derive(Clone, Debug, PartialEq)]
struct TopicRow {
    event_type: String,
    events: u32,
    producers: u32,
    consumers: u32,
}

/// Groups the per-edge volumes by event type, busiest first.
fn topic_rows(volumes: &[EdgeVolume], edges: &[TopologyEdge]) -> Vec<TopicRow> {
    let mut consumers: HashMap<&str, HashSet<&str>> = HashMap::new();
    for e in edges {
        consumers.entry(e.event_type.as_str()).or_default().insert(e.consumer_source.as_str());
    }
    let mut by_type: HashMap<&str, (u32, HashSet<&str>)> = HashMap::new();
    for v in volumes {
        let entry = by_type.entry(v.event_type.as_str()).or_default();
        entry.0 += v.count;
        entry.1.insert(v.source.as_str());
    }
    let mut rows: Vec<TopicRow> = by_type
        .into_iter()
        .map(|(t, (events, sources))| TopicRow {
            event_type: t.to_string(),
            events,
            producers: sources.len() as u32,
            consumers: consumers.get(t).map(|s| s.len() as u32).unwrap_or(0),
        })
        .collect();
    rows.sort_by(|a, b| b.events.cmp(&a.events).then_with(|| a.event_type.cmp(&b.event_type)));
    rows
}

/// Events per minute over a window, or `None` for "all time" (no window to divide by).
fn per_minute(events: u32, window_seconds: i32) -> Option<u32> {
    (window_seconds > 0).then(|| (f64::from(events) / (f64::from(window_seconds) / 60.0)).round() as u32)
}

#[component]
pub fn Metrics() -> Element {
    let mut range = use_signal(|| Range::Hour3);
    let mut throughput: Signal<Option<Throughput>> = use_signal(|| None);
    let mut resources: Signal<Option<Resources>> = use_signal(|| None);
    let mut failed = use_signal(|| false);
    let mut updated_at: Signal<Option<i64>> = use_signal(|| None);
    let mut tick = use_signal(|| 0u32);

    // A tick every few seconds; the fetch effect reads it, so it re-runs each time.
    use_future(move || async move {
        loop {
            TimeoutFuture::new(POLL_MS).await;
            tick += 1;
        }
    });

    use_effect(move || {
        let _ = tick();
        let window = range().window_seconds();
        let host = crate::CATALYST_DOMAIN.clone();
        let host_for_resources = host.clone();
        spawn(async move {
            let metrics = MetricsClient::new(WasmClient::new(host.clone())).get_metrics(GetMetricsRequest { window_seconds: window }).await;
            let edges = TopologyClient::new(WasmClient::new(host))
                .get_topology(GetTopologyRequest {})
                .await
                .ok()
                .map(|t| t.into_inner().edges)
                .unwrap_or_default();
            match metrics {
                Ok(resp) => {
                    let r = resp.into_inner();
                    throughput.set(Some(Throughput {
                        msgs_per_min: r.total_messages_per_min,
                        median_ms: r.median_latency_ms,
                        p95_ms: r.p95_latency_ms,
                        volumes: r.edge_volumes,
                        edges,
                    }));
                    updated_at.set(Some(Utc::now().timestamp()));
                    failed.set(false);
                }
                Err(_) => failed.set(true),
            }
        });
        // Resource counters are point-in-time and independent of the window.
        spawn(async move {
            if let Ok(resp) = ResourceMetricsClient::new(WasmClient::new(host_for_resources)).get_resource_metrics(GetResourceMetricsRequest {}).await {
                let r = resp.into_inner();
                resources.set(Some(Resources {
                    published: r.messages_published_total,
                    dropped: r.messages_dropped_total,
                    queue_depth: r.queue_depth,
                    queue_capacity: r.queue_capacity,
                    consumers: r.active_consumers,
                    producers: r.active_producers,
                    flush_p95_ms: r.store_flush_p95_ms,
                    flush_count: r.store_flush_count,
                }));
            }
        });
    });

    use_page_chrome(move || {
        let status = if failed() { ChromeStatus::new(StatusKind::Err, "Catalyst unreachable") } else { ChromeStatus::live("Catalyst · polling") };
        Chrome {
            crumbs: vec!["Blueprint".into(), "Events".into(), "Metrics".into()],
            status: Some(status),
            left: vec![BarItem::kv("Source", "catalyst")],
            right: vec![BarItem::kv("Refresh", format!("{}s", POLL_MS / 1000))],
        }
    });

    let window = range().window_seconds();
    let updated = match updated_at() {
        Some(t) => relative_time((Utc::now().timestamp() - t).max(0)),
        None => "loading…".to_string(),
    };
    let summary = format!("{} · updated {}", range().description(), updated);

    rsx! {
        PageHead { inline: true, title: "Metrics".to_string(),
            span { class: "d-label", "{summary}" }
            span { class: "d-spacer" }
            Seg::<Range> {
                options: Range::ALL.iter().map(|r| (*r, r.label().to_string())).collect::<Vec<_>>(),
                value: range(),
                on_change: move |r| {
                    throughput.set(None);
                    range.set(r);
                },
                label: "Time range".to_string(),
            }
        }

        if failed() && throughput().is_none() {
            Empty { title: "Catalyst is not answering".to_string(), "Metrics come from Catalyst. Check that it is running." }
        } else if let Some(t) = throughput() {
            {
                let topics = topic_rows(&t.volumes, &t.edges);
                let max_events = topics.first().map(|r| r.events).unwrap_or(1).max(1);
                rsx! {
                    SectionTitle { "Event metrics" }
                    div { class: "d-stats",
                        StatTile { label: "Messages / min".to_string(), value: format_count(u64::from(t.msgs_per_min)), delta: Delta::neutral("live") }
                        StatTile { label: "Median latency".to_string(), value: format!("{:.1}", t.median_ms), unit: "ms".to_string() }
                        StatTile { label: "P95 latency".to_string(), value: format!("{:.1}", t.p95_ms), unit: "ms".to_string() }
                    }
                    {resource_section(resources())}
                    div { class: "d-panel", style: "margin-top:24px",
                        div { class: "d-panel-head",
                            span { class: "d-label", "Event types · {topics.len()} in {range().description()}" }
                        }
                        if topics.is_empty() {
                            Empty { title: "No events".to_string(), compact: true, "Nothing was published in this window." }
                        } else {
                            div { class: "d-table-wrap",
                                table { class: "d-table",
                                    thead {
                                        tr {
                                            th { "Event type" }
                                            th { class: "is-right", "Producers" }
                                            th { class: "is-right", "Consumers" }
                                            th { class: "is-right", "Events" }
                                            if window > 0 {
                                                th { class: "is-right", "/ min" }
                                            }
                                            th { style: "width:160px", "Share" }
                                        }
                                    }
                                    tbody {
                                        for row in topics {
                                            {
                                                let tone = Tone::for_name(&row.event_type);
                                                let has_consumers = row.consumers > 0;
                                                let dot = if has_consumers { Tone::Primary } else { Tone::Err };
                                                let dot_title = if has_consumers { "Has consumers" } else { "No consumers: events go nowhere" };
                                                let per_min = per_minute(row.events, window).map(|n| n.to_string()).unwrap_or_default();
                                                let percent = f64::from(row.events) / f64::from(max_events) * 100.0;
                                                let events = format_count(u64::from(row.events));
                                                rsx! {
                                                    tr { key: "{row.event_type}",
                                                        td { span { class: "d-row", style: "gap:8px", title: "{dot_title}", Dot { tone: dot } "{row.event_type}" } }
                                                        td { class: "is-right is-dim", "{row.producers}" }
                                                        td { class: "is-right is-dim", "{row.consumers}" }
                                                        td { class: "is-right", "{events}" }
                                                        if window > 0 {
                                                            td { class: "is-right is-dim", "{per_min}" }
                                                        }
                                                        td { Meter { percent, tone, label: format!("{} share of the busiest type", row.event_type) } }
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
        } else {
            Loading {}
        }
    }
}

fn resource_section(resources: Option<Resources>) -> Element {
    let Some(r) = resources else {
        return rsx! {};
    };
    let capacity = r.queue_capacity.max(1);
    let percent = (f64::from(r.queue_depth) / f64::from(capacity) * 100.0).round() as u32;
    let queue_delta = if percent >= 80 {
        Delta::bad(format!("{percent}% full"))
    } else {
        Delta::neutral(format!("{percent}% full"))
    };
    let dropped_delta = if r.dropped > 0 { Delta::bad("since start") } else { Delta::good("none dropped") };
    rsx! {
        div { style: "margin-top:24px" }
        SectionTitle { "Resource metrics" }
        div { class: "d-stats",
            StatTile { label: "Queue depth".to_string(), value: format_count(u64::from(r.queue_depth)), unit: format!("/ {}", format_count(u64::from(r.queue_capacity))), delta: queue_delta }
            StatTile { label: "Active producers".to_string(), value: r.producers.to_string() }
            StatTile { label: "Active consumers".to_string(), value: r.consumers.to_string() }
            StatTile { label: "Published".to_string(), value: format_count(r.published), delta: Delta::neutral("since start") }
            StatTile { label: "Dropped".to_string(), value: format_count(r.dropped), delta: dropped_delta }
            StatTile { label: "Store flush p95".to_string(), value: format!("{:.1}", r.flush_p95_ms), unit: "ms".to_string() }
            StatTile { label: "Store flushes".to_string(), value: format_count(u64::from(r.flush_count)), delta: Delta::neutral("since start") }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn volume(source: &str, event_type: &str, count: u32) -> EdgeVolume {
        EdgeVolume { source: source.into(), event_type: event_type.into(), count, ..Default::default() }
    }

    fn edge(consumer: &str, event_type: &str) -> TopologyEdge {
        TopologyEdge { producer_source: "p".into(), consumer_source: consumer.into(), event_type: event_type.into(), ..Default::default() }
    }

    #[test]
    fn types_are_summed_across_producers_and_ordered_busiest_first() {
        let rows = topic_rows(
            &[volume("a", "x", 5), volume("b", "x", 7), volume("a", "y", 20)],
            &[edge("c1", "x"), edge("c2", "x"), edge("c1", "x")],
        );
        assert_eq!(
            rows,
            [
                TopicRow { event_type: "y".into(), events: 20, producers: 1, consumers: 0 },
                TopicRow { event_type: "x".into(), events: 12, producers: 2, consumers: 2 },
            ]
        );
    }

    #[test]
    fn equal_counts_fall_back_to_name_order() {
        let rows = topic_rows(&[volume("a", "b", 1), volume("a", "a", 1)], &[]);
        assert_eq!(rows.iter().map(|r| r.event_type.as_str()).collect::<Vec<_>>(), ["a", "b"]);
    }

    #[test]
    fn a_rate_needs_a_window() {
        assert_eq!(per_minute(90, 900), Some(6));
        assert_eq!(per_minute(1, 3600), Some(0));
        assert_eq!(per_minute(500, -1), None);
    }
}
