use dioxus::prelude::*;

/// Protocol pills for a route. gRPC-Web and WebSockets are enabled cluster-wide in Fuse's filter
/// chain (not per-route), so they're always shown; HTTP/2 (and gRPC, which rides on it) reflect
/// the route's own `enable_http2` flag.
#[component]
pub fn ProtocolBadges(enable_http2: bool) -> Element {
    rsx! {
        span { class: "flex flex-wrap gap-1",
            span { class: "badge badge-xs badge-info", "HTTP" }
            if enable_http2 {
                span { class: "badge badge-xs badge-accent", "HTTP2" }
                span { class: "badge badge-xs badge-accent", "gRPC" }
            }
            span { class: "badge badge-xs badge-secondary", "gRPC-Web" }
            span { class: "badge badge-xs badge-warning", "WS" }
        }
    }
}
