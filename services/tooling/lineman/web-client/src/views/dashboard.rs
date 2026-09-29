use dioxus::prelude::*;
use draft_ui::data::{Progress, ProgressSegment, StatTile};
use draft_ui::layout::PageHead;
use draft_ui::shell::use_page_chrome;
use draft_ui::ui::{Btn, BtnVariant, Empty, Loading, RouteLink, Tag};
use draft_ui::Tone;

use crate::components::format_date;
use crate::rail::use_rail;
use crate::state::Summary;

/// Every objective at a glance: totals, then one card per objective with its progress.
#[component]
pub fn Dashboard() -> Element {
    let rail = use_rail();
    use_page_chrome(move || rail.chrome(&["Lineman", "Overview"]));

    let snapshot = rail.get();

    rsx! {
        PageHead {
            title: "Objectives".to_string(),
            eyebrow: "Overview".to_string(),
            description: "Every objective and how far along its tasks are.".to_string(),
            actions: rsx! {
                Btn { variant: BtnVariant::Primary, to: "/objectives/new".to_string(), "+ New objective" }
            },
        }

        match snapshot {
            None => rsx! { Loading {} },
            Some(s) if s.objectives.is_empty() => rsx! {
                Empty { title: "No objectives yet".to_string(),
                    "Create one to start tracking tasks and the agents working them."
                }
            },
            Some(s) => {
                let total: u32 = s.objectives.iter().map(|o| o.total).sum();
                let open: u32 = s.objectives.iter().map(|o| o.queued + o.in_flight).sum();
                let in_flight: u32 = s.objectives.iter().map(|o| o.in_flight).sum();
                let done: u32 = s.objectives.iter().map(|o| o.done).sum();
                rsx! {
                    div { class: "d-stats",
                        StatTile { label: "Objectives".to_string(), value: s.objectives.len().to_string() }
                        StatTile { label: "Open tasks".to_string(), value: open.to_string(), unit: format!("/ {total}") }
                        StatTile { label: "In flight".to_string(), value: in_flight.to_string() }
                        StatTile { label: "Done".to_string(), value: done.to_string() }
                    }
                    div { class: "d-card-grid", style: "margin-top:20px",
                        for o in s.objectives {
                            ObjectiveCard { key: "{o.id}", summary: o }
                        }
                    }
                }
            }
        }
    }
}

#[component]
fn ObjectiveCard(summary: Summary) -> Element {
    let (done, in_flight, queued) = summary.percents();
    let segments = vec![
        ProgressSegment::new(Tone::Primary, done),
        ProgressSegment::new(Tone::Ca, in_flight),
        ProgressSegment::new(Tone::Quiet, queued),
    ];
    let label = format!("{} done, {} in flight, {} queued", summary.done, summary.in_flight, summary.queued);
    let created = format_date(summary.created_at);
    let counts = format!("{} done · {} in flight · {} queued", summary.done, summary.in_flight, summary.queued);
    let to = format!("/objectives/{}", summary.id);
    rsx! {
        RouteLink { to, class: "d-panel d-card d-card--link".to_string(),
            div { class: "d-card-meta",
                Tag { "{summary.total} tasks" }
                span { class: "d-spacer" }
                span { "{created}" }
            }
            div { class: "d-card-title", style: "font-family:var(--font-mono);font-weight:600", "{summary.name}" }
            if !summary.description.is_empty() {
                p { style: "margin:0;color:var(--dim)", "{summary.description}" }
            }
            Progress { segments, label }
            div { class: "d-card-meta",
                span { "{counts}" }
                span { class: "d-spacer" }
                if summary.high_open > 0 {
                    Tag { tone: Tone::Err, "{summary.high_open} high" }
                }
            }
        }
    }
}
