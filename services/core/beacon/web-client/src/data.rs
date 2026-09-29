//! Pure helpers for the Beacon views: how timestamps, severities and statuses read, how rows fall
//! into histogram buckets, how a time range becomes a query clause, and how spans lay out as a
//! waterfall. No rendering and no RPC, so all of it is unit-tested natively.

use std::collections::HashMap;

use chrono::{DateTime, Duration, Utc};
use draft_ui::query::{string_literal, LiteralStyle};
use draft_ui::util::format_duration_ns;
use draft_ui::viz::{CompactRow, HistoBucket, WaterfallRow};
use draft_ui::{StatusKind, Tone};
use prost_types::Timestamp;

// Time -------------------------------------------------------------------------------------------

pub fn ts_nanos(ts: &Option<Timestamp>) -> Option<i64> {
    let t = ts.as_ref()?;
    Some(t.seconds * 1_000_000_000 + i64::from(t.nanos))
}

pub fn ts_datetime(ts: &Option<Timestamp>) -> Option<DateTime<Utc>> {
    let t = ts.as_ref()?;
    DateTime::from_timestamp(t.seconds, t.nanos as u32)
}

/// The time cell of a table row: the clock time when the row is from today (UTC), the date too
/// when it is not, so a 7-day window never shows two different days as the same time.
pub fn row_time(ts: &Option<Timestamp>, now: DateTime<Utc>) -> String {
    match ts_datetime(ts) {
        Some(dt) if dt.date_naive() == now.date_naive() => dt.format("%H:%M:%S%.3f").to_string(),
        Some(dt) => dt.format("%b %-d %H:%M:%S").to_string(),
        None => "—".to_string(),
    }
}

/// `2026-09-26 01:47:09.201`, for drawers and tooltips.
pub fn full_time(ts: &Option<Timestamp>) -> String {
    ts_datetime(ts).map(|dt| dt.format("%Y-%m-%d %H:%M:%S%.3f").to_string()).unwrap_or_else(|| "—".to_string())
}

pub fn rfc3339(ts: &Option<Timestamp>) -> Option<String> {
    ts_datetime(ts).map(|dt| dt.to_rfc3339())
}

/// `15m`, `6h`, `2d` — a range's label on the segmented control.
pub fn short_duration(d: Duration) -> String {
    let mins = d.num_minutes();
    if mins < 60 {
        format!("{mins}m")
    } else if mins < 60 * 24 {
        format!("{}h", mins / 60)
    } else {
        format!("{}d", mins / (60 * 24))
    }
}

// Severity and status ----------------------------------------------------------------------------

/// A log's severity, collapsed to the four things the UI distinguishes.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Severity {
    Error,
    Warn,
    Info,
    Other,
}

impl Severity {
    pub fn of(text: &str) -> Self {
        match text.to_ascii_uppercase().as_str() {
            "FATAL" | "ERROR" => Severity::Error,
            "WARN" | "WARNING" => Severity::Warn,
            "INFO" => Severity::Info,
            _ => Severity::Other,
        }
    }

    pub const fn tone(self) -> Tone {
        match self {
            Severity::Error => Tone::Err,
            Severity::Warn => Tone::Warn,
            Severity::Info => Tone::Ca,
            Severity::Other => Tone::Quiet,
        }
    }
}

/// The severity as a table cell: the record's own text, title-cased, or `—` when it has none.
pub fn severity_label(text: &str) -> String {
    if text.is_empty() {
        return "—".to_string();
    }
    let lower = text.to_lowercase();
    let mut chars = lower.chars();
    chars.next().map(|c| c.to_uppercase().collect::<String>() + chars.as_str()).unwrap_or_default()
}

/// A span's or wide event's status. Emitters disagree on spelling (`ERROR` vs
/// `STATUS_CODE_ERROR`), so both are accepted everywhere.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum SpanStatus {
    Error,
    Ok,
    Other,
}

impl SpanStatus {
    pub fn of(code: &str) -> Self {
        match code.to_ascii_uppercase().as_str() {
            "ERROR" | "STATUS_CODE_ERROR" => SpanStatus::Error,
            "OK" | "STATUS_CODE_OK" => SpanStatus::Ok,
            _ => SpanStatus::Other,
        }
    }

    pub const fn kind(self) -> StatusKind {
        match self {
            SpanStatus::Error => StatusKind::Err,
            SpanStatus::Ok => StatusKind::Ok,
            SpanStatus::Other => StatusKind::Idle,
        }
    }

    pub fn label(self) -> &'static str {
        match self {
            SpanStatus::Error => "Error",
            SpanStatus::Ok => "OK",
            SpanStatus::Other => "Unset",
        }
    }
}

// Histogram --------------------------------------------------------------------------------------

/// How many bars a histogram has. More than the pixel width would resolve is wasted; fewer makes
/// a short window look blocky.
pub const HISTO_BUCKETS: usize = 60;

pub struct Bucketed {
    pub buckets: Vec<HistoBucket>,
    /// Rows per class, over the whole window (for the legend).
    pub totals: Vec<u32>,
    pub start_ns: i64,
    pub end_ns: i64,
}

/// Spreads `(time in ns, class)` points over `n` equal buckets between the first and last point.
/// A bucket's `values` hold one count per class in class order, so the class order is the stacking
/// order (index 0 at the bottom). `names` label the classes in each bar's tooltip, top of the
/// stack first. Returns `None` when there is no time span to spread over.
pub fn bucketize(points: &[(i64, usize)], names: &[&str], n: usize) -> Option<Bucketed> {
    let classes = names.len();
    let start = points.iter().map(|p| p.0).min()?;
    let end = points.iter().map(|p| p.0).max()?;
    if start >= end || n == 0 {
        return None;
    }
    let span = (end - start) as f64;
    let mut counts = vec![vec![0u32; classes]; n];
    let mut totals = vec![0u32; classes];
    for &(ns, class) in points {
        let class = class.min(classes - 1);
        let idx = (((ns - start) as f64 / span * n as f64) as usize).min(n - 1);
        counts[idx][class] += 1;
        totals[class] += 1;
    }
    let buckets = counts
        .into_iter()
        .map(|c| {
            let title = (0..classes).rev().map(|i| format!("{} {}", names[i], c[i])).collect::<Vec<_>>().join(" · ");
            HistoBucket { title, values: c.into_iter().map(f64::from).collect() }
        })
        .collect();
    Some(Bucketed { buckets, totals, start_ns: start, end_ns: end })
}

/// Six evenly spaced axis labels from `start_ns` to `end_ns`, the last with `UTC`. Seconds are
/// shown when the window is short enough for minutes to be too coarse.
pub fn axis_labels(start_ns: i64, end_ns: i64) -> Vec<String> {
    const TICKS: i64 = 6;
    let with_seconds = end_ns - start_ns < 10 * 60 * 1_000_000_000;
    let fmt = if with_seconds { "%H:%M:%S" } else { "%H:%M" };
    (0..TICKS)
        .map(|i| {
            let ns = start_ns + (end_ns - start_ns) * i / (TICKS - 1);
            let label = DateTime::from_timestamp(ns.div_euclid(1_000_000_000), ns.rem_euclid(1_000_000_000) as u32)
                .map(|dt| dt.format(fmt).to_string())
                .unwrap_or_else(|| "—".to_string());
            if i == TICKS - 1 {
                format!("{label} UTC")
            } else {
                label
            }
        })
        .collect()
}

// Queries ----------------------------------------------------------------------------------------

/// A BeaconQL string literal for arbitrary text (attribute keys and values, ids): backslash and
/// quote escaped, exactly as the server's lexer un-escapes them.
pub fn beaconql_string(text: &str) -> String {
    string_literal(LiteralStyle::DoubleQuoteBackslash, text)
}

/// The `start_time` bounds a time range adds to a search. ClickHouse parses a comparison against
/// `start_time` only in `YYYY-MM-DD hh:mm:ss` form (an RFC 3339 `T…Z` literal is rejected), read
/// as UTC.
pub fn window_clause(start: DateTime<Utc>, end: Option<DateTime<Utc>>) -> String {
    let fmt = |t: DateTime<Utc>| beaconql_string(&t.format("%Y-%m-%d %H:%M:%S").to_string());
    match end {
        Some(end) => format!("start_time > {} AND start_time < {}", fmt(start), fmt(end)),
        None => format!("start_time > {}", fmt(start)),
    }
}

/// The user's expression ANDed with the window. The expression is parenthesised first: BeaconQL's
/// AND binds tighter than OR, so a bare `a OR b AND window` would only scope `b`.
pub fn with_window(expression: &str, window: &str) -> String {
    let expression = expression.trim();
    if expression.is_empty() {
        window.to_string()
    } else {
        format!("({expression}) AND {window}")
    }
}

/// Attributes as sorted `(key, value)` pairs, so a row always lists them in the same order.
pub fn sorted_attrs(map: &HashMap<String, String>) -> Vec<(String, String)> {
    let mut pairs: Vec<(String, String)> = map.iter().map(|(k, v)| (k.clone(), v.clone())).collect();
    pairs.sort();
    pairs
}

// Waterfall --------------------------------------------------------------------------------------

/// A span from either source (a trace's spans or a wide event's trace), reduced to what a
/// waterfall needs.
#[derive(Clone, Debug, PartialEq)]
pub struct SpanLike {
    pub id: String,
    pub parent: String,
    pub service: String,
    pub name: String,
    pub start_ns: i128,
    pub duration_ns: u64,
    pub err: bool,
}

pub struct Waterfall {
    pub rows: Vec<WaterfallRow>,
    /// `(fraction of the width, label)`.
    pub ticks: Vec<(f64, String)>,
    pub total_ns: u64,
    /// Nanoseconds from the trace start to each span's start, by span id.
    pub offsets: HashMap<String, u64>,
}

/// The narrowest a bar may be, as a fraction of the lane, so a microsecond span stays visible.
const MIN_BAR: f64 = 0.004;

/// Lays spans out depth-first, children after their parent in start order, on one shared
/// timeline. A span whose parent is missing (a truncated trace) or is itself is a root, so bad
/// data cannot loop.
pub fn waterfall(spans: &[SpanLike]) -> Waterfall {
    let t0 = spans.iter().map(|s| s.start_ns).min().unwrap_or(0);
    let t1 = spans.iter().map(|s| s.start_ns + i128::from(s.duration_ns)).max().unwrap_or(t0 + 1);
    let total = (t1 - t0).max(1) as f64;

    let ids: HashMap<&str, usize> = spans.iter().enumerate().map(|(i, s)| (s.id.as_str(), i)).collect();
    let mut children: HashMap<usize, Vec<usize>> = HashMap::new();
    let mut roots: Vec<usize> = Vec::new();
    for (i, s) in spans.iter().enumerate() {
        match ids.get(s.parent.as_str()) {
            Some(&p) if !s.parent.is_empty() && p != i => children.entry(p).or_default().push(i),
            _ => roots.push(i),
        }
    }
    let by_start = |list: &mut Vec<usize>| list.sort_by_key(|&i| (spans[i].start_ns, i));
    by_start(&mut roots);
    for list in children.values_mut() {
        by_start(list);
    }

    // Iterative depth-first walk: a cycle in the data (a → b → a) is cut by the visited set.
    let mut order: Vec<(usize, u32)> = Vec::with_capacity(spans.len());
    let mut seen = vec![false; spans.len()];
    let mut stack: Vec<(usize, u32)> = roots.iter().rev().map(|&r| (r, 0)).collect();
    while let Some((i, depth)) = stack.pop() {
        if std::mem::replace(&mut seen[i], true) {
            continue;
        }
        order.push((i, depth));
        if let Some(kids) = children.get(&i) {
            stack.extend(kids.iter().rev().map(|&k| (k, depth + 1)));
        }
    }

    let mut offsets = HashMap::new();
    let rows = order
        .into_iter()
        .map(|(i, depth)| {
            let s = &spans[i];
            // Size the bar first, then keep it inside the lane: a span at the very end that is
            // widened to the minimum shifts left rather than overflowing.
            let width = (s.duration_ns as f64 / total).clamp(MIN_BAR, 1.0);
            let left = ((s.start_ns - t0) as f64 / total).min(1.0 - width);
            offsets.insert(s.id.clone(), (s.start_ns - t0).max(0) as u64);
            WaterfallRow {
                id: s.id.clone(),
                name: s.name.clone(),
                service: s.service.clone(),
                tone: Tone::for_name(&s.service),
                depth,
                left,
                width,
                label: Some(format_duration_ns(s.duration_ns)),
                err: s.err,
            }
        })
        .collect();

    let total_ns = total as u64;
    let ticks = (0..=4)
        .map(|i| {
            let f = f64::from(i) / 4.0;
            let label = if i == 0 { "0".to_string() } else { format_duration_ns((total * f) as u64) };
            (f, label)
        })
        .collect();
    Waterfall { rows, ticks, total_ns, offsets }
}

/// The waterfall as one-line rows for a drawer: the name, a bar on the shared timeline and the
/// duration. Errors take the error colour, everything else its service's.
pub fn compact_rows(spans: &[SpanLike]) -> Vec<CompactRow> {
    let w = waterfall(spans);
    let by_id: HashMap<&str, &SpanLike> = spans.iter().map(|s| (s.id.as_str(), s)).collect();
    w.rows
        .into_iter()
        .map(|r| {
            let span = by_id.get(r.id.as_str());
            CompactRow {
                name: r.name,
                tone: if r.err { Tone::Err } else { r.tone },
                left: r.left,
                width: r.width,
                duration: span.map(|s| format_duration_ns(s.duration_ns)).unwrap_or_default(),
            }
        })
        .collect()
}

// Metrics ----------------------------------------------------------------------------------------

/// A series' identity: the metric name (absent once an aggregation drops it) and its sorted label
/// pairs — `http_requests_total{method=GET, route=/api}`. Used for the legend and table rows.
pub fn label_signature(metric_name: &str, labels: &HashMap<String, String>) -> String {
    let mut pairs: Vec<String> = labels.iter().map(|(k, v)| format!("{k}={v}")).collect();
    pairs.sort();
    if metric_name.is_empty() {
        pairs.join(", ")
    } else if pairs.is_empty() {
        metric_name.to_string()
    } else {
        format!("{metric_name}{{{}}}", pairs.join(", "))
    }
}

/// A sample value for a stat tile: `1.23M`, `4.56k`, `123.4`, `0.123`.
pub fn format_value(v: f64) -> String {
    if v.abs() >= 1_000_000.0 {
        format!("{:.2}M", v / 1_000_000.0)
    } else if v.abs() >= 1_000.0 {
        format!("{:.2}k", v / 1_000.0)
    } else if v.abs() >= 100.0 {
        format!("{v:.1}")
    } else {
        format!("{v:.3}")
    }
}

// JSON -------------------------------------------------------------------------------------------

/// A JSON string literal (escaped). Written by hand: the prost types do not derive `Serialize`
/// and the JSON tab is a small, fixed shape.
pub fn json_string(s: &str) -> String {
    let mut out = String::with_capacity(s.len() + 2);
    out.push('"');
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if (c as u32) < 0x20 => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push(c),
        }
    }
    out.push('"');
    out
}

/// A map as an indented JSON object nested one level down.
pub fn json_object(pairs: &[(String, String)]) -> String {
    if pairs.is_empty() {
        return "{}".to_string();
    }
    let body: Vec<String> = pairs.iter().map(|(k, v)| format!("    {}: {}", json_string(k), json_string(v))).collect();
    format!("{{\n{}\n  }}", body.join(",\n"))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn ts(seconds: i64, nanos: i32) -> Option<Timestamp> {
        Some(Timestamp { seconds, nanos })
    }

    fn at(s: &str) -> DateTime<Utc> {
        DateTime::parse_from_rfc3339(s).unwrap().with_timezone(&Utc)
    }

    #[test]
    fn row_time_drops_the_date_only_for_today() {
        let now = at("2026-09-26T12:00:00Z");
        let today = ts(at("2026-09-26T01:47:09Z").timestamp(), 201_000_000);
        let earlier = ts(at("2026-09-24T23:05:00Z").timestamp(), 0);
        assert_eq!(row_time(&today, now), "01:47:09.201");
        assert_eq!(row_time(&earlier, now), "Sep 24 23:05:00");
        assert_eq!(row_time(&None, now), "—");
    }

    #[test]
    fn severity_and_status_accept_every_spelling() {
        assert_eq!(Severity::of("fatal"), Severity::Error);
        assert_eq!(Severity::of("Warning"), Severity::Warn);
        assert_eq!(Severity::of("DEBUG"), Severity::Other);
        assert_eq!(Severity::of(""), Severity::Other);
        assert_eq!(severity_label("WARN"), "Warn");
        assert_eq!(severity_label(""), "—");
        assert_eq!(SpanStatus::of("STATUS_CODE_ERROR"), SpanStatus::Error);
        assert_eq!(SpanStatus::of("error"), SpanStatus::Error);
        assert_eq!(SpanStatus::of("STATUS_CODE_OK"), SpanStatus::Ok);
        assert_eq!(SpanStatus::of("STATUS_CODE_UNSET"), SpanStatus::Other);
    }

    #[test]
    fn durations_read_short() {
        assert_eq!(short_duration(Duration::minutes(15)), "15m");
        assert_eq!(short_duration(Duration::hours(6)), "6h");
        assert_eq!(short_duration(Duration::days(2)), "2d");
    }

    #[test]
    fn buckets_split_the_span_and_stack_by_class() {
        // Two classes; ten points over 0..90ns into 3 buckets.
        let points: Vec<(i64, usize)> = (0..10).map(|i| (i * 10, if i < 3 { 1 } else { 0 })).collect();
        let b = bucketize(&points, &["ok", "error"], 3).unwrap();
        assert_eq!((b.start_ns, b.end_ns), (0, 90));
        assert_eq!(b.totals, vec![7, 3]);
        assert_eq!(b.buckets.len(), 3);
        // Every point lands in exactly one bucket, including the last (the maximum maps to n - 1).
        let all: f64 = b.buckets.iter().flat_map(|b| b.values.iter()).sum();
        assert_eq!(all, 10.0);
        assert_eq!(b.buckets[0].values, vec![0.0, 3.0]);
        assert_eq!(b.buckets[0].title, "error 3 · ok 0");
    }

    #[test]
    fn no_span_means_no_histogram() {
        assert!(bucketize(&[], &["a"], 10).is_none());
        assert!(bucketize(&[(5, 0), (5, 0)], &["a"], 10).is_none());
    }

    #[test]
    fn axis_shows_seconds_only_for_short_windows() {
        let long = axis_labels(at("2026-09-26T00:00:00Z").timestamp() * 1_000_000_000, at("2026-09-26T05:00:00Z").timestamp() * 1_000_000_000);
        assert_eq!(long, ["00:00", "01:00", "02:00", "03:00", "04:00", "05:00 UTC"]);
        let short = axis_labels(0, 5_000_000_000);
        assert_eq!(short.first().unwrap(), "00:00:00");
        assert_eq!(short.last().unwrap(), "00:00:05 UTC");
    }

    #[test]
    fn window_clauses_use_the_format_clickhouse_accepts() {
        let start = at("2026-09-26T13:00:00Z");
        assert_eq!(window_clause(start, None), r#"start_time > "2026-09-26 13:00:00""#);
        assert_eq!(
            window_clause(start, Some(at("2026-09-26T14:30:00Z"))),
            r#"start_time > "2026-09-26 13:00:00" AND start_time < "2026-09-26 14:30:00""#
        );
    }

    #[test]
    fn the_window_never_binds_to_only_the_last_or_operand() {
        let w = r#"start_time > "2026-09-26 13:00:00""#;
        assert_eq!(with_window("", w), w);
        assert_eq!(with_window("  ", w), w);
        assert_eq!(with_window(r#"a = "1" OR b = "2""#, w), format!(r#"(a = "1" OR b = "2") AND {w}"#));
    }

    #[test]
    fn beaconql_strings_escape_quotes_and_backslashes() {
        assert_eq!(beaconql_string(r#"a"b\c"#), r#""a\"b\\c""#);
    }

    fn span(id: &str, parent: &str, start: i128, dur: u64) -> SpanLike {
        SpanLike { id: id.into(), parent: parent.into(), service: "fuse".into(), name: id.into(), start_ns: start, duration_ns: dur, err: false }
    }

    #[test]
    fn waterfall_orders_depth_first_by_start() {
        // root(0..100) → b(50..80), a(10..40) → a1(20..30); given out of order.
        let spans = [span("b", "root", 50, 30), span("a1", "a", 20, 10), span("root", "", 0, 100), span("a", "root", 10, 30)];
        let w = waterfall(&spans);
        let order: Vec<(&str, u32)> = w.rows.iter().map(|r| (r.id.as_str(), r.depth)).collect();
        assert_eq!(order, [("root", 0), ("a", 1), ("a1", 2), ("b", 1)]);
        assert_eq!(w.total_ns, 100);
        assert!((w.rows[1].left - 0.10).abs() < 1e-9);
        assert!((w.rows[1].width - 0.30).abs() < 1e-9);
        assert_eq!(w.offsets["b"], 50);
        assert_eq!(w.ticks.first().unwrap().1, "0");
        assert_eq!(w.ticks.last().unwrap().0, 1.0);
    }

    #[test]
    fn waterfall_survives_bad_parents() {
        // A self-parent, a missing parent and a two-span cycle must terminate and show every span
        // that is reachable from a root.
        let spans = [span("self", "self", 0, 10), span("orphan", "gone", 5, 10), span("x", "y", 6, 1), span("y", "x", 7, 1)];
        let w = waterfall(&spans);
        let ids: Vec<&str> = w.rows.iter().map(|r| r.id.as_str()).collect();
        assert!(ids.contains(&"self") && ids.contains(&"orphan"));
        assert_eq!(ids.len(), ids.iter().collect::<std::collections::HashSet<_>>().len(), "no span is drawn twice");
    }

    #[test]
    fn a_tiny_span_keeps_a_visible_bar_inside_the_lane() {
        let spans = [span("root", "", 0, 1_000_000), span("blip", "root", 999_999, 1)];
        let w = waterfall(&spans);
        let blip = &w.rows[1];
        assert!(blip.width >= MIN_BAR);
        assert!(blip.left + blip.width <= 1.0 + 1e-9);
    }

    #[test]
    fn a_series_is_named_by_its_metric_and_sorted_labels() {
        let labels: HashMap<String, String> = [("route".to_string(), "/api".to_string()), ("method".to_string(), "GET".to_string())].into();
        assert_eq!(label_signature("reqs_total", &labels), "reqs_total{method=GET, route=/api}");
        assert_eq!(label_signature("", &labels), "method=GET, route=/api");
        assert_eq!(label_signature("up", &HashMap::new()), "up");
    }

    #[test]
    fn values_scale_to_a_readable_size() {
        assert_eq!(format_value(1_234_567.0), "1.23M");
        assert_eq!(format_value(4_560.0), "4.56k");
        assert_eq!(format_value(123.45), "123.5");
        assert_eq!(format_value(0.1234), "0.123");
        assert_eq!(format_value(-2_500.0), "-2.50k");
    }

    #[test]
    fn json_escapes_control_characters() {
        assert_eq!(json_string("a\"b\n\u{1}"), "\"a\\\"b\\n\\u0001\"");
        assert_eq!(json_object(&[]), "{}");
        assert_eq!(json_object(&[("k".into(), "v".into())]), "{\n    \"k\": \"v\"\n  }");
    }
}
