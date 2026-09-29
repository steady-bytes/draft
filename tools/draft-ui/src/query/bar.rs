use dioxus::prelude::*;

use super::grammar::GrammarId;
use crate::ui::{Btn, BtnVariant, Kbd};

/// The query bar (`d-query`): a language label, the input and Run. One component for CESQL,
/// BeaconQL and PromQL; replaces `CesqlBar` and Beacon's `QueryBar`.
///
/// The input is grammar-agnostic (the server's parser is the authority); `grammar` supplies the
/// label and placeholder, and `error` shows the parser's message below the bar.
#[component]
pub fn QueryBar(
    grammar: GrammarId,
    value: Signal<String>,
    on_run: EventHandler<()>,
    on_clear: Option<EventHandler<()>>,
    error: Option<String>,
) -> Element {
    let g = grammar.grammar();
    let invalid = if error.is_some() { "true" } else { "false" };
    rsx! {
        div {
            div { class: "d-query", aria_invalid: "{invalid}",
                span { class: "d-query-lang", "{g.label}" }
                input {
                    r#type: "text",
                    value: "{value}",
                    placeholder: "{g.placeholder}",
                    aria_label: "{g.label} query",
                    spellcheck: "false",
                    oninput: move |e| value.set(e.value()),
                    onkeydown: move |e| {
                        if e.key() == Key::Enter {
                            on_run.call(());
                        }
                    },
                }
                if let Some(clear) = on_clear {
                    Btn {
                        variant: BtnVariant::Ghost,
                        aria_label: "Clear query".to_string(),
                        onclick: move |_| clear.call(()),
                        "✕"
                    }
                }
                Btn { variant: BtnVariant::Primary, onclick: move |_| on_run.call(()),
                    "Run "
                    Kbd { "↵" }
                }
            }
            if let Some(e) = error {
                div { class: "d-error-text", role: "alert", style: "margin-top:6px", "{e}" }
            }
        }
    }
}
