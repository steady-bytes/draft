use dioxus::prelude::*;
use draft_api::proto::core_control_plane_networking_v1::{
    networking_service_client::NetworkingServiceClient, AddRouteRequest, AuthPolicy,
    DeleteRouteRequest, Endpoint, ListRoutesRequest, MatchType, Route, RouteAuth, RouteMatch,
    ValidateRouteRequest,
};
use tonic_web_wasm_client::Client;

use crate::Route as AppRoute;

/// `/gateway/:name` — loads the existing route by scanning `ListRoutes` (there is no `GetRoute`
/// RPC yet) and renders the shared form pre-filled. `key` on `RouteForm` forces a fresh mount
/// (and fresh signal init) when navigating from one route's detail straight to another's.
#[component]
pub fn RouteDetail(name: String) -> Element {
    let lookup = use_resource({
        let name = name.clone();
        move || {
            let name = name.clone();
            async move {
                let mut client =
                    NetworkingServiceClient::new(Client::new(crate::FUSE_DOMAIN.clone()));
                client
                    .list_routes(ListRoutesRequest {})
                    .await
                    .map(|r| r.into_inner().routes.into_iter().find(|r| r.name == name))
            }
        }
    });

    rsx! {
        div { class: "p-4",
            match &*lookup.read() {
                Some(Ok(Some(route))) => rsx! {
                    RouteForm {
                        key: "{name}",
                        existing: Some(route.clone()),
                        original_name: Some(name.clone()),
                    }
                },
                Some(Ok(None)) => rsx! {
                    div { class: "alert alert-warning",
                        span { "No route named \"{name}\" — it may have just been deleted." }
                    }
                    Link { to: AppRoute::Gateway {}, class: "link mt-4 inline-block", "Back to Gateway" }
                },
                Some(Err(e)) => rsx! {
                    div { class: "alert alert-error", span { "Error: {e}" } }
                },
                None => rsx! { div { class: "text-center", "Loading..." } },
            }
        }
    }
}

/// `/gateway/new` — same form, no lookup, no delete action.
#[component]
pub fn NewRoute() -> Element {
    rsx! {
        div { class: "p-4",
            RouteForm { existing: None, original_name: None }
        }
    }
}

fn match_type_to_str(match_type: i32) -> &'static str {
    match MatchType::try_from(match_type).unwrap_or(MatchType::Unspecified) {
        MatchType::Exact => "exact",
        MatchType::Unspecified | MatchType::Prefix => "prefix",
    }
}

fn auth_policy_to_str(auth: &Option<RouteAuth>) -> &'static str {
    match auth.as_ref().filter(|a| a.enabled) {
        None => "bypass",
        Some(a) => match AuthPolicy::try_from(a.policy).unwrap_or(AuthPolicy::Bypass) {
            AuthPolicy::Bypass => "bypass",
            AuthPolicy::Authenticated => "authenticated",
            AuthPolicy::Groups => "groups",
            AuthPolicy::Scopes => "scopes",
        },
    }
}

fn join_csv(values: &[String]) -> String {
    values.join(", ")
}

fn split_csv(value: &str) -> Vec<String> {
    value
        .split(',')
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty())
        .collect()
}

#[component]
fn RouteForm(existing: Option<Route>, original_name: Option<String>) -> Element {
    let navigator = use_navigator();
    let is_editing = original_name.is_some();

    let m = existing
        .as_ref()
        .and_then(|r| r.r#match.clone())
        .unwrap_or_default();
    let ep = existing
        .as_ref()
        .and_then(|r| r.endpoint.clone())
        .unwrap_or_default();
    let auth = existing.as_ref().and_then(|r| r.auth.clone());

    let mut form_name = use_signal({
        let v = existing
            .as_ref()
            .map(|r| r.name.clone())
            .unwrap_or_default();
        move || v.clone()
    });
    let mut form_prefix = use_signal({
        let v = m.prefix.clone();
        move || v.clone()
    });
    let mut form_host = use_signal({
        let v = m.host.clone();
        move || v.clone()
    });
    let mut form_match_type = use_signal({
        let v = match_type_to_str(m.match_type).to_string();
        move || v.clone()
    });
    let mut form_ep_host = use_signal({
        let v = ep.host.clone();
        move || v.clone()
    });
    let mut form_ep_port = use_signal({
        let v = if ep.port == 0 {
            String::new()
        } else {
            ep.port.to_string()
        };
        move || v.clone()
    });
    let mut form_http2 = use_signal({
        let v = existing.as_ref().map(|r| r.enable_http2).unwrap_or(false);
        move || v
    });
    let mut form_auth_enabled = use_signal({
        let v = auth.as_ref().map(|a| a.enabled).unwrap_or(false);
        move || v
    });
    let mut form_auth_policy = use_signal({
        let v = auth_policy_to_str(&auth).to_string();
        move || v.clone()
    });
    let mut form_required_groups = use_signal({
        let v = auth
            .as_ref()
            .map(|a| join_csv(&a.required_groups))
            .unwrap_or_default();
        move || v.clone()
    });
    let mut form_required_scopes = use_signal({
        let v = auth
            .as_ref()
            .map(|a| join_csv(&a.required_scopes))
            .unwrap_or_default();
        move || v.clone()
    });

    let mut status: Signal<Option<String>> = use_signal(|| None);
    let mut conflicts: Signal<Vec<String>> = use_signal(Vec::new);

    let original_name_for_submit = original_name.clone();
    let submit = move |_| {
        let name = form_name();
        let prefix = form_prefix();
        let host = form_host();
        let match_type = match form_match_type().as_str() {
            "exact" => MatchType::Exact,
            _ => MatchType::Prefix,
        };
        let ep_host = form_ep_host();
        let port: u32 = form_ep_port().parse().unwrap_or(0);
        let http2 = form_http2();
        let auth_enabled = form_auth_enabled();
        let auth_policy = match form_auth_policy().as_str() {
            "authenticated" => AuthPolicy::Authenticated,
            "groups" => AuthPolicy::Groups,
            "scopes" => AuthPolicy::Scopes,
            _ => AuthPolicy::Bypass,
        };
        let required_groups = split_csv(&form_required_groups());
        let required_scopes = split_csv(&form_required_scopes());
        let original = original_name_for_submit.clone();

        spawn(async move {
            let route = Route {
                name: name.clone(),
                r#match: Some(RouteMatch {
                    prefix,
                    host,
                    headers: None,
                    grpc_match_options: None,
                    dynamic_metadata: None,
                    match_type: match_type as i32,
                }),
                endpoint: Some(Endpoint {
                    host: ep_host,
                    port,
                }),
                enable_http2: http2,
                auth: if auth_enabled {
                    Some(RouteAuth {
                        enabled: true,
                        policy: auth_policy as i32,
                        required_groups,
                        required_scopes,
                    })
                } else {
                    None
                },
            };

            let mut client = NetworkingServiceClient::new(Client::new(crate::FUSE_DOMAIN.clone()));

            match client
                .validate_route(ValidateRouteRequest {
                    route: Some(route.clone()),
                })
                .await
            {
                Ok(resp) => {
                    let resp = resp.into_inner();
                    if !resp.valid {
                        // The conflict banner below already carries this detail; no separate
                        // status message needed.
                        conflicts.set(resp.conflicting_routes);
                        return;
                    }
                }
                Err(e) => {
                    status.set(Some(format!("Validation error: {e}")));
                    return;
                }
            }
            conflicts.set(Vec::new());

            // A rename needs the old key removed — an unchanged name is a plain upsert (AddRoute
            // sets by name), so no delete-then-add round trip is needed in the common case.
            if let Some(old_name) = &original {
                if old_name != &name {
                    if let Err(e) = client
                        .delete_route(DeleteRouteRequest {
                            name: old_name.clone(),
                        })
                        .await
                    {
                        status.set(Some(format!("Error removing old route: {e}")));
                        return;
                    }
                }
            }

            match client
                .add_route(AddRouteRequest { route: Some(route) })
                .await
            {
                Ok(_) => {
                    navigator.push(AppRoute::Gateway {});
                }
                Err(e) => status.set(Some(format!("Error: {e}"))),
            };
        });
    };

    let delete_name = original_name.clone();
    let delete = move |_| {
        let Some(name) = delete_name.clone() else {
            return;
        };
        spawn(async move {
            let mut client = NetworkingServiceClient::new(Client::new(crate::FUSE_DOMAIN.clone()));
            match client.delete_route(DeleteRouteRequest { name }).await {
                Ok(_) => {
                    navigator.push(AppRoute::Gateway {});
                }
                Err(e) => status.set(Some(format!("Error: {e}"))),
            }
        });
    };

    let title = if is_editing {
        "Edit Route"
    } else {
        "New Route"
    };

    rsx! {
        div { class: "flex items-center justify-between mb-4",
            h1 { class: "text-2xl font-bold", "{title}" }
            Link { to: AppRoute::Gateway {}, class: "link", "Back to Gateway" }
        }

        if let Some(msg) = status() {
            div { class: "alert alert-info mb-4", span { "{msg}" } }
        }

        if !conflicts().is_empty() {
            div { class: "alert alert-error mb-4",
                span { "Conflicts with existing route(s): {conflicts().join(\", \")}" }
            }
        }

        div { class: "max-w-xl flex flex-col gap-3",
            div { class: "form-control",
                label { class: "label", span { class: "label-text", "Name" } }
                input {
                    class: "input input-bordered input-sm w-full",
                    value: "{form_name}",
                    oninput: move |e| form_name.set(e.value()),
                }
            }
            div { class: "form-control",
                label { class: "label", span { class: "label-text", "Match Type" } }
                select {
                    class: "select select-bordered select-sm w-full",
                    value: "{form_match_type}",
                    onchange: move |e| form_match_type.set(e.value()),
                    option { value: "prefix", "Prefix" }
                    option { value: "exact", "Exact" }
                }
            }
            div { class: "form-control",
                label { class: "label", span { class: "label-text", "Match Prefix" } }
                input {
                    class: "input input-bordered input-sm w-full",
                    value: "{form_prefix}",
                    oninput: move |e| form_prefix.set(e.value()),
                }
            }
            div { class: "form-control",
                label { class: "label", span { class: "label-text", "Match Host (eg. *.draft.localhost)" } }
                input {
                    class: "input input-bordered input-sm w-full",
                    value: "{form_host}",
                    oninput: move |e| form_host.set(e.value()),
                }
            }
            div { class: "form-control",
                label { class: "label", span { class: "label-text", "Endpoint Host" } }
                input {
                    class: "input input-bordered input-sm w-full",
                    value: "{form_ep_host}",
                    oninput: move |e| form_ep_host.set(e.value()),
                }
            }
            div { class: "form-control",
                label { class: "label", span { class: "label-text", "Endpoint Port" } }
                input {
                    r#type: "number",
                    class: "input input-bordered input-sm w-full",
                    value: "{form_ep_port}",
                    oninput: move |e| form_ep_port.set(e.value()),
                }
            }
            div { class: "form-control",
                label { class: "label cursor-pointer justify-start gap-3",
                    input {
                        r#type: "checkbox",
                        class: "toggle",
                        checked: form_http2(),
                        onchange: move |e| form_http2.set(e.checked()),
                    }
                    span { class: "label-text", "Enable HTTP2" }
                }
            }

            div { class: "divider", "Auth" }

            div { class: "form-control",
                label { class: "label cursor-pointer justify-start gap-3",
                    input {
                        r#type: "checkbox",
                        class: "toggle",
                        checked: form_auth_enabled(),
                        onchange: move |e| form_auth_enabled.set(e.checked()),
                    }
                    span { class: "label-text", "Require auth" }
                }
            }
            if form_auth_enabled() {
                div { class: "form-control",
                    label { class: "label", span { class: "label-text", "Policy" } }
                    select {
                        class: "select select-bordered select-sm w-full",
                        value: "{form_auth_policy}",
                        onchange: move |e| form_auth_policy.set(e.value()),
                        option { value: "bypass", "Bypass" }
                        option { value: "authenticated", "Authenticated" }
                        option { value: "groups", "Groups" }
                        option { value: "scopes", "Scopes" }
                    }
                }
                if form_auth_policy() == "groups" {
                    div { class: "form-control",
                        label { class: "label", span { class: "label-text", "Required Groups (comma separated)" } }
                        input {
                            class: "input input-bordered input-sm w-full",
                            value: "{form_required_groups}",
                            oninput: move |e| form_required_groups.set(e.value()),
                        }
                    }
                }
                if form_auth_policy() == "scopes" {
                    div { class: "form-control",
                        label { class: "label", span { class: "label-text", "Required Scopes (comma separated)" } }
                        input {
                            class: "input input-bordered input-sm w-full",
                            value: "{form_required_scopes}",
                            oninput: move |e| form_required_scopes.set(e.value()),
                        }
                    }
                }
            }

            div { class: "flex gap-2 mt-2",
                button { class: "btn btn-primary btn-sm", onclick: submit, "Save" }
                if is_editing {
                    button { class: "btn btn-error btn-sm btn-outline", onclick: delete, "Delete" }
                }
            }
        }
    }
}
