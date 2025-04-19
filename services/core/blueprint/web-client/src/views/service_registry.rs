use dioxus::prelude::*;

// const SERVICE_REGISTRY_CSS: Asset = asset!("/assets/styling/service_registry.css");

#[component]
pub fn ServiceRegistry() -> Element {
    rsx! {
        // document::Link { rel: "stylesheet", href: SERVICE_REGISTRY_CSS }

        div {
            id: "service-registry",

            // Content
            h1 { "Service Registry" }
            p { "This is the service registry page. Here, we will show how to use the Dioxus service registry to manage services in our application." }
        }
    }
}