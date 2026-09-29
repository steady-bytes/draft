use dioxus::prelude::*;

use crate::kinds::Tone;

/// A definition list (`d-kv`): mono keys in `--dim`, values beside them. Fill it with [`KvItem`]s.
#[component]
pub fn Kv(children: Element) -> Element {
    rsx! {
        dl { class: "d-kv", {children} }
    }
}

/// One `dt` / `dd` pair. `tone` colours the value (an error count, a trace id link colour).
#[component]
pub fn KvItem(label: String, tone: Option<Tone>, label_tone: Option<Tone>, children: Element) -> Element {
    let value_style = tone.map(|t| format!("color:{}", t.css_var())).unwrap_or_default();
    let label_style = label_tone.map(|t| format!("color:{}", t.css_var())).unwrap_or_default();
    rsx! {
        dt { style: "{label_style}", "{label}" }
        dd { style: "{value_style}", {children} }
    }
}
