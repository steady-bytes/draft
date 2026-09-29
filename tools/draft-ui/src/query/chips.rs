use dioxus::prelude::*;

use super::grammar::{GrammarId, GrammarKind, Op};
use super::{Connector, Selection};
use crate::ui::{Btn, BtnSize, BtnVariant, Chip, Field, Select, TextInput};

/// The grammar's preset chips only (`d-chips`). Picking one calls `on_pick` with its text.
#[component]
pub fn QueryChips(grammar: GrammarId, on_pick: EventHandler<String>, children: Element) -> Element {
    let g = grammar.grammar();
    rsx! {
        div { class: "d-chips",
            for chip in g.chips.iter().copied() {
                Chip { key: "{chip}", onclick: move |_| on_pick.call(chip.to_string()), "{chip}" }
            }
            {children}
        }
    }
}

/// Preset chips plus a **Filter…** chip that opens the grammar-driven builder — the single
/// replacement for Blueprint's, Beacon's and Beacon's wide-event query builders.
///
/// * Picking a chip or adding a clause joins it into `expression` with the grammar's
///   precedence-safe rule, then calls `on_run`.
/// * For PromQL (not a predicate language) picking a chip replaces the expression and there is no
///   builder.
#[component]
pub fn QueryFilters(grammar: GrammarId, expression: Signal<String>, on_run: EventHandler<()>) -> Element {
    let g = grammar.grammar();
    let mut open = use_signal(|| false);
    let predicate = g.kind == GrammarKind::Predicate;

    let mut apply = move |fragment: String, connector: Connector| {
        let next = if predicate {
            g.combine(&expression.peek(), connector, &fragment)
        } else {
            fragment
        };
        expression.set(next);
        on_run.call(());
    };

    rsx! {
        div {
            QueryChips { grammar, on_pick: move |chip: String| apply(chip, Connector::And),
                if predicate {
                    Chip {
                        pressed: open(),
                        onclick: move |_| open.toggle(),
                        "Filter…"
                    }
                }
            }
            if predicate && open() {
                Builder { grammar, has_expression: !expression.read().trim().is_empty(), on_add: move |(f , c)| apply(f, c) }
            }
        }
    }
}

/// The builder form: field, key, operator, value, connector, a live preview and Add.
#[component]
fn Builder(grammar: GrammarId, has_expression: bool, on_add: EventHandler<(String, Connector)>) -> Element {
    let g = grammar.grammar();
    let mut field = use_signal(|| g.fields.first().map(|f| f.name.to_string()).unwrap_or_default());
    let mut key = use_signal(String::new);
    let mut op = use_signal(|| Op::Eq);
    let mut value = use_signal(String::new);
    let mut connector = use_signal(|| Connector::And);
    let mut error: Signal<Option<String>> = use_signal(|| None);

    let spec = g.field(&field.read());
    let ops: Vec<Op> = spec.map(|s| s.ops().to_vec()).unwrap_or_default();
    // Keep the operator valid when the field (and so its operator set) changes.
    let current_op = if ops.contains(&op()) { op() } else { ops.first().copied().unwrap_or(Op::Eq) };
    let needs_key = spec.is_some_and(|s| s.kind.needs_key());
    let takes_value = current_op.takes_value();

    let selection = Selection {
        field: field.read().clone(),
        key: needs_key.then(|| key.read().clone()),
        op: current_op,
        value: value.read().clone(),
    };
    let preview = g.fragment(&selection);

    let field_options: Vec<(String, String)> = g.fields.iter().map(|f| (f.name.to_string(), f.name.to_string())).collect();
    let op_options: Vec<(String, String)> = ops.iter().map(|o| (o.symbol().to_string(), o.label().to_string())).collect();
    let key_label = match spec.map(|s| s.kind) {
        Some(crate::query::FieldKind::Path) => "Path",
        _ => "Key",
    };

    let mut add = move || {
        // Resolve the fragment first so the read guards are dropped before the signals are written.
        let result = g.fragment(&selection_from(g, &field.read(), &key.read(), current_op, &value.read()));
        match result {
            Ok(f) => {
                error.set(None);
                value.set(String::new());
                on_add.call((f, connector()));
            }
            Err(e) => error.set(Some(e.to_string())),
        }
    };

    rsx! {
        div { class: "d-panel d-panel-body", style: "margin-top:8px",
            div { class: "d-row", style: "flex-wrap:wrap;align-items:flex-end",
                Field { label: "Field".to_string(),
                    Select {
                        value: field(),
                        options: field_options,
                        aria_label: "Field".to_string(),
                        on_change: move |v: String| {
                            field.set(v);
                            error.set(None);
                        },
                    }
                }
                if needs_key {
                    Field { label: key_label.to_string(),
                        TextInput { value: key(), oninput: move |v| key.set(v), placeholder: "e.g. route".to_string() }
                    }
                }
                Field { label: "Operator".to_string(),
                    Select {
                        value: current_op.symbol().to_string(),
                        options: op_options,
                        aria_label: "Operator".to_string(),
                        on_change: move |v: String| {
                            if let Some(o) = ops.iter().find(|o| o.symbol() == v) {
                                op.set(*o);
                            }
                        },
                    }
                }
                if takes_value {
                    Field { label: if current_op.is_list() { "Values (comma-separated)".to_string() } else { "Value".to_string() },
                        TextInput {
                            value: value(),
                            oninput: move |v| value.set(v),
                            placeholder: "value…".to_string(),
                            onkeydown: move |e: KeyboardEvent| {
                                if e.key() == Key::Enter {
                                    add();
                                }
                            },
                        }
                    }
                }
            }
            div { class: "d-row", style: "margin-top:12px;flex-wrap:wrap",
                code { class: "d-code-inline d-trunc", style: "flex:1;min-width:0",
                    match &preview {
                        Ok(f) => rsx! { "{f}" },
                        Err(_) => rsx! { span { class: "d-muted", "preview appears here" } },
                    }
                }
                if has_expression {
                    crate::ui::Seg {
                        options: vec![(Connector::And, "AND".to_string()), (Connector::Or, "OR".to_string())],
                        value: connector(),
                        on_change: move |c| connector.set(c),
                        label: "Join with".to_string(),
                    }
                }
                Btn {
                    variant: BtnVariant::Primary,
                    size: BtnSize::Sm,
                    disabled: preview.is_err(),
                    onclick: move |_| add(),
                    "Add"
                }
            }
            if let Some(e) = error() {
                div { class: "d-error-text", role: "alert", style: "margin-top:8px", "{e}" }
            }
        }
    }
}

fn selection_from(g: &super::Grammar, field: &str, key: &str, op: Op, value: &str) -> Selection {
    let needs_key = g.field(field).is_some_and(|f| f.kind.needs_key());
    Selection { field: field.to_string(), key: needs_key.then(|| key.to_string()), op, value: value.to_string() }
}
