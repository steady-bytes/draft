use dioxus::logger::tracing::{info, Level};
use dioxus::prelude::*;
use draft_api::hook::core_registry_key_value_v1::{
    key_value_service_client::KeyValueServiceClient, GetRequest, ListKindsRequest,
};
use draft_api::hook::core_registry_service_discovery_v1::{filter, Filter, QueryRequest};
use draft_api::proto::core_control_plane_networking_v1::{
    networking_service_client::NetworkingServiceClient, ListRoutesRequest,
};
use draft_api::proto::core_registry_service_discovery_v1::service_discovery_service_client::ServiceDiscoveryServiceClient;
use draft_api::proto::core_registry_key_value_v1::{
    NavigationConfig, NavigationItem, NavigationSection,
};
use draft_ui::kinds::AppKind;
use draft_ui::shell::{use_app_links, AppShell, NavItem, NavSection};
use draft_ui::ui::NotFound;
use draft_ui::DraftStyles;
use once_cell::sync::Lazy;
use prost::Message as _;
use prost_types::Any;
use tonic_web_wasm_client::Client as WasmClient;
use web_sys::window;

mod cesql;
mod events;
mod kv;
mod raft;
mod registry;
mod routes;
mod topology;
mod views;

use views::{
    Cluster, Gateway, KeyValueDetail, KeyValueView, Metrics, NewRoute, RouteDetail, ServiceDetail,
    ServiceRegistry, Settings, Store, Topology,
};

pub const NAV_CONFIG_KV_KEY: &str = "ui/navigation";
pub const NAV_CONFIG_TYPE_URL: &str =
    "type.googleapis.com/core.registry.key_value.v1.NavigationConfig";

pub const KNOWN_ROUTES: &[(&str, &str)] = &[
    ("/", "Key/Value"),
    ("/service-registry", "Service Registry"),
    ("/gateway", "Gateway"),
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
        #[route("/kv/:..kv_key_parts")]
        KeyValueDetail { kv_key_parts: Vec<String> },
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
        // Unknown paths keep the shell, so the rail is still there to navigate from.
        #[route("/:..route")]
        NotFound { route: Vec<String> },
}

pub fn path_to_route(path: &str) -> Option<Route> {
    match path {
        "/" => Some(Route::KeyValueView {}),
        "/service-registry" => Some(Route::ServiceRegistry {}),
        "/gateway" => Some(Route::Gateway {}),
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

/// The Key/Value list → detail hand-off channel: a row click needs to tell the detail page which
/// *kind* (type_url) it's looking at, since `Get`'s `value.type_url` determines the physical key
/// to fetch (see `key_value/model.go`'s `makeKey`) -- passing the wrong one for a non-`Value` kind
/// would look up a key that doesn't exist. There's no existing precedent for a Dioxus Router query
/// param in this codebase, and a type_url contains its own literal `/`
/// ("type.googleapis.com/...") which would collide with a path segment anyway, so this mirrors
/// Beacon's own `PENDING_TRACE_ID` GlobalSignal for exactly this shape of one-shot cross-view
/// hand-off. Read once and cleared on the detail page's mount.
/// The Topology → Events hand-off: a CESQL filter (`type = '…'`) for the events view to apply on
/// arrival. Same one-shot pattern as `PENDING_KV_KIND`.
pub static PENDING_EVENT_QUERY: GlobalSignal<Option<String>> = Signal::global(|| None);

pub static PENDING_KV_KIND: GlobalSignal<Option<String>> = Signal::global(|| None);

fn main() {
    dioxus::logger::init(Level::INFO).expect("logger failed to init");

    dioxus::launch(|| {
        use_context_provider(|| dioxus_grpc::GrpcConfig {
            host: API_DOMAIN.clone(),
        });
        rsx! {
            DraftStyles {}
            // Blueprint's own few rules (the cluster canvas, KV drawer), on top of the shared design system.
            document::Stylesheet { href: asset!("/assets/blueprint.css") }
            Router::<Route> {}
        }
    });
}

/// The nav config, shared with Settings (which edits it) through context. Fetched from KV once;
/// the hardcoded default stands in until it arrives and if there is none.
fn use_nav_config() -> Signal<Option<NavigationConfig>> {
    let mut nav_config: Signal<Option<NavigationConfig>> = use_context_provider(|| Signal::new(None));
    let _fetch = use_resource(move || async move {
        let mut client = KeyValueServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
        let config = match client
            .get(GetRequest {
                key: NAV_CONFIG_KV_KEY.to_string(),
                value: Some(Any { type_url: NAV_CONFIG_TYPE_URL.to_string(), value: vec![] }),
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
    nav_config
}

/// How many entries, services and routes there are, for the rail's count badges. Fetched once when
/// the shell mounts; each is `None` until it arrives (or if its service is unreachable), and a
/// missing count simply shows no badge.
#[derive(Clone, Copy, Default, PartialEq)]
struct RailCounts {
    entries: Option<u32>,
    services: Option<u32>,
    routes: Option<u32>,
}

fn use_rail_counts() -> RailCounts {
    let entries = use_resource(|| async {
        let mut client = KeyValueServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
        let kinds = client.list_kinds(ListKindsRequest {}).await.ok()?.into_inner().kinds;
        Some(kinds.iter().find(|k| k.type_url == kv::VALUE_TYPE_URL).map(|k| k.count as u32).unwrap_or(0))
    });
    let services = use_resource(|| async {
        let mut client = ServiceDiscoveryServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
        let all = QueryRequest { filter: Some(Filter { attribute: Some(filter::Attribute::All(String::new())) }) };
        let data = client.query(all).await.ok()?.into_inner().data;
        // One service per name: a restart or a replica adds a registry row, not a service.
        let names: std::collections::HashSet<String> = data.values().map(|p| p.name.clone()).collect();
        Some(names.len() as u32)
    });
    let routes = use_resource(|| async {
        let mut client = NetworkingServiceClient::new(WasmClient::new(crate::FUSE_DOMAIN.clone()));
        Some(client.list_routes(ListRoutesRequest {}).await.ok()?.into_inner().routes.len() as u32)
    });
    let (entries, services, routes) = (
        entries.read().clone().flatten(),
        services.read().clone().flatten(),
        routes.read().clone().flatten(),
    );
    RailCounts { entries, services, routes }
}

/// The shell: rail (the configured sections, System, and the Apps Fuse advertises), topbar and
/// status bar around the views. Sections come from the `ui/navigation` KV entry, unchanged.
fn dashboard_layout() -> Element {
    let route = use_route::<Route>();
    let nav_config = use_nav_config();
    let _raft = raft::use_raft_provider();
    let counts = use_rail_counts();
    let apps = use_app_links(FUSE_DOMAIN.clone(), AppKind::Blueprint);

    let config = nav_config().unwrap_or_else(default_nav_config);
    let mut sections: Vec<NavSection> = config
        .sections
        .into_iter()
        .map(|s| {
            // An item whose path is not a page of this app (a stale config) is dropped, as before.
            let items = s
                .items
                .into_iter()
                .filter(|i| path_to_route(&i.path).is_some())
                .map(|i| {
                    let count = match i.path.as_str() {
                        "/" => counts.entries,
                        "/service-registry" => counts.services,
                        "/gateway" => counts.routes,
                        _ => None,
                    };
                    let item = NavItem::new(i.label, i.path.clone());
                    let item = if i.path == "/" { item.exact() } else { item };
                    match count {
                        Some(n) => item.count(n),
                        None => item,
                    }
                })
                .collect();
            NavSection::new(s.label, items)
        })
        .collect();
    sections.push(NavSection::new("System", vec![NavItem::new("Settings", "/settings")]));

    // A key's detail page is the Key/Value list with its drawer open, so it keeps that item lit.
    let current = match &route {
        Route::KeyValueDetail { .. } => "/".to_string(),
        other => other.to_string(),
    };

    // The cluster canvas fills the whole main area.
    let flush = matches!(route, Route::Cluster {});

    rsx! {
        AppShell { app: AppKind::Blueprint, current, sections, apps, flush, Outlet::<Route> {} }
    }
}
