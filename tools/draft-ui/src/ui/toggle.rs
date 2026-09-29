use dioxus::prelude::*;

/// A switch (`d-toggle`): Stream, Snap, Flow, Live, Errors only. A real `<button role="switch">`,
/// so it is keyboard reachable.
#[component]
pub fn Toggle(
    checked: bool,
    on_change: EventHandler<bool>,
    #[props(default)] disabled: bool,
    children: Element,
) -> Element {
    let aria = if checked { "true" } else { "false" };
    rsx! {
        button {
            class: "d-toggle",
            r#type: "button",
            role: "switch",
            aria_checked: "{aria}",
            disabled,
            onclick: move |_| on_change.call(!checked),
            i {}
            {children}
        }
    }
}
