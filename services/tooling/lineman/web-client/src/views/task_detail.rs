//! One task, live: who has it, what they are doing, and — when the agent is blocked — the
//! question and the buttons to answer it.
//!
//! No Retry/Cancel: Lineman's RPC interface has no corresponding calls, so those buttons would be
//! inert. They are left out rather than shipped as fake controls.

use dioxus::prelude::*;
use draft_api::proto::tooling_lineman_v1::{
    lineman_service_client::LinemanServiceClient, watch_response, GetObjectiveRequest, GetTaskRequest,
    ProvideGuidanceRequest, Task, UpdateTaskStateRequest, WatchRequest,
};
use draft_ui::data::{Kv, KvItem};
use draft_ui::layout::{PageHead, SectionTitle};
use draft_ui::shell::{use_page_chrome, BarItem};
use draft_ui::ui::{use_toast, Alert, Btn, BtnVariant, Loading, Select, Status, Tag, Toast};
use draft_ui::StatusKind;
use tonic_web_wasm_client::Client as WasmClient;

use crate::components::{format_when, PriorityTag};
use crate::rail::use_rail;
use crate::state::{classify, short_id};

fn client() -> LinemanServiceClient<WasmClient> {
    LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()))
}

#[component]
pub fn TaskDetail(id: String, task_id: String) -> Element {
    rsx! { TaskView { key: "{task_id}", id: id.clone(), task_id: task_id.clone() } }
}

#[component]
fn TaskView(id: String, task_id: String) -> Element {
    let rail = use_rail();
    let toast = use_toast();

    let task_for_get = task_id.clone();
    let get_result = use_resource(move || {
        let id = task_for_get.clone();
        async move { client().get_task(GetTaskRequest { id }).await.map(|r| r.into_inner()).map_err(|e| e.message().to_string()) }
    });
    // The objective's own states are what a manual state change can move the task into; they are
    // fully custom per objective, not a fixed enum.
    let objective_for_get = id.clone();
    let objective = use_resource(move || {
        let id = objective_for_get.clone();
        async move { client().get_objective(GetObjectiveRequest { id }).await.map(|r| r.into_inner()) }
    });

    let mut task: Signal<Option<Task>> = use_signal(|| None);
    use_effect(move || {
        if let Some(Ok(t)) = &*get_result.read() {
            task.set(Some(t.clone()));
        }
    });

    let task_for_watch = task_id.clone();
    use_coroutine(move |_rx: UnboundedReceiver<()>| {
        let watched = task_for_watch.clone();
        async move {
            let Ok(response) = client().watch(WatchRequest {}).await else {
                return;
            };
            let mut stream = response.into_inner();
            while let Ok(Some(msg)) = stream.message().await {
                if let Some(watch_response::Item::Task(t)) = msg.item {
                    if t.id == watched && !msg.removed {
                        task.set(Some(t));
                    }
                }
            }
        }
    });

    let task_id_for_bar = task_id.clone();
    use_page_chrome(move || {
        let objective_name = match &*objective.read() {
            Some(Ok(o)) => o.name.clone(),
            _ => "…".to_string(),
        };
        let name = task().map(|t| t.name).unwrap_or_else(|| "…".to_string());
        let mut chrome = rail.chrome(&["Lineman", "Objectives", &objective_name, &name]);
        chrome.left = vec![BarItem::kv("Task", short_id(&task_id_for_bar))];
        chrome.right = vec![BarItem::Text("Live updates on".to_string())];
        chrome
    });

    let task_for_move = task_id.clone();
    let move_to = use_callback(move |new_state: String| {
        let task_id = task_for_move.clone();
        spawn(async move {
            let moved = client().update_task_state(UpdateTaskStateRequest { task_id, new_state: new_state.clone() }).await;
            match moved {
                Ok(_) => {
                    toast.show(format!("Moved to {new_state}"), 3000);
                    rail.refresh();
                }
                Err(e) => toast.show(format!("Could not move task: {}", e.message()), 6000),
            }
        });
    });

    let task_for_guidance = task_id.clone();
    let respond = use_callback(move |response: String| {
        let task_id = task_for_guidance.clone();
        let next_state = task.peek().as_ref().map(|t| t.state.clone()).unwrap_or_default();
        spawn(async move {
            let sent = client().provide_guidance(ProvideGuidanceRequest { task_id, response, next_state }).await;
            match sent {
                Ok(_) => toast.show("Answer sent", 3000),
                Err(e) => toast.show(format!("Could not send answer: {}", e.message()), 6000),
            }
        });
    });

    if let Some(Err(err)) = &*get_result.read() {
        return rsx! {
            Alert { kind: StatusKind::Err, "Failed to load task: {err}" }
        };
    }
    let Some(t) = task() else {
        return rsx! { Loading {} };
    };

    let states: Vec<String> = match &*objective.read() {
        Some(Ok(o)) => o.states.clone(),
        _ => Vec::new(),
    };
    let kind = classify(&states, &t.state);
    let state_options: Vec<(String, String)> = states.iter().map(|s| (s.clone(), s.clone())).collect();
    let eyebrow = format!("Task {}", short_id(&t.id));
    let created = format_when(t.created_at.as_ref().map(|x| x.seconds));
    let updated = format_when(t.updated_at.as_ref().map(|x| x.seconds));
    let has_agent = !t.agent_id.is_empty();
    let agent_label = if has_agent { t.agent_id.clone() } else { "Unassigned".to_string() };
    let blocked = t.needs_input.clone();

    rsx! {
        PageHead {
            title: t.name.clone(),
            eyebrow: eyebrow,
            meta: rsx! {
                PriorityTag { priority: t.priority }
                Tag { tone: kind.tone(), "{t.state}" }
                if blocked.is_some() {
                    Status { kind: StatusKind::Warn, "Waiting on you" }
                } else if has_agent {
                    Status { kind: StatusKind::Info, live: true, "{agent_label}" }
                }
            },
            actions: rsx! {
                if !state_options.is_empty() {
                    Select {
                        value: t.state.clone(),
                        options: state_options,
                        aria_label: "Move to state".to_string(),
                        on_change: move |s| move_to.call(s),
                    }
                }
                Btn { to: format!("/objectives/{id}"), "Back to objective" }
            },
        }

        div { class: "td-layout",
            div { class: "td-main",
                if let Some(q) = blocked {
                    section {
                        SectionTitle { "Needs input" }
                        Alert { kind: StatusKind::Warn,
                            div { class: "td-needs",
                                p { class: "td-question", "{q.question}" }
                                div { class: "td-answers",
                                    for option in q.options.clone() {
                                        {
                                            let choice = option.clone();
                                            rsx! {
                                                Btn {
                                                    key: "{option}",
                                                    variant: BtnVariant::Primary,
                                                    onclick: move |_| respond.call(choice.clone()),
                                                    "{option}"
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }

                if !t.current_action.is_empty() {
                    section {
                        SectionTitle { "Current action" }
                        div { class: "d-panel td-block",
                            Status { kind: StatusKind::Info, live: true, "{t.current_action}" }
                        }
                    }
                }

                if !t.details.is_empty() {
                    section {
                        SectionTitle { "Details" }
                        div { class: "d-panel td-block td-details", "{t.details}" }
                    }
                }
            }

            aside { class: "td-side",
                SectionTitle { "Metadata" }
                div { class: "d-panel td-block",
                    Kv {
                        KvItem { label: "Task".to_string(), "{t.id}" }
                        KvItem { label: "State".to_string(), tone: kind.tone(), "{t.state}" }
                        KvItem { label: "Agent".to_string(), "{agent_label}" }
                        KvItem { label: "Created".to_string(), "{created}" }
                        KvItem { label: "Updated".to_string(), "{updated}" }
                    }
                }
            }
        }

        Toast { state: toast }
    }
}
