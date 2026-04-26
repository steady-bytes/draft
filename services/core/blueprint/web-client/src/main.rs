use dioxus::prelude::*;
use dioxus::logger::tracing::{Level, info};
use once_cell::sync::Lazy;
use web_sys::window;

mod views;
mod components;

use components::{navbar_menu_button, navbar_icon, navbar_secondary_menu_button};
use views::{KeyValueView, ServiceRegistry, Gateway, PageNotFound};

#[derive(Debug, Clone, Routable, PartialEq)]
#[rustfmt::skip]
enum Route {
    #[layout(dashboard_layout)]
        #[route("/")]
        KeyValueView {},
        #[route("/service-registry")]
        ServiceRegistry{},
        #[route("/gateway")]
        Gateway{},
    #[end_layout]

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
        use_context_provider(|| dioxus_grpc::GrpcConfig {
            host: API_DOMAIN.clone(),
        });
        rsx!{
            Router::<Route> {}
        }
    });
}

fn dashboard_layout() -> Element {
    let mut control_plane_open = use_signal(|| false);

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
                    div {class: "divider", "style": "margin: 0px 0px 0px 0px;"}
                    li {
                        button {
                            class: "font-bold",
                            onclick: move |_| control_plane_open.toggle(),
                            "Control Plane"
                        }
                        if control_plane_open() {
                            ul {
                                li {
                                    Link { to: Route::KeyValueView {}, "Key/Value" }
                                }
                                li {
                                    Link { to: Route::ServiceRegistry {}, "Service Registry" }
                                }
                                li {
                                    Link { to: Route::Gateway {}, "Gateway" }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
