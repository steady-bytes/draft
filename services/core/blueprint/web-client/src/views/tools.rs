use dioxus::prelude::*;

#[component]
pub fn Tools() -> Element {
    rsx! {
        div { class: "p-4",
            h1 { class: "text-2xl font-bold mb-4", "Tools" }
            p { class: "text-base-content/60", "Automation tools will appear here." }
        }
    }
}
