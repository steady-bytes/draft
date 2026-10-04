//! A thin `RelayServiceClient` constructor, reused by every view. Relay has no cross-page shared
//! data loader the way Lineman's `rail.rs` does (no aggregate counts every page needs) -- each
//! view calls the RPCs it actually needs directly, following this same construction.

use draft_api::proto::tooling_relay_v1::relay_service_client::RelayServiceClient;
use tonic_web_wasm_client::Client as WasmClient;

pub fn client() -> RelayServiceClient<WasmClient> {
    RelayServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()))
}
