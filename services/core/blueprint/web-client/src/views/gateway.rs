use dioxus::prelude::*;
use draft_api::proto::core_control_plane_networking_v1::{
    networking_service_client::NetworkingServiceClient, ListRoutesRequest, MatchType, Route,
};
use tonic_web_wasm_client::Client;

use crate::components::{AuthBadge, ProtocolBadges, ValidationDot};
use crate::Route as AppRoute;

/// Two routes are in conflict if they'd compile to the same (host, match_type, prefix) tuple —
/// mirrors Fuse's own `routeKey` in `control_plane/controller.go`. Used client-side so the list
/// can flag conflicts at a glance without a round trip per row.
fn conflicts_with_any(route: &Route, all: &[Route]) -> bool {
    let key = |r: &Route| -> (String, i32, String) {
        let host = r
            .r#match
            .as_ref()
            .map(|m| m.host.clone())
            .unwrap_or_default();
        let match_type = r.r#match.as_ref().map(|m| m.match_type).unwrap_or(0);
        let prefix = r
            .r#match
            .as_ref()
            .map(|m| m.prefix.clone())
            .unwrap_or_default();
        // MATCH_TYPE_UNSPECIFIED behaves as PREFIX (see the MatchType proto docs) — normalize so
        // an unset route and an explicit PREFIX route are treated as the same key.
        (host, if match_type == 0 { 2 } else { match_type }, prefix)
    };
    let target = key(route);
    all.iter()
        .any(|other| other.name != route.name && key(other) == target)
}

fn match_type_label(match_type: i32) -> &'static str {
    match MatchType::try_from(match_type).unwrap_or(MatchType::Unspecified) {
        MatchType::Exact => "exact",
        MatchType::Unspecified | MatchType::Prefix => "prefix",
    }
}

#[component]
pub fn Gateway() -> Element {
    let list_result = use_resource(|| async {
        // Fuse's control-plane API is same-origin only when this page happens to be served
        // through Fuse itself; with per-service subdomains it generally isn't, so this uses
        // Fuse's own dedicated address rather than API_DOMAIN. See main.rs's FUSE_DOMAIN doc.
        let mut client = NetworkingServiceClient::new(Client::new(crate::FUSE_DOMAIN.clone()));
        client
            .list_routes(ListRoutesRequest {})
            .await
            .map(|r| r.into_inner())
    });
    let navigator = use_navigator();

    rsx! {
        div { class: "p-4",
            div { class: "flex items-center justify-between mb-4",
                h1 { class: "text-2xl font-bold", "Gateway" }
                Link {
                    to: AppRoute::NewRoute {},
                    class: "btn btn-primary btn-sm",
                    "+ Add Route"
                }
            }

            div { class: "overflow-x-auto",
                table { class: "table table-xs",
                    thead {
                        tr {
                            th { "Name" }
                            th { "Match" }
                            th { "Protocols" }
                            th { "Target" }
                            th { "Auth" }
                            th { "Status" }
                        }
                    }
                    tbody {
                        match &*list_result.read() {
                            Some(Ok(response)) => {
                                let all_routes = response.routes.clone();
                                if all_routes.is_empty() {
                                    rsx! {
                                        tr {
                                            td { colspan: "6", class: "text-center text-base-content/50",
                                                "No routes registered"
                                            }
                                        }
                                    }
                                } else {
                                    rsx! {
                                        for route in all_routes.clone() {
                                            {
                                                let row_name = route.name.clone();
                                                let host = route.r#match.as_ref().map(|m| m.host.clone()).unwrap_or_default();
                                                let prefix = route.r#match.as_ref().map(|m| m.prefix.clone()).unwrap_or_default();
                                                let match_type = route.r#match.as_ref().map(|m| m.match_type).unwrap_or(0);
                                                let target = route
                                                    .endpoint
                                                    .as_ref()
                                                    .map(|e| format!("{}:{}", e.host, e.port))
                                                    .unwrap_or_else(|| "—".to_string());
                                                let is_conflicting = conflicts_with_any(&route, &all_routes);

                                                rsx! {
                                                    tr {
                                                        class: "hover:bg-base-300 cursor-pointer",
                                                        onclick: move |_| { navigator.push(AppRoute::RouteDetail { name: row_name.clone() }); },
                                                        td { "{route.name}" }
                                                        td {
                                                            div { class: "flex flex-col",
                                                                if !host.is_empty() {
                                                                    span { class: "font-mono text-xs", "{host}" }
                                                                }
                                                                span { class: "font-mono text-xs text-base-content/70", "{prefix}" }
                                                                span { class: "text-[10px] text-base-content/50", "{match_type_label(match_type)}" }
                                                            }
                                                        }
                                                        td { ProtocolBadges { enable_http2: route.enable_http2 } }
                                                        td { class: "font-mono text-xs", "{target}" }
                                                        td { AuthBadge { auth: route.auth.clone() } }
                                                        td { ValidationDot { valid: !is_conflicting } }
                                                    }
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                            Some(Err(e)) => rsx! {
                                tr {
                                    td { colspan: "6", class: "text-center text-error", "Error: {e}" }
                                }
                            },
                            None => rsx! {
                                tr {
                                    td { colspan: "6", class: "text-center", "Loading..." }
                                }
                            },
                        }
                    }
                }
            }
        }
    }
}
