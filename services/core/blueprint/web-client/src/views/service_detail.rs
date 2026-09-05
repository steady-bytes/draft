use std::collections::HashMap;

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
use prost_types::Timestamp;
use tonic_web_wasm_client::Client as WasmClient;

use crate::Route as AppRoute;

/// `/service-registry/:name` — one card per registered `pid` sharing this
/// `name`, so a service with several rows (past or present restarts, or
/// genuinely horizontally-scaled replicas) can be inspected individually
/// instead of being flattened into the grouped list's aggregate counts.
#[component]
pub fn ServiceDetail(name: String) -> Element {
    let query_request = use_signal(|| QueryRequest {
        filter: Some(Filter {
            attribute: Some(filter::Attribute::All(String::new())),
        }),
    });

    let service = use_service_discovery_service_service();
    let query_result = service.query(query_request);

    let mut processes: Signal<HashMap<String, Process>> = use_signal(HashMap::new);

    use_effect(move || {
        if let Some(Ok(ref resp)) = *query_result.read() {
            *processes.write() = resp.data.clone();
        }
    });

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

    let mut instances: Vec<Process> = processes
        .read()
        .values()
        .filter(|p| p.name == name)
        .cloned()
        .collect();
    instances.sort_by(|a, b| {
        b.last_status_time
            .as_ref()
            .map(|t| (t.seconds, t.nanos))
            .cmp(&a.last_status_time.as_ref().map(|t| (t.seconds, t.nanos)))
    });

    let total = instances.len();
    let healthy_count = instances.iter().filter(|p| p.health_state == ProcessHealthState::ProcessHealthy as i32).count();
    let unhealthy_count = instances.iter().filter(|p| p.health_state == ProcessHealthState::ProcessUnhealthy as i32).count();

    rsx! {
        div { class: "p-4 flex flex-col gap-4",
            div { class: "breadcrumbs text-sm",
                ul {
                    li { Link { to: AppRoute::ServiceRegistry {}, "Service Registry" } }
                    li { "{name}" }
                }
            }

            div { class: "stats shadow bg-base-200 w-fit",
                div { class: "stat",
                    div { class: "stat-title", "Registered" }
                    div { class: "stat-value", "{total}" }
                }
                div { class: "stat",
                    div { class: "stat-title", "Healthy" }
                    div { class: "stat-value text-success", "{healthy_count}" }
                }
                div { class: "stat",
                    div { class: "stat-title", "Unhealthy" }
                    div { class: "stat-value text-error", "{unhealthy_count}" }
                }
            }

            if instances.is_empty() {
                div { class: "text-center text-base-content/50 py-12",
                    if query_result.read().is_none() {
                        "Loading..."
                    } else {
                        "No processes registered under this name."
                    }
                }
            } else {
                div { class: "grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4",
                    for process in instances {
                        ProcessCard { key: "{process.pid}", process }
                    }
                }
            }
        }
    }
}

#[component]
fn ProcessCard(process: Process) -> Element {
    let running = ProcessRunningState::try_from(process.running_state)
        .map(|s| s.as_str_name().trim_start_matches("PROCESS_").to_string())
        .unwrap_or_else(|_| "UNKNOWN".to_string());
    let health = ProcessHealthState::try_from(process.health_state)
        .map(|s| s.as_str_name().trim_start_matches("PROCESS_").to_string())
        .unwrap_or_else(|_| "UNKNOWN".to_string());

    let running_class = match process.running_state {
        s if s == ProcessRunningState::ProcessRunning as i32 => "badge badge-success",
        s if s == ProcessRunningState::ProcessDiconnected as i32 => "badge badge-error",
        s if s == ProcessRunningState::ProcessStarting as i32 => "badge badge-warning",
        _ => "badge badge-ghost",
    };
    let health_class = match process.health_state {
        s if s == ProcessHealthState::ProcessHealthy as i32 => "badge badge-success",
        s if s == ProcessHealthState::ProcessUnhealthy as i32 => "badge badge-error",
        _ => "badge badge-ghost",
    };

    rsx! {
        div { class: "card bg-base-200 border border-base-300 shadow-sm",
            div { class: "card-body p-4 gap-2",
                div { class: "flex items-center justify-between gap-2",
                    span { class: "font-mono text-xs break-all", "{process.pid}" }
                }
                div { class: "flex gap-2 flex-wrap",
                    span { class: "{running_class}", "{running}" }
                    span { class: "{health_class}", "{health}" }
                }
                div { class: "text-xs text-base-content/70 flex flex-col gap-1 mt-1",
                    div { "IP Address: " span { class: "font-mono", "{process.ip_address}" } }
                    if !process.group.is_empty() {
                        div { "Group: " span { class: "font-mono", "{process.group}" } }
                    }
                    div { "Joined: " "{format_timestamp(process.joined_time.as_ref())}" }
                    div { "Last Status: " "{format_timestamp(process.last_status_time.as_ref())}" }
                }
            }
        }
    }
}

fn format_timestamp(ts: Option<&Timestamp>) -> String {
    match ts {
        Some(ts) => match chrono::DateTime::from_timestamp(ts.seconds, ts.nanos as u32) {
            Some(dt) => dt.format("%Y-%m-%d %H:%M:%S").to_string(),
            None => "—".to_string(),
        },
        None => "—".to_string(),
    }
}
