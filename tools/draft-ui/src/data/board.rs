use dioxus::prelude::*;

use crate::kinds::Tone;
use crate::ui::{cx, Count};

/// The Lineman board (`d-board`). States are free-form per objective, so columns flow
/// horizontally and scroll.
#[component]
pub fn Board(children: Element) -> Element {
    rsx! {
        div { class: "d-board", {children} }
    }
}

/// One column: a coloured head with its count, then cards. It is also the drop target for
/// drag-and-drop; `drop_active` highlights the head while a card is over it.
#[component]
pub fn BoardColumn(
    title: String,
    #[props(default)] tone: Tone,
    count: u32,
    #[props(default)] drop_active: bool,
    on_drag_over: Option<EventHandler<()>>,
    on_drag_leave: Option<EventHandler<()>>,
    on_drop: Option<EventHandler<()>>,
    children: Element,
) -> Element {
    let head = cx(&["d-col-head", if drop_active { "is-drop" } else { "" }]);
    let style = format!("--c:{}", tone.css_var());
    rsx! {
        section {
            ondragover: move |e| {
                // Allow dropping here.
                e.prevent_default();
                if let Some(h) = &on_drag_over {
                    h.call(());
                }
            },
            ondragleave: move |_| {
                if let Some(h) = &on_drag_leave {
                    h.call(());
                }
            },
            ondrop: move |e| {
                e.prevent_default();
                if let Some(h) = &on_drop {
                    h.call(());
                }
            },
            div { class: "{head}", style: "{style}",
                span { class: "d-label", style: "color:var(--ink)", "{title}" }
                Count { "{count}" }
            }
            {children}
        }
    }
}

/// A board card (`d-panel d-card`). `bracket` marks the card in flight with corner brackets in
/// that colour; `locked` cards cannot be dragged (a pending question); `group` is the dashed
/// loop card.
///
/// Drag and drop: set `draggable` and `drag_id`. On drag start the id is written to the drag's
/// `text/plain` data (Firefox will not start a drag without it) and `move` is the allowed effect.
/// A card is also a drop target for reordering: hovering shows a marker above it
/// (`drop_before`), and `on_drop` fires when something is dropped on it.
#[component]
pub fn Card(
    #[props(default)] draggable: bool,
    #[props(default)] dragging: bool,
    #[props(default)] locked: bool,
    #[props(default)] group: bool,
    #[props(default)] dim: bool,
    /// Marks the position a dragged card would land in: above this card.
    #[props(default)]
    drop_before: bool,
    bracket: Option<Tone>,
    /// Written to the drag's `text/plain` data.
    drag_id: Option<String>,
    on_drag_start: Option<EventHandler<()>>,
    on_drag_end: Option<EventHandler<()>>,
    on_drag_over: Option<EventHandler<()>>,
    on_drop: Option<EventHandler<()>>,
    onclick: Option<EventHandler<MouseEvent>>,
    children: Element,
) -> Element {
    let class = cx(&[
        "d-panel d-card",
        if bracket.is_some() { "d-brackets" } else { "" },
        if group { "d-card--group" } else { "" },
        if dragging { "is-dragging" } else { "" },
        if locked { "is-locked" } else { "" },
        if drop_before { "is-drop-before" } else { "" },
    ]);
    let style = match (bracket, dim) {
        (Some(t), _) => format!("--bk:{}", t.css_var()),
        (None, true) => "opacity:.8".to_string(),
        _ => String::new(),
    };
    let can_drag = draggable && !locked;
    rsx! {
        article {
            class: "{class}",
            style: "{style}",
            draggable: "{can_drag}",
            ondragstart: move |e| {
                let dt = e.data().data_transfer();
                if let Some(id) = &drag_id {
                    let _ = dt.set_data("text/plain", id);
                }
                dt.set_effect_allowed("move");
                if let Some(h) = &on_drag_start {
                    h.call(());
                }
            },
            ondragend: move |_| {
                if let Some(h) = &on_drag_end {
                    h.call(());
                }
            },
            ondragover: move |e| {
                if on_drop.is_some() {
                    // Mark this card as a valid drop target.
                    e.prevent_default();
                    e.stop_propagation();
                    if let Some(h) = &on_drag_over {
                        h.call(());
                    }
                }
            },
            ondrop: move |e| {
                if let Some(h) = &on_drop {
                    e.prevent_default();
                    // The column must not also treat this as a drop at its end.
                    e.stop_propagation();
                    h.call(());
                }
            },
            onclick: move |e| {
                if let Some(h) = &onclick {
                    h.call(e);
                }
            },
            {children}
        }
    }
}

/// The small mono row on a card (priority, id, assignee).
#[component]
pub fn CardMeta(children: Element) -> Element {
    rsx! {
        div { class: "d-card-meta", {children} }
    }
}

#[component]
pub fn CardTitle(children: Element) -> Element {
    rsx! {
        div { class: "d-card-title", {children} }
    }
}

/// Initials in a circle; `agent` gives it the square violet agent style.
#[component]
pub fn Avatar(text: String, #[props(default)] agent: bool, title: Option<String>) -> Element {
    let class = if agent { "d-avatar d-avatar--agent" } else { "d-avatar" };
    rsx! {
        span { class: "{class}", title: title, "{text}" }
    }
}
