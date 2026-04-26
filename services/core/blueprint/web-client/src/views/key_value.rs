use dioxus::prelude::*;
use gloo_timers::future::TimeoutFuture;
use draft_api::hook::core_registry_key_value_v1::{
    key_value_service_client::KeyValueServiceClient,
    DeleteRequest, ListRequest, SetRequest, Value,
};
use prost::Message as _;
use prost_types::Any;
use tonic_web_wasm_client::Client as WasmClient;

const VALUE_TYPE_URL: &str = "type.googleapis.com/core.registry.key_value.v1.Value";

#[component]
pub fn KeyValueView() -> Element {
    let mut list_result = use_resource(|| async {
        let mut client = KeyValueServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
        client.list(ListRequest {
            value: Some(Any {
                type_url: VALUE_TYPE_URL.to_string(),
                value: vec![],
            }),
        }).await.map(|r| r.into_inner())
    });

    let mut show_modal = use_signal(|| false);
    let mut form_key = use_signal(String::new);
    let mut form_value = use_signal(String::new);
    let mut status: Signal<Option<String>> = use_signal(|| None);

    let submit = move |_| {
        let key = form_key();
        let value = form_value();
        spawn(async move {
            let mut client = KeyValueServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
            let any = Any {
                type_url: VALUE_TYPE_URL.to_string(),
                value: Value { data: value }.encode_to_vec(),
            };
            match client.set(SetRequest { key, value: Some(any) }).await {
                Ok(_) => {
                    show_modal.set(false);
                    form_key.set(String::new());
                    form_value.set(String::new());
                    list_result.restart();
                    status.set(Some("Key/value saved.".to_string()));
                    spawn(async move {
                        TimeoutFuture::new(3_000).await;
                        status.set(None);
                    });
                }
                Err(e) => status.set(Some(format!("Error: {e}"))),
            }
        });
    };

    rsx! {
        div { class: "p-4",
            div { class: "flex items-center justify-between mb-4",
                h1 { class: "text-2xl font-bold", "Key / Value" }
                button {
                    class: "btn btn-primary btn-sm",
                    onclick: move |_| {
                        form_key.set(String::new());
                        form_value.set(String::new());
                        show_modal.set(true);
                    },
                    "+ Add Entry"
                }
            }

            if let Some(msg) = status() {
                div { class: "toast toast-end toast-bottom z-50",
                    div { class: "alert alert-success",
                        span { "{msg}" }
                    }
                }
            }

            div { class: "overflow-x-auto",
                table { class: "table table-xs",
                    thead {
                        tr {
                            th { "Key" }
                            th { "Value" }
                            th { "Type Url" }
                            th { "" }
                        }
                    }
                    tbody {
                        match &*list_result.read() {
                            Some(Ok(response)) => {
                                let key_prefix = format!("{}-", VALUE_TYPE_URL);
                                let mut rows: Vec<(String, String, String)> = response.values
                                    .iter()
                                    .map(|(key, any)| {
                                        let type_url = any.type_url.clone();
                                        let data = Value::decode(any.value.as_slice())
                                            .map(|v| v.data)
                                            .unwrap_or_else(|_| "(binary)".to_string());
                                        let display_key = key.strip_prefix(key_prefix.as_str()).unwrap_or(key).to_string();
                                        (display_key, data, type_url)
                                    })
                                    .collect();
                                rows.sort_by(|a, b| a.0.cmp(&b.0));
                                rsx! {
                                    for (key, data, type_url) in rows {
                                        {
                                            let delete_key = key.clone();
                                            rsx! {
                                                tr { class: "hover:bg-base-300",
                                                    td { "{key}" }
                                                    td { "{data}" }
                                                    td { "{type_url}" }
                                                    td {
                                                        button {
                                                            class: "btn btn-xs btn-error",
                                                            onclick: move |_| {
                                                                let key = delete_key.clone();
                                                                spawn(async move {
                                                                    let mut client = KeyValueServiceClient::new(
                                                                        WasmClient::new(crate::API_DOMAIN.clone())
                                                                    );
                                                                    let _ = client.delete(DeleteRequest {
                                                                        key,
                                                                        value: Some(Any {
                                                                            type_url: VALUE_TYPE_URL.to_string(),
                                                                            value: vec![],
                                                                        }),
                                                                    }).await;
                                                                    list_result.restart();
                                                                });
                                                            },
                                                            "Delete"
                                                        }
                                                    }
                                                }
                                            }
                                        }
                                    }
                                }
                            },
                            Some(Err(err)) => rsx! {
                                tr {
                                    td { colspan: "4", class: "text-center text-red-500",
                                        "Error: {err}"
                                    }
                                }
                            },
                            None => rsx! {
                                tr {
                                    td { colspan: "4", class: "text-center",
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
                            th { "" }
                        }
                    }
                }
            }

            if show_modal() {
                div { class: "modal modal-open",
                    div { class: "modal-box",
                        h3 { class: "font-bold text-lg mb-4", "Add Key / Value" }
                        div { class: "form-control mb-2",
                            label { class: "label", span { class: "label-text", "Key" } }
                            input {
                                class: "input input-bordered input-sm w-full",
                                value: "{form_key}",
                                oninput: move |e| form_key.set(e.value()),
                            }
                        }
                        div { class: "form-control mb-4",
                            label { class: "label", span { class: "label-text", "Value" } }
                            input {
                                class: "input input-bordered input-sm w-full",
                                value: "{form_value}",
                                oninput: move |e| form_value.set(e.value()),
                            }
                        }
                        div { class: "modal-action",
                            button {
                                class: "btn btn-ghost btn-sm",
                                onclick: move |_| show_modal.set(false),
                                "Cancel"
                            }
                            button {
                                class: "btn btn-primary btn-sm",
                                onclick: submit,
                                "Save"
                            }
                        }
                    }
                }
            }
        }
    }
}
