use dioxus::prelude::*;
use dioxus::logger::tracing::{info, Level};
use once_cell::sync::Lazy;
use web_sys::window;

mod components;
mod views;

use views::{Metrics, PageNotFound, Stream, Traces, WideEvents};

#[derive(Debug, Clone, Routable, PartialEq)]
#[rustfmt::skip]
enum Route {
    #[layout(dashboard_layout)]
        #[route("/")]
        WideEvents {},
        #[route("/logs")]
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

/// PENDING_TRACE_ID is the Logs → Traces hand-off channel (Phase 14): clicking
/// a log row's Trace pill sets this before navigating to the Traces view,
/// which reads and clears it on mount to pre-select that trace's flame graph
/// immediately rather than just filtering the search list. A GlobalSignal
/// rather than a route query param — this codebase has no existing precedent
/// for Dioxus Router query segments, and a global is simpler for exactly this
/// kind of one-shot cross-view state (mirrors the pattern API_DOMAIN already
/// uses above, via Blueprint's `BLUEPRINT_NAME: GlobalSignal<String>`).
pub static PENDING_TRACE_ID: GlobalSignal<Option<String>> = Signal::global(|| None);

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
                    span { class: "text-lg font-bold tracking-tight", "{{beacon}}" }
                }
                div { class: "px-2 flex gap-1",
                    Link {
                        to: Route::WideEvents {},
                        class: "btn btn-sm btn-ghost",
                        "Events"
                    }
                    Link {
                        to: Route::Stream {},
                        class: "btn btn-sm btn-ghost",
                        "Logs"
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
            div { class: "flex-1 min-h-0",
                Outlet::<Route> {}
            }
        }
    }
}
