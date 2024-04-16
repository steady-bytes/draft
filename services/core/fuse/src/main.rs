use tonic::{transport::Server, Request, Response, Status};

mod api;

use api::registry::health::v1::health_service_server::{HealthService, HealthServiceServer};
use api::registry::health::v1::{HealthZRequest, HealthZResponse};

#[derive(Debug, Default)]
pub struct HealthZ {}

#[tonic::async_trait]
impl HealthService for HealthZ {
    async fn health_z(&self, request: Request<HealthZRequest>) -> Result<Response<HealthZResponse>, Status> {
        println!("Got a request: {:?}", request);

        Ok(Response::new(HealthZResponse {
            ok: true
        }))
    }
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let addr = "[::1]:50051".parse().unwrap();
    let health_z = HealthZ::default();

    Server::builder()
        .add_service(HealthServiceServer::new(health_z))
        .serve(addr)
        .await?;

    Ok(())
}