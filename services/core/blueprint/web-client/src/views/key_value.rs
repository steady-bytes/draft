use dioxus::prelude::*;
use draft_api::hook::core_registry_key_value_v1::{
    key_value_service_client::KeyValueServiceClient, DecodeValuesRequest, DeleteRequest,
    ListKindsRequest, ListRequest, SetRequest, Value,
};
use gloo_timers::future::TimeoutFuture;
use prost::Message as _;
use prost_types::Any;
use std::collections::HashMap;
use tonic_web_wasm_client::Client as WasmClient;

use crate::Route as AppRoute;

const VALUE_TYPE_URL: &str = "type.googleapis.com/core.registry.key_value.v1.Value";

/// Strips the "type.googleapis.com/" prefix (or, failing that, everything up to the last '.')
/// so the kind dropdown reads "Value"/"NavigationConfig" instead of the full type_url.
fn short_kind_name(type_url: &str) -> &str {
    type_url
        .strip_prefix("type.googleapis.com/")
        .and_then(|s| s.rsplit('.').next())
        .unwrap_or(type_url)
}

/// Truncates a decoded value's JSON to a single-line preview for the list row — the full tree
/// only makes sense on the detail page, but the row itself should show something real rather
/// than just an indicator that a preview exists elsewhere.
const PREVIEW_MAX_CHARS: usize = 40;
fn truncate_preview(json: &str) -> String {
    let flattened: String = json.split_whitespace().collect::<Vec<_>>().join(" ");
    if flattened.chars().count() <= PREVIEW_MAX_CHARS {
        flattened
    } else {
        let truncated: String = flattened.chars().take(PREVIEW_MAX_CHARS).collect();
        format!("{truncated}…")
    }
}

#[component]
pub fn KeyValueView() -> Element {
    let mut selected_kind = use_signal(|| VALUE_TYPE_URL.to_string());

    let kinds_result = use_resource(|| async {
        let mut client = KeyValueServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
        client
            .list_kinds(ListKindsRequest {})
            .await
            .map(|r| r.into_inner().kinds)
    });

    let mut list_result = use_resource(move || {
        let kind = selected_kind();
        async move {
            let mut client = KeyValueServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
            client
                .list(ListRequest {
                    value: Some(Any {
                        type_url: kind,
                        value: vec![],
                    }),
                })
                .await
                .map(|r| r.into_inner())
        }
    });

    // Decodes the current page's raw bytes for whatever kind is selected, via Blueprint's type
    // registry (see kv-type-registry-implementation-plan.md) -- empty for `Value` (already
    // decoded directly below, no registry involved) and for any kind nobody has registered a
    // descriptor for, in which case every key is simply absent from the result and the byte-count
    // fallback below applies exactly as it always has. Reading `list_result` here (a Resource,
    // not a plain signal) still subscribes this resource to it, so a kind change or a
    // list_result.restart() after add/delete both naturally re-run this too.
    let decoded_result = use_resource(move || {
        let kind = selected_kind();
        async move {
            if kind == VALUE_TYPE_URL {
                return HashMap::new();
            }
            let values: HashMap<String, Vec<u8>> = match &*list_result.read() {
                Some(Ok(response)) => response
                    .values
                    .iter()
                    .map(|(key, any)| (key.clone(), any.value.clone()))
                    .collect(),
                _ => return HashMap::new(),
            };
            if values.is_empty() {
                return HashMap::new();
            }
            let mut client = KeyValueServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
            client
                .decode_values(DecodeValuesRequest {
                    type_url: kind,
                    values,
                })
                .await
                .map(|r| r.into_inner().json)
                .unwrap_or_default()
        }
    });

    let mut show_modal = use_signal(|| false);
    let mut form_key = use_signal(String::new);
    let mut form_value = use_signal(String::new);
    let mut status: Signal<Option<String>> = use_signal(|| None);
    let navigator = use_navigator();

    let submit = move |_| {
        let key = form_key();
        let value = form_value();
        spawn(async move {
            let mut client = KeyValueServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
            let any = Any {
                type_url: VALUE_TYPE_URL.to_string(),
                value: Value { data: value }.encode_to_vec(),
            };
            match client
                .set(SetRequest {
                    key,
                    value: Some(any),
                })
                .await
            {
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

            div { class: "flex items-center gap-2 mb-4",
                label { class: "text-sm text-base-content/70", "Kind" }
                select {
                    class: "select select-bordered select-sm",
                    value: "{selected_kind}",
                    onchange: move |e| selected_kind.set(e.value()),
                    if let Some(Ok(kinds)) = &*kinds_result.read() {
                        for kind in kinds {
                            option {
                                value: "{kind.type_url}",
                                selected: kind.type_url == selected_kind(),
                                "{short_kind_name(&kind.type_url)} ({kind.count})"
                            }
                        }
                    } else {
                        option { value: "{VALUE_TYPE_URL}", "Value" }
                    }
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
                                let kind = selected_kind();
                                let key_prefix = format!("{kind}-");
                                let decoded = match &*decoded_result.read() {
                                    Some(map) => map.clone(),
                                    None => HashMap::new(),
                                };
                                let mut rows: Vec<(String, String, String)> = response.values
                                    .iter()
                                    .map(|(key, any)| {
                                        let type_url = any.type_url.clone();
                                        // Only the generic `Value` kind is decoded/previewed as text
                                        // directly here — decoding an unrelated kind's bytes as
                                        // `Value` can silently "succeed" with garbage, since protobuf
                                        // strings and embedded messages share a wire type. Anything
                                        // else falls back to `decoded` (Blueprint's type registry,
                                        // populated above for whatever kind is selected) when a
                                        // descriptor has been registered for it, or the byte count
                                        // when nobody has.
                                        let data = if type_url == VALUE_TYPE_URL {
                                            Value::decode(any.value.as_slice())
                                                .map(|v| v.data)
                                                .unwrap_or_else(|_| "(unreadable)".to_string())
                                        } else if let Some(json) = decoded.get(key) {
                                            truncate_preview(json)
                                        } else {
                                            format!("{} — {} bytes", short_kind_name(&type_url), any.value.len())
                                        };
                                        let display_key = key.strip_prefix(key_prefix.as_str()).unwrap_or(key).to_string();
                                        (display_key, data, type_url)
                                    })
                                    .collect();
                                rows.sort_by(|a, b| a.0.cmp(&b.0));
                                if rows.is_empty() {
                                    rsx! {
                                        tr {
                                            td { colspan: "4", class: "text-center text-base-content/50",
                                                "No entries for this kind"
                                            }
                                        }
                                    }
                                } else {
                                    rsx! {
                                        for (key, data, type_url) in rows {
                                            {
                                                let delete_key = key.clone();
                                                let row_key = key.clone();
                                                let kind_for_delete = kind.clone();
                                                let kind_for_nav = kind.clone();
                                                rsx! {
                                                    tr {
                                                        class: "hover:bg-base-300 cursor-pointer",
                                                        onclick: move |_| {
                                                            *crate::PENDING_KV_KIND.write() = Some(kind_for_nav.clone());
                                                            let parts = row_key.split('/').map(String::from).collect();
                                                            navigator.push(AppRoute::KeyValueDetail { kv_key_parts: parts });
                                                        },
                                                        td { "{key}" }
                                                        td { "{data}" }
                                                        td { "{type_url}" }
                                                        td {
                                                            button {
                                                                class: "btn btn-xs btn-error",
                                                                onclick: move |ev| {
                                                                    ev.stop_propagation();
                                                                    let key = delete_key.clone();
                                                                    let type_url = kind_for_delete.clone();
                                                                    spawn(async move {
                                                                        let mut client = KeyValueServiceClient::new(
                                                                            WasmClient::new(crate::API_DOMAIN.clone())
                                                                        );
                                                                        let _ = client.delete(DeleteRequest {
                                                                            key,
                                                                            value: Some(Any {
                                                                                type_url,
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
