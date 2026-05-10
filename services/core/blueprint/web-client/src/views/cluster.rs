use dioxus::prelude::*;

#[component]
pub fn Cluster() -> Element {
    rsx! {
        div { class: "p-4",
            h1 { class: "text-2xl font-bold mb-4", "Cluster" }
            p { class: "text-base-content/60", "Cluster event configuration will appear here." }
        }
    }
}
