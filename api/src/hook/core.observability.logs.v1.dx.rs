mod proto {
    pub use crate::proto::core_observability_logs_v1::*;
}
pub use proto::*;
use ::dioxus::prelude::*;

pub struct LogsServiceServiceHook(proto::logs_service_client::LogsServiceClient<::tonic_web_wasm_client::Client>);

pub fn use_logs_service_service() -> LogsServiceServiceHook {
    LogsServiceServiceHook({ let config = use_context::<::dioxus_grpc::GrpcConfig>(); proto::logs_service_client::LogsServiceClient::new(::tonic_web_wasm_client::Client::new(config.host.clone())) })
}

impl LogsServiceServiceHook {
    pub fn query_logs(&self, req: Signal<proto::QueryLogsRequest>) -> Resource<Result<proto::QueryLogsResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.query_logs(req()).await.map(|resp| resp.into_inner()) }
        })
    }
}