use dioxus::prelude::*;
use draft_api::proto::tooling_lineman_v1::{
    lineman_service_client::LinemanServiceClient, CancelScheduledTaskRequest,
    CreateScheduledTaskRequest, ListScheduledTasksRequest, Priority, ScheduledTaskStatus,
};
use tonic_web_wasm_client::Client as WasmClient;

/// Page 5 -- Scheduler. Fire a single task once, at a specific future time
/// (idea.md). fire_at is parsed from an HTML datetime-local input as UTC --
/// a documented simplification, not timezone-aware.
#[component]
pub fn Scheduler() -> Element {
    let mut objective_id = use_signal(String::new);
    let mut description = use_signal(String::new);
    let mut priority = use_signal(|| "PRIORITY_MEDIUM".to_string());
    let mut fire_at = use_signal(String::new);
    let mut error = use_signal(|| Option::<String>::None);
    let mut refresh = use_signal(|| 0u32);

    let list_req = use_signal(ListScheduledTasksRequest::default);
    let scheduled = use_resource(move || {
        let req = list_req();
        let _ = refresh(); // tracked read -- re-fetches after submit/cancel bump this
        async move {
            let mut client = LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
            client
                .list_scheduled_tasks(req)
                .await
                .map(|r| r.into_inner())
        }
    });

    let submit = move |_| {
        let objective_id_v = objective_id();
        let description_v = description();
        let priority_v = Priority::from_str_name(&priority()).unwrap_or(Priority::Medium);
        let fire_at_v = fire_at();

        let Some(ts) = crate::components::parse_datetime_local(&fire_at_v) else {
            error.set(Some("Enter a valid date/time".to_string()));
            return;
        };

        spawn(async move {
            let mut client = LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
            match client
                .create_scheduled_task(CreateScheduledTaskRequest {
                    objective_id: objective_id_v,
                    description: description_v,
                    priority: priority_v as i32,
                    fire_at: Some(ts),
                })
                .await
            {
                Ok(_) => {
                    description.set(String::new());
                    fire_at.set(String::new());
                    refresh.set(refresh() + 1);
                }
                Err(err) => error.set(Some(err.to_string())),
            }
        });
    };

    let cancel = move |sched_id: String| {
        spawn(async move {
            let mut client = LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
            let _ = client
                .cancel_scheduled_task(CancelScheduledTaskRequest { id: sched_id })
                .await;
            refresh.set(refresh() + 1);
        });
    };

    rsx! {
        div { class: "flex flex-col gap-6",
            h1 { class: "text-2xl font-bold", "Scheduler" }
            p { class: "text-base-content/70 text-sm max-w-2xl",
                "Schedule a single task to activate once, at a specific future time."
            }

            div { class: "card bg-base-200 border border-base-300 max-w-lg",
                div { class: "card-body gap-3",
                    if let Some(msg) = error() {
                        div { class: "alert alert-error text-sm", "{msg}" }
                    }
                    div { class: "form-control",
                        label { class: "label", span { class: "label-text", "Objective ID (optional)" } }
                        input {
                            class: "input input-bordered input-sm w-full",
                            placeholder: "leave empty for a standalone task",
                            value: "{objective_id}",
                            oninput: move |e| objective_id.set(e.value()),
                        }
                    }
                    div { class: "form-control",
                        label { class: "label", span { class: "label-text", "Description" } }
                        input {
                            class: "input input-bordered input-sm w-full",
                            value: "{description}",
                            oninput: move |e| description.set(e.value()),
                        }
                    }
                    div { class: "grid grid-cols-2 gap-3",
                        div { class: "form-control",
                            label { class: "label", span { class: "label-text", "Priority" } }
                            select {
                                class: "select select-bordered select-sm w-full",
                                value: "{priority}",
                                onchange: move |e| priority.set(e.value()),
                                option { value: "PRIORITY_LOW", "Low" }
                                option { value: "PRIORITY_MEDIUM", "Medium" }
                                option { value: "PRIORITY_HIGH", "High" }
                            }
                        }
                        div { class: "form-control",
                            label { class: "label", span { class: "label-text", "Fire at" } }
                            input {
                                r#type: "datetime-local",
                                class: "input input-bordered input-sm w-full",
                                value: "{fire_at}",
                                oninput: move |e| fire_at.set(e.value()),
                            }
                        }
                    }
                    div { class: "card-actions justify-end",
                        button { class: "btn btn-primary btn-sm", onclick: submit, "Schedule" }
                    }
                }
            }

            match &*scheduled.read() {
                Some(Ok(resp)) if !resp.scheduled_tasks.is_empty() => rsx! {
                    div { class: "overflow-x-auto",
                        table { class: "table table-sm",
                            thead { tr { th { "Fire at" } th { "Description" } th { "Status" } th {} } }
                            tbody {
                                for s in resp.scheduled_tasks.clone() {
                                    tr { key: "{s.id}",
                                        td { class: "font-mono text-xs",
                                            {s.fire_at.map(|t| t.seconds).unwrap_or(0).to_string()}
                                        }
                                        td { "{s.description}" }
                                        td {
                                            {ScheduledTaskStatus::try_from(s.status).unwrap_or(ScheduledTaskStatus::Unspecified).as_str_name()}
                                        }
                                        td {
                                            if s.status == ScheduledTaskStatus::Pending as i32 {
                                                {
                                                    let sid = s.id.clone();
                                                    rsx! {
                                                        button {
                                                            class: "btn btn-ghost btn-xs",
                                                            onclick: move |_| cancel(sid.clone()),
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
                Some(Ok(_)) => rsx! { div { class: "text-base-content/50 text-sm", "No scheduled tasks yet." } },
                Some(Err(err)) => rsx! { div { class: "text-error text-sm", "{err}" } },
                None => rsx! { div { class: "text-base-content/50 text-sm", "Loading…" } },
            }
        }
    }
}
