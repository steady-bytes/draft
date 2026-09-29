//! The rail's **Apps** block: links to the other Draft apps, discovered from Fuse's route table.
//!
//! Convention, not a flag: any route matching prefix `/` on a non-empty host is a service UI worth
//! linking to (the shape every service's `<name>.draft.localhost` `WithRoute` call uses). Routes
//! with an empty host or a non-`/` prefix are RPC-only registrations. Ported from Blueprint's
//! `dashboard_layout`, so Beacon and Lineman get it too.

use super::nav::NavItem;
use crate::kinds::AppKind;
use crate::util::sibling_url;

/// `beacon.draft.localhost` → `Beacon`. Falls back to the host if it has no leading label.
pub fn host_label(host: &str) -> String {
    let name = host.split('.').next().unwrap_or(host);
    let mut chars = name.chars();
    match chars.next() {
        Some(first) => first.to_uppercase().collect::<String>() + chars.as_str(),
        None => host.to_string(),
    }
}

/// Builds the Apps links from `(host, prefix)` route matches.
///
/// `exclude` is the app hosting this UI (linking to yourself is noise). `protocol` and `port` come
/// from the current page so links work whether Fuse listens on :10000 locally or :80/:443.
pub fn app_links<'a>(
    routes: impl IntoIterator<Item = (&'a str, &'a str)>,
    exclude: AppKind,
    protocol: &str,
    port: &str,
) -> Vec<NavItem> {
    let mut links: Vec<NavItem> = routes
        .into_iter()
        .filter(|(host, prefix)| *prefix == "/" && !host.is_empty())
        .filter(|(host, _)| AppKind::from_name(host) != exclude || exclude == AppKind::Service)
        .map(|(host, _)| NavItem::app(AppKind::from_name(host), host_label(host), sibling_url(protocol, port, host)))
        .collect();
    links.sort_by(|a, b| a.label.cmp(&b.label));
    links.dedup_by(|a, b| a.path == b.path);
    links
}

#[cfg(feature = "discovery")]
pub use net::use_app_links;

#[cfg(feature = "discovery")]
mod net {
    use dioxus::prelude::*;
    use draft_api::proto::core_control_plane_networking_v1::networking_service_client::NetworkingServiceClient;
    use draft_api::proto::core_control_plane_networking_v1::ListRoutesRequest;
    use tonic_web_wasm_client::Client as WasmClient;

    use super::app_links;
    use crate::kinds::AppKind;
    use crate::shell::NavItem;

    /// The Apps links for the rail, fetched once from Fuse's `ListRoutes` at `fuse_domain`
    /// (its control-plane address; it does not route itself through the proxy it manages).
    /// Empty until loaded and on any error — the rail simply omits the block.
    pub fn use_app_links(fuse_domain: String, exclude: AppKind) -> Vec<NavItem> {
        let links = use_resource(move || {
            let fuse = fuse_domain.clone();
            async move {
                let mut client = NetworkingServiceClient::new(WasmClient::new(fuse));
                let routes = client.list_routes(ListRoutesRequest {}).await.ok()?.into_inner().routes;
                let (protocol, port) = web_sys::window()
                    .map(|w| {
                        let l = w.location();
                        (l.protocol().unwrap_or_else(|_| "http:".into()), l.port().unwrap_or_default())
                    })
                    .unwrap_or_else(|| ("http:".into(), String::new()));
                let pairs: Vec<(String, String)> = routes
                    .into_iter()
                    .filter_map(|r| r.r#match.map(|m| (m.host, m.prefix)))
                    .collect();
                Some(app_links(pairs.iter().map(|(h, p)| (h.as_str(), p.as_str())), exclude, &protocol, &port))
            }
        });
        let value = links.read();
        value.clone().flatten().unwrap_or_default()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const ROUTES: &[(&str, &str)] = &[
        ("beacon.draft.localhost", "/"),
        ("blueprint.draft.localhost", "/"),
        ("bench.draft.localhost", "/"),
        ("", "/examples.crud.v1.CrudService/"),
        ("lineman.draft.localhost", "/tooling.lineman.v1.LinemanService/"),
        ("", "/"),
    ];

    #[test]
    fn only_host_routes_with_a_root_prefix_become_links() {
        let links = app_links(ROUTES.iter().copied(), AppKind::Service, "http:", "10000");
        let labels: Vec<_> = links.iter().map(|l| l.label.as_str()).collect();
        assert_eq!(labels, ["Bench", "Beacon", "Blueprint"].iter().copied().collect::<std::collections::BTreeSet<_>>().into_iter().collect::<Vec<_>>());
    }

    #[test]
    fn the_current_app_is_excluded() {
        let links = app_links(ROUTES.iter().copied(), AppKind::Blueprint, "http:", "10000");
        assert!(links.iter().all(|l| l.label != "Blueprint"));
        assert!(links.iter().any(|l| l.label == "Beacon"));
    }

    #[test]
    fn links_keep_the_page_protocol_and_port() {
        let links = app_links(ROUTES.iter().copied(), AppKind::Blueprint, "http:", "10000");
        let beacon = links.iter().find(|l| l.label == "Beacon").unwrap();
        assert_eq!(beacon.path, "http://beacon.draft.localhost:10000/");
        assert!(beacon.external);
        assert_eq!(beacon.glyph.as_ref().unwrap().0, "Bc");
    }

    #[test]
    fn labels_are_capitalised_host_prefixes() {
        assert_eq!(host_label("foundry.draft.localhost"), "Foundry");
        assert_eq!(host_label(""), "");
    }
}
