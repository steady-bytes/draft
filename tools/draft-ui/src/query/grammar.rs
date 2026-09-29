//! Query grammars as data.
//!
//! One [`Grammar`] describes everything the UI needs to know about a query language: its fields and
//! their kinds, which operators each accepts, how literals are written and how a new clause is
//! combined into an existing expression. The builder form, the preset chips and (later) the
//! autocomplete all read it, so they cannot disagree with each other.
//!
//! The field tables mirror the backends' closed switches (`beacon/query/beaconql.go` `columnFor*`,
//! `catalyst/broker/evaluator.go`). They are compiled-in fallbacks: the plan is for the server to
//! publish them (`DescribeQuerySchema`), at which point these become the offline default.

#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub enum GrammarId {
    /// CloudEvents SQL against Catalyst's stored events.
    Cesql,
    /// BeaconQL against the logs table.
    BeaconLogs,
    /// BeaconQL against trace roots.
    BeaconTraces,
    /// BeaconQL against wide events.
    BeaconWideEvents,
    /// The PromQL subset Beacon evaluates.
    PromQl,
}

impl GrammarId {
    pub const fn grammar(self) -> &'static Grammar {
        match self {
            GrammarId::Cesql => &CESQL,
            GrammarId::BeaconLogs => &BEACON_LOGS,
            GrammarId::BeaconTraces => &BEACON_TRACES,
            GrammarId::BeaconWideEvents => &BEACON_WIDE_EVENTS,
            GrammarId::PromQl => &PROMQL,
        }
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub enum Op {
    Eq,
    Neq,
    Lt,
    Lte,
    Gt,
    Gte,
    /// Contains (`LIKE '%v%'`) unless the value already holds a `%` pattern.
    Like,
    NotLike,
    In,
    NotIn,
    /// `EXISTS attr` — no value.
    Exists,
}

impl Op {
    /// The operator as written in a query.
    pub const fn symbol(self) -> &'static str {
        match self {
            Op::Eq => "=",
            Op::Neq => "!=",
            Op::Lt => "<",
            Op::Lte => "<=",
            Op::Gt => ">",
            Op::Gte => ">=",
            Op::Like => "LIKE",
            Op::NotLike => "NOT LIKE",
            Op::In => "IN",
            Op::NotIn => "NOT IN",
            Op::Exists => "EXISTS",
        }
    }

    /// The operator as shown in the builder's select.
    pub const fn label(self) -> &'static str {
        match self {
            Op::Eq => "= (equals)",
            Op::Neq => "!= (not equal)",
            Op::Lt => "< (less than)",
            Op::Lte => "<= (at most)",
            Op::Gt => "> (greater than)",
            Op::Gte => ">= (at least)",
            Op::Like => "LIKE (contains)",
            Op::NotLike => "NOT LIKE (does not contain)",
            Op::In => "IN (any of)",
            Op::NotIn => "NOT IN (none of)",
            Op::Exists => "EXISTS (is present)",
        }
    }

    pub const fn is_list(self) -> bool {
        matches!(self, Op::In | Op::NotIn)
    }

    pub const fn takes_value(self) -> bool {
        !matches!(self, Op::Exists)
    }
}

/// What a field holds. Decides the default operators and how values are written.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum FieldKind {
    Text,
    /// Written bare (no quotes) and validated as a number.
    Number,
    /// A number with a unit shown in the builder (`ms`). The unit is documentation; scaling, if
    /// any, happens server-side.
    Duration(&'static str),
    Timestamp,
    /// One of a fixed set of values.
    Enum(&'static [&'static str]),
    /// A map: needs a key, written `name["key"]`.
    Map,
    /// A dotted path below the field, written `name.path` (CESQL `body.*`).
    Path,
}

impl FieldKind {
    pub const fn is_numeric(self) -> bool {
        matches!(self, FieldKind::Number | FieldKind::Duration(_))
    }

    pub const fn needs_key(self) -> bool {
        matches!(self, FieldKind::Map | FieldKind::Path)
    }

    /// Operators when a field does not override them.
    pub const fn default_ops(self) -> &'static [Op] {
        match self {
            FieldKind::Text | FieldKind::Map | FieldKind::Path => {
                &[Op::Eq, Op::Neq, Op::Like, Op::NotLike, Op::In, Op::NotIn]
            }
            FieldKind::Number | FieldKind::Duration(_) | FieldKind::Timestamp => {
                &[Op::Eq, Op::Neq, Op::Lt, Op::Lte, Op::Gt, Op::Gte, Op::In, Op::NotIn]
            }
            FieldKind::Enum(_) => &[Op::Eq, Op::Neq, Op::In, Op::NotIn],
        }
    }
}

/// Where a field's values can be suggested from.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum ValueSource {
    None,
    /// A fixed list known at compile time.
    Static(&'static [&'static str]),
    /// Looked up on demand by a `ValueProvider` (distinct `service_name`, attribute keys, event
    /// types…). Needs the backend value-lookup RPCs; until they exist nothing is suggested.
    Remote(ValueQuery),
}

/// Identifies a lookup for a `ValueProvider`.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct ValueQuery {
    /// The dataset: `"logs"`, `"traces"`, `"wide_events"`, `"events"`.
    pub dataset: &'static str,
    /// The field (or map field, for key suggestions) to enumerate.
    pub field: &'static str,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct FieldSpec {
    /// The name written into queries.
    pub name: &'static str,
    /// Other names the backend accepts for the same field (`severity_text` for `severity`).
    pub aliases: &'static [&'static str],
    pub kind: FieldKind,
    /// `None` = the kind's default operators.
    pub ops: Option<&'static [Op]>,
    pub values: ValueSource,
    pub doc: &'static str,
}

impl FieldSpec {
    pub const fn ops(&self) -> &'static [Op] {
        match self.ops {
            Some(o) => o,
            None => self.kind.default_ops(),
        }
    }

    pub fn matches_name(&self, name: &str) -> bool {
        self.name.eq_ignore_ascii_case(name) || self.aliases.iter().any(|a| a.eq_ignore_ascii_case(name))
    }
}

/// How string literals are written.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum LiteralStyle {
    /// `'text'`, with `\` and `'` escaped by a backslash (CESQL).
    SingleQuote,
    /// `"text"`, with `\` and `"` escaped by a backslash (BeaconQL).
    DoubleQuoteBackslash,
}

/// How a new clause joins an existing expression.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum CombineRule {
    /// `OR` is the loosest binder, so `a OR b` needs no parentheses but `AND x` must not attach
    /// only to the last OR operand: the existing expression is parenthesised first. True of both
    /// CESQL and BeaconQL.
    OrLoosest,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum GrammarKind {
    /// `field op value` clauses joined by AND / OR.
    Predicate,
    /// Not a predicate language: no fields, only a bar and chips.
    PromQl,
}

#[derive(Clone, Copy, Debug)]
pub struct Grammar {
    pub id: GrammarId,
    pub kind: GrammarKind,
    /// The label on the query bar (`BEACONQL`).
    pub label: &'static str,
    pub placeholder: &'static str,
    pub fields: &'static [FieldSpec],
    pub literal: LiteralStyle,
    pub combine: CombineRule,
    /// Preset chips. Only constructs the backend accepts — a chip must never produce a parse error.
    pub chips: &'static [&'static str],
}

impl Grammar {
    pub fn field(&self, name: &str) -> Option<&'static FieldSpec> {
        self.fields.iter().find(|f| f.matches_name(name))
    }
}

// Field tables ------------------------------------------------------------------------------------------

const fn text(name: &'static str, doc: &'static str) -> FieldSpec {
    FieldSpec { name, aliases: &[], kind: FieldKind::Text, ops: None, values: ValueSource::None, doc }
}

const fn field(name: &'static str, kind: FieldKind, doc: &'static str) -> FieldSpec {
    FieldSpec { name, aliases: &[], kind, ops: None, values: ValueSource::None, doc }
}

const fn remote(mut f: FieldSpec, dataset: &'static str) -> FieldSpec {
    f.values = ValueSource::Remote(ValueQuery { dataset, field: f.name });
    f
}

const CESQL_OPS: &[Op] = &[Op::Eq, Op::Neq, Op::Like, Op::NotLike, Op::In, Op::NotIn, Op::Exists];

const CESQL_FIELDS: &[FieldSpec] = &[
    FieldSpec { ops: Some(CESQL_OPS), ..remote(text("type", "CloudEvent type"), "events") },
    FieldSpec { ops: Some(CESQL_OPS), ..remote(text("source", "CloudEvent source"), "events") },
    FieldSpec { ops: Some(CESQL_OPS), ..text("id", "CloudEvent id") },
    FieldSpec { ops: Some(CESQL_OPS), ..text("subject", "CloudEvent subject") },
    field("body", FieldKind::Path, "A field of the event payload, written body.<path>"),
];

pub const CESQL: Grammar = Grammar {
    id: GrammarId::Cesql,
    kind: GrammarKind::Predicate,
    label: "CESQL",
    placeholder: "e.g.  type = 'order.created' AND source LIKE '%shop%'",
    fields: CESQL_FIELDS,
    literal: LiteralStyle::SingleQuote,
    combine: CombineRule::OrLoosest,
    chips: &["type LIKE '%order%'", "source LIKE '%shop%'", "EXISTS subject", "body.status = 'failed'"],
};

const SEVERITIES: &[&str] = &["TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"];

const BEACON_LOGS_FIELDS: &[FieldSpec] = &[
    FieldSpec {
        name: "severity",
        aliases: &["severity_text"],
        kind: FieldKind::Enum(SEVERITIES),
        ops: None,
        values: ValueSource::Static(SEVERITIES),
        doc: "OpenTelemetry severity text",
    },
    remote(text("service_name", "Emitting service"), "logs"),
    text("trace_id", "Trace the line belongs to"),
    text("span_id", "Span the line belongs to"),
    text("body", "Log message"),
    field("severity_number", FieldKind::Number, "OpenTelemetry severity number (17+ is ERROR)"),
    field("attributes", FieldKind::Map, "Log attributes, written attributes[\"key\"]"),
    field("resource_attributes", FieldKind::Map, "Resource attributes, written resource_attributes[\"key\"]"),
];

pub const BEACON_LOGS: Grammar = Grammar {
    id: GrammarId::BeaconLogs,
    kind: GrammarKind::Predicate,
    label: "BEACONQL",
    placeholder: r#"e.g.  severity = "error" AND service_name = "beacon"  or  attributes["route"] LIKE "/api/%""#,
    fields: BEACON_LOGS_FIELDS,
    literal: LiteralStyle::DoubleQuoteBackslash,
    combine: CombineRule::OrLoosest,
    chips: &[
        r#"severity = "error""#,
        r#"severity IN ("warn", "error")"#,
        r#"trace_id != """#,
        r#"body LIKE "%timeout%""#,
    ],
};

const BEACON_TRACES_FIELDS: &[FieldSpec] = &[
    remote(text("service_name", "Root span's service"), "traces"),
    text("trace_id", "Trace id"),
    text("span_name", "Root span name"),
    text("status_code", "Root span status"),
    field("start_time", FieldKind::Timestamp, "Trace start"),
    field("duration_ms", FieldKind::Duration("ms"), "Trace duration in milliseconds"),
    field("duration_ns", FieldKind::Duration("ns"), "Trace duration in nanoseconds"),
    field("attributes", FieldKind::Map, "Root span attributes, written attributes[\"key\"]"),
];

pub const BEACON_TRACES: Grammar = Grammar {
    id: GrammarId::BeaconTraces,
    kind: GrammarKind::Predicate,
    label: "BEACONQL",
    placeholder: r#"e.g.  duration_ms > 100 OR status_code = "ERROR""#,
    fields: BEACON_TRACES_FIELDS,
    literal: LiteralStyle::DoubleQuoteBackslash,
    combine: CombineRule::OrLoosest,
    chips: &[r#"status_code LIKE "%ERROR%""#, "duration_ms > 100", "duration_ms > 1000"],
};

const BEACON_WIDE_EVENTS_FIELDS: &[FieldSpec] = &[
    remote(text("service_name", "Emitting service"), "wide_events"),
    text("trace_id", "Trace id"),
    text("span_id", "Span id"),
    text("parent_span_id", "Parent span id"),
    text("span_name", "Span name"),
    // Stored as "ERROR" or "STATUS_CODE_ERROR" depending on the emitter — match both with LIKE.
    text("status_code", "Span status (ERROR / STATUS_CODE_ERROR, OK / STATUS_CODE_OK)"),
    field("start_time", FieldKind::Timestamp, "Span start"),
    field("duration_ns", FieldKind::Duration("ns"), "Span duration in nanoseconds"),
    field("attributes", FieldKind::Map, "Span attributes, written attributes[\"key\"]"),
    field("business_attributes", FieldKind::Map, "Business attributes, written business_attributes[\"key\"]"),
    field("runtime_attributes", FieldKind::Map, "Runtime attributes, written runtime_attributes[\"key\"]"),
];

pub const BEACON_WIDE_EVENTS: Grammar = Grammar {
    id: GrammarId::BeaconWideEvents,
    kind: GrammarKind::Predicate,
    label: "BEACONQL",
    placeholder: r#"e.g.  service_name IN ("fuse", "foundry") AND duration_ns > 5000000"#,
    fields: BEACON_WIDE_EVENTS_FIELDS,
    literal: LiteralStyle::DoubleQuoteBackslash,
    combine: CombineRule::OrLoosest,
    // `duration_ms` exists only for trace search; wide events take nanoseconds.
    chips: &[
        r#"status_code LIKE "%ERROR%""#,
        "duration_ns > 500000000",
        r#"attributes["http.status_code"] = "503""#,
        r#"attributes["route.name"] = "core-blueprint-ui""#,
    ],
};

pub const PROMQL: Grammar = Grammar {
    id: GrammarId::PromQl,
    kind: GrammarKind::PromQl,
    label: "PROMQL",
    placeholder: r#"e.g.  rate(http_requests_total{route="/api"}[5m])  or  sum by (route) (my_gauge)"#,
    fields: &[],
    literal: LiteralStyle::DoubleQuoteBackslash,
    combine: CombineRule::OrLoosest,
    // The backend supports rate() and sum/avg/max/min by(...) only.
    chips: &[
        "rate(fuse_requests_total[5m])",
        "sum by (route) (rate(fuse_requests_total[5m]))",
        "avg by (service) (fuse_routes_active)",
        "max by (route) (fuse_upstream_retries_total)",
    ],
};
