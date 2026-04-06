use std::collections::HashMap;
use once_cell::sync::Lazy;
use dioxus::prelude::*;

// Import gRPC hook and types
use crate::grpc_client::KeyValueServiceHook;
use draft_api::ListRequest;
use draft_api::prost_types::Any;

pub static PATH: Lazy<String> = Lazy::new(|| {
    "/core.registry.key_value.v1.KeyValueService/List".to_string()
});

#[component]
pub fn KeyValueView() -> Element {
    // Create the gRPC list request with empty value filter
    let mut list_request = use_signal(|| {
        ListRequest {
            value: Some(Any {
                type_url: "type.googleapis.com/core.registry.key_value.v1.Value".to_string(),
                value: vec![],
            }),
        }
    });

    // Initialize the gRPC service hook
    let service = KeyValueServiceHook::new();

    // Use the hook to make the gRPC call
    let list_result = service.list(list_request);

    rsx! {
        div {
            // TODO: Add a loading spinner
            div { class: "overflow-x-auto",
                table { class: "table table-xs",
                    thead {
                        tr {
                            th { "Key" }
                            th { "Value" }
                            th { "Type Url" }
                        }
                    }
                    tbody {
                        match &*list_result.read() {
                            Some(Ok(response)) => {
                                let values = response
                                    .get("values")
                                    .and_then(|v| v.as_object())
                                    .map(|m| m.clone())
                                    .unwrap_or_default();
                                rsx! {
                                    {
                                        values.iter().map(|(key, val)| {
                                            let type_url = val
                                                .get("typeUrl")
                                                .and_then(|v| v.as_str())
                                                .unwrap_or("")
                                                .to_string();
                                            let data = val
                                                .get("value")
                                                .and_then(|v| v.as_str())
                                                .unwrap_or("")
                                                .to_string();
                                            rsx! {
                                                tr { class: "hover:bg-base-300",
                                                    td { "{key}" }
                                                    td { "{data}" }
                                                    td { "{type_url}" }
                                                }
                                            }
                                        })
                                    }
                                }
                            },
                            Some(Err(err)) => rsx! {
                                tr {
                                    td { colspan: "3", class: "text-center text-red-500",
                                        "Error: {err}"
                                    }
                                }
                            },
                            None => rsx! {
                                tr {
                                    td { colspan: "3", class: "text-center",
                                        "Loading..."
                                    }
                                }
                            },
                        }
                    }
                    tfoot {
                        tr {
                            th { "Key" }
                            th { "Value" }
                            th { "Type Url" }
                        }
                    }
                }
            }
        }
    }
}
