//! Autocomplete extension points.
//!
//! Autocomplete is the same schema read from the cursor: at any position the query is *expecting*
//! something (a field, an operator, a value …) and the [`Grammar`](super::Grammar) says what is
//! valid there. The types below are the contract; a tokenizer implementing [`Completer`] and a
//! combobox on `QueryBar` are the follow-up (plan §3.8). Nothing here is implemented yet on
//! purpose — defining it now keeps `FieldSpec::values` and the builder honest about what they
//! must be able to express.

use super::grammar::{FieldSpec, Op, ValueQuery};

/// What a query is expecting at the cursor.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum Expect {
    /// Start of a clause: a field name (or `(` / `NOT`).
    Field,
    /// Inside `attributes[` — a map key.
    MapKey(&'static FieldSpec),
    /// After a field: an operator valid for it.
    Operator(&'static FieldSpec),
    /// After an operator: a value.
    Value(&'static FieldSpec, Op),
    /// After `IN (` or `,` — the next list item.
    ListItem(&'static FieldSpec),
    /// After a complete clause: `AND` / `OR` / end.
    Connector,
    Nothing,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum SuggestKind {
    Field,
    Operator,
    Value,
    Key,
    Connector,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Suggestion {
    /// Shown in the list.
    pub label: String,
    /// Inserted at the cursor.
    pub insert: String,
    /// Right-aligned hint (`field`, `event type`, `12 matches`).
    pub detail: String,
    pub kind: SuggestKind,
}

/// Decides what is expected at a cursor position. Client-side and synchronous, so suggestions
/// appear instantly; the schema and values it consults come from the server.
pub trait Completer {
    fn expect(&self, text: &str, cursor: usize) -> Expect;

    /// Suggestions known without a lookup (fields, operators, static values).
    fn suggest_static(&self, expect: &Expect, prefix: &str) -> Vec<Suggestion>;
}

/// Supplies values that need a backend lookup (`ValueSource::Remote`): distinct service names,
/// attribute keys, event types, metric and label names. Needs the value-lookup RPCs (plan §7,
/// item 26); until they exist there is no implementation and nothing is suggested.
pub trait ValueProvider {
    fn lookup(&self, query: ValueQuery, prefix: &str, limit: usize) -> Vec<Suggestion>;
}
