use dioxus::logger::tracing::Level;
use dioxus::prelude::*;
use draft_ui::kinds::AppKind;
use draft_ui::shell::{use_app_links, AppShell, NavItem, NavSection};
use draft_ui::ui::NotFound;
use draft_ui::DraftStyles;
use once_cell::sync::Lazy;
use web_sys::window;

mod components;
mod data;
mod range;
mod views;

use views::{Metrics, Stream, Traces, WideEvents};

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
        // Unknown paths keep the shell, so the rail is still there to navigate from.
        #[route("/:..route")]
        NotFound { route: Vec<String> },
}

fn get_domain() -> String {
    let window = window().expect("no global `window` exists");
    window.location().origin().expect("failed to get origin")
}

/// Beacon's own Connect-RPC address (logs, traces, metrics, wide events): same-origin by default,
/// which works behind Fuse and through `dx serve`'s `[[web.proxy]]` entries; overridable at build
/// time with `API_DOMAIN` for a non-proxied deployment.
pub static API_DOMAIN: Lazy<String> =
    Lazy::new(|| draft_ui::util::resolve_domain(option_env!("API_DOMAIN"), get_domain));

/// Fuse's own control-plane address, used to discover the other apps for the rail's Apps block.
/// It does not route itself through the proxy it manages, so it is reached directly.
pub static FUSE_DOMAIN: Lazy<String> = Lazy::new(|| {
    draft_ui::util::resolve_domain(option_env!("FUSE_DOMAIN"), || "http://localhost:18000".to_string())
});

/// The Logs / Wide events → Traces hand-off: opening a trace sets this and navigates to Traces,
/// which reads and clears it on mount to select that trace immediately. A global rather than a URL
/// parameter: it is one-shot state between two views, and there is no `?q=` deep-link support yet.
pub static PENDING_TRACE_ID: GlobalSignal<Option<String>> = Signal::global(|| None);

/// The same hand-off for Logs: a BeaconQL filter (`trace_id = "…"`) to run on arrival.
pub static PENDING_LOG_FILTER: GlobalSignal<Option<String>> = Signal::global(|| None);

fn main() {
    dioxus::logger::init(Level::INFO).expect("logger failed to init");

    dioxus::launch(|| {
        use_context_provider(|| dioxus_grpc::GrpcConfig { host: API_DOMAIN.clone() });
        rsx! {
            DraftStyles {}
            // Beacon's own few rules, on top of the shared design system.
            document::Stylesheet { href: asset!("/assets/beacon.css") }
            Router::<Route> {}
        }
    });
}

/// The shell: rail (Signals / Apps), topbar and status bar around the four signal views.
fn dashboard_layout() -> Element {
    let route = use_route::<Route>();
    let apps = use_app_links(FUSE_DOMAIN.clone(), AppKind::Beacon);

    let sections = vec![NavSection::new(
        "Signals",
        vec![
            NavItem::new("Wide events", "/").exact(),
            NavItem::new("Logs", "/logs"),
            NavItem::new("Traces", "/traces"),
            NavItem::new("Metrics", "/metrics"),
        ],
    )];

    rsx! {
        AppShell { app: AppKind::Beacon, current: route.to_string(), sections, apps, Outlet::<Route> {} }
    }
}
