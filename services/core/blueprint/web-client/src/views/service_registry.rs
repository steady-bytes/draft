use dioxus::prelude::*;
use draft_api::hook::core_registry_service_discovery_v1::{
    filter, use_service_discovery_service_service, Filter, ProcessHealthState,
    ProcessRunningState, QueryRequest,
};

#[component]
pub fn ServiceRegistry() -> Element {
    let query_request = use_signal(|| QueryRequest {
        filter: Some(Filter {
            attribute: Some(filter::Attribute::All(String::new())),
        }),
    });

    let service = use_service_discovery_service_service();
    let query_result = service.query(query_request);

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
                        match &*query_result.read() {
                            Some(Ok(response)) => {
                                let mut processes: Vec<_> = response.data.values().collect();
                                processes.sort_by(|a, b| a.name.cmp(&b.name));
                                rsx! {
                                    for process in processes {
                                        tr { class: "hover:bg-base-300",
                                            td { "{process.name}" }
                                            td { "{process.pid}" }
                                            td { "{process.ip_address}" }
                                            td {
                                                {ProcessRunningState::try_from(process.running_state)
                                                    .map(|s| s.as_str_name().trim_start_matches("PROCESS_"))
                                                    .unwrap_or("UNKNOWN")}
                                            }
                                            td {
                                                {ProcessHealthState::try_from(process.health_state)
                                                    .map(|s| s.as_str_name().trim_start_matches("PROCESS_"))
                                                    .unwrap_or("UNKNOWN")}
                                            }
                                        }
                                    }
                                }
                            },
                            Some(Err(err)) => rsx! {
                                tr {
                                    td { colspan: "5", class: "text-center text-red-500",
                                        "Error: {err}"
                                    }
                                }
                            },
                            None => rsx! {
                                tr {
                                    td { colspan: "5", class: "text-center",
                                        "Loading..."
                                    }
                                }
                            },
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
