# API
To serve as the home for type definitions, rpc service's, model's, validation of messages, and gateway generation.
All of the above are declared in [protocol buffer's]() and compiled using [buf]() inside of a docker container
to reduce protoc compiler, and compiler plugin environment dependencies.

## Generating files for local testing

### Using `dctl`

The easiest way to generate protos is using [`dctl`](../tools/dctl/):

```shell
dctl api init
dctl api build
```

Any time you want to regenerate your protos just run `dctl api build` again.

## Rust Code Generation

The API crate (`draft-api`) automatically generates Rust gRPC code from protocol buffer definitions using `build.rs`.

### How it works

The build process is configured in [build.rs](./build.rs) and uses:

- **tonic-build**: Generates Rust gRPC service clients and servers from `.proto` files
- **prost-build**: Generates Rust message types and serialization code
- **dioxus-grpc**: Generates Dioxus hooks for easy integration in web clients
- **Output directories**:
  - Generated types in `src/pb/` for visibility and debugging
  - Generated hooks in `target/debug/build/draft-api-*/out/*.dx.rs` for web integration

### Generating code

Generated code is created automatically when you build the crate:

```bash
cargo build
```

This will:
1. Locate proto files in:
   - `core/registry/key_value/v1/service.proto` (gRPC service definitions)
   - `core/registry/key_value/v1/models.proto` (message types)
2. Generate Rust code in `src/pb/`
3. Include the generated module in `src/lib.rs`

### Generated files

After building, you'll find:

- **`src/pb/core.registry.key_value.v1.rs`** - Contains:
  - Message types (SetRequest, GetRequest, ListRequest, ListResponse, Value, etc.)
  - KeyValueServiceClient for calling gRPC services
  - KeyValueServiceServer trait for implementing services

### Using generated types

Export generated types in your application via:

```rust
use draft_api::{ListRequest, ListResponse, KeyValueServiceClient};
use draft_api::prost_types::Any; // For Any message type
```

### Dioxus-gRPC Hooks

The build process also generates Dioxus hooks for web clients using [dioxus-grpc](https://github.com/tkr-sh/dioxus-grpc). These hooks provide a convenient reactive interface for calling gRPC services.

Generated hook files are placed in `target/debug/build/draft-api-*/out/*.dx.rs` and include:

- `use_<service>_service()` hook functions (e.g., `use_key_value_service`)
- Full integration with Dioxus signals and resources
- Automatic async handling with `Resource` type

**Example hook usage in a web component:**

```rust
use draft_api::use_key_value_service;
use dioxus::prelude::*;

fn key_value_component() -> Element {
    let service = use_key_value_service();
    let mut request = use_signal(|| ListRequest::default());
    
    // Call the service - returns Resource<Result<ListResponse, Status>>
    let result = service.list(request);
    
    rsx! {
        match &*result.read() {
            Some(Ok(response)) => rsx! { "Success: {response:#?}" },
            Some(Err(err)) => rsx! { "Error: {err}" },
            None => rsx! { "Loading..." },
        }
    }
}
```

### Adding new services or messages

1. Create or modify `.proto` files in `core/registry/<service>/v1/`
2. Run `cargo build` in the api crate
3. The build.rs script will automatically generate:
   - Rust types and clients in `src/pb/`
   - Dioxus hooks in `target/debug/build/draft-api-*/out/`
4. Import and use the generated types and hooks in your code

## Using locally generated files

To use the generated files locally, you will need to add a `replace` to the project's go.mod:

```go.mod
...

replace github.com/steady-bytes/draft/api => ../../../api

...
```
