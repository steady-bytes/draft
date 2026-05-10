mod proto {
    pub use crate::proto::core_registry_service_discovery_v1::*;
}
pub use proto::*;
use ::dioxus::prelude::*;

pub struct ServiceDiscoveryServiceServiceHook(proto::service_discovery_service_client::ServiceDiscoveryServiceClient<::tonic_web_wasm_client::Client>);

pub fn use_service_discovery_service_service() -> ServiceDiscoveryServiceServiceHook {
    ServiceDiscoveryServiceServiceHook({ let config = use_context::<::dioxus_grpc::GrpcConfig>(); proto::service_discovery_service_client::ServiceDiscoveryServiceClient::new(::tonic_web_wasm_client::Client::new(config.host.clone())) })
}

impl ServiceDiscoveryServiceServiceHook {
    pub fn initialize(&self, req: Signal<proto::InitializeRequest>) -> Resource<Result<proto::InitializeResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.initialize(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn finalize(&self, req: Signal<proto::FinalizeRequest>) -> Resource<Result<proto::FinalizeResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.finalize(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn report_health(&self, req: Signal<proto::ReportHealthRequest>) -> Resource<Result<proto::ReportHealthResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.report_health(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn query(&self, req: Signal<proto::QueryRequest>) -> Resource<Result<proto::QueryResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.query(req()).await.map(|resp| resp.into_inner()) }
        })
    }
}