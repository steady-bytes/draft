use dioxus::prelude::*;

#[component]
pub fn Memory() -> Element {
    rsx! {
        div { class: "p-4",
            h1 { class: "text-2xl font-bold mb-4", "Memory" }
            p { "Memory view" }
        }
    }
}
