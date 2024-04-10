mod api;

use api::registry::health::v1::health_service_client::HealthServiceClient;
use api::registry::health::v1::HealthZRequest;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let mut client = HealthServiceClient::connect("http://[::1]:50051").await?;
    let request = tonic::Request::new(HealthZRequest {
        ..Default::default()
    });

    let response = client.health_z(request).await?;
    println!("RESPONSE={:?}", response);

    Ok(())
}