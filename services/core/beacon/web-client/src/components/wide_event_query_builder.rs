use dioxus::prelude::*;

use crate::components::query_bar::beaconql_string;

// Same collapsible-form shape and interaction as components/query_builder.rs
// (field / operator / value row, live preview, AND/OR connector, "Add to
// query"), but built against wide_events' own schema (query/beaconql.go's
// CompileWideEvent / columnForWideEvent) rather than logs' — the field list
// and its three separate attribute maps are specific to WideEvent, not a
// generic port. See query_builder.rs's own doc comment for why this
// codebase keeps one QueryBuilder per schema rather than a parameterized one.

#[derive(Clone, Copy, PartialEq)]
enum FieldKind {
    ServiceName,
    SpanName,
    StatusCode,
    TraceId,
    SpanId,
    ParentSpanId,
    DurationNs,
    Attributes,
    BusinessAttributes,
    RuntimeAttributes,
}

impl FieldKind {
    // Whether this field takes a map key (renders the extra "Key" input) and
    // whether its value is a bare numeric literal rather than a quoted
    // string — duration_ns is wide_events' one numeric field
    // (query/beaconql.go's columnForWideEvent); everything else is compared
    // as text.
    fn needs_key(self) -> bool {
        matches!(
            self,
            FieldKind::Attributes | FieldKind::BusinessAttributes | FieldKind::RuntimeAttributes
        )
    }

    fn is_numeric(self) -> bool {
        matches!(self, FieldKind::DurationNs)
    }

    // The BeaconQL field reference this selection compiles to — matches
    // columnForWideEvent's own accepted names exactly.
    fn field_ref(self, key: &str) -> String {
        match self {
            FieldKind::ServiceName => "service_name".to_string(),
            FieldKind::SpanName => "span_name".to_string(),
            FieldKind::StatusCode => "status_code".to_string(),
            FieldKind::TraceId => "trace_id".to_string(),
            FieldKind::SpanId => "span_id".to_string(),
            FieldKind::ParentSpanId => "parent_span_id".to_string(),
            FieldKind::DurationNs => "duration_ns".to_string(),
            FieldKind::Attributes => format!("attributes[{}]", beaconql_string(key)),
            FieldKind::BusinessAttributes => {
                format!("business_attributes[{}]", beaconql_string(key))
            }
            FieldKind::RuntimeAttributes => {
                format!("runtime_attributes[{}]", beaconql_string(key))
            }
        }
    }
}

#[derive(Clone, Copy, PartialEq)]
enum Operator {
    Eq,
    Neq,
    Lt,
    Lte,
    Gt,
    Gte,
    Like,
    NotLike,
    In,
    NotIn,
}

impl Operator {
    fn is_list(self) -> bool {
        matches!(self, Operator::In | Operator::NotIn)
    }
}

#[derive(Clone, Copy, PartialEq)]
enum Connector {
    Or,
    And,
}

// Renders value as a BeaconQL literal: a bare number for a numeric field
// (query/beaconql.go's lexer accepts unquoted numeric literals only), a
// quoted-and-escaped string otherwise.
fn literal(value: &str, numeric: bool) -> String {
    if numeric {
        value.trim().to_string()
    } else {
        beaconql_string(value)
    }
}

// Splits a comma-separated Value input into a BeaconQL `(v1, v2, …)` list for
// IN/NOT IN — the one place this UI's single Value input represents more
// than one literal.
fn literal_list(value: &str, numeric: bool) -> String {
    value
        .split(',')
        .map(str::trim)
        .filter(|v| !v.is_empty())
        .map(|v| literal(v, numeric))
        .collect::<Vec<_>>()
        .join(", ")
}

fn format_fragment(field: FieldKind, key: &str, op: Operator, value: &str) -> String {
    let field_ref = field.field_ref(key);
    let numeric = field.is_numeric();
    match op {
        Operator::Eq => format!("{field_ref} = {}", literal(value, numeric)),
        Operator::Neq => format!("{field_ref} != {}", literal(value, numeric)),
        Operator::Lt => format!("{field_ref} < {}", literal(value, numeric)),
        Operator::Lte => format!("{field_ref} <= {}", literal(value, numeric)),
        Operator::Gt => format!("{field_ref} > {}", literal(value, numeric)),
        Operator::Gte => format!("{field_ref} >= {}", literal(value, numeric)),
        Operator::Like => format!(
            "{field_ref} LIKE {}",
            beaconql_string(&format!("%{value}%"))
        ),
        Operator::NotLike => {
            format!(
                "{field_ref} NOT LIKE {}",
                beaconql_string(&format!("%{value}%"))
            )
        }
        Operator::In => format!("{field_ref} IN ({})", literal_list(value, numeric)),
        Operator::NotIn => format!("{field_ref} NOT IN ({})", literal_list(value, numeric)),
    }
}

// Combines a new fragment into the existing expression — identical
// precedence-safe rule to query_builder.rs's combine (see that file's own
// comment for the full BeaconQL AND/OR precedence rationale).
fn combine(current: &str, connector: Connector, fragment: &str) -> String {
    let current = current.trim();
    if current.is_empty() {
        return fragment.to_string();
    }
    match connector {
        Connector::Or => format!("{current} OR {fragment}"),
        Connector::And => format!("({current}) AND {fragment}"),
    }
}

/// Collapsible form that builds a BeaconQL fragment (against wide_events'
/// schema) and combines it into `expression` — same interaction and
/// precedence-safe combine rule as QueryBuilder (logs), see that
/// component's doc comment.
#[component]
pub fn WideEventQueryBuilder(expression: Signal<String>, on_add: EventHandler<String>) -> Element {
    let mut open = use_signal(|| false);
    let mut field = use_signal(|| FieldKind::ServiceName);
    let mut key = use_signal(String::new);
    let mut operator = use_signal(|| Operator::Eq);
    let mut value = use_signal(String::new);
    let mut connector = use_signal(|| Connector::Or);

    let has_expression = !expression.read().trim().is_empty();
    let preview = format_fragment(field(), &key(), operator(), &value());
    let can_add = !value().trim().is_empty() && (!field().needs_key() || !key().trim().is_empty());

    let mut add = move || {
        let v = value();
        if v.trim().is_empty() {
            return;
        }
        if field().needs_key() && key().trim().is_empty() {
            return;
        }
        let fragment = format_fragment(field(), &key(), operator(), &v);
        let next = combine(&expression.peek(), connector(), &fragment);
        on_add.call(next);
        value.set(String::new());
    };

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
                    "build a BeaconQL expression"
                }
            }

            // ── Expanded body ───────────────────────────────────────────────
            if open() {
                div { class: "px-3 pb-3 flex flex-col gap-2 border-t border-base-300",

                    // Row 1 — field / key / operator / value
                    div { class: "pt-2 flex flex-wrap items-end gap-2",

                        div { class: "flex flex-col gap-1",
                            span { class: "text-[10px] text-base-content/40 uppercase tracking-wide", "Field" }
                            select {
                                class: "select select-xs select-bordered",
                                onchange: move |e| {
                                    field.set(match e.value().as_str() {
                                        "span_name"             => FieldKind::SpanName,
                                        "status_code"            => FieldKind::StatusCode,
                                        "trace_id"                => FieldKind::TraceId,
                                        "span_id"                  => FieldKind::SpanId,
                                        "parent_span_id"            => FieldKind::ParentSpanId,
                                        "duration_ns"                => FieldKind::DurationNs,
                                        "attributes"                  => FieldKind::Attributes,
                                        "business_attributes"          => FieldKind::BusinessAttributes,
                                        "runtime_attributes"             => FieldKind::RuntimeAttributes,
                                        _                                  => FieldKind::ServiceName,
                                    });
                                },
                                option { value: "service_name",          selected: field() == FieldKind::ServiceName,         "service_name" }
                                option { value: "span_name",             selected: field() == FieldKind::SpanName,            "span_name" }
                                option { value: "status_code",           selected: field() == FieldKind::StatusCode,          "status_code" }
                                option { value: "trace_id",              selected: field() == FieldKind::TraceId,             "trace_id" }
                                option { value: "span_id",               selected: field() == FieldKind::SpanId,              "span_id" }
                                option { value: "parent_span_id",        selected: field() == FieldKind::ParentSpanId,        "parent_span_id" }
                                option { value: "duration_ns",           selected: field() == FieldKind::DurationNs,          "duration_ns" }
                                option { value: "attributes",            selected: field() == FieldKind::Attributes,          "attributes[…]" }
                                option { value: "business_attributes",   selected: field() == FieldKind::BusinessAttributes,  "business_attributes[…]" }
                                option { value: "runtime_attributes",    selected: field() == FieldKind::RuntimeAttributes,   "runtime_attributes[…]" }
                            }
                        }

                        // Map key — only shown for the three attribute maps
                        if field().needs_key() {
                            div { class: "flex flex-col gap-1",
                                span { class: "text-[10px] text-base-content/40 uppercase tracking-wide", "Key" }
                                input {
                                    class: "input input-xs input-bordered font-mono w-32",
                                    placeholder: "e.g. user_id",
                                    value: "{key}",
                                    oninput: move |e| key.set(e.value()),
                                }
                            }
                        }

                        div { class: "flex flex-col gap-1",
                            span { class: "text-[10px] text-base-content/40 uppercase tracking-wide", "Operator" }
                            select {
                                class: "select select-xs select-bordered",
                                onchange: move |e| {
                                    operator.set(match e.value().as_str() {
                                        "neq"      => Operator::Neq,
                                        "lt"       => Operator::Lt,
                                        "lte"      => Operator::Lte,
                                        "gt"       => Operator::Gt,
                                        "gte"      => Operator::Gte,
                                        "like"     => Operator::Like,
                                        "not_like" => Operator::NotLike,
                                        "in"       => Operator::In,
                                        "not_in"   => Operator::NotIn,
                                        _          => Operator::Eq,
                                    });
                                },
                                option { value: "eq",       selected: operator() == Operator::Eq,      "= (equals)" }
                                option { value: "neq",      selected: operator() == Operator::Neq,     "!= (not equals)" }
                                option { value: "lt",       selected: operator() == Operator::Lt,       "< (less than)" }
                                option { value: "lte",      selected: operator() == Operator::Lte,     "<= (less or equal)" }
                                option { value: "gt",       selected: operator() == Operator::Gt,       "> (greater than)" }
                                option { value: "gte",      selected: operator() == Operator::Gte,     ">= (greater or equal)" }
                                option { value: "like",     selected: operator() == Operator::Like,     "LIKE (contains)" }
                                option { value: "not_like", selected: operator() == Operator::NotLike, "NOT LIKE" }
                                option { value: "in",       selected: operator() == Operator::In,       "IN (…)" }
                                option { value: "not_in",   selected: operator() == Operator::NotIn,   "NOT IN (…)" }
                            }
                        }

                        div { class: "flex flex-col gap-1 flex-1 min-w-28",
                            span { class: "text-[10px] text-base-content/40 uppercase tracking-wide",
                                if operator().is_list() { "Values (comma-separated)" } else { "Value" }
                            }
                            input {
                                class: "input input-xs input-bordered font-mono w-full",
                                placeholder: if operator().is_list() { "v1, v2, …" } else { "value…" },
                                value: "{value}",
                                oninput: move |e| value.set(e.value()),
                                onkeydown: move |e| {
                                    if e.key() != Key::Enter || !can_add { return; }
                                    add();
                                },
                            }
                        }
                    }

                    // Row 2 — live preview + connector toggle + add button
                    div { class: "flex items-center gap-2 flex-wrap",

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
                            disabled: !can_add,
                            onclick: move |_| add(),
                            "Add to query →"
                        }
                    }
                }
            }
        }
    }
}
