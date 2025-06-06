use std::fmt;
use std::collections::HashMap;
use once_cell::sync::Lazy;
use serde::{Deserialize, Serialize};
use dioxus::prelude::*;
use dioxus_logger::tracing::debug;

use crate::API_DOMAIN;

pub static PATH: Lazy<String> = Lazy::new(|| {
    "/core.registry.key_value.v1.KeyValueService/List".to_string()
});

#[component]
pub fn KeyValueView() -> Element {
    let mut list = use_signal(|| {
        KeyValueListResponse {
            values: HashMap::new(),
        }
    });

    // TODO: When adding a spinner, use the output from `use_resource` to show the spinner, display data, and handle the error
    //       currently using the `use_signal` hook to show the data which is not the most efficient way
    let _key_value = use_resource(move || async move {
        let url = format!("{}{}", *API_DOMAIN, *PATH);
        debug!("URL: {}", url);
        let response = reqwest::Client::new()
            .post(url)
            .json(&KeyValueListRequest {
                value: QueryValue {
                    type_url: "type.googleapis.com/core.registry.key_value.v1.Value".to_string(),
                },
            })
            .send()
            .await;

            // TODO: Error handling
            match response {
                Ok(resp) => {
                    let json = resp.json::<KeyValueListResponse>().await;
                    match json {
                        Ok(data) => {
                            let mut d = HashMap::new();
                            // iterate over the values and remove the key type prefix
                            // (type.googleapis.com/core.registry.key_value.v1.Value-)
                            data.values.iter().for_each(|(key, val)| {
                                let mut new_key = key.clone();
                                if let Some(pos) = new_key.find("type.googleapis.com/core.registry.key_value.v1.Value-") {
                                    new_key.replace_range(..pos + "type.googleapis.com/core.registry.key_value.v1.Value-".len(), "");
                                }
                                // info!("Key: {}", new_key);
                                d.insert(new_key, val.clone());
                            });

                            list.set(KeyValueListResponse {
                                values: d,
                            });

                            Ok(())
                        },
                        // Error so we need to render something on the screen (Maybe some popup)
                        Err(err) => Err(format!("Failed to parse JSON: {}", err)),
                    }
                }
                // Error so we need to render something on the screen (Maybe some popup)
                Err(err) => Err(format!("Failed to fetch data: {}", err)),
            }
    });

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
                        for (key, val) in list().values.iter() {
                            tr { class: "hover:bg-base-300",
                                td { "{key}" }
                                td { "{val.data}" }
                                td { "{val.type_url}" }
                            }
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

/// curl:
/// curl --header "Content-Type: application/json" \
/// --data '{"value": {"type_url": "type.googleapis.com/core.registry.key_value.v1.Value"}}' \
/// http://localhost:2221/core.registry.key_value.v1.KeyValueService/List
///
/// RES: {"value": {"type_url": "type.googleapis.com/core.registry.key_value.v1.Value"}}
#[derive(Serialize, Deserialize)]
struct KeyValueListRequest {
    value: QueryValue,
}

#[derive(Serialize, Deserialize )]
struct QueryValue {
    type_url: String
}

/// Response from the server
#[derive(Serialize, Deserialize, Clone)]
struct KeyValueListResponse {
    values: HashMap<String, Value>,
}

// Implementing Display trait for KeyValueListResponse
// so it can be printed in a readable format
impl fmt::Display for KeyValueListResponse {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        for (key, value) in &self.values {
            writeln!(f, "Key: {}\nType URL: {}\nData: {}\n", key, value.type_url, value.data)?;
        }
        Ok(())
    }
}

#[derive(Serialize, Deserialize, Clone)]
struct Value {
    #[serde(rename = "@type")]
    type_url: String,
    data: String,
}