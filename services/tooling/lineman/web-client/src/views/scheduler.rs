//! Scheduler: fire a single task once, at a specific future time.
//!
//! `fire_at` comes from an HTML `datetime-local` input and is read as UTC — a documented
//! simplification, not timezone-aware.

use dioxus::prelude::*;
use draft_api::proto::tooling_lineman_v1::{
    lineman_service_client::LinemanServiceClient, CancelScheduledTaskRequest, CreateScheduledTaskRequest,
    ListScheduledTasksRequest, Priority, ScheduledTask, ScheduledTaskStatus,
};
use draft_ui::layout::PageHead;
use draft_ui::shell::use_page_chrome;
use draft_ui::ui::{Alert, Btn, BtnSize, BtnVariant, Empty, Field, Loading, Modal, Select, Tag, TextInput};
use draft_ui::{StatusKind, Tone};
use tonic_web_wasm_client::Client as WasmClient;

use crate::components::{format_when, objective_name, objective_options, parse_datetime_local, priority_options, PriorityTag};
use crate::rail::use_rail;

fn client() -> LinemanServiceClient<WasmClient> {
    LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()))
}

/// A scheduled task's status as a tag: pending is blue, fired is primary, cancelled is quiet.
fn status_tag(status: i32) -> (Tone, &'static str) {
    match ScheduledTaskStatus::try_from(status).unwrap_or(ScheduledTaskStatus::Unspecified) {
        ScheduledTaskStatus::Pending => (Tone::Ca, "Pending"),
        ScheduledTaskStatus::Fired => (Tone::Primary, "Fired"),
        ScheduledTaskStatus::Cancelled => (Tone::Quiet, "Cancelled"),
        ScheduledTaskStatus::Unspecified => (Tone::Quiet, "—"),
    }
}

#[component]
pub fn Scheduler() -> Element {
    let rail = use_rail();
    let mut show_create = use_signal(|| false);
    let mut objective_id = use_signal(String::new);
    let mut description = use_signal(String::new);
    let mut priority = use_signal(|| "PRIORITY_MEDIUM".to_string());
    let mut fire_at = use_signal(String::new);
    let mut error: Signal<Option<String>> = use_signal(|| None);
    let mut refresh = use_signal(|| 0u32);

    use_page_chrome(move || rail.chrome(&["Lineman", "Automation", "Scheduler"]));

    let scheduled = use_resource(move || {
        let _ = refresh(); // tracked read — re-fetches after a create or cancel bumps it
        async move { client().list_scheduled_tasks(ListScheduledTasksRequest::default()).await.map(|r| r.into_inner().scheduled_tasks) }
    });

    let submit = use_callback(move |_: ()| {
        let Some(ts) = parse_datetime_local(&fire_at()) else {
            error.set(Some("Enter a valid date and time".to_string()));
            return;
        };
        if description().trim().is_empty() {
            error.set(Some("Description is required".to_string()));
            return;
        }
        let request = CreateScheduledTaskRequest {
            objective_id: objective_id(),
            description: description(),
            priority: Priority::from_str_name(&priority()).unwrap_or(Priority::Medium) as i32,
            fire_at: Some(ts),
        };
        spawn(async move {
            match client().create_scheduled_task(request).await {
                Ok(_) => {
                    description.set(String::new());
                    fire_at.set(String::new());
                    error.set(None);
                    show_create.set(false);
                    refresh += 1;
                    rail.refresh();
                }
                Err(err) => error.set(Some(err.message().to_string())),
            }
        });
    });

    let cancel = use_callback(move |id: String| {
        spawn(async move {
            let _ = client().cancel_scheduled_task(CancelScheduledTaskRequest { id }).await;
            refresh += 1;
            rail.refresh();
        });
    });

    let snapshot = rail.get().unwrap_or_default();
    let options = objective_options(&snapshot);
    let rows: Vec<ScheduledTask> = match &*scheduled.read() {
        Some(Ok(list)) => list.clone(),
        _ => Vec::new(),
    };

    rsx! {
        PageHead {
            title: "Scheduler".to_string(),
            eyebrow: "Automation".to_string(),
            description: "Schedule a single task to activate once, at a specific future time.".to_string(),
            actions: rsx! {
                Btn { variant: BtnVariant::Primary, onclick: move |_| show_create.set(true), "+ Schedule task" }
            },
        }

        match &*scheduled.read() {
            Some(Err(err)) => {
                let msg = err.message().to_string();
                rsx! {
                    Alert { kind: StatusKind::Err, "Failed to load scheduled tasks: {msg}" }
                }
            }
            None => rsx! { Loading {} },
            Some(Ok(_)) if rows.is_empty() => rsx! {
                Empty { title: "Nothing scheduled".to_string(), "Schedule a task to have it created at a set time." }
            },
            Some(Ok(_)) => rsx! {
                div { class: "d-panel d-table-wrap",
                    table { class: "d-table",
                        thead {
                            tr {
                                th { "Fire at" }
                                th { "Description" }
                                th { "Objective" }
                                th { "Priority" }
                                th { "Status" }
                                th {}
                            }
                        }
                        tbody {
                            for s in rows {
                                {
                                    let when = format_when(s.fire_at.as_ref().map(|t| t.seconds));
                                    let objective = objective_name(&snapshot, &s.objective_id);
                                    let (tone, label) = status_tag(s.status);
                                    let pending = s.status == ScheduledTaskStatus::Pending as i32;
                                    let sid = s.id.clone();
                                    rsx! {
                                        tr { key: "{s.id}",
                                            td { class: "is-shrink", "{when}" }
                                            td { class: "is-fill", "{s.description}" }
                                            td { class: "is-dim", "{objective}" }
                                            td { class: "is-shrink", PriorityTag { priority: s.priority } }
                                            td { class: "is-shrink", Tag { tone, "{label}" } }
                                            td { class: "is-right",
                                                if pending {
                                                    Btn {
                                                        variant: BtnVariant::Danger,
                                                        size: BtnSize::Sm,
                                                        onclick: move |_| cancel.call(sid.clone()),
                                                        "Cancel"
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
            title: "Schedule task".to_string(),
            on_close: move |_| show_create.set(false),
            footer: rsx! {
                Btn { variant: BtnVariant::Ghost, onclick: move |_| show_create.set(false), "Cancel" }
                Btn { variant: BtnVariant::Primary, onclick: move |_| submit.call(()), "Schedule" }
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
                Field { label: "Fire at (UTC)".to_string(),
                    TextInput { r#type: "datetime-local".to_string(), value: fire_at(), oninput: move |v| fire_at.set(v) }
                }
            }
        }
    }
}
