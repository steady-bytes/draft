use dioxus::prelude::*;

// const METRICS_CSS: Asset = asset!("/assets/styling/metrics.css");

#[component]
pub fn Metrics() -> Element {
    rsx! {
        // document::Link { rel: "stylesheet", href: METRICS_CSS }

        div {
            id: "metrics",

            // Content
            h1 { "Metrics" }
            p { "This is the metrics page. Here, we will show how to use the Dioxus metrics to monitor our application." }
        }
    }
}