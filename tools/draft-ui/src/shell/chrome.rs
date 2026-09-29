use dioxus::prelude::*;

use crate::kinds::StatusKind;

/// The status pill in the topbar (`Raft · 5 nodes · leader node_1`).
#[derive(Clone, Debug, PartialEq)]
pub struct ChromeStatus {
    pub kind: StatusKind,
    pub live: bool,
    pub text: String,
}

impl ChromeStatus {
    pub fn live(text: impl Into<String>) -> Self {
        Self { kind: StatusKind::Ok, live: true, text: text.into() }
    }

    pub fn new(kind: StatusKind, text: impl Into<String>) -> Self {
        Self { kind, live: false, text: text.into() }
    }
}

/// One item of the status bar.
#[derive(Clone, Debug, PartialEq)]
pub enum BarItem {
    /// `Node node_1` — a label and a value.
    Kv(String, String),
    /// `⌘ S save` — a key hint.
    Hint(String, String),
    Text(String),
}

impl BarItem {
    pub fn kv(label: impl Into<String>, value: impl Into<String>) -> Self {
        BarItem::Kv(label.into(), value.into())
    }

    pub fn hint(key: impl Into<String>, text: impl Into<String>) -> Self {
        BarItem::Hint(key.into(), text.into())
    }
}

/// What a page contributes to the shell: its breadcrumb, status pill and status bar.
#[derive(Clone, Debug, Default, PartialEq)]
pub struct Chrome {
    pub crumbs: Vec<String>,
    pub status: Option<ChromeStatus>,
    /// Status bar, left-aligned.
    pub left: Vec<BarItem>,
    /// Status bar, right-aligned.
    pub right: Vec<BarItem>,
}

/// The slot `AppShell` provides and reads.
#[derive(Clone, Copy, PartialEq)]
pub(crate) struct ChromeSlot(pub Signal<Chrome>);

/// Sets the page's crumbs, status pill and status bar. Call it from a view; the closure is
/// re-run whenever a signal it reads changes, so live values (`"3 agents online"`) stay current.
///
/// ```ignore
/// use_page_chrome(move || Chrome {
///     crumbs: vec!["Blueprint".into(), "Control plane".into(), "Key · Value".into()],
///     status: Some(ChromeStatus::live(format!("{} entries", entries.read().len()))),
///     ..Chrome::default()
/// });
/// ```
pub fn use_page_chrome(mut build: impl FnMut() -> Chrome + 'static) {
    let mut slot = use_context::<ChromeSlot>().0;
    use_effect(move || {
        let next = build();
        if *slot.peek() != next {
            slot.set(next);
        }
    });
}
