use dioxus::prelude::*;
use draft_api::proto::core_control_plane_networking_v1::{AuthPolicy, RouteAuth};

#[component]
pub fn AuthBadge(auth: Option<RouteAuth>) -> Element {
    let enabled_policy = auth.as_ref().filter(|a| a.enabled).map(|a| a.policy);

    let (label, color) = match enabled_policy {
        None => ("bypass".to_string(), "badge-ghost"),
        Some(policy) => match AuthPolicy::try_from(policy).unwrap_or(AuthPolicy::Bypass) {
            AuthPolicy::Bypass => ("bypass".to_string(), "badge-ghost"),
            AuthPolicy::Authenticated => ("authenticated".to_string(), "badge-info"),
            AuthPolicy::Groups => ("groups".to_string(), "badge-warning"),
            AuthPolicy::Scopes => ("scopes".to_string(), "badge-accent"),
        },
    };

    rsx! {
        span { class: "badge badge-xs {color}", "{label}" }
    }
}
