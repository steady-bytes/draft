mod proto {
    pub use crate::proto::core_message_broker_actors_v1::*;
}
pub use proto::*;
use ::dioxus::prelude::*;

pub struct ProducerServiceHook(proto::producer_client::ProducerClient<::tonic_web_wasm_client::Client>);

pub fn use_producer_service() -> ProducerServiceHook {
    ProducerServiceHook({ let config = use_context::<::dioxus_grpc::GrpcConfig>(); proto::producer_client::ProducerClient::new(::tonic_web_wasm_client::Client::new(config.host.clone())) })
}

impl ProducerServiceHook {
}