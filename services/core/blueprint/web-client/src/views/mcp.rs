use dioxus::prelude::*;

#[component]
pub fn Mcp() -> Element {
    rsx! {
        div { class: "p-4",
            h1 { class: "text-2xl font-bold mb-4", "MCP" }
            p { class: "text-base-content/60", "Model Context Protocol servers will appear here." }
        }
    }
}
