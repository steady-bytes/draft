use dioxus::prelude::*;

mod views;
mod components;

use components::{NavbarMenuButton, NavbarIcon, NavbarSecondaryMenuButton};
use views::{Home, KeyValue, ServiceRegistry, Metrics, PageNotFound};

#[derive(Debug, Clone, Routable, PartialEq)]
#[rustfmt::skip]
enum Route {
    #[layout(DashboardLayout)]
        #[route("/")]
        Home {},
        #[route("/key-val")]
        KeyValue {},
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

fn main() {
    dioxus::launch(|| {
        rsx!{
            Router::<Route> {}
        }
    });
}

static BLUEPRINT_NAME: GlobalSignal<String> = Signal::global(|| "{blueprint}".to_string());

fn DashboardLayout() -> Element {
    rsx! {
        div { class: "drawer lg:drawer-open",
            input { class: "drawer-toggle", id: "my-drawer", type: "checkbox" }
            div { class: "drawer-content flex flex-col",

                // navbar
                div { class: "navbar bg-base-300 shadow-sm w-full",
                    div { class: "flex-none lg:hidden",
                        NavbarMenuButton{}
                    }
                    div { class: "flex-1 lg:hidden",
                        NavbarIcon {}
                    }
                    div{ class: "hidden flex-1 lg:block"}
                    div { class: "flex-none",
                        NavbarSecondaryMenuButton {}
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
                    NavbarIcon {}
                    div {class: "divider", "style":  "margin: 0px 0px 0px 0px;"}
                    li {
                        Link { to: Route::KeyValue {}, "Key/Value" }
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