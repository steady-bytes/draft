//! A thin `AlleleServiceClient` constructor, reused by every view -- same shape as
//! services/tooling/relay/web-client/src/api.rs (Allele has no cross-page shared data loader
//! either; each view calls the RPCs it actually needs).

use draft_api::proto::tooling_allele_v1::allele_service_client::AlleleServiceClient;
use tonic_web_wasm_client::Client as WasmClient;

pub fn client() -> AlleleServiceClient<WasmClient> {
    AlleleServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()))
}
