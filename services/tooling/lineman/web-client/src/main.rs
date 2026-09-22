use dioxus::logger::tracing::Level;
use dioxus::prelude::*;
use once_cell::sync::Lazy;
use web_sys::window;

mod components;
mod views;

use views::{
    CreateObjective, Dashboard, Loops, ObjectiveDetail, PageNotFound, Scheduler, TaskBoard,
    TaskDetail,
};

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
        #[route("/objectives/:id/board")]
        TaskBoard { id: String },
        #[route("/objectives/:id/tasks/:task_id")]
        TaskDetail { id: String, task_id: String },
        #[route("/scheduler")]
        Scheduler {},
        #[route("/loops")]
        Loops {},
    #[end_layout]

    #[route("/:..route")]
    PageNotFound {
        route: Vec<String>,
    },
}

fn get_domain() -> String {
    let window = window().expect("no global `window` exists");
    window.location().origin().expect("failed to get origin")
}

/// Same "same-origin as whatever page is currently loaded" resolution
/// Blueprint's own web client uses (see service_url/API_DOMAIN there) --
/// Lineman is served on its own subdomain through Fuse
/// (lineman.draft.localhost), so its own RPC prefix is reachable at that
/// same origin.
pub static API_DOMAIN: Lazy<String> = Lazy::new(|| {
    if let Some(api_domain) = option_env!("API_DOMAIN") {
        if !api_domain.is_empty() {
            return api_domain.to_string();
        }
    }
    get_domain()
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

/// Static sidebar -- three sections (Overview / Objectives / Automation),
/// matching assets/lineman/design-brief.html's Navigation & IA exactly.
/// Unlike Blueprint's own dashboard_layout, this doesn't drive the sidebar
/// from a KV-stored NavigationConfig -- Lineman's IA is small and fixed
/// enough that the extra indirection isn't worth it for a first pass.
fn dashboard_layout() -> Element {
    rsx! {
        div { class: "drawer lg:drawer-open",
            input { id: "lineman-drawer", r#type: "checkbox", class: "drawer-toggle" }
            div { class: "drawer-content flex flex-col",
                div { class: "navbar bg-base-300 lg:hidden",
                    label { r#for: "lineman-drawer", class: "btn btn-square btn-ghost", "☰" }
                    span { class: "text-lg font-bold ml-2", "Lineman" }
                }
                main { class: "flex-1 p-4", Outlet::<Route> {} }
            }
            div { class: "drawer-side",
                label { r#for: "lineman-drawer", class: "drawer-overlay" }
                ul { class: "menu bg-base-200 w-80 min-h-full p-4",
                    li { class: "mb-4",
                        Link { to: Route::Dashboard {}, class: "text-xl font-bold", "Lineman" }
                    }
                    li { class: "menu-title", "Overview" }
                    li { Link { to: Route::Dashboard {}, "Dashboard" } }
                    li { class: "menu-title mt-2", "Objectives" }
                    li { Link { to: Route::Dashboard {}, "All Objectives" } }
                    li { Link { to: Route::CreateObjective {}, "+ Create Objective" } }
                    li { class: "menu-title mt-2", "Automation" }
                    li { Link { to: Route::Scheduler {}, "Scheduler" } }
                    li { Link { to: Route::Loops {}, "Loops" } }
                }
            }
        }
    }
}
