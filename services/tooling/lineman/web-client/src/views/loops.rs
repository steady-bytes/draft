//! Loops: a recurring schedule that spawns a new task every time it fires.
//!
//! "Custom (cron)" is a deliberate non-goal for now — only daily, weekly and every-N-days are
//! offered.

use dioxus::prelude::*;
use draft_api::proto::tooling_lineman_v1::{
    lineman_service_client::LinemanServiceClient, CreateLoopRequest, ListLoopsRequest, Loop, LoopStatus,
    PauseLoopRequest, Priority, Recurrence, ResumeLoopRequest,
};
use draft_ui::layout::PageHead;
use draft_ui::shell::use_page_chrome;
use draft_ui::ui::{Alert, Btn, BtnSize, BtnVariant, Empty, Field, Loading, Modal, Select, Tag, TextInput};
use draft_ui::{StatusKind, Tone};
use tonic_web_wasm_client::Client as WasmClient;

use crate::components::{format_when, objective_name, objective_options, priority_options, recurrence_label, PriorityTag};
use crate::rail::use_rail;

fn client() -> LinemanServiceClient<WasmClient> {
    LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()))
}

fn status_tag(status: LoopStatus) -> (Tone, &'static str) {
    match status {
        LoopStatus::Active => (Tone::Primary, "Active"),
        LoopStatus::Paused => (Tone::Warn, "Paused"),
        LoopStatus::Finished => (Tone::Quiet, "Finished"),
        LoopStatus::Unspecified => (Tone::Quiet, "—"),
    }
}

fn repeat_options() -> Vec<(String, String)> {
    vec![
        ("daily".to_string(), "Daily".to_string()),
        ("weekly".to_string(), "Weekly".to_string()),
        ("every_n_days".to_string(), "Every N days".to_string()),
    ]
}

#[component]
pub fn Loops() -> Element {
    let rail = use_rail();
    let mut show_create = use_signal(|| false);
    let mut objective_id = use_signal(String::new);
    let mut description = use_signal(String::new);
    let mut priority = use_signal(|| "PRIORITY_MEDIUM".to_string());
    let mut kind = use_signal(|| "daily".to_string());
    let mut interval_days = use_signal(|| "1".to_string());
    let mut at = use_signal(|| "09:00".to_string());
    let mut error: Signal<Option<String>> = use_signal(|| None);
    let mut refresh = use_signal(|| 0u32);

    use_page_chrome(move || rail.chrome(&["Lineman", "Automation", "Loops"]));

    let loops = use_resource(move || {
        let _ = refresh();
        async move { client().list_loops(ListLoopsRequest::default()).await.map(|r| r.into_inner().loops) }
    });

    let submit = use_callback(move |_: ()| {
        if description().trim().is_empty() {
            error.set(Some("Description is required".to_string()));
            return;
        }
        let request = CreateLoopRequest {
            objective_id: objective_id(),
            description: description(),
            priority: Priority::from_str_name(&priority()).unwrap_or(Priority::Medium) as i32,
            recurrence: Some(Recurrence {
                kind: kind(),
                interval_days: interval_days().parse().unwrap_or(1),
                cron: String::new(),
                at: at(),
                starts_at: None,
                ends_at: None,
                ends_after_occurrences: 0,
            }),
        };
        spawn(async move {
            match client().create_loop(request).await {
                Ok(_) => {
                    description.set(String::new());
                    error.set(None);
                    show_create.set(false);
                    refresh += 1;
                    rail.refresh();
                }
                Err(err) => error.set(Some(err.message().to_string())),
            }
        });
    });

    let set_paused = use_callback(move |(id, pause): (String, bool)| {
        spawn(async move {
            let _ = if pause {
                client().pause_loop(PauseLoopRequest { id }).await.map(|_| ())
            } else {
                client().resume_loop(ResumeLoopRequest { id }).await.map(|_| ())
            };
            refresh += 1;
            rail.refresh();
        });
    });

    let snapshot = rail.get().unwrap_or_default();
    let options = objective_options(&snapshot);
    let rows: Vec<Loop> = match &*loops.read() {
        Some(Ok(list)) => list.clone(),
        _ => Vec::new(),
    };
    let every_n = kind() == "every_n_days";

    rsx! {
        PageHead {
            title: "Loops".to_string(),
            eyebrow: "Automation".to_string(),
            description: "A recurring schedule: one definition spawns a new task every time it fires.".to_string(),
            actions: rsx! {
                Btn { variant: BtnVariant::Primary, onclick: move |_| show_create.set(true), "+ New loop" }
            },
        }

        match &*loops.read() {
            Some(Err(err)) => {
                let msg = err.message().to_string();
                rsx! {
                    Alert { kind: StatusKind::Err, "Failed to load loops: {msg}" }
                }
            }
            None => rsx! { Loading {} },
            Some(Ok(_)) if rows.is_empty() => rsx! {
                Empty { title: "No loops yet".to_string(), "Create a loop to spawn a task on a schedule." }
            },
            Some(Ok(_)) => rsx! {
                div { class: "d-panel d-table-wrap",
                    table { class: "d-table",
                        thead {
                            tr {
                                th { "Description" }
                                th { "Objective" }
                                th { "Repeats" }
                                th { "Next" }
                                th { "Priority" }
                                th { "Status" }
                                th { class: "is-right", "Runs" }
                                th {}
                            }
                        }
                        tbody {
                            for l in rows {
                                {
                                    let status = LoopStatus::try_from(l.status).unwrap_or(LoopStatus::Unspecified);
                                    let (tone, label) = status_tag(status);
                                    let repeats = l.recurrence.as_ref().map(|r| recurrence_label(&r.kind, r.interval_days, &r.at)).unwrap_or_default();
                                    let objective = objective_name(&snapshot, &l.objective_id);
                                    let next = if status == LoopStatus::Active { format_when(l.next_fire_at.as_ref().map(|t| t.seconds)) } else { "—".to_string() };
                                    let is_active = status == LoopStatus::Active;
                                    let can_toggle = is_active || status == LoopStatus::Paused;
                                    let lid = l.id.clone();
                                    rsx! {
                                        tr { key: "{l.id}",
                                            td { class: "is-fill", "{l.description}" }
                                            td { class: "is-dim", "{objective}" }
                                            td { class: "is-shrink", "{repeats}" }
                                            td { class: "is-shrink is-dim", "{next}" }
                                            td { class: "is-shrink", PriorityTag { priority: l.priority } }
                                            td { class: "is-shrink", Tag { tone, "{label}" } }
                                            td { class: "is-right", "{l.occurrence_count}" }
                                            td { class: "is-right",
                                                if can_toggle {
                                                    Btn {
                                                        size: BtnSize::Sm,
                                                        onclick: move |_| set_paused.call((lid.clone(), is_active)),
                                                        if is_active { "Pause" } else { "Resume" }
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
            },
        }

        Modal {
            open: show_create(),
            title: "New loop".to_string(),
            on_close: move |_| show_create.set(false),
            footer: rsx! {
                Btn { variant: BtnVariant::Ghost, onclick: move |_| show_create.set(false), "Cancel" }
                Btn { variant: BtnVariant::Primary, onclick: move |_| submit.call(()), "Create loop" }
            },
            if let Some(msg) = error() {
                Alert { kind: StatusKind::Err, "{msg}" }
            }
            Field { label: "Objective".to_string(),
                Select { value: objective_id(), options, on_change: move |v| objective_id.set(v) }
            }
            Field { label: "Description".to_string(),
                TextInput { value: description(), oninput: move |v| description.set(v) }
            }
            div { class: "form-row",
                Field { label: "Priority".to_string(),
                    Select { value: priority(), options: priority_options(), on_change: move |v| priority.set(v) }
                }
                Field { label: "Repeats".to_string(),
                    Select { value: kind(), options: repeat_options(), on_change: move |v| kind.set(v) }
                }
            }
            div { class: "form-row",
                if every_n {
                    Field { label: "Every N days".to_string(),
                        TextInput { r#type: "number".to_string(), value: interval_days(), oninput: move |v| interval_days.set(v) }
                    }
                }
                Field { label: "At (UTC)".to_string(),
                    TextInput { r#type: "time".to_string(), value: at(), oninput: move |v| at.set(v) }
                }
            }
        }
    }
}
