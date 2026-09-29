use dioxus::prelude::*;

/// One row of a [`Menu`].
#[derive(Clone, Debug, PartialEq)]
pub struct MenuItem {
    /// Passed to `on_select`.
    pub id: String,
    pub label: String,
    pub danger: bool,
    /// Shows a check mark (single-choice menus such as "Move to").
    pub checked: bool,
    /// A separator is drawn *above* this item.
    pub separator: bool,
}

impl MenuItem {
    pub fn new(id: impl Into<String>, label: impl Into<String>) -> Self {
        Self { id: id.into(), label: label.into(), danger: false, checked: false, separator: false }
    }

    pub fn danger(mut self) -> Self {
        self.danger = true;
        self
    }

    pub fn checked(mut self, checked: bool) -> Self {
        self.checked = checked;
        self
    }

    pub fn separated(mut self) -> Self {
        self.separator = true;
        self
    }
}

/// A menu (`d-menu`): a right-click context menu at (`x`, `y`), or anchored under its parent when
/// no position is given. Closes on Escape, on selection, or via `on_close` (wire a backdrop).
#[component]
pub fn Menu(
    items: Vec<MenuItem>,
    on_select: EventHandler<String>,
    on_close: EventHandler<()>,
    heading: Option<String>,
    x: Option<f64>,
    y: Option<f64>,
) -> Element {
    let (class, style) = match (x, y) {
        (Some(x), Some(y)) => ("d-menu", format!("left:{x}px; top:{y}px")),
        _ => ("d-menu d-menu--anchored", String::new()),
    };
    rsx! {
        // A transparent layer under the menu: a click anywhere else closes it.
        div { class: "d-menu-backdrop", onclick: move |e| {
            e.stop_propagation();
            on_close.call(());
        } }
        div {
            class: "{class}",
            style: "{style}",
            role: "menu",
            onkeydown: move |e| {
                if e.key() == Key::Escape {
                    on_close.call(());
                }
            },
            if let Some(h) = heading {
                div { class: "d-menu-label", "{h}" }
            }
            for item in items {
                {
                    let id = item.id.clone();
                    let class = if item.danger { "d-menu-item d-menu-item--danger" } else { "d-menu-item" };
                    let checked = if item.checked { "true" } else { "false" };
                    rsx! {
                        Fragment { key: "{item.id}",
                            if item.separator {
                                div { class: "d-menu-sep", role: "separator" }
                            }
                            button {
                                class: "{class}",
                                r#type: "button",
                                role: "menuitemradio",
                                aria_checked: "{checked}",
                                onclick: move |_| {
                                    on_select.call(id.clone());
                                    on_close.call(());
                                },
                                "{item.label}"
                            }
                        }
                    }
                }
            }
        }
    }
}
