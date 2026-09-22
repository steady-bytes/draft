use dioxus::prelude::*;
use draft_api::hook::tooling_lineman_v1::use_lineman_service_service;
use draft_api::proto::tooling_lineman_v1::{
    lineman_service_client::LinemanServiceClient, watch_response, GetObjectiveRequest,
    ListTasksRequest, Task, WatchRequest,
};
use tonic_web_wasm_client::Client as WasmClient;

use crate::components::{agent_badge, priority_badge};
use crate::Route;

/// Page 2 -- Task Board. Columns are the objective's own states (fully
/// custom, not a fixed enum -- see the implementation plan's Decisions).
/// Live-updates via the raw Watch stream (not the generated hook, which
/// only covers unary RPCs) -- upserts by task id so a card's state/agent/
/// current_action reflects the real system without polling.
#[component]
pub fn TaskBoard(id: String) -> Element {
    let service = use_lineman_service_service();

    let id_for_obj = id.clone();
    let obj_req = use_signal(move || GetObjectiveRequest {
        id: id_for_obj.clone(),
    });
    let obj_result = service.get_objective(obj_req);

    let id_for_list = id.clone();
    let list_req = use_signal(move || ListTasksRequest {
        objective_id: id_for_list.clone(),
        ..Default::default()
    });
    let list_result = service.list_tasks(list_req);

    let mut tasks: Signal<Vec<Task>> = use_signal(Vec::new);

    use_effect(move || {
        if let Some(Ok(resp)) = &*list_result.read() {
            tasks.set(resp.tasks.clone());
        }
    });

    let id_for_watch = id.clone();
    use_coroutine(move |_rx: UnboundedReceiver<()>| {
        let objective_id = id_for_watch.clone();
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
                        let removed = msg.removed;
                        if let Some(watch_response::Item::Task(t)) = msg.item {
                            if t.objective_id != objective_id {
                                continue;
                            }
                            let mut list = tasks.write();
                            if removed {
                                list.retain(|x| x.id != t.id);
                            } else if let Some(existing) = list.iter_mut().find(|x| x.id == t.id) {
                                *existing = t;
                            } else {
                                list.push(t);
                            }
                        }
                    }
                    Ok(None) => break,
                    Err(_) => break,
                }
            }
        }
    });

    let obj_id = id.clone();
    let states = match &*obj_result.read() {
        Some(Ok(obj)) => obj.states.clone(),
        _ => Vec::new(),
    };

    rsx! {
        div { class: "flex flex-col gap-4",
            div { class: "breadcrumbs text-sm",
                ul {
                    li { Link { to: Route::Dashboard {}, "Objectives" } }
                    li { Link { to: Route::ObjectiveDetail { id: obj_id.clone() }, "Detail" } }
                    li { "Board" }
                }
            }
            div { class: "flex gap-4 overflow-x-auto pb-4",
                for state in states {
                    {
                        let col_tasks: Vec<Task> = tasks().into_iter().filter(|t| t.state == state).collect();
                        let count = col_tasks.len();
                        rsx! {
                            div { key: "{state}", class: "w-64 shrink-0",
                                div { class: "flex items-center justify-between px-1 mb-2",
                                    span { class: "font-semibold text-sm", "{state}" }
                                    span { class: "badge badge-sm", "{count}" }
                                }
                                div { class: "space-y-2",
                                    for t in col_tasks {
                                        {
                                            let pulsing = t.needs_input.is_none();
                                            let obj_id2 = obj_id.clone();
                                            rsx! {
                                                Link {
                                                    key: "{t.id}",
                                                    to: Route::TaskDetail { id: obj_id2, task_id: t.id.clone() },
                                                    class: "card bg-base-200 border border-base-300 hover:border-info/50 block",
                                                    div { class: "card-body p-3 gap-1",
                                                        {priority_badge(t.priority)}
                                                        div { class: "text-sm font-medium", "{t.name}" }
                                                        if !t.agent_id.is_empty() {
                                                            {agent_badge(&t.agent_id, pulsing)}
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
}
