use dioxus::prelude::*;

/// QueryGrammar is the set of filter grammars QueryBar can be parameterized
/// over. It only affects placeholder/help text today — the input itself is
/// grammar-agnostic (a plain text field + run/clear buttons); the grammar's
/// server-side parser (BeaconQL for logs today, a PromQL subset for metrics
/// later — see the design doc's Query Language section) is what actually
/// interprets the text.
///
/// This is Blueprint's `CesqlBar` generalized per Beacon's Phase 3 plan: same
/// shape, no longer hardcoded to CESQL's placeholder/example text.
#[derive(Clone, Copy, PartialEq, Default)]
pub enum QueryGrammar {
    #[default]
    BeaconQl,
    PromQl,
}

impl QueryGrammar {
    fn placeholder(&self) -> &'static str {
        match self {
            QueryGrammar::BeaconQl => {
                r#"e.g.  severity = "error" AND service_name = "beacon"  or  attributes["route"] LIKE "/api/%""#
            }
            QueryGrammar::PromQl => {
                r#"e.g.  rate(http_requests_total{route="/api"}[5m])  or  sum by (route) (my_gauge)"#
            }
        }
    }
}

#[component]
pub fn QueryBar(
    #[props(default)] grammar: QueryGrammar,
    expression: Signal<String>,
    on_run: EventHandler<()>,
    on_clear: EventHandler<()>,
) -> Element {
    let placeholder = grammar.placeholder();

    rsx! {
        div { class: "join w-full",
            input {
                class: "input input-bordered input-sm join-item flex-1 font-mono bg-base-200",
                placeholder: "{placeholder}",
                value: "{expression}",
                oninput: move |e| expression.set(e.value()),
                onkeydown: move |e| {
                    if e.key() == Key::Enter {
                        on_run.call(());
                    }
                },
            }
            button {
                class: "btn btn-sm btn-ghost join-item text-base-content/50 hover:text-base-content",
                onclick: move |_| on_clear.call(()),
                "✕"
            }
            button {
                class: "btn btn-sm btn-neutral join-item",
                onclick: move |_| on_run.call(()),
                "run"
            }
        }
    }
}
