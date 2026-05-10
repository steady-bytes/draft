use dioxus::prelude::*;

#[component]
pub fn Producers() -> Element {
    rsx! {
        div { class: "p-4",
            h1 { class: "text-2xl font-bold mb-4", "Producers" }
            p { class: "text-base-content/60", "Event producers will appear here." }
        }
    }
}
