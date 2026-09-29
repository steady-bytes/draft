use dioxus::logger::tracing::Level;
use dioxus::prelude::*;
use draft_ui::kinds::AppKind;
use draft_ui::shell::{use_app_links, AppShell, NavItem, NavSection};
use draft_ui::ui::NotFound;
use draft_ui::DraftStyles;
use once_cell::sync::Lazy;
use web_sys::window;

mod components;
mod rail;
mod state;
mod views;

use views::{CreateObjective, Dashboard, Loops, ObjectiveDetail, Scheduler, TaskBoard, TaskDetail};

#[derive(Debug, Clone, Routable, PartialEq)]
#[rustfmt::skip]
enum Route {
    #[layout(dashboard_layout)]
        #[route("/")]
        Dashboard {},
        #[route("/objectives/new")]
        CreateObjective {},
        #[route("/objectives/:id")]
        ObjectiveDetail { id: String },
        // The board is the objective view in Board mode; the URL is kept as a deep link.
        #[route("/objectives/:id/board")]
        TaskBoard { id: String },
        #[route("/objectives/:id/tasks/:task_id")]
        TaskDetail { id: String, task_id: String },
        #[route("/scheduler")]
        Scheduler {},
        #[route("/loops")]
        Loops {},
        // Unknown paths keep the shell, so the rail is still there to navigate from.
        #[route("/:..route")]
        NotFound { route: Vec<String> },
}

fn get_domain() -> String {
    let window = window().expect("no global `window` exists");
    window.location().origin().expect("failed to get origin")
}

/// Same "same-origin as whatever page is currently loaded" resolution Blueprint's own web client
/// uses -- Lineman is served on its own subdomain through Fuse (lineman.draft.localhost), so its
/// own RPC prefix is reachable at that same origin.
pub static API_DOMAIN: Lazy<String> =
    Lazy::new(|| draft_ui::util::resolve_domain(option_env!("API_DOMAIN"), get_domain));

/// Fuse's own control-plane address, used to discover the other apps for the rail's Apps block.
/// It does not route itself through the proxy it manages, so it is reached directly.
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
            // Lineman's own few rules (the objective view, forms), on top of the shared design system.
            document::Stylesheet { href: asset!("/assets/lineman.css") }
            Router::<Route> {}
        }
    });
}

/// The shell: rail (Overview / Objectives / Automation / Apps), topbar and status bar. The
/// objectives and their counts come from the shared data the layout loads once.
fn dashboard_layout() -> Element {
    let route = use_route::<Route>();
    let rail = rail::use_rail_provider();
    let apps = use_app_links(FUSE_DOMAIN.clone(), AppKind::Lineman);

    let snapshot = rail.get().unwrap_or_default();
    let mut objective_items: Vec<NavItem> = snapshot
        .objectives
        .iter()
        .map(|o| NavItem::new(o.name.clone(), format!("/objectives/{}", o.id)).count(o.total))
        .collect();
    objective_items.push(NavItem::new("+ New objective", "/objectives/new").action().exact());

    let sections = vec![
        NavSection::new("Overview", vec![NavItem::new("Dashboard", "/").exact()]),
        NavSection::new("Objectives", objective_items),
        NavSection::new(
            "Automation",
            vec![
                NavItem::new("Scheduler", "/scheduler").count(snapshot.scheduled_pending),
                NavItem::new("Loops", "/loops").count(snapshot.loops_active),
            ],
        ),
    ];

    rsx! {
        AppShell { app: AppKind::Lineman, current: route.to_string(), sections, apps,
            Outlet::<Route> {}
        }
    }
}
