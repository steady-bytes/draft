// gRPC-web client module with Dioxus hooks
// Uses reqwest for HTTP transport with JSON serialization

use dioxus::prelude::*;
use draft_api::ListRequest;
use once_cell::sync::Lazy;
use serde_json::json;
use web_sys::window;

/// Get the RPC endpoint from environment or derive from current window location
fn get_rpc_endpoint() -> String {
    // Check if RPC_ENDPOINT environment variable is set
    // if let Some(rpc_endpoint) = option_env!("RPC_ENDPOINT") {
    //     if !rpc_endpoint.is_empty() {
    //         return rpc_endpoint.to_string();
    //     }
    // }

    // Fall back to current window origin
    // if let Some(window_obj) = window() {
    //     if let Ok(location) = window_obj.location().origin() {
    //         return location;
    //     }
    // }

    // Final fallback - blueprint default port
    "http://localhost:2221".to_string()
}

pub static RPC_ENDPOINT: Lazy<String> = Lazy::new(get_rpc_endpoint);

pub struct KeyValueServiceHook {
    endpoint: String,
}

impl KeyValueServiceHook {
    pub fn new() -> Self {
        KeyValueServiceHook {
            endpoint: RPC_ENDPOINT.clone(),
        }
    }

    /// Create a hook with a custom endpoint
    pub fn with_endpoint(endpoint: String) -> Self {
        KeyValueServiceHook { endpoint }
    }

    pub fn list(
        &self,
        req: Signal<ListRequest>,
    ) -> Resource<Result<serde_json::Value, String>> {
        let endpoint = self.endpoint.clone();
        use_resource(move || {
            let endpoint = endpoint.clone();
            async move {
                let _request = req();
                let url = format!("{}/core.registry.key_value.v1.KeyValueService/List", endpoint);

                let response = reqwest::Client::new()
                    .post(&url)
                    .header("Content-Type", "application/json")
                    .json(&json!({
                        "value": {
                            "type_url": "type.googleapis.com/core.registry.key_value.v1.Value"
                        }
                    }))
                    .send()
                    .await
                    .map_err(|e| format!("Failed to fetch: {}", e))?;

                response
                    .json::<serde_json::Value>()
                    .await
                    .map_err(|e| format!("Failed to parse response: {}", e))
            }
        })
    }
}

impl Clone for KeyValueServiceHook {
    fn clone(&self) -> Self {
        KeyValueServiceHook {
            endpoint: self.endpoint.clone(),
        }
    }
}
