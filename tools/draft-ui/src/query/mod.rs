//! Query grammars, fragment compilation and the autocomplete contract.
//!
//! Pure Rust (no Dioxus) so it is unit-tested natively. The `QueryBar` / `QueryChips` /
//! `QueryBuilder` components in this module's `ui` half render it.

#[cfg(feature = "dioxus")]
mod bar;
#[cfg(feature = "dioxus")]
mod chips;
mod compile;
mod completer;
mod grammar;

#[cfg(feature = "dioxus")]
pub use bar::QueryBar;
#[cfg(feature = "dioxus")]
pub use chips::{QueryChips, QueryFilters};

pub use compile::{string_literal, CompileError, Connector, Selection};
pub use completer::{Completer, Expect, SuggestKind, Suggestion, ValueProvider};
pub use grammar::{
    CombineRule, FieldKind, FieldSpec, Grammar, GrammarId, GrammarKind, LiteralStyle, Op, ValueQuery, ValueSource,
    BEACON_LOGS, BEACON_TRACES, BEACON_WIDE_EVENTS, CESQL, PROMQL,
};
