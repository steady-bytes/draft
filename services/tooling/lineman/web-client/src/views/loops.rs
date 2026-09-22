use dioxus::prelude::*;
use draft_api::proto::tooling_lineman_v1::{
    lineman_service_client::LinemanServiceClient, CreateLoopRequest, ListLoopsRequest, LoopStatus,
    PauseLoopRequest, Priority, Recurrence, ResumeLoopRequest,
};
use tonic_web_wasm_client::Client as WasmClient;

/// Page 6 -- Loops. A recurring schedule that spawns a new task instance
/// every time it fires (idea.md). "Custom (cron)" is a deliberate non-goal
/// of this pass (see the implementation plan) -- only daily/weekly/every_n_days
/// are wired up here.
#[component]
pub fn Loops() -> Element {
    let mut objective_id = use_signal(String::new);
    let mut description = use_signal(String::new);
    let mut priority = use_signal(|| "PRIORITY_MEDIUM".to_string());
    let mut kind = use_signal(|| "daily".to_string());
    let mut interval_days = use_signal(|| "1".to_string());
    let mut at = use_signal(|| "09:00".to_string());
    let mut error = use_signal(|| Option::<String>::None);
    let mut refresh = use_signal(|| 0u32);

    let list_req = use_signal(ListLoopsRequest::default);
    let loops = use_resource(move || {
        let req = list_req();
        let _ = refresh();
        async move {
            let mut client = LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
            client.list_loops(req).await.map(|r| r.into_inner())
        }
    });

    let submit = move |_| {
        let objective_id_v = objective_id();
        let description_v = description();
        let priority_v = Priority::from_str_name(&priority()).unwrap_or(Priority::Medium);
        let interval: i32 = interval_days().parse().unwrap_or(1);

        spawn(async move {
            let mut client = LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
            match client
                .create_loop(CreateLoopRequest {
                    objective_id: objective_id_v,
                    description: description_v,
                    priority: priority_v as i32,
                    recurrence: Some(Recurrence {
                        kind: kind(),
                        interval_days: interval,
                        cron: String::new(),
                        at: at(),
                        starts_at: None,
                        ends_at: None,
                        ends_after_occurrences: 0,
                    }),
                })
                .await
            {
                Ok(_) => {
                    description.set(String::new());
                    refresh.set(refresh() + 1);
                }
                Err(err) => error.set(Some(err.to_string())),
            }
        });
    };

    let set_status = move |(loop_id, pause): (String, bool)| {
        spawn(async move {
            let mut client = LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
            let _ = if pause {
                client.pause_loop(PauseLoopRequest { id: loop_id }).await
            } else {
                client.resume_loop(ResumeLoopRequest { id: loop_id }).await
            };
            refresh.set(refresh() + 1);
        });
    };

    rsx! {
        div { class: "flex flex-col gap-6",
            h1 { class: "text-2xl font-bold", "Loops" }
            p { class: "text-base-content/70 text-sm max-w-2xl",
                "A recurring schedule -- one definition spawns a new task instance every time it fires."
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
                            placeholder: "leave empty for a standalone loop",
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
                            label { class: "label", span { class: "label-text", "Repeats" } }
                            select {
                                class: "select select-bordered select-sm w-full",
                                value: "{kind}",
                                onchange: move |e| kind.set(e.value()),
                                option { value: "daily", "Daily" }
                                option { value: "weekly", "Weekly" }
                                option { value: "every_n_days", "Every N days" }
                            }
                        }
                    }
                    div { class: "grid grid-cols-2 gap-3",
                        if kind() == "every_n_days" {
                            div { class: "form-control",
                                label { class: "label", span { class: "label-text", "Every N days" } }
                                input {
                                    r#type: "number",
                                    class: "input input-bordered input-sm w-full",
                                    value: "{interval_days}",
                                    oninput: move |e| interval_days.set(e.value()),
                                }
                            }
                        }
                        div { class: "form-control",
                            label { class: "label", span { class: "label-text", "At" } }
                            input {
                                r#type: "time",
                                class: "input input-bordered input-sm w-full",
                                value: "{at}",
                                oninput: move |e| at.set(e.value()),
                            }
                        }
                    }
                    div { class: "card-actions justify-end",
                        button { class: "btn btn-primary btn-sm", onclick: submit, "Create Loop" }
                    }
                }
            }

            match &*loops.read() {
                Some(Ok(resp)) if !resp.loops.is_empty() => rsx! {
                    div { class: "overflow-x-auto",
                        table { class: "table table-sm",
                            thead { tr { th { "Description" } th { "Recurrence" } th { "Status" } th { "Occurrences" } th {} } }
                            tbody {
                                for l in resp.loops.clone() {
                                    {
                                        let status = LoopStatus::try_from(l.status).unwrap_or(LoopStatus::Unspecified);
                                        let recurrence_label = l.recurrence.as_ref().map(|r| r.kind.clone()).unwrap_or_default();
                                        let lid = l.id.clone();
                                        let is_active = status == LoopStatus::Active;
                                        rsx! {
                                            tr { key: "{l.id}",
                                                td { "{l.description}" }
                                                td { class: "font-mono text-xs", "{recurrence_label}" }
                                                td { {status.as_str_name()} }
                                                td { "{l.occurrence_count}" }
                                                td {
                                                    if status == LoopStatus::Active || status == LoopStatus::Paused {
                                                        button {
                                                            class: "btn btn-ghost btn-xs",
                                                            onclick: move |_| set_status((lid.clone(), is_active)),
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
                Some(Ok(_)) => rsx! { div { class: "text-base-content/50 text-sm", "No loops yet." } },
                Some(Err(err)) => rsx! { div { class: "text-error text-sm", "{err}" } },
                None => rsx! { div { class: "text-base-content/50 text-sm", "Loading…" } },
            }
        }
    }
}
