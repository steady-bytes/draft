use dioxus::prelude::*;
use tonic_web_wasm_client::Client as WasmClient;
use draft_api::proto::core_message_broker_actors_v1::{
    metrics_client::MetricsClient,
    resource_metrics_client::ResourceMetricsClient,
    topology_client::TopologyClient,
    EdgeVolume,
    GetMetricsRequest,
    GetResourceMetricsRequest,
    GetTopologyRequest,
    TopologyEdge,
};

use gloo_timers::callback::Interval;
use crate::components::{MetricCard, MetricIcon, event_color};


#[derive(Clone, PartialEq, Copy)]
enum TimeRange {
    Min15,
    Hour1,
    Hour3,
    Hour24,
    AllTime,
}

impl TimeRange {
    fn label(self) -> &'static str {
        match self {
            TimeRange::Min15  => "15m",
            TimeRange::Hour1  => "1h",
            TimeRange::Hour3  => "3h",
            TimeRange::Hour24 => "24h",
            TimeRange::AllTime => "all",
        }
    }

    fn description(self) -> &'static str {
        match self {
            TimeRange::Min15  => "last 15 minutes",
            TimeRange::Hour1  => "last 1 hour",
            TimeRange::Hour3  => "last 3 hours",
            TimeRange::Hour24 => "last 24 hours",
            TimeRange::AllTime => "all time",
        }
    }

    /// Returns the window in seconds. -1 means no time filter (all stored events).
    fn window_seconds(self) -> i32 {
        match self {
            TimeRange::Min15  => 900,
            TimeRange::Hour1  => 3_600,
            TimeRange::Hour3  => 10_800,
            TimeRange::Hour24 => 86_400,
            TimeRange::AllTime => -1,
        }
    }
}

#[derive(Clone, Default)]
struct MetricsData {
    msgs_per_min:   u32,
    median_ms:      f64,
    p95_ms:         f64,
    edge_volumes:   Vec<EdgeVolume>,
    topology_edges: Vec<TopologyEdge>,
    loaded:         bool,
}

#[derive(Clone, Default)]
struct ResourceData {
    published_total:  u64,
    dropped_total:    u64,
    queue_depth:      u32,
    queue_capacity:   u32,
    active_consumers: u32,
    active_producers: u32,
    flush_p95_ms:     f64,
    flush_count:      u32,
    loaded:           bool,
}

#[component]
pub fn Metrics() -> Element {
    let mut time_range      = use_signal(|| TimeRange::Hour3);
    let mut metrics         = use_signal(MetricsData::default);
    let mut resource        = use_signal(ResourceData::default);
    let mut updated_ago     = use_signal(|| "loading…".to_string());
    let mut tick = use_signal(|| 0u32);

    // Increment tick every 5 s — effects that read tick() re-run automatically.
    let _poll_interval = use_signal(move || {
        Interval::new(5_000, move || {
            *tick.write() += 1;
        })
    });

    use_effect(move || {
        let _ = tick();   // re-run on every 5 s tick
        let range = time_range();
        let host  = crate::CATALYST_DOMAIN.clone();
        spawn(async move {
            let metrics_res = {
                let mut client = MetricsClient::new(WasmClient::new(host.clone()));
                client.get_metrics(GetMetricsRequest { window_seconds: range.window_seconds() }).await
            };
            let topo_edges = {
                let mut client = TopologyClient::new(WasmClient::new(host));
                client.get_topology(GetTopologyRequest {}).await
                    .ok()
                    .map(|t| t.into_inner().edges)
                    .unwrap_or_default()
            };
            match metrics_res {
                Ok(resp) => {
                    let r = resp.into_inner();
                    metrics.set(MetricsData {
                        msgs_per_min:   r.total_messages_per_min,
                        median_ms:      r.median_latency_ms,
                        p95_ms:         r.p95_latency_ms,
                        edge_volumes:   r.edge_volumes,
                        topology_edges: topo_edges,
                        loaded:         true,
                    });
                    updated_ago.set("just now".to_string());
                }
                Err(_) => {
                    updated_ago.set("unavailable".to_string());
                }
            }
        });

        // Resource metrics are point-in-time and independent of the time window.
        let host = crate::CATALYST_DOMAIN.clone();
        spawn(async move {
            let mut client = ResourceMetricsClient::new(WasmClient::new(host));
            if let Ok(resp) = client.get_resource_metrics(GetResourceMetricsRequest {}).await {
                let r = resp.into_inner();
                resource.set(ResourceData {
                    published_total:  r.messages_published_total,
                    dropped_total:    r.messages_dropped_total,
                    queue_depth:      r.queue_depth,
                    queue_capacity:   r.queue_capacity,
                    active_consumers: r.active_consumers,
                    active_producers: r.active_producers,
                    flush_p95_ms:     r.store_flush_p95_ms,
                    flush_count:      r.store_flush_count,
                    loaded:           true,
                });
            }
        });
    });


    let m = metrics();
    let res = resource();
    let flush_p95_str = if res.loaded { format!("{:.1}", res.flush_p95_ms) } else { "—".to_string() };

    // Consumers per event_type from live topology edges.
    let consumer_idx: std::collections::HashMap<String, std::collections::HashSet<String>> = {
        let mut idx: std::collections::HashMap<String, std::collections::HashSet<String>> =
            std::collections::HashMap::new();
        for e in &m.topology_edges {
            idx.entry(e.event_type.clone())
                .or_default()
                .insert(e.consumer_source.clone());
        }
        idx
    };

    let is_all_time = time_range() == TimeRange::AllTime;
    // Aggregate edge_volumes by event_type: sum counts and count distinct producer sources.
    let window_mins = if is_all_time { 0.0 } else { time_range().window_seconds() as f64 / 60.0 };
    // rows: (event_type, total_count, producer_count, consumer_count)
    let topic_rows: Vec<(String, u32, u32, u32)> = {
        let mut map: std::collections::HashMap<String, (u32, std::collections::HashSet<String>)> =
            std::collections::HashMap::new();
        for ev in &m.edge_volumes {
            let entry = map.entry(ev.event_type.clone()).or_default();
            entry.0 += ev.count;
            entry.1.insert(ev.source.clone());
        }
        let mut rows: Vec<(String, u32, u32, u32)> = map.into_iter()
            .map(|(typ, (count, srcs))| {
                let consumers = consumer_idx.get(&typ).map(|s| s.len() as u32).unwrap_or(0);
                (typ, count, srcs.len() as u32, consumers)
            })
            .collect();
        rows.sort_by(|a, b| b.1.cmp(&a.1));
        rows
    };
    let max_count = topic_rows.first().map(|r| r.1).unwrap_or(1).max(1);

    // Sparklines are placeholders — a time-series endpoint would provide real buckets.
    let msgs_data   = vec![280.0f32, 295.0, 310.0, 290.0, 305.0, 298.0, m.msgs_per_min as f32];
    let median_data = vec![32.0f32,  30.0,  28.0,  31.0,  27.0,  29.0,  m.median_ms as f32];
    let p95_data    = vec![80.0f32,  85.0,  88.0,  90.0,  92.0,  91.0,  m.p95_ms as f32];
    let error_data  = vec![1.8f32,   1.6,   1.5,   1.7,   1.4,   1.5,   1.4];

    rsx! {
        div { class: "p-6 flex flex-col gap-6",
            div { class: "flex items-center justify-between flex-wrap gap-3",
                div { class: "flex items-baseline gap-3",
                    h1 { class: "text-xl font-bold", "Metrics" }
                    span { class: "text-sm text-base-content/40",
                        "{time_range().description()} · updated {updated_ago()}"
                    }
                }
                div { class: "join",
                    for range in [TimeRange::Min15, TimeRange::Hour1, TimeRange::Hour3, TimeRange::Hour24, TimeRange::AllTime] {
                        button {
                            class: if time_range() == range {
                                "btn btn-sm join-item btn-neutral"
                            } else {
                                "btn btn-sm join-item btn-ghost"
                            },
                            onclick: move |_| time_range.set(range),
                            "{range.label()}"
                        }
                    }
                }
            }

            div { class: "flex flex-col gap-3",
                h2 { class: "text-xs font-semibold tracking-wide text-base-content/40 uppercase", "Event Metrics" }
                div { class: "grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4",
                MetricCard {
                    icon: MetricIcon::Messages,
                    label: "MESSAGES / MIN".to_string(),
                    value: if m.loaded { m.msgs_per_min.to_string() } else { "—".to_string() },
                    unit: None,
                    trend_up: true,
                    trend_good: true,
                    trend_delta: "live".to_string(),
                    sparkline_data: msgs_data,
                    sparkline_color: "#4ade80".to_string(),
                }
                MetricCard {
                    icon: MetricIcon::Clock,
                    label: "MEDIAN LATENCY".to_string(),
                    value: if m.loaded { format!("{:.1}", m.median_ms) } else { "—".to_string() },
                    unit: Some("ms".to_string()),
                    trend_up: false,
                    trend_good: true,
                    trend_delta: "live".to_string(),
                    sparkline_data: median_data,
                    sparkline_color: "#60a5fa".to_string(),
                }
                MetricCard {
                    icon: MetricIcon::Clock,
                    label: "P95 LATENCY".to_string(),
                    value: if m.loaded { format!("{:.1}", m.p95_ms) } else { "—".to_string() },
                    unit: Some("ms".to_string()),
                    trend_up: true,
                    trend_good: false,
                    trend_delta: "live".to_string(),
                    sparkline_data: p95_data,
                    sparkline_color: "#fb923c".to_string(),
                }
                MetricCard {
                    icon: MetricIcon::Warning,
                    label: "ERROR RATE".to_string(),
                    value: "—".to_string(),
                    unit: Some("%".to_string()),
                    trend_up: false,
                    trend_good: true,
                    trend_delta: "n/a".to_string(),
                    sparkline_data: error_data,
                    sparkline_color: "#f87171".to_string(),
                }
            }
            }

            // Resource metrics
            div { class: "flex flex-col gap-3",
                h2 { class: "text-xs font-semibold tracking-wide text-base-content/40 uppercase", "Resource Metrics" }
                div { class: "grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4",
                    {
                        let v = res.queue_capacity.max(1);
                        let depth = res.queue_depth;
                        let flat = vec![depth as f32; 7];
                        rsx! {
                            MetricCard {
                                icon: MetricIcon::Server,
                                label: "QUEUE DEPTH".to_string(),
                                value: if res.loaded { format!("{} / {}", depth, res.queue_capacity) } else { "—".to_string() },
                                unit: None,
                                trend_up: false,
                                trend_good: depth < v / 2,
                                trend_delta: "live".to_string(),
                                sparkline_data: flat,
                                sparkline_color: if depth > v * 4 / 5 { "#ef4444".to_string() }
                                                 else if depth > v / 2   { "#f97316".to_string() }
                                                 else                     { "#4ade80".to_string() },
                            }
                        }
                    }
                    MetricCard {
                        icon: MetricIcon::Users,
                        label: "ACTIVE CONSUMERS".to_string(),
                        value: if res.loaded { res.active_consumers.to_string() } else { "—".to_string() },
                        unit: None,
                        trend_up: res.active_consumers > 0,
                        trend_good: res.active_consumers > 0,
                        trend_delta: "live".to_string(),
                        sparkline_data: vec![res.active_consumers as f32; 7],
                        sparkline_color: "#60a5fa".to_string(),
                    }
                    MetricCard {
                        icon: MetricIcon::Users,
                        label: "ACTIVE PRODUCERS".to_string(),
                        value: if res.loaded { res.active_producers.to_string() } else { "—".to_string() },
                        unit: None,
                        trend_up: true,
                        trend_good: true,
                        trend_delta: "live".to_string(),
                        sparkline_data: vec![res.active_producers as f32; 7],
                        sparkline_color: "#fbbf24".to_string(),
                    }
                    MetricCard {
                        icon: MetricIcon::ArrowUp,
                        label: "PUBLISHED TOTAL".to_string(),
                        value: if res.loaded { res.published_total.to_string() } else { "—".to_string() },
                        unit: None,
                        trend_up: true,
                        trend_good: true,
                        trend_delta: "since start".to_string(),
                        sparkline_data: vec![res.published_total as f32; 7],
                        sparkline_color: "#4ade80".to_string(),
                    }
                    MetricCard {
                        icon: MetricIcon::Warning,
                        label: "DROPPED TOTAL".to_string(),
                        value: if res.loaded { res.dropped_total.to_string() } else { "—".to_string() },
                        unit: None,
                        trend_up: res.dropped_total > 0,
                        trend_good: res.dropped_total == 0,
                        trend_delta: "since start".to_string(),
                        sparkline_data: vec![res.dropped_total as f32; 7],
                        sparkline_color: if res.dropped_total > 0 { "#f87171".to_string() } else { "#4ade80".to_string() },
                    }
                    MetricCard {
                        icon: MetricIcon::Clock,
                        label: "FLUSH P95".to_string(),
                        value: if res.loaded { flush_p95_str.clone() } else { "—".to_string() },
                        unit: Some("ms".to_string()),
                        trend_up: false,
                        trend_good: res.flush_p95_ms < 50.0,
                        trend_delta: "live".to_string(),
                        sparkline_data: vec![res.flush_p95_ms as f32; 7],
                        sparkline_color: "#fb923c".to_string(),
                    }
                    MetricCard {
                        icon: MetricIcon::Server,
                        label: "FLUSH COUNT".to_string(),
                        value: if res.loaded { res.flush_count.to_string() } else { "—".to_string() },
                        unit: None,
                        trend_up: true,
                        trend_good: true,
                        trend_delta: "since start".to_string(),
                        sparkline_data: vec![res.flush_count as f32; 7],
                        sparkline_color: "#a78bfa".to_string(),
                    }
                }
            }

            // Topics table
            div { class: "rounded-xl border border-base-content/10 overflow-hidden",
                div { class: "px-4 py-3 border-b border-base-content/10 flex items-center gap-2",
                    h2 { class: "text-sm font-semibold tracking-wide text-base-content/70 uppercase",
                        "Topics"
                    }
                    if m.loaded {
                        span { class: "text-xs text-base-content/40",
                            "· {topic_rows.len()} event types in {time_range().description()}"
                        }
                    }
                }
                if !m.loaded {
                    div { class: "px-4 py-8 text-center text-base-content/30 text-sm", "Loading…" }
                } else if topic_rows.is_empty() {
                    div { class: "px-4 py-8 text-center text-base-content/30 text-sm", "No events in this window." }
                } else {
                    table { class: "w-full text-sm",
                        thead {
                            tr { class: "border-b border-base-content/10 text-left text-xs text-base-content/40 uppercase tracking-wide",
                                th { class: "px-4 py-2 font-medium", "Topic" }
                                th { class: "px-4 py-2 font-medium text-right", "Producers" }
                                th { class: "px-4 py-2 font-medium text-right", "Consumers" }
                                th { class: "px-4 py-2 font-medium text-right", "Events" }
                                if !is_all_time {
                                    th { class: "px-4 py-2 font-medium text-right", "/ min" }
                                }
                                th { class: "px-4 py-2", style: "width:160px;", "" }
                            }
                        }
                        tbody {
                            for (topic, count, producers, consumers) in topic_rows.iter() {
                                {
                                    let mut hovered = use_signal(|| false);
                                    let per_min = if window_mins > 0.0 {
                                        (*count as f64 / window_mins).round() as u32
                                    } else { 0 };
                                    let bar_pct = (*count as f64 / max_count as f64 * 100.0).round() as u32;
                                    let color = event_color(topic.as_str());
                                    let topic = topic.clone();
                                    let count = *count;
                                    let producers = *producers;
                                    let consumers = *consumers;
                                    let dot_color = if consumers > 0 { "#4ade80" } else { "#f87171" };
                                    let row_bg = if hovered() { "rgba(255,255,255,0.08)" } else { "transparent" };
                                    rsx! {
                                        tr {
                                            style: "border-bottom:1px solid rgba(255,255,255,0.05);transition:background 120ms;background:{row_bg};",
                                            onmouseenter: move |_| hovered.set(true),
                                            onmouseleave: move |_| hovered.set(false),
                                            td { class: "px-4 py-3",
                                                div { class: "flex items-center gap-2",
                                                    span {
                                                        style: "display:inline-block;width:8px;height:8px;border-radius:50%;background:{dot_color};flex-shrink:0;",
                                                    }
                                                    span { class: "font-mono text-xs", "{topic}" }
                                                }
                                            }
                                            td { class: "px-4 py-3 text-right tabular-nums text-base-content/60",
                                                "{producers}"
                                            }
                                            td { class: "px-4 py-3 text-right tabular-nums text-base-content/60",
                                                "{consumers}"
                                            }
                                            td { class: "px-4 py-3 text-right tabular-nums",
                                                "{count}"
                                            }
                                            if !is_all_time {
                                                td { class: "px-4 py-3 text-right tabular-nums text-base-content/70",
                                                    "{per_min}"
                                                }
                                            }
                                            td { class: "px-4 py-3",
                                                div { class: "w-full bg-base-content/10 rounded-full overflow-hidden",
                                                    style: "height:4px;",
                                                    div {
                                                        style: "height:4px;width:{bar_pct}%;background:{color};border-radius:9999px;",
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

    }
}
