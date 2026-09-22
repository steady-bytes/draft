use dioxus::prelude::*;
use draft_api::hook::tooling_lineman_v1::use_lineman_service_service;
use draft_api::proto::tooling_lineman_v1::{
    lineman_service_client::LinemanServiceClient, watch_response, GetObjectiveRequest,
    GetTaskRequest, ProvideGuidanceRequest, Task, UpdateTaskStateRequest, WatchRequest,
};
use gloo_timers::future::TimeoutFuture;
use tonic_web_wasm_client::Client as WasmClient;

use crate::components::{agent_badge, priority_badge, toast};
use crate::Route;

/// How long the "state updated" toast stays up before auto-clearing.
const TOAST_DURATION_MS: u32 = 3000;

/// Page 7 -- Task Detail. The primary place a human watches one task's live
/// state: current agent, current action, and -- when the task is blocked on
/// Needs Input -- the question and a way to answer it. No Retry/Cancel here
/// (unlike the design brief's mockup): this pass's RPC interface has no
/// corresponding RPC for either, so the buttons would be inert; left out
/// rather than shipped as fake controls.
#[component]
pub fn TaskDetail(id: String, task_id: String) -> Element {
    let service = use_lineman_service_service();
    let task_id_for_display = task_id.clone();

    let task_id_for_get = task_id.clone();
    let get_req = use_signal(move || GetTaskRequest {
        id: task_id_for_get.clone(),
    });
    let get_result = service.get_task(get_req);

    // The objective's own states -- what a manual state change can move a
    // task into (states are fully custom per objective, not a fixed enum).
    let id_for_obj = id.clone();
    let obj_req = use_signal(move || GetObjectiveRequest {
        id: id_for_obj.clone(),
    });
    let obj_result = service.get_objective(obj_req);

    let mut task: Signal<Option<Task>> = use_signal(|| None);
    use_effect(move || {
        if let Some(Ok(t)) = &*get_result.read() {
            task.set(Some(t.clone()));
        }
    });

    let task_id_for_watch = task_id.clone();
    use_coroutine(move |_rx: UnboundedReceiver<()>| {
        let watched_id = task_id_for_watch.clone();
        async move {
            let wasm_client = WasmClient::new(crate::API_DOMAIN.clone());
            let mut client = LinemanServiceClient::new(wasm_client);
            let Ok(response) = client.watch(WatchRequest {}).await else {
                return;
            };
            let mut stream = response.into_inner();
            loop {
                match stream.message().await {
                    Ok(Some(msg)) => {
                        if let Some(watch_response::Item::Task(t)) = msg.item {
                            if t.id == watched_id && !msg.removed {
                                task.set(Some(t));
                            }
                        }
                    }
                    Ok(None) => break,
                    Err(_) => break,
                }
            }
        }
    });

    let mut toast_message: Signal<Option<String>> = use_signal(|| None);

    let objective_id = id.clone();
    let task_id_for_move = task_id.clone();
    let move_task = move |new_state: String| {
        let task_id = task_id_for_move.clone();
        let new_state_for_toast = new_state.clone();
        spawn(async move {
            let mut client = LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
            if client
                .update_task_state(UpdateTaskStateRequest { task_id, new_state })
                .await
                .is_ok()
            {
                toast_message.set(Some(format!("Moved to {new_state_for_toast}")));
                TimeoutFuture::new(TOAST_DURATION_MS).await;
                toast_message.set(None);
            }
        });
    };

    let respond = move |response: String| {
        let task_id = task_id.clone();
        let next_state = task().map(|t| t.state.clone()).unwrap_or_default();
        spawn(async move {
            let mut client = LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
            let _ = client
                .provide_guidance(ProvideGuidanceRequest {
                    task_id,
                    response,
                    next_state,
                })
                .await;
        });
    };

    rsx! {
        div { class: "flex flex-col gap-4",
            div { class: "breadcrumbs text-sm",
                ul {
                    li { Link { to: Route::Dashboard {}, "Objectives" } }
                    li { Link { to: Route::ObjectiveDetail { id: objective_id.clone() }, "Detail" } }
                    li { "#{task_id_for_display}" }
                }
            }

            match task() {
                Some(t) => {
                    let has_needs_input = t.needs_input.is_some();
                    let states: Vec<String> = match &*obj_result.read() {
                        Some(Ok(obj)) => obj.states.clone(),
                        _ => Vec::new(),
                    };
                    let current_state = t.state.clone();
                    rsx! {
                        div { class: "flex flex-col md:flex-row md:items-start justify-between gap-4 bg-base-200 border border-base-300 rounded-box p-6",
                            div { class: "space-y-2",
                                div { class: "flex items-center gap-2 flex-wrap",
                                    {priority_badge(t.priority)}
                                    span { class: "badge badge-info badge-sm", "{t.state}" }
                                    span { class: "text-xl font-semibold", "{t.name}" }
                                }
                                if !t.agent_id.is_empty() {
                                    {agent_badge(&t.agent_id, !has_needs_input)}
                                }
                            }
                            if !states.is_empty() {
                                div { class: "flex items-center gap-2 shrink-0",
                                    select {
                                        class: "select select-bordered select-sm",
                                        value: "{current_state}",
                                        onchange: move |e| move_task(e.value()),
                                        for s in states {
                                            option { key: "{s}", value: "{s}", "{s}" }
                                        }
                                    }
                                }
                            }
                        }

                        if !t.details.is_empty() {
                            div {
                                h2 { class: "text-sm font-semibold text-base-content/70 mb-2", "Details" }
                                div { class: "card bg-base-200 border border-base-300",
                                    div { class: "card-body p-4",
                                        p { class: "text-sm whitespace-pre-wrap", "{t.details}" }
                                    }
                                }
                            }
                        }

                        if !t.current_action.is_empty() {
                            div {
                                h2 { class: "text-sm font-semibold text-base-content/70 mb-2", "Current Action" }
                                div { class: "card bg-base-200 border border-base-300",
                                    div { class: "card-body p-4 gap-1",
                                        div { class: "flex items-center gap-2 text-sm font-mono",
                                            span { class: "w-1.5 h-1.5 rounded-full bg-info animate-pulse" }
                                            "{t.current_action}"
                                        }
                                    }
                                }
                            }
                        }

                        if let Some(needs_input) = t.needs_input.clone() {
                            div {
                                h2 { class: "text-sm font-semibold text-base-content/70 mb-2", "Needs Input" }
                                div { class: "card bg-warning/10 border border-warning/40",
                                    div { class: "card-body p-4 gap-3",
                                        div { class: "flex items-center gap-2",
                                            span { class: "badge badge-warning badge-sm", "Needs Input" }
                                        }
                                        div { class: "text-sm", "{needs_input.question}" }
                                        div { class: "flex gap-2 flex-wrap",
                                            for option in needs_input.options.clone() {
                                                {
                                                    let opt = option.clone();
                                                    let respond = respond.clone();
                                                    rsx! {
                                                        button {
                                                            key: "{option}",
                                                            class: "btn btn-warning btn-sm",
                                                            onclick: move |_| respond(opt.clone()),
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

                        div {
                            h2 { class: "text-sm font-semibold text-base-content/70 mb-2", "Metadata" }
                            div { class: "card bg-base-200 border border-base-300",
                                div { class: "card-body p-4 gap-2 text-sm",
                                    div { class: "flex justify-between", span { class: "text-base-content/50", "Task" } span { class: "font-mono", "{t.id}" } }
                                    div { class: "flex justify-between", span { class: "text-base-content/50", "State" } span { "{t.state}" } }
                                    if !t.agent_id.is_empty() {
                                        div { class: "flex justify-between", span { class: "text-base-content/50", "Agent" } span { class: "font-mono", "{t.agent_id}" } }
                                    }
                                }
                            }
                        }
                    }
                }
                None => rsx! { div { class: "text-base-content/50 text-sm", "Loading…" } },
            }

            if let Some(msg) = toast_message() {
                {toast(&msg)}
            }
        }
    }
}
