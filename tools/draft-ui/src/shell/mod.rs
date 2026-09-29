//! The app shell: brand, topbar, rail, status bar, responsive rail toggle.

mod app_shell;
mod chrome;
mod discovery;
mod nav;

pub use app_shell::AppShell;
pub use discovery::{app_links, host_label};
#[cfg(feature = "discovery")]
pub use discovery::use_app_links;
pub use chrome::{use_page_chrome, BarItem, Chrome, ChromeStatus};
pub use nav::{NavItem, NavSection};
