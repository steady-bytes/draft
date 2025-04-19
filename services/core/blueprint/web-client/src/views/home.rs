use dioxus::prelude::*;

use crate::components::Hero;

#[component]
pub fn Home() -> Element {
    rsx! {
        Hero {}
    }
}
