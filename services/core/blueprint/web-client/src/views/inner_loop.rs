use dioxus::prelude::*;

#[component]
pub fn InnerLoop() -> Element {
    rsx! {
        div { class: "p-4",
            h1 { class: "text-2xl font-bold mb-4", "Inner Loop (subconscious)" }
            p { "Inner Loop (subconscious) view" }
        }
    }
}
