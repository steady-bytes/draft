use dioxus::prelude::*;
use draft_api::hook::core_registry_key_value_v1::{
    key_value_service_client::KeyValueServiceClient, DecodeValuesRequest, GetRequest, Value,
};
use prost::Message as _;
use prost_types::Any;
use std::collections::HashMap;
use tonic_web_wasm_client::Client as WasmClient;

use crate::Route as AppRoute;

const VALUE_TYPE_URL: &str = "type.googleapis.com/core.registry.key_value.v1.Value";

/// `/kv/:..kv_key_parts` — fetches the single entry fresh by key (rather
/// than passing its value through navigation state, matching the
/// RouteDetail/ServiceDetail convention) and pretty-prints it: valid JSON
/// is reformatted with indentation, everything else is shown as-is in a
/// monospace block — still a real improvement over the list page's
/// single-line table cell. The key is a catch-all segment (`Vec<String>`,
/// rejoined with `/`) rather than a single dynamic segment, since real keys
/// contain `/` (e.g. `cluster/layout`, `ui/navigation`) which a single
/// segment can't capture. Named `kv_key`/`kv_key_parts` rather than `key`,
/// which Dioxus reserves for its list-iteration key attribute.
#[component]
pub fn KeyValueDetail(kv_key_parts: Vec<String>) -> Element {
    let kv_key = kv_key_parts.join("/");

    // The list page sets PENDING_KV_KIND immediately before navigating here (see main.rs's doc
    // comment on it) so `Get` fetches under the *actual* kind this key is stored as -- falling
    // back to the generic `Value` kind on a direct URL visit/refresh, where nothing set it. The
    // read-then-clear has to happen inside use_effect, not directly in the component body: a
    // plain top-level `.read()` subscribes this render to the signal, and immediately `.write()`-
    // ing it afterward (even just to clear it) then re-triggers this same component, forever --
    // mirrors Beacon's identical PENDING_TRACE_ID hand-off in views/traces.rs, which uses the same
    // use_effect wrapper for the same reason.
    let mut kind = use_signal(|| VALUE_TYPE_URL.to_string());
    use_effect(move || {
        let Some(pending) = crate::PENDING_KV_KIND.read().clone() else {
            return;
        };
        *crate::PENDING_KV_KIND.write() = None;
        kind.set(pending);
    });

    let lookup = use_resource({
        let kv_key = kv_key.clone();
        move || {
            let kv_key = kv_key.clone();
            let kind = kind();
            async move {
                let mut client =
                    KeyValueServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
                let response = client
                    .get(GetRequest {
                        key: kv_key.clone(),
                        value: Some(Any {
                            type_url: kind.clone(),
                            value: vec![],
                        }),
                    })
                    .await
                    .map(|r| r.into_inner().value);

                match response {
                    Ok(None) => Ok(None),
                    Ok(Some(any)) => {
                        // The generic `Value` kind is decoded as text directly — see
                        // key_value.rs's identical guard for why an unrelated kind's bytes
                        // aren't blindly decoded as `Value`. Anything else goes through
                        // Blueprint's type registry (see kv-type-registry-implementation-plan.md)
                        // instead, which comes back as `None` exactly like today whenever nobody
                        // has registered a descriptor for it.
                        let data = if any.type_url == VALUE_TYPE_URL {
                            Value::decode(any.value.as_slice()).ok().map(|v| v.data)
                        } else {
                            let mut values = HashMap::new();
                            values.insert(kv_key.clone(), any.value.clone());
                            client
                                .decode_values(DecodeValuesRequest {
                                    type_url: any.type_url.clone(),
                                    values,
                                })
                                .await
                                .ok()
                                .and_then(|r| r.into_inner().json.remove(&kv_key))
                        };
                        Ok(Some((data, any.type_url, any.value.len())))
                    }
                    Err(e) => Err(e),
                }
            }
        }
    });

    rsx! {
        div { class: "p-4 flex flex-col gap-4",
            div { class: "flex items-center justify-between",
                div { class: "breadcrumbs text-sm",
                    ul {
                        li { Link { to: AppRoute::KeyValueView {}, "Key / Value" } }
                        li { "{kv_key}" }
                    }
                }
            }

            match &*lookup.read() {
                Some(Ok(Some((Some(data), type_url, _)))) => {
                    let (pretty, is_json) = pretty_print(data);
                    rsx! {
                        div { class: "flex flex-col gap-3",
                            div { class: "text-xs text-base-content/50 font-mono", "{type_url}" }
                            div { class: "flex items-center gap-2",
                                span { class: "badge badge-ghost badge-sm", if is_json { "JSON" } else { "TEXT" } }
                                span { class: "text-xs text-base-content/50", "{pretty.lines().count()} lines" }
                            }
                            pre {
                                class: "bg-base-200 border border-base-300 rounded p-4 text-xs font-mono overflow-x-auto whitespace-pre",
                                "{pretty}"
                            }
                        }
                    }
                }
                Some(Ok(Some((None, type_url, byte_len)))) => rsx! {
                    div { class: "flex flex-col gap-3",
                        div { class: "text-xs text-base-content/50 font-mono", "{type_url}" }
                        div { class: "alert alert-info",
                            span { "No preview available for this kind yet — {byte_len} raw bytes stored." }
                        }
                    }
                },
                Some(Ok(None)) => rsx! {
                    div { class: "alert alert-warning",
                        span { "No entry found for key \"{kv_key}\" — it may have just been deleted." }
                    }
                },
                Some(Err(e)) => rsx! {
                    div { class: "alert alert-error", span { "Error: {e}" } }
                },
                None => rsx! { div { class: "text-center", "Loading..." } },
            }
        }
    }
}

/// Pretty-prints `data` as indented JSON when it parses as JSON, otherwise
/// returns it unchanged. Also reports which case applied, for the badge.
fn pretty_print(data: &str) -> (String, bool) {
    match serde_json::from_str::<serde_json::Value>(data) {
        Ok(parsed) => (
            serde_json::to_string_pretty(&parsed).unwrap_or_else(|_| data.to_string()),
            true,
        ),
        Err(_) => (data.to_string(), false),
    }
}
