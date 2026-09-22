use dioxus::prelude::*;

#[component]
pub fn PageNotFound(route: Vec<String>) -> Element {
    let path = route.join("/");
    rsx! {
        div { class: "text-center text-base-content/50 py-24",
            h1 { class: "text-2xl font-bold mb-2", "404" }
            p { "No page at /{path}" }
        }
    }
}
