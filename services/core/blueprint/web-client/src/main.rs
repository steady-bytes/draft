use dioxus::prelude::*;
use dioxus::logger::tracing::{Level, info};
use once_cell::sync::Lazy;
use web_sys::window;

mod views;
mod components;

use components::{navbar_menu_button, navbar_icon, navbar_secondary_menu_button};
use views::{Home, KeyValueView, ServiceRegistry, CronJobs, LLM, InnerLoop, OuterLoop, Memory, Workflows, Agents, Skills, Specs, KnowledgeResources, Gateway, Webhooks, Logs, Metrics, PageNotFound};

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
        #[route("/cron-jobs")]
        CronJobs{},
        #[route("/llm")]
        LLM{},
        #[route("/inner-loop")]
        InnerLoop{},
        #[route("/outer-loop")]
        OuterLoop{},
        #[route("/memory")]
        Memory{},
        #[route("/workflows")]
        Workflows{},
        #[route("/agents")]
        Agents{},
        #[route("/skills")]
        Skills{},
        #[route("/specs")]
        Specs{},
        #[route("/knowledge-resources")]
        KnowledgeResources{},
        #[route("/gateway")]
        Gateway{},
        #[route("/webhooks")]
        Webhooks{},
        #[route("/logs")]
        Logs{},
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
        use_context_provider(|| dioxus_grpc::GrpcConfig {
            host: API_DOMAIN.clone(),
        });
        rsx!{
            Router::<Route> {}
        }
    });
}

fn dashboard_layout() -> Element {
    let mut control_open = use_signal(|| false);
    let mut automata_open = use_signal(|| false);
    let mut projects_open = use_signal(|| false);
    let mut settings_open = use_signal(|| false);
    let mut brain_open = use_signal(|| false);

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
                        button {
                            class: "font-bold",
                            onclick: move |_| control_open.toggle(),
                            "Control"
                        }
                        if control_open() {
                            ul {
                                li {
                                    Link { to: Route::ServiceRegistry {}, "Service Registry" }
                                }

                                li {
                                    Link { to: Route::CronJobs {}, "Jobs" }
                                }

                                li {
                                    button {
                                        class: "w-full text-left",
                                        onclick: move |_| brain_open.toggle(),
                                        "Brain"
                                    }
                                    if brain_open() {
                                        ul {
                                            li {
                                                Link { to: Route::InnerLoop {}, "Inner Loop (subconscious)" }
                                            }
                                            li {
                                                Link { to: Route::OuterLoop {}, "Outer Loop (conscious)" }
                                            }
                                        }
                                    }
                                }

                                li {
                                    Link { to: Route::Memory {}, "Memory" }
                                }

                                li {
                                    Link { to: Route::KnowledgeResources {}, "Knowledge" }
                                }
                            }
                        }
                    }
                    li {
                        button {
                            class: "font-bold",
                            onclick: move |_| automata_open.toggle(),
                            "Automata"
                        }
                        if automata_open() {
                            ul {
                                li {
                                    Link { to: Route::Agents {}, "Agents" }
                                }
                                li {
                                    Link { to: Route::Workflows {}, "Workflows" }
                                }
                                li {
                                    Link { to: Route::Skills {}, "Skills" }
                                }
                            }
                        }
                    }
                    li {
                        button {
                            class: "font-bold",
                            onclick: move |_| projects_open.toggle(),
                            "Projects"
                        }
                        if projects_open() {
                            ul {
                                li {
                                    Link { to: Route::Specs {}, "Specs" }
                                }
                            }
                        }
                    }
                    li {
                        button {
                            class: "font-bold",
                            onclick: move |_| settings_open.toggle(),
                            "Settings"
                        }
                        if settings_open() {
                            ul {
                                li {
                                    Link { to: Route::KeyValueView { }, "Key/Value View" }
                                }
                                li {
                                    Link { to: Route::Gateway {}, "Gateway" }
                                }
                                li {
                                    Link { to: Route::Webhooks {}, "Webhooks" }
                                }
                                li {
                                    Link { to: Route::Logs {}, "Logs" }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}