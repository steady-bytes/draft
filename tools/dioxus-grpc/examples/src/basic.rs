#[cfg(feature = "clnt")]
mod hook;
mod proto;

#[cfg(feature = "clnt")]
use dioxus::prelude::*;

#[cfg(feature = "clnt")]
fn main() {
    launch(app);
}

#[cfg(feature = "clnt")]
fn app() -> Element {
    use hook::{helloworld::use_greeter_service, messages::HelloRequest};

    let mut req = use_signal(|| {
        HelloRequest {
            name: String::new(),
        }
    });
    let greeter = use_greeter_service();

    rsx! {
        input {
            value: "{req().name}",
            oninput: move |event| req.write().name = event.value()
        }
        match &*greeter.say_hello(req).read() {
            Some(Ok(resp)) => rsx!{"[{resp.message}] - From server"},
            Some(Err(err)) => rsx!{"Couldn't get the name: {err:#?}"},
            None => rsx!{"..."},
        }
    }
}


#[cfg(feature = "srv")]
use {
    proto::helloworld::greeter_server::Greeter,
    proto::messages::{HelloReply, HelloRequest},
    tonic::{Request, Response, Status, transport::Server},
};

#[cfg(feature = "srv")]
pub struct MyGreeter;

#[cfg(feature = "srv")]
#[tonic::async_trait]
impl Greeter for MyGreeter {
    async fn say_hello(
        &self,
        request: Request<HelloRequest>,
    ) -> Result<Response<HelloReply>, Status> {
        println!("Got a request from {:?}", request.remote_addr());

        let reply = HelloReply {
            message: format!("Hello {}!", request.into_inner().name),
        };
        Ok(Response::new(reply))
    }
}

#[cfg(feature = "srv")]
#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    use {proto::helloworld::greeter_server::GreeterServer, tonic_web::GrpcWebLayer};

    let addr = "127.0.0.1:50051".parse().unwrap();

    println!("GreeterServer listening on {addr}");

    let cors = tower_http::cors::CorsLayer::new()
        .allow_methods(tower_http::cors::AllowMethods::any())
        .allow_origin(tower_http::cors::Any);

    Server::builder()
        .accept_http1(true)
        .layer(GrpcWebLayer::new())
        .layer(cors)
        .add_service(GreeterServer::new(MyGreeter))
        .serve(addr)
        .await?;

    Ok(())
}
