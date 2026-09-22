use std::collections::HashMap;

use dioxus::prelude::*;
use draft_api::hook::tooling_lineman_v1::use_lineman_service_service;
use draft_api::proto::tooling_lineman_v1::{
    GetObjectiveRequest, ListAgentsRequest, ListLoopsRequest, ListScheduledTasksRequest,
    ListTasksRequest,
};

use draft_api::proto::tooling_lineman_v1::{
    lineman_service_client::LinemanServiceClient, CreateTaskRequest, LoopStatus, Priority,
    ScheduledTaskStatus,
};
use tonic_web_wasm_client::Client as WasmClient;

use crate::components::{agent_badge, priority_badge};
use crate::Route;

/// Page 4 -- Objective Detail. Per-state task counts (not a fixed
/// Queued/In-Flight/Done split) since states are fully custom per
/// objective -- see the implementation plan's Decisions.
#[component]
pub fn ObjectiveDetail(id: String) -> Element {
    let service = use_lineman_service_service();

    let obj_req = use_signal(move || GetObjectiveRequest { id: id.clone() });
    let obj_result = service.get_objective(obj_req);

    // Raw use_resource (not the generated hook) so creating a task can force
    // a refetch by bumping `refresh` -- the hook's Resource only re-fires
    // when the request itself changes, and objective_id never does here.
    // Same pattern as scheduler.rs/loops.rs.
    let mut refresh = use_signal(|| 0u32);
    let id_for_tasks = obj_req().id.clone();
    let tasks_result = use_resource(move || {
        let objective_id = id_for_tasks.clone();
        let _ = refresh();
        async move {
            let mut client = LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
            client
                .list_tasks(ListTasksRequest {
                    objective_id,
                    ..Default::default()
                })
                .await
                .map(|r| r.into_inner())
        }
    });

    let mut new_task_name = use_signal(String::new);
    let mut new_task_details = use_signal(String::new);
    let mut new_task_priority = use_signal(|| "PRIORITY_MEDIUM".to_string());
    let mut new_task_error = use_signal(|| Option::<String>::None);
    let obj_id_for_submit = obj_req().id.clone();
    let submit_task = move |_| {
        let name = new_task_name();
        if name.trim().is_empty() {
            new_task_error.set(Some("Name is required".to_string()));
            return;
        }
        let details = new_task_details();
        let objective_id = obj_id_for_submit.clone();
        let priority = Priority::from_str_name(&new_task_priority()).unwrap_or(Priority::Medium);
        spawn(async move {
            let mut client = LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
            match client
                .create_task(CreateTaskRequest {
                    objective_id,
                    name,
                    details,
                    priority: priority as i32,
                    state: String::new(),
                })
                .await
            {
                Ok(_) => {
                    new_task_name.set(String::new());
                    new_task_details.set(String::new());
                    new_task_error.set(None);
                    refresh.set(refresh() + 1);
                }
                Err(err) => new_task_error.set(Some(err.to_string())),
            }
        });
    };

    let agents_req = use_signal(ListAgentsRequest::default);
    let agents_result = service.list_agents(agents_req);

    let id_for_sched = obj_req().id.clone();
    let sched_req = use_signal(move || ListScheduledTasksRequest {
        objective_id: id_for_sched.clone(),
    });
    let sched_result = service.list_scheduled_tasks(sched_req);

    let id_for_loops = obj_req().id.clone();
    let loops_req = use_signal(move || ListLoopsRequest {
        objective_id: id_for_loops.clone(),
    });
    let loops_result = service.list_loops(loops_req);

    let obj_id = obj_req().id.clone();

    // Agents actually assigned to one of this objective's tasks -- not the
    // whole system-wide agent roster (list_agents has no objective filter of
    // its own, since Agent doesn't carry an objective_id).
    let assigned_agent_ids: std::collections::HashSet<String> = match &*tasks_result.read() {
        Some(Ok(resp)) => resp
            .tasks
            .iter()
            .filter(|t| !t.agent_id.is_empty())
            .map(|t| t.agent_id.clone())
            .collect(),
        _ => Default::default(),
    };

    rsx! {
        div { class: "flex flex-col gap-6",
            match &*obj_result.read() {
                Some(Ok(obj)) => {
                    let obj = obj.clone();
                    rsx! {
                        div { class: "breadcrumbs text-sm",
                            ul {
                                li { Link { to: Route::Dashboard {}, "Objectives" } }
                                li { "{obj.name}" }
                            }
                        }
                        div { class: "flex items-center justify-between",
                            div {
                                h1 { class: "text-2xl font-bold", "{obj.name}" }
                                p { class: "text-base-content/70 text-sm", "{obj.description}" }
                            }
                            Link {
                                to: Route::TaskBoard { id: obj_id.clone() },
                                class: "btn btn-outline btn-sm",
                                "View Board →"
                            }
                        }
                    }
                }
                Some(Err(err)) => rsx! { div { class: "text-error text-sm", "Failed to load objective: {err}" } },
                None => rsx! { div { class: "text-base-content/50 text-sm", "Loading…" } },
            }

            div { class: "card bg-base-200 border border-base-300 max-w-lg",
                div { class: "card-body p-4 gap-2",
                    h2 { class: "text-sm font-semibold text-base-content/70", "Add Task" }
                    if let Some(msg) = new_task_error() {
                        div { class: "alert alert-error text-xs py-2", "{msg}" }
                    }
                    input {
                        class: "input input-bordered input-sm w-full",
                        placeholder: "Name",
                        value: "{new_task_name}",
                        oninput: move |e| new_task_name.set(e.value()),
                    }
                    textarea {
                        class: "textarea textarea-bordered textarea-sm w-full",
                        placeholder: "Details -- everything needed to complete this task (optional)",
                        value: "{new_task_details}",
                        oninput: move |e| new_task_details.set(e.value()),
                    }
                    div { class: "flex gap-2",
                        select {
                            class: "select select-bordered select-sm",
                            value: "{new_task_priority}",
                            onchange: move |e| new_task_priority.set(e.value()),
                            option { value: "PRIORITY_LOW", "Low" }
                            option { value: "PRIORITY_MEDIUM", "Medium" }
                            option { value: "PRIORITY_HIGH", "High" }
                        }
                        button { class: "btn btn-primary btn-sm", onclick: submit_task, "Add" }
                    }
                }
            }

            match &*tasks_result.read() {
                Some(Ok(resp)) => {
                    let mut by_state: HashMap<String, usize> = HashMap::new();
                    for t in &resp.tasks {
                        *by_state.entry(t.state.clone()).or_insert(0) += 1;
                    }
                    let mut counts: Vec<(String, usize)> = by_state.into_iter().collect();
                    counts.sort_by(|a, b| a.0.cmp(&b.0));
                    let in_flight: Vec<_> = resp.tasks.iter().filter(|t| !t.agent_id.is_empty()).cloned().collect();
                    let unassigned: Vec<_> = resp.tasks.iter().filter(|t| t.agent_id.is_empty()).cloned().collect();

                    rsx! {
                        div { class: "stats shadow bg-base-200 w-fit flex-wrap",
                            div { class: "stat",
                                div { class: "stat-title", "Total tasks" }
                                div { class: "stat-value text-lg", "{resp.tasks.len()}" }
                            }
                            for (state, count) in counts {
                                div { key: "{state}", class: "stat",
                                    div { class: "stat-title", "{state}" }
                                    div { class: "stat-value text-lg", "{count}" }
                                }
                            }
                        }

                        if !unassigned.is_empty() {
                            div {
                                h2 { class: "text-sm font-semibold text-base-content/70 mb-2", "Unassigned" }
                                ul { class: "menu bg-base-200 rounded-box border border-base-300 p-0",
                                    for t in unassigned {
                                        li { key: "{t.id}",
                                            Link { to: Route::TaskDetail { id: obj_id.clone(), task_id: t.id.clone() },
                                                span { class: "flex-1", "{t.name}" }
                                                span { class: "badge badge-ghost badge-xs", "{t.state}" }
                                            }
                                        }
                                    }
                                }
                            }
                        }

                        if !in_flight.is_empty() {
                            div {
                                h2 { class: "text-sm font-semibold text-base-content/70 mb-2", "In Flight" }
                                div { class: "space-y-2",
                                    for t in in_flight {
                                        Link {
                                            key: "{t.id}",
                                            to: Route::TaskDetail { id: obj_id.clone(), task_id: t.id.clone() },
                                            class: "card bg-base-200 border border-base-300 hover:border-info/50 block",
                                            div { class: "card-body p-3 gap-1",
                                                {priority_badge(t.priority)}
                                                div { class: "text-sm font-medium", "{t.name}" }
                                                {agent_badge(&t.agent_id, t.needs_input.is_none())}
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
                Some(Err(err)) => rsx! { div { class: "text-error text-sm", "Failed to load tasks: {err}" } },
                None => rsx! {},
            }

            match &*agents_result.read() {
                Some(Ok(resp)) => {
                    let assigned: Vec<_> = resp
                        .agents
                        .iter()
                        .filter(|a| assigned_agent_ids.contains(&a.id))
                        .cloned()
                        .collect();
                    if assigned.is_empty() {
                        rsx! {}
                    } else {
                        rsx! {
                            div {
                                h2 { class: "text-sm font-semibold text-base-content/70 mb-2", "Active Agents" }
                                div { class: "overflow-x-auto",
                                    table { class: "table table-xs",
                                        thead { tr { th { "Agent" } th { "Kind" } th { "Last seen" } } }
                                        tbody {
                                            for a in assigned {
                                                tr { key: "{a.id}",
                                                    td { class: "font-mono text-xs", "{a.display_name}" }
                                                    td { {crate::components::agent_kind_label(a.kind)} }
                                                    td { class: "text-xs text-base-content/50",
                                                        {a.last_seen_at.map(|t| t.seconds).unwrap_or(0).to_string()}
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
                _ => rsx! {},
            }

            match &*sched_result.read() {
                Some(Ok(resp)) if !resp.scheduled_tasks.is_empty() => rsx! {
                    div {
                        h2 { class: "text-sm font-semibold text-base-content/70 mb-2", "Scheduled" }
                        ul { class: "menu bg-base-200 rounded-box border border-base-300 p-0",
                            for s in resp.scheduled_tasks.clone() {
                                li { key: "{s.id}",
                                    Link { to: Route::Scheduler {},
                                        span { class: "flex-1", "{s.description}" }
                                        span { class: "badge badge-ghost badge-xs",
                                            {ScheduledTaskStatus::try_from(s.status).unwrap_or(ScheduledTaskStatus::Unspecified).as_str_name()}
                                        }
                                    }
                                }
                            }
                        }
                    }
                },
                _ => rsx! {},
            }

            match &*loops_result.read() {
                Some(Ok(resp)) if !resp.loops.is_empty() => rsx! {
                    div {
                        h2 { class: "text-sm font-semibold text-base-content/70 mb-2", "Loops" }
                        ul { class: "menu bg-base-200 rounded-box border border-base-300 p-0",
                            for l in resp.loops.clone() {
                                li { key: "{l.id}",
                                    Link { to: Route::Loops {},
                                        span { class: "flex-1", "{l.description}" }
                                        span { class: "badge badge-ghost badge-xs",
                                            {LoopStatus::try_from(l.status).unwrap_or(LoopStatus::Unspecified).as_str_name()}
                                        }
                                    }
                                }
                            }
                        }
                    }
                },
                _ => rsx! {},
            }
        }
    }
}
