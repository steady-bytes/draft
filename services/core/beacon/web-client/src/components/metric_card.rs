// Ported from services/core/blueprint/web-client/src/components/metric_card.rs
// (Phase 9 of the Beacon design doc — "the same visual language already
// shipped in Blueprint's web client") — same DaisyUI stat-card + sparkline
// shape, unmodified. Blueprint's original is left untouched; this is Beacon's
// own copy so Beacon's web-client crate has no cross-crate dependency on
// blueprint/web-client.
use dioxus::prelude::*;

#[derive(Clone, PartialEq)]
pub enum MetricIcon {
    Messages,
    Clock,
    Warning,
    Users,
    Server,
    ArrowUp,
}

fn sparkline_points_str(data: &[f32]) -> String {
    if data.len() < 2 {
        return String::new();
    }
    let min = data.iter().cloned().fold(f32::INFINITY, f32::min);
    let max = data.iter().cloned().fold(f32::NEG_INFINITY, f32::max);
    let range = if (max - min).abs() < 0.01 { 1.0 } else { max - min };
    let last = (data.len() - 1) as f32;
    data.iter()
        .enumerate()
        .map(|(i, &v)| {
            let x = i as f32 / last * 80.0;
            let y = 26.0 - (v - min) / range * 24.0;
            format!("{x:.1},{y:.1}")
        })
        .collect::<Vec<_>>()
        .join(" ")
}

fn icon_svg(icon: &MetricIcon) -> Element {
    match icon {
        MetricIcon::Messages => rsx! {
            svg {
                class: "w-4 h-4 shrink-0",
                xmlns: "http://www.w3.org/2000/svg",
                fill: "none",
                view_box: "0 0 24 24",
                stroke: "currentColor",
                stroke_width: "1.5",
                path {
                    stroke_linecap: "round",
                    stroke_linejoin: "round",
                    d: "M8.625 9.75a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0Zm0 0H8.25m4.125 0a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0Zm0 0H12m4.125 0a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0Zm0 0h-.375M21 12c0 4.556-4.03 8.25-9 8.25a9.764 9.764 0 0 1-2.555-.337A5.972 5.972 0 0 1 5.41 20.97a5.969 5.969 0 0 1-.474-.065 4.48 4.48 0 0 0 .978-2.025c.09-.457-.133-.901-.467-1.226C3.93 16.178 3 14.189 3 12c0-4.556 4.03-8.25 9-8.25s9 3.694 9 8.25Z",
                }
            }
        },
        MetricIcon::Clock => rsx! {
            svg {
                class: "w-4 h-4 shrink-0",
                xmlns: "http://www.w3.org/2000/svg",
                fill: "none",
                view_box: "0 0 24 24",
                stroke: "currentColor",
                stroke_width: "1.5",
                path {
                    stroke_linecap: "round",
                    stroke_linejoin: "round",
                    d: "M12 6v6h4.5m4.5 0a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z",
                }
            }
        },
        MetricIcon::Warning => rsx! {
            svg {
                class: "w-4 h-4 shrink-0",
                xmlns: "http://www.w3.org/2000/svg",
                fill: "none",
                view_box: "0 0 24 24",
                stroke: "currentColor",
                stroke_width: "1.5",
                path {
                    stroke_linecap: "round",
                    stroke_linejoin: "round",
                    d: "M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z",
                }
            }
        },
        MetricIcon::Users => rsx! {
            svg {
                class: "w-4 h-4 shrink-0",
                xmlns: "http://www.w3.org/2000/svg",
                fill: "none",
                view_box: "0 0 24 24",
                stroke: "currentColor",
                stroke_width: "1.5",
                path {
                    stroke_linecap: "round",
                    stroke_linejoin: "round",
                    d: "M15 19.128a9.38 9.38 0 0 0 2.625.372 9.337 9.337 0 0 0 4.121-.952 4.125 4.125 0 0 0-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 0 1 8.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0 1 11.964-3.07M12 6.375a3.375 3.375 0 1 1-6.75 0 3.375 3.375 0 0 1 6.75 0Zm8.25 2.25a2.625 2.625 0 1 1-5.25 0 2.625 2.625 0 0 1 5.25 0Z",
                }
            }
        },
        MetricIcon::Server => rsx! {
            svg {
                class: "w-4 h-4 shrink-0",
                xmlns: "http://www.w3.org/2000/svg",
                fill: "none",
                view_box: "0 0 24 24",
                stroke: "currentColor",
                stroke_width: "1.5",
                path {
                    stroke_linecap: "round",
                    stroke_linejoin: "round",
                    d: "M5.25 14.25h13.5m-13.5 0a3 3 0 0 1-3-3m3 3a3 3 0 1 0 0 6h13.5a3 3 0 1 0 0-6m-16.5-3a3 3 0 0 1 3-3h13.5a3 3 0 0 1 3 3m-19.5 0a4.5 4.5 0 0 1 .9-2.7L5.737 5.1a3.375 3.375 0 0 1 2.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 0 1 .9 2.7m0 0a3 3 0 0 1-3 3m0 3h.008v.008h-.008v-.008Zm0-6h.008v.008h-.008v-.008Zm-3 6h.008v.008h-.008v-.008Zm0-6h.008v.008h-.008v-.008Z",
                }
            }
        },
        MetricIcon::ArrowUp => rsx! {
            svg {
                class: "w-4 h-4 shrink-0",
                xmlns: "http://www.w3.org/2000/svg",
                fill: "none",
                view_box: "0 0 24 24",
                stroke: "currentColor",
                stroke_width: "1.5",
                path {
                    stroke_linecap: "round",
                    stroke_linejoin: "round",
                    d: "M3 16.5v2.25A2.25 2.25 0 0 0 5.25 21h13.5A2.25 2.25 0 0 0 21 18.75V16.5m-13.5-9L12 3m0 0 4.5 4.5M12 3v13.5",
                }
            }
        },
    }
}

#[component]
pub fn MetricCard(
    icon: MetricIcon,
    label: String,
    value: String,
    unit: Option<String>,
    trend_up: bool,
    trend_good: bool,
    trend_delta: String,
    sparkline_data: Vec<f32>,
    sparkline_color: String,
) -> Element {
    let points = sparkline_points_str(&sparkline_data);
    let trend_color = if trend_good { "#4ade80" } else { "#f87171" };
    let arrow = if trend_up { "↑" } else { "↓" };

    rsx! {
        div { class: "bg-base-200 rounded-xl p-4 flex flex-col gap-3",
            div { class: "flex items-center gap-2 text-xs text-base-content/40 uppercase tracking-wider",
                {icon_svg(&icon)}
                span { "{label}" }
            }
            div { class: "text-5xl font-bold leading-none tracking-tight",
                "{value}"
                if let Some(u) = unit {
                    span { class: "text-2xl font-semibold ml-0.5 text-base-content/70", "{u}" }
                }
            }
            div { class: "flex items-end justify-between gap-2",
                span {
                    class: "text-sm font-medium",
                    style: "color: {trend_color}",
                    "{arrow} {trend_delta} vs prev"
                }
                if !points.is_empty() {
                    svg {
                        width: "80",
                        height: "28",
                        view_box: "0 0 80 28",
                        fill: "none",
                        xmlns: "http://www.w3.org/2000/svg",
                        polyline {
                            points: "{points}",
                            stroke: "{sparkline_color}",
                            stroke_width: "2",
                            fill: "none",
                            stroke_linecap: "round",
                            stroke_linejoin: "round",
                        }
                    }
                }
            }
        }
    }
}
