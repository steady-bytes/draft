#[cfg(feature = "web")]
pub mod proto;
#[cfg(feature = "web")]
pub mod hook;

#[cfg(feature = "web")]
use dioxus::prelude::*;
#[cfg(feature = "web")]
use prost::Message as _;

fn main() {
    #[cfg(feature = "web")]
    launch(app);
}

#[cfg(feature = "web")]
const VALUE_TYPE_URL: &str = "type.googleapis.com/core.registry.key_value.v1.Value";

#[cfg(feature = "web")]
fn strip_type_prefix(key: &str) -> &str {
    key.strip_prefix(VALUE_TYPE_URL)
        .and_then(|s| s.strip_prefix('-'))
        .unwrap_or(key)
}

#[cfg(feature = "web")]
fn app() -> Element {
    use hook::core_registry_key_value_v1::{
        use_key_value_service_service, GetRequest, SetRequest, Value,
    };

    let mut key = use_signal(|| String::from("my-key"));
    let mut value_input = use_signal(|| String::from("my-value"));

    let mut set_req = use_signal(|| SetRequest {
        key: String::new(),
        value: None,
    });

    let mut get_req = use_signal(|| GetRequest {
        key: String::new(),
        value: None,
    });

    // Initialized with the Value type URL so Blueprint knows which type to scan.
    // Written again on each List click to re-trigger the resource.
    let mut list_req = use_signal(|| list_request());

    let kv = use_key_value_service_service();
    let set_result = kv.set(set_req);
    let get_result = kv.get(get_req);
    let list_result = kv.list(list_req);

    rsx! {
        style { "
            * {{ box-sizing: border-box; margin: 0; padding: 0; }}

            body {{
                background: #0f1117;
                color: #d0d0e0;
                font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
                min-height: 100vh;
                padding: 2.5rem;
            }}

            h1 {{
                font-size: 1.5rem;
                font-weight: 600;
                color: #a0b4d6;
                margin-bottom: 1.75rem;
            }}

            .controls {{
                display: flex;
                align-items: center;
                gap: 0.6rem;
                flex-wrap: wrap;
            }}

            .controls label {{
                font-size: 0.8rem;
                color: #7878a0;
                text-transform: uppercase;
                letter-spacing: 0.04em;
            }}

            .controls input {{
                background: #1c1f2e;
                border: 1px solid #2e3250;
                border-radius: 6px;
                color: #d0d0e0;
                font-size: 0.9rem;
                padding: 0.4rem 0.7rem;
                outline: none;
                transition: border-color 0.15s;
            }}

            .controls input:focus {{ border-color: #5060a0; }}

            .controls button {{
                background: #2a3580;
                border: none;
                border-radius: 6px;
                color: #c8d4f0;
                cursor: pointer;
                font-size: 0.85rem;
                padding: 0.4rem 0.9rem;
                transition: background 0.15s;
            }}

            .controls button:hover {{ background: #3a45a0; }}

            table {{
                border-collapse: collapse;
                margin-top: 1.5rem;
                width: 100%;
                max-width: 640px;
                background: #1c1f2e;
                border-radius: 8px;
                overflow: hidden;
            }}

            th {{
                background: #161929;
                color: #6878a8;
                font-size: 0.75rem;
                font-weight: 600;
                letter-spacing: 0.06em;
                padding: 0.65rem 1rem;
                text-align: left;
                text-transform: uppercase;
                border-bottom: 1px solid #2e3250;
            }}

            td {{
                font-size: 0.9rem;
                padding: 0.55rem 1rem;
                border-bottom: 1px solid #22263a;
            }}

            tbody tr:last-child td {{ border-bottom: none; }}
            tbody tr:hover {{ background: #22263a; }}

            .get-result {{
                margin-top: 1rem;
                padding: 0.6rem 1rem;
                background: #1c1f2e;
                border: 1px solid #2e3250;
                border-left: 3px solid #5060a0;
                border-radius: 6px;
                font-size: 0.9rem;
                color: #a0b4d6;
                max-width: 640px;
            }}

            .get-result .get-label {{
                font-size: 0.72rem;
                text-transform: uppercase;
                letter-spacing: 0.05em;
                color: #5868a0;
                margin-bottom: 0.2rem;
            }}

            .toast-stack {{
                position: fixed;
                top: 1.25rem;
                right: 1.25rem;
                display: flex;
                flex-direction: column;
                gap: 0.5rem;
                z-index: 100;
                pointer-events: none;
            }}

            .toast {{
                background: #162016;
                border: 1px solid #2a3e2a;
                border-left: 3px solid #4a8a4a;
                border-radius: 6px;
                color: #88cc88;
                font-size: 0.85rem;
                padding: 0.6rem 1rem;
                min-width: 220px;
                animation: slide-in 0.22s cubic-bezier(0.16, 1, 0.3, 1);
            }}

            .toast.error {{
                background: #201616;
                border-color: #3e2a2a;
                border-left-color: #8a4a4a;
                color: #cc8888;
            }}

            @keyframes slide-in {{
                from {{ transform: translateX(calc(100% + 1.25rem)); opacity: 0; }}
                to   {{ transform: translateX(0); opacity: 1; }}
            }}
        " }

        div {
            h1 { "Blueprint Key/Value Explorer" }

            div { class: "controls",
                label { "Key" }
                input {
                    value: "{key}",
                    oninput: move |e| key.set(e.value()),
                }
                label { "Value" }
                input {
                    value: "{value_input}",
                    oninput: move |e| value_input.set(e.value()),
                }
                button {
                    onclick: move |_| {
                        let v = Value { data: value_input() };
                        set_req.set(SetRequest {
                            key: key(),
                            value: Some(prost_types::Any {
                                type_url: VALUE_TYPE_URL
                                    .to_string(),
                                value: v.encode_to_vec(),
                            }),
                        });
                        // Refresh the table after each set.
                        list_req.set(list_request());
                    },
                    "Set"
                }
                button {
                    onclick: move |_| {
                        get_req.set(GetRequest {
                            key: key(),
                            value: Some(prost_types::Any {
                                type_url: VALUE_TYPE_URL
                                    .to_string(),
                                value: vec![],
                            }),
                        });
                    },
                    "Get"
                }
                button {
                    onclick: move |_| list_req.set(list_request()),
                    "List"
                }
            }

            // Inline get result
            match &*get_result.read() {
                Some(Ok(resp)) => match &resp.value {
                    Some(any) => {
                        let v = Value::decode(any.value.as_slice())
                            .map(|v| v.data)
                            .unwrap_or_else(|_| "(binary)".to_string());
                        rsx! {
                            div { class: "get-result",
                                div { class: "get-label", "Value" }
                                "{v}"
                            }
                        }
                    }
                    None => rsx! {},
                },
                _ => rsx! {},
            }

            // Results table
            match &*list_result.read() {
                None => rsx! { p { "Loading…" } },
                Some(Err(e)) => rsx! { div { class: "toast error", "List error: {e}" } },
                Some(Ok(resp)) => {
                    let mut rows: Vec<(String, String)> = resp.values
                        .iter()
                        .map(|(k, any)| {
                            let v = Value::decode(any.value.as_slice())
                                .map(|v| v.data)
                                .unwrap_or_else(|_| "(binary)".to_string());
                            (strip_type_prefix(k).to_string(), v)
                        })
                        .collect();
                    rows.sort_by(|a, b| a.0.cmp(&b.0));

                    rsx! {
                        table {
                            thead {
                                tr {
                                    th { "Key" }
                                    th { "Value" }
                                }
                            }
                            tbody {
                                if rows.is_empty() {
                                    tr {
                                        td { colspan: "2", "No entries found." }
                                    }
                                }
                                for (k, v) in rows {
                                    tr {
                                        key: "{k}",
                                        td { "{k}" }
                                        td { "{v}" }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }

        // Toast notifications (fixed, top-right)
        div { class: "toast-stack",
            match &*set_result.read() {
                Some(Ok(resp)) => rsx! { div { class: "toast", "Stored \"{strip_type_prefix(&resp.key)}\"" } },
                Some(Err(e)) => rsx! { div { class: "toast error", "Set error: {e}" } },
                None => rsx! {},
            }
            match &*get_result.read() {
                Some(Err(e)) => rsx! { div { class: "toast error", "Get error: {e}" } },
                _ => rsx! {},
            }
        }
    }
}

#[cfg(feature = "web")]
fn list_request() -> hook::core_registry_key_value_v1::ListRequest {
    hook::core_registry_key_value_v1::ListRequest {
        value: Some(prost_types::Any {
            type_url: VALUE_TYPE_URL.to_string(),
            value: vec![],
        }),
    }
}
