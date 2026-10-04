use dioxus::logger::tracing::Level;
use dioxus::prelude::*;
use draft_ui::kinds::AppKind;
use draft_ui::shell::{use_app_links, AppShell, NavItem, NavSection};
use draft_ui::ui::NotFound;
use draft_ui::DraftStyles;
use once_cell::sync::Lazy;
use web_sys::window;

mod api;
mod components;
mod views;

use views::{Library, Record, Transcripts};

#[derive(Debug, Clone, Routable, PartialEq)]
#[rustfmt::skip]
enum Route {
    #[layout(relay_layout)]
        #[route("/")]
        Library {},
        #[route("/record")]
        Record {},
        #[route("/transcripts")]
        Transcripts {},
        // Unknown paths keep the shell, so the rail is still there to navigate from.
        #[route("/:..route")]
        NotFound { route: Vec<String> },
}

fn get_domain() -> String {
    let window = window().expect("no global `window` exists");
    window.location().origin().expect("failed to get origin")
}

/// Same "same-origin as whatever page is currently loaded" resolution every other Draft web
/// client uses -- Relay is served on its own subdomain through Fuse (relay.draft.localhost, once
/// that route exists) or directly on its own port in local dev, either way reachable at whatever
/// origin actually loaded this page.
pub static API_DOMAIN: Lazy<String> =
    Lazy::new(|| draft_ui::util::resolve_domain(option_env!("API_DOMAIN"), get_domain));

/// Fuse's own control-plane address, used to discover the other apps for the rail's Apps block.
pub static FUSE_DOMAIN: Lazy<String> = Lazy::new(|| {
    draft_ui::util::resolve_domain(option_env!("FUSE_DOMAIN"), || "http://localhost:18000".to_string())
});

fn main() {
    dioxus::logger::init(Level::INFO).expect("logger failed to init");

    dioxus::launch(|| {
        use_context_provider(|| dioxus_grpc::GrpcConfig {
            host: API_DOMAIN.clone(),
        });
        rsx! {
            DraftStyles {}
            document::Stylesheet { href: asset!("/assets/relay.css") }
            Router::<Route> {}
        }
    });
}

/// The shell: rail (Listen / Capture / Apps), topbar and status bar -- matching
/// mockups/pages/relay-{library,record,transcripts}.html's own rail structure, minus its Devices
/// section: the mockups link those to nothing real (`href="#"`) and this client doesn't build a
/// devices page in this pass either, so it's left out rather than pointing "Inputs"/"Outputs" at
/// somewhere that doesn't exist. `ListDevices` (Phase 9) is real and reachable via the API, just
/// not surfaced in this shell yet.
fn relay_layout() -> Element {
    let route = use_route::<Route>();
    let apps = use_app_links(FUSE_DOMAIN.clone(), AppKind::Relay);

    let sections = vec![
        NavSection::new("Listen", vec![NavItem::new("Library", "/").exact()]),
        NavSection::new(
            "Capture",
            vec![
                NavItem::new("Record", "/record"),
                NavItem::new("Transcripts", "/transcripts"),
            ],
        ),
    ];

    rsx! {
        AppShell { app: AppKind::Relay, current: route.to_string(), sections, apps,
            Outlet::<Route> {}
        }
    }
}
