mod proto {
    pub use crate::proto::core_control_plane_networking_v1::*;
}
pub use proto::*;
use ::dioxus::prelude::*;

pub struct NetworkingServiceServiceHook(proto::networking_service_client::NetworkingServiceClient<::tonic_web_wasm_client::Client>);

pub fn use_networking_service_service() -> NetworkingServiceServiceHook {
    NetworkingServiceServiceHook({ let config = use_context::<::dioxus_grpc::GrpcConfig>(); proto::networking_service_client::NetworkingServiceClient::new(::tonic_web_wasm_client::Client::new(config.host.clone())) })
}

impl NetworkingServiceServiceHook {
    pub fn add_route(&self, req: Signal<proto::AddRouteRequest>) -> Resource<Result<proto::AddRouteResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.add_route(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn list_routes(&self, req: Signal<proto::ListRoutesRequest>) -> Resource<Result<proto::ListRoutesResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.list_routes(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn delete_route(&self, req: Signal<proto::DeleteRouteRequest>) -> Resource<Result<proto::DeleteRouteResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.delete_route(req()).await.map(|resp| resp.into_inner()) }
        })
    }
}