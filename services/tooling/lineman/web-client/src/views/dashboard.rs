use dioxus::prelude::*;
use draft_api::hook::tooling_lineman_v1::use_lineman_service_service;
use draft_api::proto::tooling_lineman_v1::ListObjectivesRequest;

use crate::Route;

/// Page 1 -- Dashboard. A simple HUD listing every objective, per idea.md
/// ("basic metrics of tasks being completed for objectives and objectives
/// that are in flight").
#[component]
pub fn Dashboard() -> Element {
    let service = use_lineman_service_service();
    let req = use_signal(ListObjectivesRequest::default);
    let result = service.list_objectives(req);

    rsx! {
        div { class: "flex flex-col gap-4",
            div { class: "flex items-center justify-between",
                h1 { class: "text-2xl font-bold", "Objectives" }
                Link { to: Route::CreateObjective {}, class: "btn btn-primary btn-sm", "+ Create Objective" }
            }

            match &*result.read() {
                Some(Ok(resp)) if !resp.objectives.is_empty() => rsx! {
                    div { class: "grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4",
                        for obj in resp.objectives.clone() {
                            Link {
                                key: "{obj.id}",
                                to: Route::ObjectiveDetail { id: obj.id.clone() },
                                class: "card bg-base-200 border border-base-300 shadow-sm hover:border-info/50",
                                div { class: "card-body p-4 gap-1",
                                    h3 { class: "card-title text-base", "{obj.name}" }
                                    p { class: "text-sm text-base-content/70", "{obj.description}" }
                                    div { class: "flex gap-1 flex-wrap mt-1",
                                        for state in obj.states.clone() {
                                            span { key: "{state}", class: "badge badge-ghost badge-xs", "{state}" }
                                        }
                                    }
                                }
                            }
                        }
                    }
                },
                Some(Ok(_)) => rsx! {
                    div { class: "text-center text-base-content/50 py-12",
                        "No objectives yet. "
                        Link { to: Route::CreateObjective {}, class: "link", "Create one" }
                        "."
                    }
                },
                Some(Err(err)) => rsx! {
                    div { class: "text-error text-sm", "Failed to load objectives: {err}" }
                },
                None => rsx! {
                    div { class: "text-base-content/50 text-sm", "Loading…" }
                },
            }
        }
    }
}
