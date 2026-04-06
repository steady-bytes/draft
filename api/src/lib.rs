//! Generated Rust gRPC code for the draft API

pub use prost_types;

#[cfg(feature = "server")]
pub mod pb {
    pub use ::prost_types;
    include!("pb/core.registry.key_value.v1.rs");
}

// Re-export commonly used types for convenience
#[cfg(feature = "server")]
pub use pb::{
    key_value_service_client::KeyValueServiceClient,
    key_value_service_server::KeyValueServiceServer,
    DeleteRequest, DeleteResponse, GetRequest, GetResponse, ListRequest, ListResponse, SetRequest,
    SetResponse, Value,
};

// For WASM/web targets without server feature, only export message types
#[cfg(not(feature = "server"))]
pub mod pb {
    pub use ::prost_types;
    
    // Include only the message definitions (proto3 syntax generates these without tonic deps)
    include!("pb/core.registry.key_value.v1.rs");
}

#[cfg(not(feature = "server"))]
pub use pb::{
    DeleteRequest, DeleteResponse, GetRequest, GetResponse, ListRequest, ListResponse, SetRequest,
    SetResponse, Value,
};

