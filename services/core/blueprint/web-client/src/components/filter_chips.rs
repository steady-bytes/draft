use dioxus::prelude::*;

const PRESETS: &[(&str, &str)] = &[
    ("type = order.created",   "type = 'order.created'"),
    ("source LIKE %shop%",     "source LIKE '%shop%'"),
    ("failed OR cancelled",    "type = 'failed' OR type = 'cancelled'"),
    ("EXISTS correlationid",   "EXISTS correlationid"),
    ("no correlation",         "NOT EXISTS correlationid"),
    ("paid or succeeded",      "type = 'paid' OR type = 'succeeded'"),
    ("inventory events",       "source LIKE '%inventory%'"),
    ("late events (>2s)",      "subject LIKE '%timeout%'"),
];

#[component]
pub fn FilterChips(on_select: EventHandler<String>) -> Element {
    rsx! {
        div { class: "flex flex-wrap gap-2",
            for (label, expr) in PRESETS.iter() {
                {
                    let expr_owned = expr.to_string();
                    rsx! {
                        button {
                            class: "badge badge-outline badge-sm cursor-pointer hover:badge-primary transition-colors",
                            onclick: move |_| on_select.call(expr_owned.clone()),
                            "{label}"
                        }
                    }
                }
            }
        }
    }
}
