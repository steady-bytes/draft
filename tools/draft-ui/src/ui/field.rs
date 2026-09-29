use dioxus::prelude::*;

/// A label, a control and an optional hint or error, stacked (`d-field`).
#[component]
pub fn Field(label: String, hint: Option<String>, error: Option<String>, children: Element) -> Element {
    rsx! {
        label { class: "d-field",
            span { class: "d-label", "{label}" }
            {children}
            if let Some(e) = error {
                span { class: "d-error-text", role: "alert", "{e}" }
            } else if let Some(h) = hint {
                span { class: "d-hint", "{h}" }
            }
        }
    }
}

/// A single-line text field (`d-input`).
#[component]
pub fn TextInput(
    value: String,
    oninput: EventHandler<String>,
    placeholder: Option<String>,
    #[props(default)] invalid: bool,
    #[props(default)] disabled: bool,
    #[props(default = "text".to_string())] r#type: String,
    aria_label: Option<String>,
    onkeydown: Option<EventHandler<KeyboardEvent>>,
) -> Element {
    let aria_invalid = if invalid { "true" } else { "false" };
    rsx! {
        input {
            class: "d-input",
            r#type: "{r#type}",
            value: "{value}",
            placeholder: placeholder,
            aria_label: aria_label,
            aria_invalid: "{aria_invalid}",
            disabled,
            oninput: move |e| oninput.call(e.value()),
            onkeydown: move |e| {
                if let Some(h) = &onkeydown {
                    h.call(e);
                }
            },
        }
    }
}

/// A `<select>` (`d-select`); `options` pairs a value with its label.
#[component]
pub fn Select(
    value: String,
    on_change: EventHandler<String>,
    options: Vec<(String, String)>,
    aria_label: Option<String>,
    #[props(default)] disabled: bool,
) -> Element {
    rsx! {
        select {
            class: "d-select",
            aria_label: aria_label,
            disabled,
            onchange: move |e| on_change.call(e.value()),
            for (v , text) in options {
                option { key: "{v}", value: "{v}", selected: v == value, "{text}" }
            }
        }
    }
}

/// A multi-line text field (`d-textarea`).
#[component]
pub fn Textarea(
    value: String,
    oninput: EventHandler<String>,
    placeholder: Option<String>,
    #[props(default)] disabled: bool,
    aria_label: Option<String>,
) -> Element {
    rsx! {
        textarea {
            class: "d-textarea",
            value: "{value}",
            placeholder: placeholder,
            aria_label: aria_label,
            disabled,
            oninput: move |e| oninput.call(e.value()),
        }
    }
}

/// A checkbox (`d-check`) with its label text.
#[component]
pub fn Check(checked: bool, on_change: EventHandler<bool>, children: Element) -> Element {
    rsx! {
        label { class: "d-row", style: "gap:8px",
            input {
                class: "d-check",
                r#type: "checkbox",
                checked,
                onchange: move |e| on_change.call(e.checked()),
            }
            {children}
        }
    }
}
