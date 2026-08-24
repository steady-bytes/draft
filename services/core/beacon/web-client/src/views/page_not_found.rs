use dioxus::prelude::*;

#[component]
pub fn PageNotFound(route: Vec<String>) -> Element {
    rsx! {
        div { class: "p-8",
            h1 { class: "text-xl font-bold", "Page not found" }
            p { class: "text-base-content/60", "We are terribly sorry, but the page you requested doesn't exist." }
            pre { class: "text-error text-xs mt-2", "attempted to navigate to: {route:?}" }
        }
    }
}
