use dioxus::prelude::*;

#[component]
pub fn ValidationDot(valid: bool) -> Element {
    let (color, label) = if valid {
        ("bg-success", "valid")
    } else {
        ("bg-error", "conflict")
    };
    rsx! {
        span { class: "inline-flex items-center gap-1.5 text-xs",
            span { class: "w-1.5 h-1.5 rounded-full {color}" }
            span { "{label}" }
        }
    }
}
