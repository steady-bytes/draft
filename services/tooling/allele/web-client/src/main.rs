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

use views::{Change, Linkage, Repos, Worktrees};

#[derive(Debug, Clone, Routable, PartialEq)]
#[rustfmt::skip]
enum Route {
    #[layout(allele_layout)]
        #[route("/")]
        Repos {},
        #[route("/changes/:worktree_id")]
        Change { worktree_id: String },
        #[route("/worktrees")]
        Worktrees {},
        #[route("/linkage")]
        Linkage {},
        // Unknown paths keep the shell, so the rail is still there to navigate from.
        #[route("/:..route")]
        NotFound { route: Vec<String> },
}

fn get_domain() -> String {
    let window = window().expect("no global `window` exists");
    window.location().origin().expect("failed to get origin")
}

/// Same "same-origin as whatever page is currently loaded" resolution every other Draft web
/// client uses -- Allele is served on its own subdomain through Fuse (allele.draft.localhost) or
/// directly on its own port in local dev, either way reachable at whatever origin actually loaded
/// this page.
pub static API_DOMAIN: Lazy<String> =
    Lazy::new(|| draft_ui::util::resolve_domain(option_env!("API_DOMAIN"), get_domain));

/// Fuse's own control-plane address, used to discover the other apps for the rail's Apps block.
pub static FUSE_DOMAIN: Lazy<String> = Lazy::new(|| {
    draft_ui::util::resolve_domain(option_env!("FUSE_DOMAIN"), || "http://localhost:18000".to_string())
});

/// The repository this UI operates on. Deliberately a single hardcoded choice, not a
/// repository-switcher: Allele's own backend serves many repositories, but every mockup page
/// (allele-worktrees.html's breadcrumb, allele-change.html's, allele-linkage.html's) shows exactly
/// one repository's own context at a time with no visible switcher anywhere in them either. A real
/// multi-repository picker is a reasonable future enhancement, not something invented here beyond
/// what the mockups themselves show. Repos (the landing page) still lists every repository Allele
/// hosts; selecting one is not wired to this constant in this pass.
pub const CURRENT_REPOSITORY: &str = "steady-bytes/draft";

fn main() {
    dioxus::logger::init(Level::INFO).expect("logger failed to init");

    dioxus::launch(|| {
        use_context_provider(|| dioxus_grpc::GrpcConfig {
            host: API_DOMAIN.clone(),
        });
        rsx! {
            DraftStyles {}
            document::Stylesheet { href: asset!("/assets/allele.css") }
            Router::<Route> {}
        }
    });
}

/// The shell: rail (Code / Composition), topbar and status bar -- matching
/// mockups/pages/allele-{repos,change,worktrees,linkage}.html's own rail structure, minus their
/// shared Evolution section: every one of those links to allele-evolution.html, and no evolution
/// plugin exists yet to serve real data for it -- the same "don't build ahead of need" discipline
/// Relay's own shell already applied to its Devices section (see that file's own comment).
fn allele_layout() -> Element {
    let route = use_route::<Route>();
    let apps = use_app_links(FUSE_DOMAIN.clone(), AppKind::Allele);

    let sections = vec![
        NavSection::new(
            "Code",
            vec![
                NavItem::new("Repositories", "/").exact(),
                NavItem::new("Worktrees", "/worktrees"),
            ],
        ),
        NavSection::new("Composition", vec![NavItem::new("Linkage manifest", "/linkage")]),
    ];

    rsx! {
        AppShell { app: AppKind::Allele, current: route.to_string(), sections, apps,
            Outlet::<Route> {}
        }
    }
}
