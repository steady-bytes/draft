use dioxus::prelude::*;

#[component]
pub fn KnowledgeResources() -> Element {
    rsx! {
        div { class: "p-4",
            h1 { class: "text-2xl font-bold mb-4", "Knowledge Resources" }
            p { "Knowledge Resources view" }
        }
    }
}
