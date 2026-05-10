use std::collections::HashMap;

use dioxus::prelude::*;
use draft_api::{
    hook::core_registry_service_discovery_v1::{
        filter, use_service_discovery_service_service, Filter, QueryRequest,
    },
    proto::core_registry_service_discovery_v1::{
        service_discovery_service_client::ServiceDiscoveryServiceClient, ProcessHealthState,
        ProcessRunningState, WatchRequest,
    },
};
use dioxus_grpc::GrpcConfig;
use tonic_web_wasm_client::Client as WasmClient;

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
    let mut processes: Signal<HashMap<String, draft_api::proto::core_registry_service_discovery_v1::Process>> =
        use_signal(HashMap::new);

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

    let mut sorted: Vec<_> = processes.read().values().cloned().collect();
    sorted.sort_by(|a, b| a.name.cmp(&b.name));

    rsx! {
        div {
            div { class: "overflow-x-auto",
                table { class: "table table-xs",
                    thead {
                        tr {
                            th { "Name" }
                            th { "PID" }
                            th { "IP Address" }
                            th { "Running State" }
                            th { "Health" }
                        }
                    }
                    tbody {
                        if sorted.is_empty() {
                            if query_result.read().is_none() {
                                tr {
                                    td { colspan: "5", class: "text-center", "Loading..." }
                                }
                            } else {
                                tr {
                                    td { colspan: "5", class: "text-center text-base-content/50",
                                        "No processes registered"
                                    }
                                }
                            }
                        }
                        for process in sorted {
                            {
                                let running = ProcessRunningState::try_from(process.running_state)
                                    .map(|s| s.as_str_name().trim_start_matches("PROCESS_").to_string())
                                    .unwrap_or_else(|_| "UNKNOWN".to_string());
                                let health = ProcessHealthState::try_from(process.health_state)
                                    .map(|s| s.as_str_name().trim_start_matches("PROCESS_").to_string())
                                    .unwrap_or_else(|_| "UNKNOWN".to_string());

                                let running_class = match process.running_state {
                                    s if s == ProcessRunningState::ProcessRunning as i32 => "badge badge-success badge-sm",
                                    s if s == ProcessRunningState::ProcessDiconnected as i32 => "badge badge-error badge-sm",
                                    s if s == ProcessRunningState::ProcessStarting as i32 => "badge badge-warning badge-sm",
                                    _ => "badge badge-ghost badge-sm",
                                };
                                let health_class = match process.health_state {
                                    s if s == ProcessHealthState::ProcessHealthy as i32 => "badge badge-success badge-sm",
                                    s if s == ProcessHealthState::ProcessUnhealthy as i32 => "badge badge-error badge-sm",
                                    _ => "badge badge-ghost badge-sm",
                                };

                                rsx! {
                                    tr { class: "hover:bg-base-300",
                                        td { "{process.name}" }
                                        td { class: "font-mono text-xs", "{process.pid}" }
                                        td { "{process.ip_address}" }
                                        td { span { class: "{running_class}", "{running}" } }
                                        td { span { class: "{health_class}", "{health}" } }
                                    }
                                }
                            }
                        }
                    }
                    tfoot {
                        tr {
                            th { "Name" }
                            th { "PID" }
                            th { "IP Address" }
                            th { "Running State" }
                            th { "Health" }
                        }
                    }
                }
            }
        }
    }
}
