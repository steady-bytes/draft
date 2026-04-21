use dioxus::prelude::*;
use draft_api::hook::core_registry_key_value_v1::{use_key_value_service_service, ListRequest, Value};
use prost::Message as _;
use prost_types::Any;

const VALUE_TYPE_URL: &str = "type.googleapis.com/core.registry.key_value.v1.Value";

#[component]
pub fn KeyValueView() -> Element {
    let list_request = use_signal(|| ListRequest {
        value: Some(Any {
            type_url: VALUE_TYPE_URL.to_string(),
            value: vec![],
        }),
    });

    let service = use_key_value_service_service();
    let list_result = service.list(list_request);

    rsx! {
        div {
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
                                let mut rows: Vec<(String, String, String)> = response.values
                                    .iter()
                                    .map(|(key, any)| {
                                        let type_url = any.type_url.clone();
                                        let data = Value::decode(any.value.as_slice())
                                            .map(|v| v.data)
                                            .unwrap_or_else(|_| "(binary)".to_string());
                                        (key.clone(), data, type_url)
                                    })
                                    .collect();
                                rows.sort_by(|a, b| a.0.cmp(&b.0));
                                rsx! {
                                    for (key, data, type_url) in rows {
                                        tr { class: "hover:bg-base-300",
                                            td { "{key}" }
                                            td { "{data}" }
                                            td { "{type_url}" }
                                        }
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
