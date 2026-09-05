use dioxus::logger::tracing::{info, Level};
use dioxus::prelude::*;
use draft_api::hook::core_registry_key_value_v1::{
    key_value_service_client::KeyValueServiceClient, GetRequest,
};
use draft_api::proto::core_control_plane_networking_v1::networking_service_client::NetworkingServiceClient;
use draft_api::proto::core_control_plane_networking_v1::ListRoutesRequest;
use draft_api::proto::core_registry_key_value_v1::{
    NavigationConfig, NavigationItem, NavigationSection,
};
use once_cell::sync::Lazy;
use prost::Message as _;
use prost_types::Any;
use std::collections::HashSet;
use tonic_web_wasm_client::Client as WasmClient;
use web_sys::window;

mod components;
mod views;

use components::{navbar_icon, navbar_menu_button, navbar_secondary_menu_button};
use views::{
    Agents, Cluster, Gateway, KeyValueView, Mcp, Metrics, NewRoute, PageNotFound, RouteDetail,
    ServiceDetail, ServiceRegistry, Settings, Store, Tools, Topology,
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
    ("/query", "Query"),
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
        #[route("/service-registry/:name")]
        ServiceDetail { name: String },
        #[route("/gateway")]
        Gateway{},
        #[route("/gateway/new")]
        NewRoute{},
        #[route("/gateway/:name")]
        RouteDetail { name: String },
        #[route("/agents")]
        Agents{},
        #[route("/mcp")]
        Mcp{},
        #[route("/tools")]
        Tools{},
        #[route("/query")]
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
        "/query" => Some(Route::Store {}),
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
                    NavigationItem {
                        label: "Key/Value".to_string(),
                        path: "/".to_string(),
                    },
                    NavigationItem {
                        label: "Service Registry".to_string(),
                        path: "/service-registry".to_string(),
                    },
                    NavigationItem {
                        label: "Gateway".to_string(),
                        path: "/gateway".to_string(),
                    },
                    NavigationItem {
                        label: "Cluster".to_string(),
                        path: "/cluster".to_string(),
                    },
                ],
            },
            NavigationSection {
                label: "Automations".to_string(),
                items: vec![
                    NavigationItem {
                        label: "Agents".to_string(),
                        path: "/agents".to_string(),
                    },
                    NavigationItem {
                        label: "MCP".to_string(),
                        path: "/mcp".to_string(),
                    },
                    NavigationItem {
                        label: "Tools".to_string(),
                        path: "/tools".to_string(),
                    },
                ],
            },
            NavigationSection {
                label: "Events".to_string(),
                items: vec![
                    NavigationItem {
                        label: "Query".to_string(),
                        path: "/query".to_string(),
                    },
                    NavigationItem {
                        label: "Topology".to_string(),
                        path: "/topology".to_string(),
                    },
                    NavigationItem {
                        label: "Metrics".to_string(),
                        path: "/metrics".to_string(),
                    },
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

/// Builds the URL for a service's UI subdomain, reusing the current page's own protocol and
/// port (so this works whether Fuse's listener is on :10000 locally or :80/:443 in a real
/// deployment) and swapping in just the host.
fn service_url(host: &str) -> String {
    let window = window().expect("no global `window` exists");
    let location = window.location();
    let protocol = location.protocol().unwrap_or_else(|_| "http:".to_string());
    let port = location.port().unwrap_or_default();
    if port.is_empty() {
        format!("{protocol}//{host}/")
    } else {
        format!("{protocol}//{host}:{port}/")
    }
}

/// A short display label derived from a UI route's host, eg. "beacon.draft.localhost" ->
/// "Beacon". Falls back to the raw host if it doesn't look like "<name>.<anything>".
fn service_label(host: &str) -> String {
    let name = host.split('.').next().unwrap_or(host);
    let mut chars = name.chars();
    match chars.next() {
        Some(first) => first.to_uppercase().collect::<String>() + chars.as_str(),
        None => host.to_string(),
    }
}

pub static API_DOMAIN: Lazy<String> = Lazy::new(|| {
    if let Some(api_domain) = option_env!("API_DOMAIN") {
        info!("API_DOMAIN: {}", api_domain);
        if api_domain.is_empty() {
            return get_domain().to_string();
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

/// Fuse's own control-plane RPC address (NetworkingService — AddRoute/ListRoutes/etc.), same
/// shape as CATALYST_DOMAIN above and for the same reason: now that each service gets its own
/// subdomain (see the "Subdomains per service" doc), API_DOMAIN is same-origin with whatever
/// page is currently loaded, which only reaches *that* service's own backend -- Blueprint's own
/// subdomain doesn't proxy to Fuse. Fuse's control-plane API is reachable directly on its own
/// bind port regardless (it doesn't route itself through the proxy it manages), so callers that
/// need it -- the Gateway views and the sidebar's own service-discovery fetch -- use this
/// instead of API_DOMAIN.
pub static FUSE_DOMAIN: Lazy<String> = Lazy::new(|| {
    if let Some(d) = option_env!("FUSE_DOMAIN") {
        if !d.is_empty() {
            info!("FUSE_DOMAIN: {}", d);
            return d.to_string();
        }
    }
    "http://localhost:18000".to_string()
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
        let config = match client
            .get(GetRequest {
                key: NAV_CONFIG_KV_KEY.to_string(),
                value: Some(Any {
                    type_url: NAV_CONFIG_TYPE_URL.to_string(),
                    value: vec![],
                }),
            })
            .await
        {
            Ok(resp) => resp
                .into_inner()
                .value
                .and_then(|any| NavigationConfig::decode(any.value.as_slice()).ok())
                .unwrap_or_else(default_nav_config),
            Err(_) => default_nav_config(),
        };
        nav_config.set(Some(config));
    });

    // Self-service discovery (#3): rather than hand-maintaining a list of every service's UI,
    // derive it from Fuse's own route table. Convention, not a dedicated flag: any route
    // matching prefix "/" on a non-default (non-empty) host is a UI worth linking to -- exactly
    // the shape every service's own "<name>.draft.localhost" WithRoute call in this repo uses
    // (see eg. services/core/beacon/main.go). A route with an empty host or a non-"/" prefix is
    // an RPC-only registration, not something to surface here.
    let service_links = use_resource(|| async move {
        let mut client = NetworkingServiceClient::new(WasmClient::new(crate::FUSE_DOMAIN.clone()));
        client.list_routes(ListRoutesRequest {}).await.map(|r| {
            let mut links: Vec<(String, String)> = r
                .into_inner()
                .routes
                .into_iter()
                .filter_map(|route| {
                    let m = route.r#match?;
                    // Blueprint is always excluded here -- this page IS Blueprint's own UI,
                    // so linking to itself in the self-discovered "Services" list is just
                    // noise (you're already looking at it).
                    if m.prefix == "/" && !m.host.is_empty() && !m.host.starts_with("blueprint.") {
                        Some((service_label(&m.host), service_url(&m.host)))
                    } else {
                        None
                    }
                })
                .collect();
            links.sort();
            links
        })
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

                    if let Some(Ok(links)) = &*service_links.read() {
                        if !links.is_empty() {
                            div { class: "divider", style: "margin: 0px;" }
                            li {
                                span { class: "menu-title", "Services" }
                                ul {
                                    for (label, url) in links.clone() {
                                        li {
                                            a { href: "{url}", target: "_blank", rel: "noopener noreferrer", "{label}" }
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
