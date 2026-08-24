use dioxus::prelude::*;
use dioxus::logger::tracing::{info, Level};
use once_cell::sync::Lazy;
use web_sys::window;

mod components;
mod views;

use views::{Metrics, PageNotFound, Stream, Traces};

#[derive(Debug, Clone, Routable, PartialEq)]
#[rustfmt::skip]
enum Route {
    #[layout(dashboard_layout)]
        #[route("/")]
        Stream {},
        #[route("/traces")]
        Traces {},
        #[route("/metrics")]
        Metrics {},
    #[end_layout]

    #[route("/:..route")]
    PageNotFound {
        route: Vec<String>,
    },
}

fn get_domain() -> String {
    let window = window().expect("no global `window` exists");
    let location = window.location();
    location.origin().expect("failed to get origin")
}

/// API_DOMAIN resolves Beacon's own Connect-RPC address (QueryLogs/StreamLogs)
/// the same way Blueprint's web client resolves its own backend — same-origin
/// by default (works through `dx serve`'s `[[web.proxy]]` entries), overridable
/// at build time via the API_DOMAIN env var for a non-proxied deployment.
pub static API_DOMAIN: Lazy<String> = Lazy::new(|| {
    if let Some(api_domain) = option_env!("API_DOMAIN") {
        info!("API_DOMAIN: {}", api_domain);
        if api_domain.is_empty() {
            return get_domain();
        }
        api_domain.to_string()
    } else {
        get_domain()
    }
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
    rsx! {
        div { class: "flex flex-col h-screen bg-base-100",
            div { class: "navbar bg-base-300 shadow-sm w-full shrink-0",
                div { class: "flex-1 px-2 flex items-center",
                    span { class: "text-lg font-bold tracking-tight", "BEACON" }
                    span { class: "ml-2 text-xs text-base-content/40", "observability" }
                    div { class: "ml-6 flex gap-1",
                        Link {
                            to: Route::Stream {},
                            class: "btn btn-sm btn-ghost",
                            "Stream"
                        }
                        Link {
                            to: Route::Traces {},
                            class: "btn btn-sm btn-ghost",
                            "Traces"
                        }
                        Link {
                            to: Route::Metrics {},
                            class: "btn btn-sm btn-ghost",
                            "Metrics"
                        }
                    }
                }
            }
            div { class: "flex-1 min-h-0",
                Outlet::<Route> {}
            }
        }
    }
}
