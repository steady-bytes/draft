mod proto {
    pub use crate::proto::core_observability_traces_v1::*;
}
pub use proto::*;
use ::dioxus::prelude::*;

pub struct TracesServiceServiceHook(proto::traces_service_client::TracesServiceClient<::tonic_web_wasm_client::Client>);

pub fn use_traces_service_service() -> TracesServiceServiceHook {
    TracesServiceServiceHook({ let config = use_context::<::dioxus_grpc::GrpcConfig>(); proto::traces_service_client::TracesServiceClient::new(::tonic_web_wasm_client::Client::new(config.host.clone())) })
}

impl TracesServiceServiceHook {
    pub fn search_traces(&self, req: Signal<proto::SearchTracesRequest>) -> Resource<Result<proto::SearchTracesResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.search_traces(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn get_trace(&self, req: Signal<proto::GetTraceRequest>) -> Resource<Result<proto::GetTraceResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.get_trace(req()).await.map(|resp| resp.into_inner()) }
        })
    }
}