
            mod proto {
                        pub use crate::proto::core_registry_key_value_v1::*;
                    }
            pub use proto::*;
            use ::dioxus::prelude::*;
            
                pub struct KeyValueServiceServiceHook(proto::key_value_service_client::KeyValueServiceClient<::tonic_web_wasm_client::Client>);

                pub fn use_key_value_service_service() -> KeyValueServiceServiceHook {
                    KeyValueServiceServiceHook
                            (proto::key_value_service_client::KeyValueServiceClient::new(::tonic_web_wasm_client::Client::new(
                                "http://127.0.0.1:2221".to_string()
                            )))
                            
                }

                impl KeyValueServiceServiceHook {
                
                    pub fn set(&self, req: Signal<proto::SetRequest>) -> Resource<Result<proto::SetResponse, tonic::Status>> {
                        let client = self.0.to_owned();
                        use_resource(move || {
                            let mut client = client.clone();
                            async move { client.set(req()).await.map(|resp| resp.into_inner()) }
                        })
                    }
                    
                    pub fn get(&self, req: Signal<proto::GetRequest>) -> Resource<Result<proto::GetResponse, tonic::Status>> {
                        let client = self.0.to_owned();
                        use_resource(move || {
                            let mut client = client.clone();
                            async move { client.get(req()).await.map(|resp| resp.into_inner()) }
                        })
                    }
                    
                    pub fn delete(&self, req: Signal<proto::DeleteRequest>) -> Resource<Result<proto::DeleteResponse, tonic::Status>> {
                        let client = self.0.to_owned();
                        use_resource(move || {
                            let mut client = client.clone();
                            async move { client.delete(req()).await.map(|resp| resp.into_inner()) }
                        })
                    }
                    
                    pub fn list(&self, req: Signal<proto::ListRequest>) -> Resource<Result<proto::ListResponse, tonic::Status>> {
                        let client = self.0.to_owned();
                        use_resource(move || {
                            let mut client = client.clone();
                            async move { client.list(req()).await.map(|resp| resp.into_inner()) }
                        })
                    }
                    }