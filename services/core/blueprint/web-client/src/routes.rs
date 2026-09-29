//! Gateway route logic, kept pure: conflict detection, grouping, the protocols a route serves and
//! the edit form's conversion to and from a `Route`.

use draft_api::proto::core_control_plane_networking_v1::{AuthPolicy, Endpoint, MatchType, Route, RouteAuth, RouteMatch};

/// Two routes conflict when they would compile to the same (host, match type, prefix) — mirrors
/// Fuse's own `routeKey` in `control_plane/controller.go`, so the list can flag conflicts without a
/// round trip per row. `MATCH_TYPE_UNSPECIFIED` behaves as `PREFIX`, so an unset route and an
/// explicit prefix route are the same key.
pub fn conflicts_with_any(route: &Route, all: &[Route]) -> bool {
    let key = |r: &Route| {
        let m = r.r#match.clone().unwrap_or_default();
        (m.host, if m.match_type == 0 { MatchType::Prefix as i32 } else { m.match_type }, m.prefix)
    };
    let target = key(route);
    all.iter().any(|other| other.name != route.name && key(other) == target)
}

/// UI routes serve a service's UI on its own host at `/`; everything else is an RPC (prefix) route.
pub fn is_ui_route(route: &Route) -> bool {
    route.r#match.as_ref().is_some_and(|m| !m.host.is_empty() && m.prefix == "/")
}

pub fn match_type_label(match_type: i32) -> &'static str {
    match MatchType::try_from(match_type).unwrap_or(MatchType::Unspecified) {
        MatchType::Exact => "exact",
        MatchType::Unspecified | MatchType::Prefix => "prefix",
    }
}

/// The protocols a route serves. HTTP, gRPC-Web and WebSocket are enabled cluster-wide in Fuse's
/// filter chain; HTTP/2 (and gRPC, which rides on it) follow the route's own flag.
pub fn protocols(route: &Route) -> Vec<&'static str> {
    let mut list = vec!["HTTP"];
    if route.enable_http2 {
        list.extend(["H2", "gRPC"]);
    }
    list.extend(["gRPC-Web", "WS"]);
    list
}

pub fn auth_label(auth: &Option<RouteAuth>) -> &'static str {
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

pub fn requires_auth(route: &Route) -> bool {
    auth_label(&route.auth) != "bypass"
}

/// Every backend of a route as `host:port`: the merged `endpoints` when Fuse has load-balanced
/// several registrations of one name, otherwise the single `endpoint`.
pub fn backends(route: &Route) -> Vec<String> {
    let list: Vec<&Endpoint> = if route.endpoints.len() > 1 || (route.endpoint.is_none() && !route.endpoints.is_empty()) {
        route.endpoints.iter().collect()
    } else {
        route.endpoint.iter().collect()
    };
    list.into_iter().map(|e| format!("{}:{}", e.host, e.port)).collect()
}

/// A route's match as the table shows it: the host for a host route, else the path.
pub fn match_text(route: &Route) -> String {
    match route.r#match.as_ref() {
        Some(m) if !m.host.is_empty() && m.prefix == "/" => m.host.clone(),
        Some(m) if !m.host.is_empty() => format!("{}{}", m.host, m.prefix),
        Some(m) => m.prefix.clone(),
        None => String::new(),
    }
}

/// The edit form's fields, all strings or flags as the inputs hold them.
#[derive(Clone, Debug, Default, PartialEq)]
pub struct RouteDraft {
    pub name: String,
    pub prefix: String,
    pub host: String,
    /// `"prefix"` or `"exact"`.
    pub match_type: String,
    pub ep_host: String,
    pub ep_port: String,
    pub http2: bool,
    pub auth_enabled: bool,
    /// `"bypass"`, `"authenticated"`, `"groups"` or `"scopes"`.
    pub auth_policy: String,
    pub groups: String,
    pub scopes: String,
}

fn split_csv(value: &str) -> Vec<String> {
    value.split(',').map(|s| s.trim().to_string()).filter(|s| !s.is_empty()).collect()
}

impl RouteDraft {
    pub fn new() -> Self {
        Self { match_type: "prefix".into(), auth_policy: "bypass".into(), ..Self::default() }
    }

    pub fn from_route(route: &Route) -> Self {
        let m = route.r#match.clone().unwrap_or_default();
        let ep = route.endpoint.clone().unwrap_or_default();
        let auth = route.auth.clone();
        Self {
            name: route.name.clone(),
            prefix: m.prefix,
            host: m.host,
            match_type: match_type_label(m.match_type).to_string(),
            ep_host: ep.host,
            ep_port: if ep.port == 0 { String::new() } else { ep.port.to_string() },
            http2: route.enable_http2,
            auth_enabled: auth.as_ref().is_some_and(|a| a.enabled),
            auth_policy: auth_label(&auth).to_string(),
            groups: auth.as_ref().map(|a| a.required_groups.join(", ")).unwrap_or_default(),
            scopes: auth.as_ref().map(|a| a.required_scopes.join(", ")).unwrap_or_default(),
        }
    }

    /// What is wrong with the form before it is worth asking Fuse. Fuse's own validation (conflicts)
    /// stays the authority.
    pub fn problem(&self) -> Option<String> {
        if self.name.trim().is_empty() {
            return Some("Name is required".to_string());
        }
        if self.prefix.trim().is_empty() && self.host.trim().is_empty() {
            return Some("Give the route a match: a prefix, a host, or both".to_string());
        }
        if self.ep_host.trim().is_empty() {
            return Some("Endpoint host is required".to_string());
        }
        match self.ep_port.trim().parse::<u32>() {
            Ok(p) if (1..=65_535).contains(&p) => None,
            _ => Some("Endpoint port must be a number from 1 to 65535".to_string()),
        }
    }

    pub fn to_route(&self) -> Route {
        let match_type = if self.match_type == "exact" { MatchType::Exact } else { MatchType::Prefix };
        let policy = match self.auth_policy.as_str() {
            "authenticated" => AuthPolicy::Authenticated,
            "groups" => AuthPolicy::Groups,
            "scopes" => AuthPolicy::Scopes,
            _ => AuthPolicy::Bypass,
        };
        Route {
            name: self.name.trim().to_string(),
            r#match: Some(RouteMatch {
                prefix: self.prefix.trim().to_string(),
                host: self.host.trim().to_string(),
                headers: None,
                grpc_match_options: None,
                dynamic_metadata: None,
                match_type: match_type as i32,
            }),
            endpoint: Some(Endpoint { host: self.ep_host.trim().to_string(), port: self.ep_port.trim().parse().unwrap_or(0) }),
            // `endpoints` is filled by Fuse when it merges same-name registrations for load
            // balancing; a caller registering or editing a route only sets its own `endpoint`.
            endpoints: Vec::new(),
            enable_http2: self.http2,
            auth: self.auth_enabled.then(|| RouteAuth {
                enabled: true,
                policy: policy as i32,
                required_groups: split_csv(&self.groups),
                required_scopes: split_csv(&self.scopes),
            }),
            // Not editable from this UI: preserved as unset rather than given controls here.
            wide_events_disabled: false,
            mtls: None,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn route(name: &str, host: &str, prefix: &str, match_type: MatchType) -> Route {
        Route {
            name: name.into(),
            r#match: Some(RouteMatch { host: host.into(), prefix: prefix.into(), match_type: match_type as i32, ..Default::default() }),
            endpoint: Some(Endpoint { host: "localhost".into(), port: 9000 }),
            ..Default::default()
        }
    }

    #[test]
    fn same_host_prefix_and_type_conflict_under_different_names() {
        let a = route("a", "x.local", "/", MatchType::Prefix);
        let b = route("b", "x.local", "/", MatchType::Unspecified);
        assert!(conflicts_with_any(&a, &[a.clone(), b]));
    }

    #[test]
    fn a_route_does_not_conflict_with_itself_or_a_different_match() {
        let a = route("a", "x.local", "/", MatchType::Prefix);
        assert!(!conflicts_with_any(&a, &[a.clone()]));
        assert!(!conflicts_with_any(&a, &[route("b", "x.local", "/", MatchType::Exact)]));
        assert!(!conflicts_with_any(&a, &[route("b", "y.local", "/", MatchType::Prefix)]));
    }

    #[test]
    fn ui_routes_are_host_routes_at_the_root() {
        assert!(is_ui_route(&route("ui", "beacon.draft.localhost", "/", MatchType::Prefix)));
        assert!(!is_ui_route(&route("rpc", "", "/core.observability.", MatchType::Prefix)));
        assert!(!is_ui_route(&route("catch-all", "", "/", MatchType::Prefix)));
    }

    #[test]
    fn http2_adds_h2_and_grpc() {
        let mut r = route("a", "", "/", MatchType::Prefix);
        assert_eq!(protocols(&r), ["HTTP", "gRPC-Web", "WS"]);
        r.enable_http2 = true;
        assert_eq!(protocols(&r), ["HTTP", "H2", "gRPC", "gRPC-Web", "WS"]);
    }

    #[test]
    fn a_load_balanced_route_lists_every_backend() {
        let mut r = route("a", "", "/", MatchType::Prefix);
        assert_eq!(backends(&r), ["localhost:9000"]);
        r.endpoints = vec![Endpoint { host: "h1".into(), port: 1 }, Endpoint { host: "h2".into(), port: 2 }];
        assert_eq!(backends(&r), ["h1:1", "h2:2"]);
    }

    #[test]
    fn the_match_reads_as_a_host_or_a_path() {
        assert_eq!(match_text(&route("a", "x.local", "/", MatchType::Prefix)), "x.local");
        assert_eq!(match_text(&route("a", "", "/api/", MatchType::Prefix)), "/api/");
        assert_eq!(match_text(&route("a", "x.local", "/api/", MatchType::Prefix)), "x.local/api/");
    }

    #[test]
    fn auth_shows_only_when_enabled() {
        let mut r = route("a", "", "/", MatchType::Prefix);
        assert!(!requires_auth(&r));
        r.auth = Some(RouteAuth { enabled: false, policy: AuthPolicy::Groups as i32, ..Default::default() });
        assert!(!requires_auth(&r), "a disabled policy is bypass whatever it says");
        r.auth = Some(RouteAuth { enabled: true, policy: AuthPolicy::Groups as i32, ..Default::default() });
        assert_eq!(auth_label(&r.auth), "groups");
        assert!(requires_auth(&r));
    }

    #[test]
    fn a_form_round_trips_through_a_route() {
        let mut r = route("svc", "svc.draft.localhost", "/", MatchType::Exact);
        r.enable_http2 = true;
        r.auth = Some(RouteAuth { enabled: true, policy: AuthPolicy::Groups as i32, required_groups: vec!["admins".into(), "ops".into()], required_scopes: vec![] });
        let draft = RouteDraft::from_route(&r);
        assert_eq!(draft.groups, "admins, ops");
        assert_eq!(draft.ep_port, "9000");
        assert_eq!(draft.to_route(), r);
    }

    #[test]
    fn a_new_form_is_a_bypass_prefix_route_and_needs_the_basics() {
        let mut d = RouteDraft::new();
        assert_eq!(d.problem().as_deref(), Some("Name is required"));
        d.name = "x".into();
        assert!(d.problem().unwrap().contains("match"));
        d.prefix = "/x/".into();
        assert!(d.problem().unwrap().contains("host"));
        d.ep_host = "localhost".into();
        assert!(d.problem().unwrap().contains("port"));
        d.ep_port = "70000".into();
        assert!(d.problem().is_some());
        d.ep_port = "9000".into();
        assert_eq!(d.problem(), None);
        assert_eq!(d.to_route().auth, None);
    }
}
