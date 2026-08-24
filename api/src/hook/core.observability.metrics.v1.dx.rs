mod proto {
    pub use crate::proto::core_observability_metrics_v1::*;
}
pub use proto::*;
use ::dioxus::prelude::*;

pub struct MetricsServiceServiceHook(proto::metrics_service_client::MetricsServiceClient<::tonic_web_wasm_client::Client>);

pub fn use_metrics_service_service() -> MetricsServiceServiceHook {
    MetricsServiceServiceHook({ let config = use_context::<::dioxus_grpc::GrpcConfig>(); proto::metrics_service_client::MetricsServiceClient::new(::tonic_web_wasm_client::Client::new(config.host.clone())) })
}

impl MetricsServiceServiceHook {
    pub fn query_metrics(&self, req: Signal<proto::QueryMetricsRequest>) -> Resource<Result<proto::QueryMetricsResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.query_metrics(req()).await.map(|resp| resp.into_inner()) }
        })
    }
}