use dioxus::prelude::*;

use super::cx;
use crate::kinds::StatusKind;

/// A banner (`d-alert`): errors, warnings, confirmations, information.
#[component]
pub fn Alert(#[props(default)] kind: StatusKind, #[props(default)] mono: bool, children: Element) -> Element {
    let tone = match kind {
        StatusKind::Err => "d-alert--err",
        StatusKind::Warn => "d-alert--warn",
        StatusKind::Ok => "d-alert--ok",
        StatusKind::Info | StatusKind::Idle => "",
    };
    let class = cx(&["d-alert", tone, if mono { "d-alert--mono" } else { "" }]);
    rsx! {
        div { class: "{class}", role: "alert", {children} }
    }
}
