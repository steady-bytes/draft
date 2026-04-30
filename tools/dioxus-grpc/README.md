<h1 align="center"> <code> dioxus-grpc </code> </h1>

`dioxus-grpc` provides a convenient way to use `gRPC` with `Dioxus`

## Example

```rs
fn app() -> Element {
    let req = use_signal(|| HelloRequest { name: String::new() });
    let greeter = use_greeter_service();

    rsx! {
        input {
            value: "{req().name}",
            oninput: move |event| req.write().name = event.value()
        }
        match &*greeter.say_hello(req).read() {
            Some(Ok(resp)) => rsx!{"[{resp.message}] - From server"},
            Some(Err(err)) => rsx!{"Couldn't get the name {err:#?}"},
            None => rsx!{"..."},
        }
    }
}
```

```proto
syntax = "proto3";

package helloworld;

service Greeter {
  rpc SayHello (HelloRequest) returns (HelloReply) {}
}

message HelloRequest {
  string name = 1;
}

message HelloReply {
  string message = 1;
}
```


## How to use ?

_By default, this is meant for `mobile` & `desktop`. But you can activate it for `web` with `feature = "web"`_

To use it, you will need to also use `tonic-build` (and disable the `transport` feature in case of `feature = "web`). Therefore, something like:

```toml
[build-dependencies]
    dioxus-grpc = "*"
    tonic-build = "0.13"
    # For web:
    # tonic-build = { version = "0.13", default-features = false, features = ["prost"] }
```

But, you will also need to import some runtime dependencies:

```toml
[dependencies]
    dioxus = { version = "0.6", features = ["mobile"] }
    tonic = "0.13"
    prost = "0.13"
    # For web:
    # tonic = { version = "0.13", default-features = false, features = ["codegen", "prost"] }
    # tonic-web-wasm-client = "0.7"
```
