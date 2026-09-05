use std::collections::{BTreeMap, HashMap};

use dioxus::prelude::*;
use dioxus_grpc::GrpcConfig;
use draft_api::{
    hook::core_registry_service_discovery_v1::{
        filter, use_service_discovery_service_service, Filter, QueryRequest,
    },
    proto::core_registry_service_discovery_v1::{
        service_discovery_service_client::ServiceDiscoveryServiceClient, Process,
        ProcessHealthState, ProcessRunningState, WatchRequest,
    },
};
use tonic_web_wasm_client::Client as WasmClient;

use crate::Route as AppRoute;

#[component]
pub fn ServiceRegistry() -> Element {
    let query_request = use_signal(|| QueryRequest {
        filter: Some(Filter {
            attribute: Some(filter::Attribute::All(String::new())),
        }),
    });

    let service = use_service_discovery_service_service();
    let query_result = service.query(query_request);

    // processes map seeded by Query and updated by Watch
    let mut processes: Signal<
        HashMap<String, draft_api::proto::core_registry_service_discovery_v1::Process>,
    > = use_signal(HashMap::new);

    // seed from query result
    use_effect(move || {
        if let Some(Ok(ref resp)) = *query_result.read() {
            *processes.write() = resp.data.clone();
        }
    });

    // background coroutine: open Watch stream and upsert updates
    let config = use_context::<GrpcConfig>();
    use_coroutine(move |_rx: UnboundedReceiver<()>| {
        let host = config.host.clone();
        async move {
            let wasm_client = WasmClient::new(host);
            let mut client = ServiceDiscoveryServiceClient::new(wasm_client);
            let Ok(response) = client.watch(WatchRequest {}).await else {
                return;
            };
            let mut stream = response.into_inner();
            loop {
                match stream.message().await {
                    Ok(Some(msg)) => {
                        if msg.removed {
                            if let Some(process) = msg.process {
                                processes.write().remove(&process.pid);
                            }
                        } else if let Some(process) = msg.process {
                            processes.write().insert(process.pid.clone(), process);
                        }
                    }
                    Ok(None) => break,
                    Err(_) => break,
                }
            }
        }
    });

    // Group by `name` — Blueprint's registry can (today, pending the
    // deterministic-identity fix) hold several rows for the same logical
    // service, one per restart's `pid`. Showing one row per `pid` here would
    // surface that as visible duplication; grouping by name instead shows
    // one row per logical service, with a per-instance breakdown one click
    // away on the detail page.
    let mut by_name: BTreeMap<String, Vec<Process>> = BTreeMap::new();
    for process in processes.read().values() {
        by_name
            .entry(process.name.clone())
            .or_default()
            .push(process.clone());
    }

    let navigator = use_navigator();

    rsx! {
        div {
            div { class: "overflow-x-auto",
                table { class: "table table-xs",
                    thead {
                        tr {
                            th { "Name" }
                            th { "Instances" }
                            th { "Running State" }
                            th { "Health" }
                        }
                    }
                    tbody {
                        if by_name.is_empty() {
                            if query_result.read().is_none() {
                                tr {
                                    td { colspan: "4", class: "text-center", "Loading..." }
                                }
                            } else {
                                tr {
                                    td { colspan: "4", class: "text-center text-base-content/50",
                                        "No processes registered"
                                    }
                                }
                            }
                        }
                        for (name , instances) in by_name {
                            {
                                let row_name = name.clone();
                                let count = instances.len();

                                let running_count = instances.iter().filter(|p| p.running_state == ProcessRunningState::ProcessRunning as i32).count();
                                let disconnected_count = instances.iter().filter(|p| p.running_state == ProcessRunningState::ProcessDiconnected as i32).count();
                                let starting_count = instances.iter().filter(|p| p.running_state == ProcessRunningState::ProcessStarting as i32).count();

                                let healthy_count = instances.iter().filter(|p| p.health_state == ProcessHealthState::ProcessHealthy as i32).count();
                                let unhealthy_count = instances.iter().filter(|p| p.health_state == ProcessHealthState::ProcessUnhealthy as i32).count();

                                rsx! {
                                    tr {
                                        class: "hover:bg-base-300 cursor-pointer",
                                        onclick: move |_| { navigator.push(AppRoute::ServiceDetail { name: row_name.clone() }); },
                                        td { "{name}" }
                                        td { "{count}" }
                                        td {
                                            div { class: "flex gap-1 flex-wrap",
                                                if running_count > 0 {
                                                    span { class: "badge badge-success badge-sm", "{running_count} running" }
                                                }
                                                if starting_count > 0 {
                                                    span { class: "badge badge-warning badge-sm", "{starting_count} starting" }
                                                }
                                                if disconnected_count > 0 {
                                                    span { class: "badge badge-error badge-sm", "{disconnected_count} disconnected" }
                                                }
                                            }
                                        }
                                        td {
                                            div { class: "flex gap-1 flex-wrap",
                                                if healthy_count > 0 {
                                                    span { class: "badge badge-success badge-sm", "{healthy_count} healthy" }
                                                }
                                                if unhealthy_count > 0 {
                                                    span { class: "badge badge-error badge-sm", "{unhealthy_count} unhealthy" }
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                    tfoot {
                        tr {
                            th { "Name" }
                            th { "Instances" }
                            th { "Running State" }
                            th { "Health" }
                        }
                    }
                }
            }
        }
    }
}
