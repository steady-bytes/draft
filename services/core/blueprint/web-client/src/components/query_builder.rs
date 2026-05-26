use dioxus::prelude::*;

#[derive(Clone, PartialEq, Copy)]
pub enum SortDir {
    Asc,
    Desc,
}

#[derive(Clone, PartialEq)]
enum FieldKind {
    Type,
    Source,
    Id,
    Subject,
    Body,
}

#[derive(Clone, PartialEq)]
enum Operator {
    Eq,
    Like,
}

#[derive(Clone, PartialEq)]
enum Connector {
    Or,
    And,
}

fn cesql_name(field: &FieldKind, body_field: &str) -> String {
    match field {
        FieldKind::Type => "type".to_string(),
        FieldKind::Source => "source".to_string(),
        FieldKind::Id => "id".to_string(),
        FieldKind::Subject => "subject".to_string(),
        FieldKind::Body => format!("body.{body_field}"),
    }
}

fn format_fragment(op: &Operator, field_name: &str, value: &str) -> String {
    match op {
        Operator::Eq => format!("{field_name} = '{value}'"),
        Operator::Like => format!("{field_name} LIKE '%{value}%'"),
    }
}

/// Collapsible form that generates a CESQL fragment and appends it to the
/// expression bar. `has_expression` controls whether the AND/OR connector
/// selector is shown. `on_add` is called with the fragment (prefixed with
/// "OR "/"AND " when joining an existing expression). `on_sort_change` is
/// called when the user changes the ORDER BY direction so the parent can
/// update its signal and trigger a re-query explicitly.
#[component]
pub fn QueryBuilder(
    has_expression: bool,
    on_add: EventHandler<String>,
    sort_dir: SortDir,
    on_sort_change: EventHandler<SortDir>,
) -> Element {
    let mut open = use_signal(|| false);
    let mut field = use_signal(|| FieldKind::Type);
    let mut body_field = use_signal(String::new);
    let mut operator = use_signal(|| Operator::Eq);
    let mut value = use_signal(String::new);
    let mut connector = use_signal(|| Connector::Or);

    let field_name = cesql_name(&field(), &body_field());
    let preview = format_fragment(&operator(), &field_name, &value());

    rsx! {
        div { class: "border border-base-300 rounded-lg",

            // ── Collapsible header ──────────────────────────────────────────
            button {
                class: "w-full flex items-center gap-2 px-3 py-2 text-xs text-base-content/50 hover:bg-base-200/50 rounded-lg transition-colors",
                onclick: move |_| open.toggle(),
                span {
                    class: if open() {
                        "transition-transform duration-150 rotate-90 inline-block"
                    } else {
                        "transition-transform duration-150 inline-block"
                    },
                    "▶"
                }
                "Query Builder"
                span { class: "ml-auto text-base-content/30 text-[10px] italic",
                    "build a CESQL expression"
                }
            }

            // ── Expanded body ───────────────────────────────────────────────
            if open() {
                div { class: "px-3 pb-3 flex flex-col gap-2 border-t border-base-300",

                    // Row 1 — field / body-field / operator / value
                    div { class: "pt-2 flex flex-wrap items-end gap-2",

                        div { class: "flex flex-col gap-1",
                            span { class: "text-[10px] text-base-content/40 uppercase tracking-wide", "Field" }
                            select {
                                class: "select select-xs select-bordered",
                                onchange: move |e| {
                                    field.set(match e.value().as_str() {
                                        "source"  => FieldKind::Source,
                                        "id"      => FieldKind::Id,
                                        "subject" => FieldKind::Subject,
                                        "body"    => FieldKind::Body,
                                        _         => FieldKind::Type,
                                    });
                                },
                                option { value: "type",    selected: field() == FieldKind::Type,    "type" }
                                option { value: "source",  selected: field() == FieldKind::Source,  "source" }
                                option { value: "id",      selected: field() == FieldKind::Id,      "id" }
                                option { value: "subject", selected: field() == FieldKind::Subject, "subject" }
                                option { value: "body",    selected: field() == FieldKind::Body,    "body.*" }
                            }
                        }

                        // Body field name — only shown when body.* is selected
                        if field() == FieldKind::Body {
                            div { class: "flex flex-col gap-1",
                                span { class: "text-[10px] text-base-content/40 uppercase tracking-wide", "Body field" }
                                input {
                                    class: "input input-xs input-bordered font-mono w-36",
                                    placeholder: "e.g. modelName",
                                    value: "{body_field}",
                                    oninput: move |e| body_field.set(e.value()),
                                }
                            }
                        }

                        div { class: "flex flex-col gap-1",
                            span { class: "text-[10px] text-base-content/40 uppercase tracking-wide", "Operator" }
                            select {
                                class: "select select-xs select-bordered",
                                onchange: move |e| {
                                    operator.set(if e.value() == "like" { Operator::Like } else { Operator::Eq });
                                },
                                option { value: "eq",   selected: operator() == Operator::Eq,   "= (equals)" }
                                option { value: "like", selected: operator() == Operator::Like, "LIKE (contains)" }
                            }
                        }

                        div { class: "flex flex-col gap-1 flex-1 min-w-28",
                            span { class: "text-[10px] text-base-content/40 uppercase tracking-wide", "Value" }
                            input {
                                class: "input input-xs input-bordered font-mono w-full",
                                placeholder: "value…",
                                value: "{value}",
                                oninput: move |e| value.set(e.value()),
                                onkeydown: move |e| {
                                    if e.key() != Key::Enter { return; }
                                    let v = value();
                                    if v.is_empty() { return; }
                                    let fn_ = cesql_name(&field(), &body_field());
                                    let frag = format_fragment(&operator(), &fn_, &v);
                                    let full = if has_expression {
                                        match connector() {
                                            Connector::Or  => format!("OR {frag}"),
                                            Connector::And => format!("AND {frag}"),
                                        }
                                    } else { frag };
                                    on_add.call(full);
                                    value.set(String::new());
                                },
                            }
                        }
                    }

                    // Row 2 — ORDER BY toggle (query-level, not a CESQL clause)
                    div { class: "flex items-center gap-2 border-t border-base-300/50 pt-2",
                        span { class: "text-[10px] text-base-content/40 uppercase tracking-wide", "Order" }
                        div { class: "join",
                            button {
                                class: if sort_dir == SortDir::Asc {
                                    "btn btn-xs join-item btn-primary"
                                } else {
                                    "btn btn-xs join-item btn-ghost"
                                },
                                onclick: move |_| on_sort_change.call(SortDir::Asc),
                                "↑ Oldest first"
                            }
                            button {
                                class: if sort_dir == SortDir::Desc {
                                    "btn btn-xs join-item btn-primary"
                                } else {
                                    "btn btn-xs join-item btn-ghost"
                                },
                                onclick: move |_| on_sort_change.call(SortDir::Desc),
                                "↓ Newest first"
                            }
                        }
                    }

                    // Row 3 — live preview + connector toggle + add button
                    div { class: "flex items-center gap-2 flex-wrap",

                        // Preview badge — shows exactly what CESQL will be inserted
                        div { class: "font-mono text-xs bg-base-300 px-2 py-1 rounded text-base-content/70 flex-1 min-w-0 truncate",
                            if has_expression {
                                span { class: "text-primary font-semibold mr-1",
                                    match connector() {
                                        Connector::Or  => "OR ",
                                        Connector::And => "AND ",
                                    }
                                }
                            }
                            "{preview}"
                        }

                        // AND / OR toggle — only relevant when joining an existing expression
                        if has_expression {
                            div { class: "join",
                                button {
                                    class: if connector() == Connector::Or {
                                        "btn btn-xs join-item btn-primary"
                                    } else {
                                        "btn btn-xs join-item btn-ghost"
                                    },
                                    onclick: move |_| connector.set(Connector::Or),
                                    "OR"
                                }
                                button {
                                    class: if connector() == Connector::And {
                                        "btn btn-xs join-item btn-primary"
                                    } else {
                                        "btn btn-xs join-item btn-ghost"
                                    },
                                    onclick: move |_| connector.set(Connector::And),
                                    "AND"
                                }
                            }
                        }

                        button {
                            class: "btn btn-xs btn-neutral",
                            disabled: value().is_empty()
                                || (field() == FieldKind::Body && body_field().is_empty()),
                            onclick: move |_| {
                                let v = value();
                                if v.is_empty() { return; }
                                let fn_ = cesql_name(&field(), &body_field());
                                let frag = format_fragment(&operator(), &fn_, &v);
                                let full = if has_expression {
                                    match connector() {
                                        Connector::Or  => format!("OR {frag}"),
                                        Connector::And => format!("AND {frag}"),
                                    }
                                } else { frag };
                                on_add.call(full);
                                value.set(String::new());
                            },
                            "Add to query →"
                        }
                    }
                }
            }
        }
    }
}
