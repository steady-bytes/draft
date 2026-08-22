use dioxus::prelude::*;
use dioxus::logger::tracing::{Level, info};
use once_cell::sync::Lazy;
use std::collections::HashSet;
use web_sys::window;
use draft_api::proto::core_registry_key_value_v1::{NavigationConfig, NavigationSection, NavigationItem};
use draft_api::hook::core_registry_key_value_v1::{
    key_value_service_client::KeyValueServiceClient,
    GetRequest,
};
use prost::Message as _;
use prost_types::Any;
use tonic_web_wasm_client::Client as WasmClient;

mod views;
mod components;

use components::{navbar_menu_button, navbar_icon, navbar_secondary_menu_button};
use views::{
    KeyValueView, ServiceRegistry, Gateway, Agents, Mcp, Tools,
    Store, Topology, Cluster, Metrics, Settings, PageNotFound,
};

pub const NAV_CONFIG_KV_KEY: &str = "ui/navigation";
pub const NAV_CONFIG_TYPE_URL: &str =
    "type.googleapis.com/core.registry.key_value.v1.NavigationConfig";

pub const KNOWN_ROUTES: &[(&str, &str)] = &[
    ("/", "Key/Value"),
    ("/service-registry", "Service Registry"),
    ("/gateway", "Gateway"),
    ("/agents", "Agents"),
    ("/mcp", "MCP"),
    ("/tools", "Tools"),
    ("/store", "Store"),
    ("/topology", "Topology"),
    ("/cluster", "Cluster"),
    ("/metrics", "Metrics"),
];

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
        #[route("/agents")]
        Agents{},
        #[route("/mcp")]
        Mcp{},
        #[route("/tools")]
        Tools{},
        #[route("/store")]
        Store{},
        #[route("/topology")]
        Topology{},
        #[route("/cluster")]
        Cluster{},
        #[route("/metrics")]
        Metrics{},
        #[route("/settings")]
        Settings{},
    #[end_layout]

    #[route("/:..route")]
    PageNotFound {
        route: Vec<String>,
    },
}

pub fn path_to_route(path: &str) -> Option<Route> {
    match path {
        "/" => Some(Route::KeyValueView {}),
        "/service-registry" => Some(Route::ServiceRegistry {}),
        "/gateway" => Some(Route::Gateway {}),
        "/agents" => Some(Route::Agents {}),
        "/mcp" => Some(Route::Mcp {}),
        "/tools" => Some(Route::Tools {}),
        "/store" => Some(Route::Store {}),
        "/topology" => Some(Route::Topology {}),
        "/cluster" => Some(Route::Cluster {}),
        "/metrics" => Some(Route::Metrics {}),
        _ => None,
    }
}

pub fn default_nav_config() -> NavigationConfig {
    NavigationConfig {
        sections: vec![
            NavigationSection {
                label: "Control Plane".to_string(),
                items: vec![
                    NavigationItem { label: "Key/Value".to_string(), path: "/".to_string() },
                    NavigationItem { label: "Service Registry".to_string(), path: "/service-registry".to_string() },
                    NavigationItem { label: "Gateway".to_string(), path: "/gateway".to_string() },
                ],
            },
            NavigationSection {
                label: "Automations".to_string(),
                items: vec![
                    NavigationItem { label: "Agents".to_string(), path: "/agents".to_string() },
                    NavigationItem { label: "MCP".to_string(), path: "/mcp".to_string() },
                    NavigationItem { label: "Tools".to_string(), path: "/tools".to_string() },
                ],
            },
            NavigationSection {
                label: "Events".to_string(),
                items: vec![
                    NavigationItem { label: "Store".to_string(), path: "/store".to_string() },
                    NavigationItem { label: "Topology".to_string(), path: "/topology".to_string() },
                    NavigationItem { label: "Cluster".to_string(), path: "/cluster".to_string() },
                    NavigationItem { label: "Metrics".to_string(), path: "/metrics".to_string() },
                ],
            },
        ],
    }
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

pub static CATALYST_DOMAIN: Lazy<String> = Lazy::new(|| {
    if let Some(d) = option_env!("CATALYST_DOMAIN") {
        if !d.is_empty() {
            info!("CATALYST_DOMAIN: {}", d);
            return d.to_string();
        }
    }
    "http://localhost:2220".to_string()
});

fn main() {
    dioxus::logger::init(Level::INFO).expect("logger failed to init");

    dioxus::launch(|| {
        use_context_provider(|| dioxus_grpc::GrpcConfig {
            host: API_DOMAIN.clone(),
        });
        rsx! {
            Router::<Route> {}
        }
    });
}

fn dashboard_layout() -> Element {
    let mut nav_config: Signal<Option<NavigationConfig>> =
        use_context_provider(|| Signal::new(None));
    let mut open_sections: Signal<HashSet<String>> = use_signal(HashSet::new);

    // Fetch nav config from KV once on mount; fall back to hardcoded default.
    let _fetch = use_resource(move || async move {
        let mut client = KeyValueServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
        let config = match client.get(GetRequest {
            key: NAV_CONFIG_KV_KEY.to_string(),
            value: Some(Any {
                type_url: NAV_CONFIG_TYPE_URL.to_string(),
                value: vec![],
            }),
        }).await {
            Ok(resp) => resp
                .into_inner()
                .value
                .and_then(|any| NavigationConfig::decode(any.value.as_slice()).ok())
                .unwrap_or_else(default_nav_config),
            Err(_) => default_nav_config(),
        };
        nav_config.set(Some(config));
    });

    // Pre-process config for rendering so rsx! sees plain owned data.
    let config = nav_config().unwrap_or_else(default_nav_config);
    let open = open_sections();
    let sections: Vec<(String, bool, Vec<(String, Option<Route>)>)> = config
        .sections
        .into_iter()
        .map(|s| {
            let is_open = open.contains(&s.label);
            let items = s
                .items
                .into_iter()
                .map(|i| (i.label, path_to_route(&i.path)))
                .collect();
            (s.label, is_open, items)
        })
        .collect();

    rsx! {
        div { class: "drawer lg:drawer-open",
            input { class: "drawer-toggle", id: "my-drawer", r#type: "checkbox" }
            div { class: "drawer-content flex flex-col",

                div { class: "navbar bg-base-300 shadow-sm w-full",
                    div { class: "flex-none lg:hidden",
                        navbar_menu_button {}
                    }
                    div { class: "flex-1 lg:hidden",
                        navbar_icon {}
                    }
                    div { class: "hidden flex-1 lg:block" }
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
                    r#for: "my-drawer",
                }

                ul { class: "menu bg-base-200 min-h-full w-80 p-4",
                    navbar_icon {}
                    div { class: "divider", style: "margin: 0px;" }

                    for (label, is_open, items) in sections {
                        {
                            let toggle_label = label.clone();
                            rsx! {
                                li {
                                    button {
                                        class: "font-bold",
                                        onclick: move |_| {
                                            let lbl = toggle_label.clone();
                                            let mut set = open_sections();
                                            if set.contains(&lbl) {
                                                set.remove(&lbl);
                                            } else {
                                                set.insert(lbl);
                                            }
                                            open_sections.set(set);
                                        },
                                        "{label}"
                                    }
                                    if is_open {
                                        ul {
                                            for (item_label, route) in items {
                                                {
                                                    if let Some(r) = route {
                                                        rsx! { li { Link { to: r, "{item_label}" } } }
                                                    } else {
                                                        rsx! {}
                                                    }
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }

                    div { class: "divider", style: "margin: 0px;" }
                    li {
                        Link { to: Route::Settings {}, "Settings" }
                    }
                }
            }
        }
    }
}
