//! Draft design system — Rust side.
//!
//! The CSS in `dist/` is the single source of truth (compiled from `scss/`); this crate adds
//! Dioxus components on top of the `d-*` class contract. Everything Dioxus-related is behind the
//! `dioxus` feature so the CSS tooling, the query grammar and the tests stay lightweight.

pub mod data;
pub mod kinds;
pub mod prefs;
pub mod query;
pub mod util;
pub mod viz;

pub use kinds::{AppKind, StatusKind, Tone};

#[cfg(feature = "dioxus")]
pub mod layout;
#[cfg(feature = "dioxus")]
pub mod shell;
#[cfg(feature = "dioxus")]
pub mod theme;
#[cfg(feature = "dioxus")]
pub mod ui;
#[cfg(feature = "dioxus")]
mod assets;
#[cfg(feature = "dioxus")]
pub use assets::{DraftStyles, DRAFT_CSS};
