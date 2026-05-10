use dioxus::prelude::*;

#[component]
pub fn Consumers() -> Element {
    rsx! {
        div { class: "p-4",
            h1 { class: "text-2xl font-bold mb-4", "Consumers" }
            p { class: "text-base-content/60", "Event consumers will appear here." }
        }
    }
}
