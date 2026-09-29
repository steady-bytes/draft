use dioxus::prelude::*;

use super::{highlight, CodeLang};
use crate::ui::cx;

/// A highlighted code block (`d-code`) for JSON, YAML and HTTP. One tokenizer serves the KV
/// drawer, the CloudEvent drawer, workflow snippets and step request/response.
#[component]
pub fn CodeBlock(
    text: String,
    #[props(default)] lang: CodeLang,
    #[props(default)] wrap: bool,
    #[props(default)] bare: bool,
    id: Option<String>,
) -> Element {
    let class = cx(&["d-code", if wrap { "d-code--wrap" } else { "" }, if bare { "d-code--bare" } else { "" }]);
    let spans = highlight(lang, &text);
    rsx! {
        pre { class: "{class}", id: id,
            for (i , s) in spans.into_iter().enumerate() {
                if let Some(c) = s.class {
                    span { key: "{i}", class: "{c}", "{s.text}" }
                } else {
                    "{s.text}"
                }
            }
        }
    }
}
