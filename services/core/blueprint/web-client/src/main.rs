use dioxus::prelude::*;
use dioxus::logger::tracing::{Level, info};
use once_cell::sync::Lazy;
use web_sys::window;

mod views;
mod components;

use components::{navbar_menu_button, navbar_icon, navbar_secondary_menu_button};
use views::{Home, KeyValueView, ServiceRegistry, Metrics, PageNotFound};

#[derive(Debug, Clone, Routable, PartialEq)]
#[rustfmt::skip]
enum Route {
    #[layout(dashboard_layout)]
        #[route("/")]
        Home {},
        #[route("/key-val")]
        KeyValueView {},
        #[route("/service-registry")]
        ServiceRegistry{},
        #[route("/metrics")]
        Metrics{},
        // end dashboard layout, all routes above will be wrapped in this layout
    #[end_layout]

    // PageNotFound is a catch all route that will match any route and placing the matched segments in the route field
    #[route("/:..route")]
    PageNotFound {
        route: Vec<String>,
    },
}

fn get_domain() -> String {
    let window = window().expect("no global `window` exists");
    let location = window.location();
    let host = location.origin().expect("failed to get origin");
    host
}

pub static API_DOMAIN: Lazy<String> = Lazy::new(|| {
    // Check if the API_DOMAIN environment variable is set
    if let Some(api_domain) = option_env!("API_DOMAIN") {
        info!("API_DOMAIN: {}", api_domain);
        if api_domain.is_empty() {
            return get_domain().to_string()
        }

        api_domain.to_string()
    } else {
        get_domain().to_string()
    }
});

fn main() {
    dioxus::logger::init(Level::INFO).expect("logger failed to init");

    dioxus::launch(|| {
        rsx!{
            Router::<Route> {}
        }
    });
}

fn dashboard_layout() -> Element {
    rsx! {
        div { class: "drawer lg:drawer-open",
            input { class: "drawer-toggle", id: "my-drawer", type: "checkbox" }
            div { class: "drawer-content flex flex-col",

                // navbar
                div { class: "navbar bg-base-300 shadow-sm w-full",
                    div { class: "flex-none lg:hidden",
                        navbar_menu_button{}
                    }
                    div { class: "flex-1 lg:hidden",
                        navbar_icon{}
                    }
                    div{ class: "hidden flex-1 lg:block"}
                    div { class: "flex-none",
                        navbar_secondary_menu_button {}
                    }
                }

                Outlet::<Route> {}
            }

            div { class: "drawer-side",
                label {
                    aria_label: "close sidebar",
                    class: "drawer-overlay",
                    for: "my-drawer",
                }

                ul { class: "menu bg-base-200 min-h-full w-80 p-4",
                    navbar_icon{}
                    div {class: "divider", "style":  "margin: 0px 0px 0px 0px;"}
                    li {
                        Link { class: "bg-base-300",
                            to: Route::KeyValueView {}, "Key/Value" }
                    }
                    li {
                        Link { to: Route::ServiceRegistry {}, "Service Registry" }
                    }
                    li {
                        Link { to: Route::Metrics {}, "Metrics" }
                    }
                }
            }
        }
    }
}