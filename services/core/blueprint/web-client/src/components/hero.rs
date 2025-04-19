use dioxus::prelude::*;

static BLUEPRINT_NAME: GlobalSignal<String> = Signal::global(|| "{blueprint}".to_string());

#[component]
pub fn Hero() -> Element {
    rsx! {
        div { class: "hero bg-base-100 min-h-screen",
            div { class: "hero-content text-center",
                div { class: "max-w-md",
                    h1 { class: "text-5xl font-bold",  "{BLUEPRINT_NAME}"}
                    p { class: "py-6",
                        "A key/value store, service registry control panel for your distributed applications."
                    }
                    Link {
                        class: "btn btn-primary",
                        to: "https://draft.steady-bytes.com",
                        "Get Started"
                    }
                }
            }
        }
    }
}
