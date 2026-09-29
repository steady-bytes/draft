//! Structured selection → query fragment, and joining a fragment into an expression.
//!
//! Ported from — and unit-tested against — the three builders this replaces
//! (`blueprint/.../query_builder.rs`, `beacon/.../query_builder.rs`,
//! `beacon/.../wide_event_query_builder.rs`).

use super::grammar::{FieldKind, Grammar, LiteralStyle, Op};

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Connector {
    And,
    Or,
}

/// What the builder form collected.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Selection {
    pub field: String,
    /// Map key (`attributes["key"]`) or path below the field (`body.path`).
    pub key: Option<String>,
    pub op: Op,
    /// The raw text typed; a comma-separated list for `IN` / `NOT IN`.
    pub value: String,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub enum CompileError {
    UnknownField(String),
    /// The operator is not offered for this field.
    OpNotAllowed(String, Op),
    MissingKey(String),
    MissingValue,
    NotANumber(String),
}

impl std::fmt::Display for CompileError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            CompileError::UnknownField(n) => write!(f, "unknown field \"{n}\""),
            CompileError::OpNotAllowed(n, op) => write!(f, "{} is not available for {n}", op.symbol()),
            CompileError::MissingKey(n) => write!(f, "{n} needs a key"),
            CompileError::MissingValue => write!(f, "a value is required"),
            CompileError::NotANumber(v) => write!(f, "\"{v}\" is not a number"),
        }
    }
}

impl std::error::Error for CompileError {}

/// Renders `text` as a string literal in the grammar's style.
pub fn string_literal(style: LiteralStyle, text: &str) -> String {
    let q = match style {
        LiteralStyle::SingleQuote => '\'',
        LiteralStyle::DoubleQuoteBackslash => '"',
    };
    let mut out = String::with_capacity(text.len() + 2);
    out.push(q);
    for c in text.chars() {
        if c == q || c == '\\' {
            out.push('\\');
        }
        out.push(c);
    }
    out.push(q);
    out
}

impl Grammar {
    fn literal(&self, kind: FieldKind, raw: &str) -> Result<String, CompileError> {
        let raw = raw.trim();
        if kind.is_numeric() {
            raw.parse::<f64>().map_err(|_| CompileError::NotANumber(raw.to_string()))?;
            Ok(raw.to_string())
        } else {
            Ok(string_literal(self.literal, raw))
        }
    }

    /// The field as written in a query: `type`, `attributes["route"]`, `body.model`.
    pub fn field_ref(&self, sel: &Selection) -> Result<String, CompileError> {
        let spec = self.field(&sel.field).ok_or_else(|| CompileError::UnknownField(sel.field.clone()))?;
        match spec.kind {
            FieldKind::Map => {
                let key = sel.key.as_deref().map(str::trim).filter(|k| !k.is_empty());
                let key = key.ok_or_else(|| CompileError::MissingKey(spec.name.to_string()))?;
                // Map keys are always double-quoted in BeaconQL.
                Ok(format!("{}[{}]", spec.name, string_literal(LiteralStyle::DoubleQuoteBackslash, key)))
            }
            FieldKind::Path => {
                let key = sel.key.as_deref().map(str::trim).filter(|k| !k.is_empty());
                let key = key.ok_or_else(|| CompileError::MissingKey(spec.name.to_string()))?;
                Ok(format!("{}.{key}", spec.name))
            }
            _ => Ok(spec.name.to_string()),
        }
    }

    /// One clause: `severity = "error"`, `type LIKE '%order%'`, `service_name IN ("a", "b")`.
    pub fn fragment(&self, sel: &Selection) -> Result<String, CompileError> {
        let spec = self.field(&sel.field).ok_or_else(|| CompileError::UnknownField(sel.field.clone()))?;
        if !spec.ops().contains(&sel.op) {
            return Err(CompileError::OpNotAllowed(spec.name.to_string(), sel.op));
        }
        let field = self.field_ref(sel)?;

        if !sel.op.takes_value() {
            return Ok(format!("{} {field}", sel.op.symbol()));
        }
        if sel.value.trim().is_empty() {
            return Err(CompileError::MissingValue);
        }

        Ok(match sel.op {
            Op::Like | Op::NotLike => {
                // Contains, unless the user already wrote a pattern.
                let v = sel.value.trim();
                let pattern = if v.contains('%') { v.to_string() } else { format!("%{v}%") };
                format!("{field} {} {}", sel.op.symbol(), string_literal(self.literal, &pattern))
            }
            Op::In | Op::NotIn => {
                let items: Result<Vec<String>, _> = sel
                    .value
                    .split(',')
                    .map(str::trim)
                    .filter(|v| !v.is_empty())
                    .map(|v| self.literal(spec.kind, v))
                    .collect();
                let items = items?;
                if items.is_empty() {
                    return Err(CompileError::MissingValue);
                }
                format!("{field} {} ({})", sel.op.symbol(), items.join(", "))
            }
            op => format!("{field} {} {}", op.symbol(), self.literal(spec.kind, &sel.value)?),
        })
    }

    /// Joins `fragment` into `current`. AND binds tighter than OR in both languages, so appending
    /// `AND x` to `a OR b` would attach `x` to `b` only; the existing expression is parenthesised
    /// first. OR is the loosest binder and needs no parentheses.
    pub fn combine(&self, current: &str, connector: Connector, fragment: &str) -> String {
        let current = current.trim();
        if current.is_empty() {
            return fragment.to_string();
        }
        match connector {
            Connector::Or => format!("{current} OR {fragment}"),
            Connector::And => format!("({current}) AND {fragment}"),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::query::grammar::{BEACON_LOGS, BEACON_TRACES, BEACON_WIDE_EVENTS, CESQL};

    fn sel(field: &str, key: Option<&str>, op: Op, value: &str) -> Selection {
        Selection { field: field.into(), key: key.map(Into::into), op, value: value.into() }
    }

    // ── behaviour of the old Blueprint CESQL builder ───────────────────────────────────────────────
    #[test]
    fn cesql_equals_and_like_match_the_old_builder() {
        assert_eq!(CESQL.fragment(&sel("type", None, Op::Eq, "order.created")).unwrap(), "type = 'order.created'");
        assert_eq!(CESQL.fragment(&sel("source", None, Op::Like, "shop")).unwrap(), "source LIKE '%shop%'");
    }

    #[test]
    fn cesql_body_path() {
        assert_eq!(
            CESQL.fragment(&sel("body", Some("modelName"), Op::Eq, "x")).unwrap(),
            "body.modelName = 'x'"
        );
        assert_eq!(CESQL.fragment(&sel("body", None, Op::Eq, "x")), Err(CompileError::MissingKey("body".into())));
    }

    #[test]
    fn cesql_quotes_are_escaped() {
        // The old builder interpolated `'{value}'` unescaped, so `it's` produced a broken query.
        assert_eq!(CESQL.fragment(&sel("subject", None, Op::Eq, "it's")).unwrap(), r"subject = 'it\'s'");
    }

    #[test]
    fn cesql_gains_the_operators_its_evaluator_supports() {
        assert_eq!(CESQL.fragment(&sel("type", None, Op::Neq, "a")).unwrap(), "type != 'a'");
        assert_eq!(CESQL.fragment(&sel("type", None, Op::NotLike, "a")).unwrap(), "type NOT LIKE '%a%'");
        assert_eq!(CESQL.fragment(&sel("type", None, Op::In, "a, b")).unwrap(), "type IN ('a', 'b')");
        assert_eq!(CESQL.fragment(&sel("type", None, Op::NotIn, "a")).unwrap(), "type NOT IN ('a')");
        assert_eq!(CESQL.fragment(&sel("subject", None, Op::Exists, "")).unwrap(), "EXISTS subject");
    }

    // ── behaviour of the old Beacon BeaconQL builders ──────────────────────────────────────────────
    #[test]
    fn beaconql_scalar_and_map_fields() {
        assert_eq!(BEACON_LOGS.fragment(&sel("severity", None, Op::Eq, "error")).unwrap(), r#"severity = "error""#);
        assert_eq!(
            BEACON_LOGS.fragment(&sel("attributes", Some("route"), Op::Like, "/api")).unwrap(),
            r#"attributes["route"] LIKE "%/api%""#
        );
        assert_eq!(
            BEACON_LOGS.fragment(&sel("resource_attributes", Some("host"), Op::Eq, "h1")).unwrap(),
            r#"resource_attributes["host"] = "h1""#
        );
    }

    #[test]
    fn beaconql_numeric_fields_are_bare() {
        assert_eq!(BEACON_LOGS.fragment(&sel("severity_number", None, Op::Gte, "17")).unwrap(), "severity_number >= 17");
        assert_eq!(
            BEACON_LOGS.fragment(&sel("severity_number", None, Op::Gte, "high")),
            Err(CompileError::NotANumber("high".into()))
        );
    }

    #[test]
    fn beaconql_in_lists() {
        assert_eq!(
            BEACON_LOGS.fragment(&sel("service_name", None, Op::In, "fuse, foundry")).unwrap(),
            r#"service_name IN ("fuse", "foundry")"#
        );
        assert_eq!(
            BEACON_LOGS.fragment(&sel("severity_number", None, Op::NotIn, "9, 10")).unwrap(),
            "severity_number NOT IN (9, 10)"
        );
    }

    #[test]
    fn beaconql_escapes_quotes_and_backslashes() {
        assert_eq!(
            BEACON_LOGS.fragment(&sel("body", None, Op::Eq, r#"say "hi"\"#)).unwrap(),
            r#"body = "say \"hi\"\\""#
        );
    }

    #[test]
    fn map_field_without_key_is_rejected() {
        assert_eq!(
            BEACON_LOGS.fragment(&sel("attributes", None, Op::Eq, "x")),
            Err(CompileError::MissingKey("attributes".into()))
        );
    }

    #[test]
    fn trace_and_wide_event_datasets_have_their_own_fields() {
        assert_eq!(BEACON_TRACES.fragment(&sel("duration_ms", None, Op::Gt, "100")).unwrap(), "duration_ms > 100");
        assert!(BEACON_WIDE_EVENTS.field("duration_ms").is_none(), "duration_ms is a trace-only field");
        assert!(BEACON_WIDE_EVENTS.field("business_attributes").is_some());
        assert!(BEACON_LOGS.field("business_attributes").is_none());
    }

    #[test]
    fn alias_resolves_to_the_field() {
        let f = BEACON_LOGS.field("severity_text").unwrap();
        assert_eq!(f.name, "severity");
    }

    #[test]
    fn operators_not_offered_for_a_field_are_rejected() {
        assert!(matches!(
            BEACON_LOGS.fragment(&sel("severity", None, Op::Like, "err")),
            Err(CompileError::OpNotAllowed(..))
        ));
    }

    #[test]
    fn an_explicit_pattern_is_not_wrapped() {
        assert_eq!(
            BEACON_LOGS.fragment(&sel("body", None, Op::Like, "err%")).unwrap(),
            r#"body LIKE "err%""#
        );
    }

    #[test]
    fn empty_value_is_rejected() {
        assert_eq!(BEACON_LOGS.fragment(&sel("body", None, Op::Eq, "  ")), Err(CompileError::MissingValue));
    }

    // ── combining ──────────────────────────────────────────────────────────────────────────────────
    #[test]
    fn combine_follows_the_precedence_safe_rule() {
        let g = &BEACON_LOGS;
        assert_eq!(g.combine("", Connector::And, "a = 1"), "a = 1");
        assert_eq!(g.combine("x = 1", Connector::Or, "y = 2"), "x = 1 OR y = 2");
        // Without the parentheses `AND` would bind only to `y = 2`.
        assert_eq!(g.combine("x = 1 OR y = 2", Connector::And, "z = 3"), "(x = 1 OR y = 2) AND z = 3");
    }

    // ── chips must be valid for their grammar ──────────────────────────────────────────────────────
    #[test]
    fn predicate_chips_only_reference_known_fields() {
        for g in [&CESQL, &BEACON_LOGS, &BEACON_TRACES, &BEACON_WIDE_EVENTS] {
            for chip in g.chips {
                let first = chip.trim_start_matches("EXISTS ").split(|c: char| !(c.is_alphanumeric() || c == '_')).next().unwrap();
                assert!(g.field(first).is_some(), "{:?}: chip `{chip}` uses unknown field `{first}`", g.id);
            }
        }
    }

    #[test]
    fn promql_chips_use_only_the_supported_subset() {
        use crate::query::grammar::PROMQL;
        for chip in PROMQL.chips {
            assert!(!chip.contains("histogram_quantile") && !chip.contains("topk"), "{chip}");
        }
    }
}
