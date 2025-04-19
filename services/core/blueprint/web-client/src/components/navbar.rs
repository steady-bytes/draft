use dioxus::prelude::*;

use crate::Route;


static BLUEPRINT_NAME: GlobalSignal<String> = Signal::global(|| "{blueprint}".to_string());

pub fn NavbarIcon() -> Element {
    rsx! {
        a { class: "btn btn-ghost text-xl",
            Link { to: Route::Home {}, "{BLUEPRINT_NAME}"},
        }
    }
}

pub fn NavbarMenuButton() -> Element {
    rsx! {
        label {
            class: "btn btn-square btn-ghost drawer-button",
            aria_label: "open sidebar",
            for: "my-drawer",
            svg {
                class: "inline-block h-5 w-5 stroke-current",
                fill: "none",
                view_box: "0 0 24 24",
                xmlns: "http://www.w3.org/2000/svg",
                path {
                    d: "M4 6h16M4 12h16M4 18h16",
                    stroke_linecap: "round",
                    stroke_linejoin: "round",
                    stroke_width: "2",
                }
            }
        }
    }
}

pub fn NavbarSecondaryMenuButton() -> Element {
    rsx! {
        button { class: "btn btn-square btn-ghost",
            svg {
                class: "inline-block h-5 w-5 stroke-current",
                fill: "none",
                view_box: "0 0 24 24",
                xmlns: "http://www.w3.org/2000/svg",
                path {
                    d: "M5 12h.01M12 12h.01M19 12h.01M6 12a1 1 0 11-2 0 1 1 0 012 0zm7 0a1 1 0 11-2 0 1 1 0 012 0zm7 0a1 1 0 11-2 0 1 1 0 012 0z",
                    stroke_linecap: "round",
                    stroke_linejoin: "round",
                    stroke_width: "2",
                }
            }
        }
    }
}