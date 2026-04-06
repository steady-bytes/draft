// gRPC-web client module with Dioxus hooks
// Uses reqwest for HTTP transport with JSON serialization

use dioxus::prelude::*;
use draft_api::ListRequest;
use serde_json::json;

pub struct KeyValueServiceHook {
    endpoint: String,
}

impl KeyValueServiceHook {
    pub fn new() -> Self {
        KeyValueServiceHook {
            endpoint: "http://localhost:3000".to_string(),
        }
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
