//! Gateway: the routes Fuse pushes to Envoy. Host routes serve a service's UI on its own
//! subdomain; prefix routes send RPC paths to the service that owns them. `/gateway/:name` is this
//! page with that route's drawer open and `/gateway/new` is it with the Add dialog open.
//!
//! Fuse's control-plane API is not same-origin (each service has its own subdomain), so it is
//! reached at `FUSE_DOMAIN`, directly.

use std::collections::HashSet;

use dioxus::prelude::*;
use draft_api::proto::core_control_plane_networking_v1::{
    networking_service_client::NetworkingServiceClient, AddRouteRequest, DeleteRouteRequest, ListRoutesRequest, Route,
    ValidateRouteRequest,
};
use draft_ui::data::{Kv, KvItem};
use draft_ui::layout::{Drawer, DrawerBlock, PageHead, Split, Toolbar};
use draft_ui::shell::{use_page_chrome, BarItem, Chrome, ChromeStatus};
use draft_ui::ui::{use_toast, Alert, Btn, BtnVariant, Chip, Empty, Field, Loading, Modal, Select, Status, Tag, TextInput, Toast, Toggle};
use draft_ui::{StatusKind, Tone};
use tonic_web_wasm_client::Client as WasmClient;

use crate::routes::{auth_label, backends, conflicts_with_any, is_ui_route, match_text, match_type_label, protocols, requires_auth, RouteDraft};
use crate::Route as AppRoute;

fn client() -> NetworkingServiceClient<WasmClient> {
    NetworkingServiceClient::new(WasmClient::new(crate::FUSE_DOMAIN.clone()))
}

/// `/gateway`.
#[component]
pub fn Gateway() -> Element {
    rsx! { GatewayPage { selected: None, adding: false } }
}

/// `/gateway/new` — the list with the Add dialog open.
#[component]
pub fn NewRoute() -> Element {
    rsx! { GatewayPage { selected: None, adding: true } }
}

/// `/gateway/:name` — the list with one route's drawer open.
#[component]
pub fn RouteDetail(name: String) -> Element {
    rsx! { GatewayPage { selected: Some(name), adding: false } }
}

fn protocol_tone(p: &str) -> Tone {
    match p {
        "HTTP" => Tone::Ca,
        "H2" => Tone::Sv,
        "gRPC" | "gRPC-Web" => Tone::Bp,
        _ => Tone::Fs,
    }
}

fn auth_tone(label: &str) -> Tone {
    match label {
        "authenticated" => Tone::Ca,
        "groups" => Tone::Warn,
        "scopes" => Tone::Sv,
        _ => Tone::Quiet,
    }
}

/// The filter chips above the table. A route must satisfy every pressed chip.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
enum Facet {
    Host,
    Prefix,
    Grpc,
    Ws,
    Auth,
}

impl Facet {
    const ALL: [Facet; 5] = [Facet::Host, Facet::Prefix, Facet::Grpc, Facet::Ws, Facet::Auth];

    fn label(self) -> &'static str {
        match self {
            Facet::Host => "Host",
            Facet::Prefix => "Prefix",
            Facet::Grpc => "gRPC",
            Facet::Ws => "WS",
            Facet::Auth => "Auth required",
        }
    }

    fn accepts(self, r: &Route) -> bool {
        match self {
            Facet::Host => is_ui_route(r),
            Facet::Prefix => !is_ui_route(r),
            Facet::Grpc => protocols(r).contains(&"gRPC"),
            Facet::Ws => protocols(r).contains(&"WS"),
            Facet::Auth => requires_auth(r),
        }
    }
}

#[component]
fn GatewayPage(selected: Option<String>, adding: bool) -> Element {
    let navigator = use_navigator();
    let toast = use_toast();
    let mut filter = use_signal(String::new);
    let mut facets: Signal<HashSet<Facet>> = use_signal(HashSet::new);
    let mut show_form = use_signal(move || adding);
    let mut editing: Signal<Option<Route>> = use_signal(|| None);
    let mut confirm_delete: Signal<Option<String>> = use_signal(|| None);

    let mut list_result = use_resource(|| async { client().list_routes(ListRoutesRequest {}).await.map(|r| r.into_inner().routes) });

    let routes: Vec<Route> = match &*list_result.read() {
        Some(Ok(list)) => list.clone(),
        _ => Vec::new(),
    };
    let loading = list_result.read().is_none();
    let load_error = match &*list_result.read() {
        Some(Err(e)) => Some(e.message().to_string()),
        _ => None,
    };

    use_page_chrome(move || {
        let n = match &*list_result.read() {
            Some(Ok(list)) => list.len(),
            _ => 0,
        };
        Chrome {
            crumbs: vec!["Blueprint".into(), "Control plane".into(), "Gateway".into()],
            status: Some(ChromeStatus::live(format!("Fuse · {n} routes"))),
            left: vec![BarItem::kv("Routes", n.to_string())],
            right: vec![],
        }
    });

    let delete = use_callback(move |name: String| {
        spawn(async move {
            match client().delete_route(DeleteRouteRequest { name: name.clone() }).await {
                Ok(_) => {
                    confirm_delete.set(None);
                    list_result.restart();
                    toast.show(format!("Deleted {name}"), 3000);
                    navigator.push(AppRoute::Gateway {});
                }
                Err(e) => {
                    confirm_delete.set(None);
                    toast.show(format!("Could not delete: {}", e.message()), 6000);
                }
            }
        });
    });

    // Derived ---------------------------------------------------------------------------------
    let needle = filter().to_lowercase();
    let active = facets();
    let matches = |r: &Route| {
        let text = format!("{} {} {}", r.name, match_text(r), backends(r).join(" ")).to_lowercase();
        (needle.is_empty() || text.contains(&needle)) && active.iter().all(|f| f.accepts(r))
    };
    let ui: Vec<Route> = routes.iter().filter(|r| is_ui_route(r) && matches(r)).cloned().collect();
    let rpc: Vec<Route> = routes.iter().filter(|r| !is_ui_route(r) && matches(r)).cloned().collect();
    let conflicts = routes.iter().filter(|r| conflicts_with_any(r, &routes)).count();
    let summary = if conflicts == 0 {
        format!("{} routes · all valid", routes.len())
    } else {
        format!("{} routes · {} valid · {} in conflict", routes.len(), routes.len() - conflicts, conflicts)
    };

    let drawer = selected.as_ref().map(|name| match routes.iter().find(|r| &r.name == name) {
        Some(route) => {
            let for_edit = route.clone();
            let for_delete = route.name.clone();
            let conflict = conflicts_with_any(route, &routes);
            rsx! {
                RouteDrawer {
                    key: "{route.name}",
                    route: route.clone(),
                    conflict,
                    on_close: move |_| { navigator.push(AppRoute::Gateway {}); },
                    on_edit: move |_| {
                        editing.set(Some(for_edit.clone()));
                        show_form.set(true);
                    },
                    on_delete: move |_| confirm_delete.set(Some(for_delete.clone())),
                }
            }
        }
        None => rsx! {
            Drawer { label: "Route detail".to_string(), on_close: move |_| { navigator.push(AppRoute::Gateway {}); }, title: name.clone(),
                if loading {
                    Loading {}
                } else {
                    Alert { kind: StatusKind::Warn, "No route with this name. It may have just been deleted." }
                }
            }
        },
    });

    rsx! {
        Split { flush: true, drawer,
            PageHead {
                title: "Gateway".to_string(),
                eyebrow: "Control plane · Fuse".to_string(),
                description: "Routes Fuse pushes to Envoy. Host routes serve a service's UI on its own subdomain; prefix routes send RPC paths to the service that owns them.".to_string(),
                actions: rsx! {
                    Btn {
                        variant: BtnVariant::Primary,
                        onclick: move |_| {
                            editing.set(None);
                            show_form.set(true);
                        },
                        "+ Add route"
                    }
                },
            }

            Toolbar {
                TextInput { value: filter(), oninput: move |v| filter.set(v), placeholder: "Filter routes, hosts, targets…".to_string(), aria_label: "Filter routes".to_string() }
                for f in Facet::ALL {
                    Chip {
                        key: "{f.label()}",
                        pressed: active.contains(&f),
                        onclick: move |_| {
                            let mut set = facets.write();
                            if !set.remove(&f) {
                                set.insert(f);
                            }
                        },
                        "{f.label()}"
                    }
                }
                span { class: "d-spacer" }
                span { class: "d-label", "{summary}" }
            }

            if let Some(err) = load_error {
                Alert { kind: StatusKind::Err, "Could not load routes from Fuse: {err}" }
            } else if loading {
                Loading {}
            } else if ui.is_empty() && rpc.is_empty() {
                Empty { title: "No routes".to_string(),
                    if routes.is_empty() { "No routes are registered with Fuse." } else { "No route matches the filters." }
                }
            } else {
                div { class: "d-panel d-table-wrap",
                    table { class: "d-table gw-table",
                        thead {
                            tr {
                                th { "Route" }
                                th { "Match" }
                                th { "Protocols" }
                                th { "Target" }
                                th { "Auth" }
                                th { "Status" }
                            }
                        }
                        tbody {
                            if !ui.is_empty() {
                                tr { class: "group", td { colspan: "6", "UI · host routes · {ui.len()}" } }
                                for r in ui {
                                    RouteRow { key: "{r.name}", route: r.clone(), all: routes.clone(), selected: selected.clone() }
                                }
                            }
                            if !rpc.is_empty() {
                                tr { class: "group", td { colspan: "6", "RPC · prefix routes · {rpc.len()}" } }
                                for r in rpc {
                                    RouteRow { key: "{r.name}", route: r.clone(), all: routes.clone(), selected: selected.clone() }
                                }
                            }
                        }
                    }
                }
            }
        }

        if show_form() {
            RouteFormModal {
                key: "{editing().map(|r| r.name).unwrap_or_default()}",
                existing: editing(),
                on_close: move |_| {
                    show_form.set(false);
                    editing.set(None);
                    if adding {
                        navigator.push(AppRoute::Gateway {});
                    }
                },
                on_saved: move |name: String| {
                    show_form.set(false);
                    editing.set(None);
                    list_result.restart();
                    toast.show(format!("Saved {name}"), 3000);
                    navigator.push(AppRoute::RouteDetail { name });
                },
            }
        }

        Modal {
            open: confirm_delete().is_some(),
            title: "Delete route?".to_string(),
            on_close: move |_| confirm_delete.set(None),
            footer: rsx! {
                Btn { variant: BtnVariant::Ghost, onclick: move |_| confirm_delete.set(None), "Cancel" }
                Btn {
                    variant: BtnVariant::Danger,
                    onclick: move |_| {
                        if let Some(name) = confirm_delete.peek().clone() {
                            delete.call(name);
                        }
                    },
                    "Delete"
                }
            },
            if let Some(name) = confirm_delete() {
                p { style: "margin:0", "Fuse stops routing " b { "{name}" } ". A service that registered it will add it again the next time it starts." }
            }
        }

        Toast { state: toast }
    }
}

#[component]
fn RouteRow(route: Route, all: Vec<Route>, selected: Option<String>) -> Element {
    let navigator = use_navigator();
    let name = route.name.clone();
    let name_for_key = route.name.clone();
    let is_selected = selected.as_deref() == Some(route.name.as_str());
    let conflict = conflicts_with_any(&route, &all);
    let kind = if is_ui_route(&route) { "host" } else { match_type_label(route.r#match.as_ref().map(|m| m.match_type).unwrap_or(0)) };
    let text = match_text(&route);
    let targets = backends(&route);
    let target = match targets.len() {
        0 => "—".to_string(),
        1 => targets[0].clone(),
        n => format!("{n} backends"),
    };
    let auth = auth_label(&route.auth);
    rsx! {
        tr {
            aria_selected: "{is_selected}",
            tabindex: "0",
            onclick: move |_| { navigator.push(AppRoute::RouteDetail { name: name.clone() }); },
            onkeydown: move |k| {
                if k.key() == Key::Enter {
                    navigator.push(AppRoute::RouteDetail { name: name_for_key.clone() });
                }
            },
            td { "{route.name}" }
            td { class: "match", span { class: "kind", "{kind}" } "{text}" }
            td { class: "protos",
                for p in protocols(&route) {
                    Tag { key: "{p}", tone: protocol_tone(p), "{p}" }
                }
            }
            td { class: "target", "{target}" }
            td { Tag { tone: auth_tone(auth), "{auth}" } }
            td {
                if conflict {
                    Status { kind: StatusKind::Err, "Conflict" }
                } else {
                    Status { "Valid" }
                }
            }
        }
    }
}

/// The drawer: the request path from a client through Fuse to the backends, then match and
/// upstream details.
#[component]
fn RouteDrawer(route: Route, conflict: bool, on_close: EventHandler<()>, on_edit: EventHandler<()>, on_delete: EventHandler<()>) -> Element {
    let targets = backends(&route);
    let host = route.r#match.as_ref().map(|m| m.host.clone()).unwrap_or_default();
    let prefix = route.r#match.as_ref().map(|m| m.prefix.clone()).unwrap_or_default();
    let mt = route.r#match.as_ref().map(|m| m.match_type).unwrap_or(0);
    let path = format!("{prefix} ({})", match_type_label(mt));
    let protos = protocols(&route).join(", ");
    let label = auth_label(&route.auth);
    let auth_text = match label {
        "bypass" => "Bypass (no authentication)".to_string(),
        other => match route.auth.as_ref() {
            Some(a) if !a.required_groups.is_empty() => format!("{other}: {}", a.required_groups.join(", ")),
            Some(a) if !a.required_scopes.is_empty() => format!("{other}: {}", a.required_scopes.join(", ")),
            _ => other.to_string(),
        },
    };
    let policy = if targets.len() > 1 { "Round robin" } else { "Direct" };
    let kind_label = if is_ui_route(&route) { "Route · host" } else { "Route · prefix" };
    let match_kind = if host.is_empty() { "PATH MATCH" } else { "HOST MATCH" };
    let lead = rsx! {
        span { class: "d-label", "{kind_label}" }
    };
    rsx! {
        Drawer { label: "Route detail".to_string(), on_close: move |_| on_close.call(()), lead, title: route.name.clone(),
            if conflict {
                Status { kind: StatusKind::Err, "Conflicts with another route" }
            } else {
                Status { "Valid" }
            }
            RouteFlow { backends: targets.clone(), match_kind: match_kind.to_string(), policy: policy.to_string() }
            DrawerBlock { title: "Match".to_string(),
                Kv {
                    if !host.is_empty() {
                        KvItem { label: "host".to_string(), "{host}" }
                    }
                    KvItem { label: "path".to_string(), "{path}" }
                    KvItem { label: "protocols".to_string(), "{protos}" }
                }
            }
            DrawerBlock { title: "Upstream".to_string(),
                Kv {
                    KvItem { label: "backends".to_string(), "{targets.len()}" }
                    KvItem { label: "policy".to_string(), "{policy}" }
                    KvItem { label: "auth".to_string(), "{auth_text}" }
                }
            }
            if route.endpoints.len() > 1 {
                Alert { kind: StatusKind::Info,
                    "Load-balanced across {route.endpoints.len()} backends. Editing here replaces them with a single endpoint; the other registered processes add themselves back the next time they start."
                }
            }
            div { class: "gw-actions",
                Btn { variant: BtnVariant::Primary, onclick: move |_| on_edit.call(()), "Edit route" }
                span { class: "d-spacer" }
                Btn { variant: BtnVariant::Danger, onclick: move |_| on_delete.call(()), "Delete" }
            }
        }
    }
}

/// The request path drawn as a diagram: client → Fuse → each backend.
#[component]
fn RouteFlow(backends: Vec<String>, match_kind: String, policy: String) -> Element {
    /// Backends drawn before the rest collapse into "+N more".
    const MAX_DRAWN: usize = 6;
    let drawn: Vec<&String> = backends.iter().take(MAX_DRAWN).collect();
    let extra = backends.len().saturating_sub(MAX_DRAWN);
    let rows = drawn.len().max(1) + usize::from(extra > 0);
    let step = 36.0;
    let height = (rows as f64 * step).max(76.0);
    let mid = height / 2.0;
    let first = 16.0;
    let last = first + (rows as f64 - 1.0) * step;
    let label = format!("Request path: a client to Fuse to {} backend{}", backends.len(), if backends.len() == 1 { "" } else { "s" });
    rsx! {
        div { class: "flow",
            svg { view_box: "0 0 380 {height}", role: "img", "aria-label": "{label}",
                rect { class: "box", x: "0", y: "{mid - 20.0}", width: "92", height: "40", rx: "2" }
                text { x: "10", y: "{mid - 2.0}", "client" }
                text { class: "sub", x: "10", y: "{mid + 11.0}", "REQUEST" }
                rect { class: "box fs", x: "128", y: "{mid - 26.0}", width: "100", height: "52", rx: "2" }
                text { x: "138", y: "{mid - 6.0}", "FUSE" }
                text { class: "sub", x: "138", y: "{mid + 8.0}", "{match_kind}" }
                text { class: "sub", x: "138", y: "{mid + 19.0}", "{policy.to_uppercase()}" }
                path { class: "wire", d: "M92 {mid} H128" }
                circle { class: "via", cx: "92", cy: "{mid}", r: "2.5" }
                circle { class: "via", cx: "128", cy: "{mid}", r: "2.5" }
                path { class: "wire", d: "M228 {mid} H254 M254 {first} V{last}" }
                circle { class: "via", cx: "254", cy: "{mid}", r: "2.5" }
                for (i , b) in drawn.iter().enumerate() {
                    {
                        let y = first + i as f64 * step;
                        rsx! {
                            g { key: "{i}",
                                path { class: "wire", d: "M254 {y} H280" }
                                rect { class: "box up", x: "280", y: "{y - 12.0}", width: "100", height: "24", rx: "2" }
                                text { x: "288", y: "{y + 4.0}", "{b}" }
                            }
                        }
                    }
                }
                if extra > 0 {
                    {
                        let y = first + drawn.len() as f64 * step;
                        rsx! {
                            text { class: "sub", x: "288", y: "{y + 4.0}", "+{extra} more" }
                        }
                    }
                }
            }
        }
    }
}

/// Add / edit dialog. Saving validates with Fuse first (conflicts), then replaces the route: the
/// old name is deleted before the new one is added even when the name is unchanged. Fuse keys each
/// registration by name *and* endpoint (so several processes can register one name and be
/// load-balanced); re-adding without deleting would orphan the old (name, old endpoint) entry as a
/// permanent extra backend.
#[component]
fn RouteFormModal(existing: Option<Route>, on_close: EventHandler<()>, on_saved: EventHandler<String>) -> Element {
    let original = existing.as_ref().map(|r| r.name.clone());
    let mut draft = use_signal(|| existing.as_ref().map(RouteDraft::from_route).unwrap_or_else(RouteDraft::new));
    let mut error: Signal<Option<String>> = use_signal(|| None);
    let mut conflicts: Signal<Vec<String>> = use_signal(Vec::new);
    let mut saving = use_signal(|| false);
    let editing = original.is_some();

    let save = use_callback(move |_: ()| {
        let form = draft();
        if let Some(problem) = form.problem() {
            error.set(Some(problem));
            return;
        }
        let route = form.to_route();
        let original = original.clone();
        saving.set(true);
        error.set(None);
        spawn(async move {
            let mut c = client();
            // `existing_name` excludes the route's pre-edit name from the conflict check: a rename
            // would otherwise always conflict with its own not-yet-deleted prior name.
            match c.validate_route(ValidateRouteRequest { route: Some(route.clone()), existing_name: original.clone().unwrap_or_default() }).await {
                Ok(resp) => {
                    let resp = resp.into_inner();
                    if !resp.valid {
                        conflicts.set(resp.conflicting_routes);
                        saving.set(false);
                        return;
                    }
                }
                Err(e) => {
                    error.set(Some(format!("Validation failed: {}", e.message())));
                    saving.set(false);
                    return;
                }
            }
            conflicts.set(Vec::new());
            if let Some(old) = &original {
                if let Err(e) = c.delete_route(DeleteRouteRequest { name: old.clone() }).await {
                    error.set(Some(format!("Could not remove the old route: {}", e.message())));
                    saving.set(false);
                    return;
                }
            }
            let name = route.name.clone();
            match c.add_route(AddRouteRequest { route: Some(route) }).await {
                Ok(_) => on_saved.call(name),
                Err(e) => {
                    error.set(Some(e.message().to_string()));
                    saving.set(false);
                }
            }
        });
    });

    let d = draft();
    let match_options = vec![("prefix".to_string(), "Prefix".to_string()), ("exact".to_string(), "Exact".to_string())];
    let policy_options = vec![
        ("bypass".to_string(), "Bypass".to_string()),
        ("authenticated".to_string(), "Authenticated".to_string()),
        ("groups".to_string(), "Groups".to_string()),
        ("scopes".to_string(), "Scopes".to_string()),
    ];

    rsx! {
        Modal {
            open: true,
            wide: true,
            title: if editing { "Edit route".to_string() } else { "Add route".to_string() },
            on_close: move |_| on_close.call(()),
            footer: rsx! {
                Btn { variant: BtnVariant::Ghost, onclick: move |_| on_close.call(()), "Cancel" }
                Btn { variant: BtnVariant::Primary, disabled: saving(), onclick: move |_| save.call(()), "Save" }
            },
            if let Some(msg) = error() {
                Alert { kind: StatusKind::Err, "{msg}" }
            }
            if !conflicts().is_empty() {
                Alert { kind: StatusKind::Err, "Conflicts with existing route(s): {conflicts().join(\", \")}" }
            }
            Field { label: "Name".to_string(),
                TextInput { value: d.name.clone(), oninput: move |v| draft.write().name = v, placeholder: "e.g. my-service-ui".to_string() }
            }
            div { class: "form-row",
                Field { label: "Match type".to_string(),
                    Select { value: d.match_type.clone(), options: match_options, on_change: move |v| draft.write().match_type = v }
                }
                Field { label: "Match prefix".to_string(),
                    TextInput { value: d.prefix.clone(), oninput: move |v| draft.write().prefix = v, placeholder: "/".to_string() }
                }
            }
            Field { label: "Match host".to_string(), hint: "Optional, e.g. *.draft.localhost".to_string(),
                TextInput { value: d.host.clone(), oninput: move |v| draft.write().host = v }
            }
            div { class: "form-row",
                Field { label: "Endpoint host".to_string(),
                    TextInput { value: d.ep_host.clone(), oninput: move |v| draft.write().ep_host = v, placeholder: "localhost".to_string() }
                }
                Field { label: "Endpoint port".to_string(),
                    TextInput { r#type: "number".to_string(), value: d.ep_port.clone(), oninput: move |v| draft.write().ep_port = v }
                }
            }
            div { style: "display:flex; gap:24px; flex-wrap:wrap",
                Toggle { checked: d.http2, on_change: move |on| draft.write().http2 = on, "HTTP/2" }
                Toggle { checked: d.auth_enabled, on_change: move |on| draft.write().auth_enabled = on, "Require auth" }
            }
            if d.auth_enabled {
                Field { label: "Policy".to_string(),
                    Select { value: d.auth_policy.clone(), options: policy_options, on_change: move |v| draft.write().auth_policy = v }
                }
                if d.auth_policy == "groups" {
                    Field { label: "Required groups".to_string(), hint: "Comma separated".to_string(),
                        TextInput { value: d.groups.clone(), oninput: move |v| draft.write().groups = v }
                    }
                }
                if d.auth_policy == "scopes" {
                    Field { label: "Required scopes".to_string(), hint: "Comma separated".to_string(),
                        TextInput { value: d.scopes.clone(), oninput: move |v| draft.write().scopes = v }
                    }
                }
            }
        }
    }
}
