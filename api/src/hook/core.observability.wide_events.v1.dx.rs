mod proto {
    pub use crate::proto::core_observability_wide_events_v1::*;
}
pub use proto::*;
use ::dioxus::prelude::*;

pub struct WideEventsServiceServiceHook(proto::wide_events_service_client::WideEventsServiceClient<::tonic_web_wasm_client::Client>);

pub fn use_wide_events_service_service() -> WideEventsServiceServiceHook {
    WideEventsServiceServiceHook({ let config = use_context::<::dioxus_grpc::GrpcConfig>(); proto::wide_events_service_client::WideEventsServiceClient::new(::tonic_web_wasm_client::Client::new(config.host.clone())) })
}

impl WideEventsServiceServiceHook {
    pub fn search_wide_events(&self, req: Signal<proto::SearchWideEventsRequest>) -> Resource<Result<proto::SearchWideEventsResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.search_wide_events(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn get_wide_event(&self, req: Signal<proto::GetWideEventRequest>) -> Resource<Result<proto::GetWideEventResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.get_wide_event(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn create_wide_event(&self, req: Signal<proto::CreateWideEventRequest>) -> Resource<Result<proto::CreateWideEventResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.create_wide_event(req()).await.map(|resp| resp.into_inner()) }
        })
    }
}