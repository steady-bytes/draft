// Package store owns Beacon's ClickHouse connection, the `logs` table migration,
// and the primitive insert/query operations used by the ingest and query packages.
// It follows the same shape as services/core/catalyst/broker/store.go (bounded
// insert path is in ingest.Writer, not here — this package only knows how to talk
// to ClickHouse), adapted with a TTL clause per the Beacon design doc's Data Model
// section (Catalyst's `events` table does not have one; Beacon's `logs` table needs
// one from day one given telemetry volume).
package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type (
	ClickHouseConfig struct {
		Enabled  bool   `mapstructure:"enabled"`
		Address  string `mapstructure:"address"`
		Database string `mapstructure:"database"`
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	}

	// LogRow mirrors one row of the `logs` table (and, field-for-field, the OTel
	// LogRecord + its Resource) — see the design doc's Data Model section.
	LogRow struct {
		Timestamp          time.Time
		TraceID            string
		SpanID             string
		SeverityText       string
		SeverityNumber     uint8
		ServiceName        string
		Body               string
		Attributes         map[string]string
		ResourceAttributes map[string]string
	}

	// SpanRow mirrors one row of the `spans` table (and, field-for-field, the
	// OTel Span proto) — see the design doc's Data Model section.
	SpanRow struct {
		TraceID      string
		SpanID       string
		ParentSpanID string
		ServiceName  string
		SpanName     string
		Kind         string
		StartTime    time.Time
		DurationNs   uint64
		StatusCode   string
		Attributes   map[string]string
	}

	// TraceRootRow mirrors one row of the `trace_roots` materialized view — the
	// root span (parent_span_id = "") of a trace, used to populate the Traces
	// view's search-result list without scanning every span of every trace.
	TraceRootRow struct {
		TraceID     string
		ServiceName string
		SpanName    string
		StartTime   time.Time
		DurationNs  uint64
		StatusCode  string
	}

	// MetricPointRow mirrors one row of the `metric_points` table — see the
	// design doc's Data Model section. A single OTLP gauge/sum data point
	// becomes one row; a histogram data point becomes several (one per bucket
	// plus `_sum`/`_count`) — see ingest/metrics.go's flattening.
	MetricPointRow struct {
		MetricName string
		Labels     map[string]string
		Timestamp  time.Time
		Value      float64
	}

	// WideEventLogLine is one log line emitted during a WideEvent's span —
	// see docs/website/content/docs/architecture/wide-events.md.
	WideEventLogLine struct {
		Timestamp time.Time
		Severity  string
		Body      string
	}

	// WideEventRow mirrors one row of the `wide_events` table (and,
	// field-for-field, the WideEvent proto) — see
	// docs/website/content/docs/architecture/wide-events.md's Data Model.
	WideEventRow struct {
		TraceID            string
		SpanID             string
		ParentSpanID       string
		ServiceName        string
		SpanName           string
		StartTime          time.Time
		DurationNs         uint64
		StatusCode         string
		Logs               []WideEventLogLine
		Attributes         map[string]string
		BusinessAttributes map[string]string
		RuntimeAttributes  map[string]string
	}

	// Storer is the interface the ingest and query packages depend on, so both
	// can be tested against a noop implementation without a live ClickHouse.
	Storer interface {
		// InsertLogs writes a batch of rows. Called by ingest.Writer's flusher.
		InsertLogs(ctx context.Context, rows []LogRow) error
		// QueryLogs runs a bounded, parameterized query against the `logs` table.
		// whereSQL is a fragment produced exclusively by query.Compile (never raw
		// user text) using `?` placeholders bound by args — see query/beaconql.go.
		// An empty whereSQL means "no filter". If after/before is non-empty it must
		// be an RFC3339 timestamp; only rows strictly after/before it (respectively)
		// are returned — either, both, or neither may be set, bounding one or both
		// ends of the timestamp range. ascending controls sort order — StreamLogs
		// replays ascending (oldest-first continuity), QueryLogs defaults to
		// descending (most-recent-first) but the caller may request ascending too
		// (e.g. the rows immediately after a cursor, for a log detail "context" view).
		QueryLogs(ctx context.Context, whereSQL string, args []any, limit int32, after, before string, ascending bool) ([]LogRow, error)

		// InsertSpans writes a batch of rows. Called by ingest.SpanWriter's flusher.
		InsertSpans(ctx context.Context, rows []SpanRow) error
		// QueryTraceRoots runs a bounded, parameterized query against the
		// `trace_roots` materialized view. whereSQL is a fragment produced
		// exclusively by query.CompileTraceRoot (never raw user text) using `?`
		// placeholders bound by args. An empty whereSQL means "no filter". If
		// before is non-empty it must be an RFC3339 timestamp; only traces with
		// start_time strictly before it are returned (backward pagination
		// through most-recent-first results).
		QueryTraceRoots(ctx context.Context, whereSQL string, args []any, limit int32, before string) ([]TraceRootRow, error)
		// GetTraceSpans returns every span for one trace_id, ordered by
		// start_time ascending so a parent always precedes its children —
		// suitable for direct flame-graph assembly by the caller.
		GetTraceSpans(ctx context.Context, traceID string) ([]SpanRow, error)

		// InsertMetrics writes a batch of rows. Called by ingest.MetricWriter's
		// flusher.
		InsertMetrics(ctx context.Context, rows []MetricPointRow) error
		// QueryMetricPoints returns raw metric_points rows for one metricName
		// matching labelWhereSQL (a label-matcher fragment produced exclusively
		// by query.CompileSelector — see query/promql.go — never raw user
		// text) within [start, end], ordered by timestamp ascending. The
		// PromQL-subset evaluator (query/promql.go) does its own
		// windowing/aggregation over these raw points — mirroring how
		// Prometheus's own query engine evaluates functions over samples read
		// back from the TSDB, rather than pushing rate()/sum-by windowing into
		// SQL. Bounded by maxMetricPoints to protect against a selector
		// matching an unbounded number of raw points.
		QueryMetricPoints(ctx context.Context, metricName string, labelWhereSQL string, args []any, start, end time.Time) ([]MetricPointRow, error)

		// InsertWideEvents writes a batch of rows. Called by ingest's WideEvent
		// consumer (see ingest/wide_events.go).
		InsertWideEvents(ctx context.Context, rows []WideEventRow) error
		// QueryWideEvents runs a bounded, parameterized query against the
		// `wide_events` table. whereSQL is a fragment produced exclusively by
		// query.CompileWideEvent (never raw user text) using `?` placeholders
		// bound by args. An empty whereSQL means "no filter". If before is
		// non-empty it must be an RFC3339 timestamp; only WideEvents with
		// start_time strictly before it are returned (backward pagination
		// through most-recent-first results).
		QueryWideEvents(ctx context.Context, whereSQL string, args []any, limit int32, before string) ([]WideEventRow, error)
		// GetWideEvent fetches one row by span_id, full logs/attributes included.
		GetWideEvent(ctx context.Context, spanID string) (WideEventRow, error)
	}

	noopStore struct{}

	clickhouseStore struct {
		conn driver.Conn
	}
)

func NewNoopStore() Storer { return &noopStore{} }

func (n *noopStore) InsertLogs(_ context.Context, _ []LogRow) error { return nil }

func (n *noopStore) QueryLogs(_ context.Context, _ string, _ []any, _ int32, _, _ string, _ bool) ([]LogRow, error) {
	return nil, nil
}

func (n *noopStore) InsertSpans(_ context.Context, _ []SpanRow) error { return nil }

func (n *noopStore) QueryTraceRoots(_ context.Context, _ string, _ []any, _ int32, _ string) ([]TraceRootRow, error) {
	return nil, nil
}

func (n *noopStore) GetTraceSpans(_ context.Context, _ string) ([]SpanRow, error) {
	return nil, nil
}

func (n *noopStore) InsertMetrics(_ context.Context, _ []MetricPointRow) error { return nil }

func (n *noopStore) QueryMetricPoints(_ context.Context, _ string, _ string, _ []any, _, _ time.Time) ([]MetricPointRow, error) {
	return nil, nil
}

func (n *noopStore) InsertWideEvents(_ context.Context, _ []WideEventRow) error { return nil }

func (n *noopStore) QueryWideEvents(_ context.Context, _ string, _ []any, _ int32, _ string) ([]WideEventRow, error) {
	return nil, nil
}

func (n *noopStore) GetWideEvent(_ context.Context, _ string) (WideEventRow, error) {
	return WideEventRow{}, nil
}

// defaultLimit/maxLimit bound QueryLogs/StreamLogs historical replay so a client
// can't accidentally (or maliciously, since this is browser-facing) request an
// unbounded scan. defaultTraceLimit/maxTraceLimit do the same for
// QueryTraceRoots.
const (
	defaultLimit = 200
	maxLimit     = 5_000

	defaultTraceLimit = 100
	maxTraceLimit     = 1_000

	defaultWideEventLimit = 100
	maxWideEventLimit     = 1_000

	// maxMetricPoints bounds a single QueryMetricPoints call — the PromQL-subset
	// evaluator (query/promql.go) already scopes each selector to a bounded
	// [start, end] window and a bounded step count, but this is a second,
	// server-side backstop against a pathological selector (e.g. a wide-open
	// time range) scanning an unbounded number of raw points.
	maxMetricPoints = 200_000
)

const createLogsTable = `
CREATE TABLE IF NOT EXISTS logs (
    timestamp            DateTime64(9),
    trace_id             String,
    span_id              String,
    severity_text        LowCardinality(String),
    severity_number      UInt8,
    service_name         LowCardinality(String),
    body                 String,
    attributes            Map(String, String),
    resource_attributes  Map(String, String)
) ENGINE = MergeTree()
ORDER BY (service_name, timestamp)
PARTITION BY toYYYYMMDD(timestamp)
TTL toDateTime(timestamp) + INTERVAL 30 DAY
`

const createSpansTable = `
CREATE TABLE IF NOT EXISTS spans (
    trace_id       String,
    span_id        String,
    parent_span_id String,
    service_name   LowCardinality(String),
    span_name      String,
    kind           LowCardinality(String),
    start_time     DateTime64(9),
    duration_ns    UInt64,
    status_code    LowCardinality(String),
    attributes     Map(String, String)
) ENGINE = MergeTree()
ORDER BY (service_name, start_time)
PARTITION BY toYYYYMMDD(start_time)
TTL toDateTime(start_time) + INTERVAL 14 DAY
`

// trace_roots maintains one row per trace_id (the trace's root span, i.e. the
// span with an empty parent_span_id) so the Traces view's search-result list
// can be populated without scanning every span of every trace — see the design
// doc's Data Model section. ReplacingMergeTree is used defensively (a retried
// OTLP export could otherwise land the same root span twice); queries read
// it with FINAL to guarantee exactly one row per trace_id.
const createTraceRootsView = `
CREATE MATERIALIZED VIEW IF NOT EXISTS trace_roots
ENGINE = ReplacingMergeTree()
ORDER BY trace_id
AS SELECT trace_id, service_name, span_name, start_time, duration_ns, status_code
FROM spans WHERE parent_span_id = ''
`

// metric_points holds one row per (metric_name, labels, timestamp) sample —
// see the design doc's Data Model section. A histogram data point is
// flattened into several rows at ingest time (ingest/metrics.go): one per
// bucket (metric_name suffixed `_bucket`, with a synthetic `le` label) plus
// `_sum`/`_count` rows, following the Prometheus exposition convention, so
// this table's shape needs no special-casing between counters/gauges and
// histograms — the PromQL-subset query layer (query/promql.go) only ever
// sees flat (metric_name, labels, timestamp, value) rows.
const createMetricPointsTable = `
CREATE TABLE IF NOT EXISTS metric_points (
    metric_name  LowCardinality(String),
    labels       Map(String, String),
    timestamp    DateTime64(3),
    value        Float64
) ENGINE = MergeTree()
ORDER BY (metric_name, timestamp)
PARTITION BY toYYYYMMDD(timestamp)
TTL toDateTime(timestamp) + INTERVAL 15 DAY
`

// wide_events holds one row per span, carrying its trace context, every log
// line emitted during it (as a Nested sub-table -- logs.timestamp/severity/body
// parallel arrays, confirmed live against clickhouse-go/v2 v2.46.0's
// PrepareBatch/Append/Scan handling), and whatever attributes the producer
// attached — see docs/website/content/docs/architecture/wide-events.md's Data
// Model. ORDER BY goes low-to-high cardinality, same as `spans`, with
// trace_id deliberately left out of the sort key for the same reason: an
// exact-match trace lookup still runs fine within an already-pruned
// partition.
const createWideEventsTable = `
CREATE TABLE IF NOT EXISTS wide_events (
    trace_id             String,
    span_id              String,
    parent_span_id       String,
    service_name         LowCardinality(String),
    span_name            String,
    start_time           DateTime64(9),
    duration_ns          UInt64,
    status_code          LowCardinality(String),
    logs                 Nested (
        timestamp            DateTime64(9),
        severity             LowCardinality(String),
        body                 String
    ),
    attributes           Map(String, String),
    business_attributes  Map(String, String),
    runtime_attributes   Map(String, String)
) ENGINE = MergeTree()
ORDER BY (service_name, start_time)
PARTITION BY toYYYYMMDD(start_time)
TTL toDateTime(start_time) + INTERVAL 14 DAY
`

// NewClickHouseStore connects to the ClickHouse server, ensures the `beacon`
// database exists (a bootstrap connection against the server's default database
// is used for the CREATE DATABASE statement, since a connection can't target a
// database that doesn't exist yet), then opens the real connection scoped to it
// and runs the `logs` table migration.
func NewClickHouseStore(cfg ClickHouseConfig) (Storer, error) {
	ctx := context.Background()

	bootstrap, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Address},
		Auth: clickhouse.Auth{
			Database: "default",
			Username: cfg.Username,
			Password: cfg.Password,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open bootstrap clickhouse connection: %w", err)
	}
	// cfg.Database is operator-controlled config, not end-user input, so a
	// formatted identifier here does not carry the injection risk BeaconQL's
	// query compiler is designed to avoid (see query/beaconql.go).
	if err := bootstrap.Exec(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", cfg.Database)); err != nil {
		_ = bootstrap.Close()
		return nil, fmt.Errorf("create database %q: %w", cfg.Database, err)
	}
	if err := bootstrap.Close(); err != nil {
		return nil, fmt.Errorf("close bootstrap clickhouse connection: %w", err)
	}

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Address},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open clickhouse: %w", err)
	}

	s := &clickhouseStore{conn: conn}
	if err := s.migrate(ctx); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *clickhouseStore) migrate(ctx context.Context) error {
	if err := s.conn.Exec(ctx, createLogsTable); err != nil {
		return err
	}
	if err := s.conn.Exec(ctx, createSpansTable); err != nil {
		return err
	}
	// The materialized view's SELECT reads from `spans`, so it must be created
	// after the spans table migration above.
	if err := s.conn.Exec(ctx, createTraceRootsView); err != nil {
		return err
	}
	if err := s.conn.Exec(ctx, createWideEventsTable); err != nil {
		return err
	}
	return s.conn.Exec(ctx, createMetricPointsTable)
}

func (s *clickhouseStore) InsertLogs(ctx context.Context, rows []LogRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx,
		"INSERT INTO logs (timestamp, trace_id, span_id, severity_text, severity_number, service_name, body, attributes, resource_attributes)",
	)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.Timestamp,
			r.TraceID,
			r.SpanID,
			r.SeverityText,
			r.SeverityNumber,
			r.ServiceName,
			r.Body,
			r.Attributes,
			r.ResourceAttributes,
		); err != nil {
			return fmt.Errorf("append row: %w", err)
		}
	}
	return batch.Send()
}

// parseCursorTimestamp validates an RFC3339 cursor string (the after/before
// query params) and re-formats it at full nanosecond precision as a string,
// for binding to a ClickHouse query via parseDateTime64BestEffort(?, 9) rather
// than as a Go time.Time positional parameter directly.
//
// This indirection exists because of a clickhouse-go v2 driver quirk: a bare
// `?` placeholder gives the driver no way to know the destination column is
// DateTime64(9), so it defaults an unadorned time.Time argument to ClickHouse's
// DateTime type — second precision, no fractional component — silently
// truncating everything after the decimal point. `logs`/`spans` routinely have
// many rows within the same second (see e.g. the reaper's per-tick burst), so
// that truncation doesn't just lose sub-second ordering — it can make a
// `timestamp < cursor` condition match a wildly wrong row count (observed
// firsthand while building Phase 12: 500 real matches collapsed to 1 once
// enough same-second rows existed on either side of the truncation boundary).
// Binding the value as a string and letting ClickHouse's own parser handle it
// at full precision sidesteps the driver's default entirely.
func parseCursorTimestamp(label, s string) (string, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return "", fmt.Errorf("parse %s timestamp: %w", label, err)
	}
	return t.UTC().Format(time.RFC3339Nano), nil
}

func (s *clickhouseStore) QueryLogs(ctx context.Context, whereSQL string, args []any, limit int32, after, before string, ascending bool) ([]LogRow, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	var (
		conditions []string
		queryArgs  []any
	)
	if whereSQL != "" {
		conditions = append(conditions, "("+whereSQL+")")
		queryArgs = append(queryArgs, args...)
	}
	if after != "" {
		v, err := parseCursorTimestamp("after", after)
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, "timestamp > parseDateTime64BestEffort(?, 9)")
		queryArgs = append(queryArgs, v)
	}
	if before != "" {
		v, err := parseCursorTimestamp("before", before)
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, "timestamp < parseDateTime64BestEffort(?, 9)")
		queryArgs = append(queryArgs, v)
	}

	order := "DESC"
	if ascending {
		order = "ASC"
	}

	sql := "SELECT timestamp, trace_id, span_id, severity_text, severity_number, service_name, body, attributes, resource_attributes FROM logs"
	if len(conditions) > 0 {
		sql += " WHERE " + strings.Join(conditions, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY timestamp %s LIMIT ?", order)
	queryArgs = append(queryArgs, limit)

	rows, err := s.conn.Query(ctx, sql, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("query logs: %w", err)
	}
	defer rows.Close()

	var result []LogRow
	for rows.Next() {
		var r LogRow
		if err := rows.Scan(
			&r.Timestamp,
			&r.TraceID,
			&r.SpanID,
			&r.SeverityText,
			&r.SeverityNumber,
			&r.ServiceName,
			&r.Body,
			&r.Attributes,
			&r.ResourceAttributes,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *clickhouseStore) InsertSpans(ctx context.Context, rows []SpanRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx,
		"INSERT INTO spans (trace_id, span_id, parent_span_id, service_name, span_name, kind, start_time, duration_ns, status_code, attributes)",
	)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.TraceID,
			r.SpanID,
			r.ParentSpanID,
			r.ServiceName,
			r.SpanName,
			r.Kind,
			r.StartTime,
			r.DurationNs,
			r.StatusCode,
			r.Attributes,
		); err != nil {
			return fmt.Errorf("append row: %w", err)
		}
	}
	return batch.Send()
}

func (s *clickhouseStore) QueryTraceRoots(ctx context.Context, whereSQL string, args []any, limit int32, before string) ([]TraceRootRow, error) {
	if limit <= 0 {
		limit = defaultTraceLimit
	}
	if limit > maxTraceLimit {
		limit = maxTraceLimit
	}

	var (
		conditions []string
		queryArgs  []any
	)
	if whereSQL != "" {
		conditions = append(conditions, "("+whereSQL+")")
		queryArgs = append(queryArgs, args...)
	}
	if before != "" {
		v, err := parseCursorTimestamp("before", before)
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, "start_time < parseDateTime64BestEffort(?, 9)")
		queryArgs = append(queryArgs, v)
	}

	// FINAL forces ReplacingMergeTree dedup at query time so trace_roots
	// always reports exactly one row per trace_id, regardless of whether a
	// background merge has run yet (see createTraceRootsView).
	sql := "SELECT trace_id, service_name, span_name, start_time, duration_ns, status_code FROM trace_roots FINAL"
	if len(conditions) > 0 {
		sql += " WHERE " + strings.Join(conditions, " AND ")
	}
	sql += " ORDER BY start_time DESC LIMIT ?"
	queryArgs = append(queryArgs, limit)

	rows, err := s.conn.Query(ctx, sql, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("query trace_roots: %w", err)
	}
	defer rows.Close()

	var result []TraceRootRow
	for rows.Next() {
		var r TraceRootRow
		if err := rows.Scan(
			&r.TraceID,
			&r.ServiceName,
			&r.SpanName,
			&r.StartTime,
			&r.DurationNs,
			&r.StatusCode,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *clickhouseStore) InsertMetrics(ctx context.Context, rows []MetricPointRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx,
		"INSERT INTO metric_points (metric_name, labels, timestamp, value)",
	)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.MetricName,
			r.Labels,
			r.Timestamp,
			r.Value,
		); err != nil {
			return fmt.Errorf("append row: %w", err)
		}
	}
	return batch.Send()
}

func (s *clickhouseStore) QueryMetricPoints(ctx context.Context, metricName string, labelWhereSQL string, args []any, start, end time.Time) ([]MetricPointRow, error) {
	// Bound as strings via parseDateTime64BestEffort, not as time.Time directly —
	// see parseCursorTimestamp's doc comment for why a bare positional time.Time
	// silently truncates to second precision.
	conditions := []string{"metric_name = ?", "timestamp >= parseDateTime64BestEffort(?, 9)", "timestamp <= parseDateTime64BestEffort(?, 9)"}
	queryArgs := []any{metricName, start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano)}
	if labelWhereSQL != "" {
		conditions = append(conditions, "("+labelWhereSQL+")")
		queryArgs = append(queryArgs, args...)
	}

	sql := "SELECT metric_name, labels, timestamp, value FROM metric_points WHERE " +
		strings.Join(conditions, " AND ") + " ORDER BY timestamp ASC LIMIT ?"
	queryArgs = append(queryArgs, maxMetricPoints)

	rows, err := s.conn.Query(ctx, sql, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("query metric_points: %w", err)
	}
	defer rows.Close()

	var result []MetricPointRow
	for rows.Next() {
		var r MetricPointRow
		if err := rows.Scan(&r.MetricName, &r.Labels, &r.Timestamp, &r.Value); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *clickhouseStore) GetTraceSpans(ctx context.Context, traceID string) ([]SpanRow, error) {
	sql := "SELECT trace_id, span_id, parent_span_id, service_name, span_name, kind, start_time, duration_ns, status_code, attributes " +
		"FROM spans WHERE trace_id = ? ORDER BY start_time ASC"

	rows, err := s.conn.Query(ctx, sql, traceID)
	if err != nil {
		return nil, fmt.Errorf("query spans: %w", err)
	}
	defer rows.Close()

	var result []SpanRow
	for rows.Next() {
		var r SpanRow
		if err := rows.Scan(
			&r.TraceID,
			&r.SpanID,
			&r.ParentSpanID,
			&r.ServiceName,
			&r.SpanName,
			&r.Kind,
			&r.StartTime,
			&r.DurationNs,
			&r.StatusCode,
			&r.Attributes,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// wideEventLogColumns splits a WideEventRow's Logs into the parallel slices
// the `logs` Nested column's dotted sub-columns (logs.timestamp/severity/body)
// expect — confirmed live against clickhouse-go/v2 v2.46.0 that a Nested
// column's Append/Scan works as parallel arrays, one slice argument per
// sub-column, in declaration order, not as a single composite value.
func wideEventLogColumns(logs []WideEventLogLine) (timestamps []time.Time, severities []string, bodies []string) {
	timestamps = make([]time.Time, len(logs))
	severities = make([]string, len(logs))
	bodies = make([]string, len(logs))
	for i, l := range logs {
		timestamps[i] = l.Timestamp
		severities[i] = l.Severity
		bodies[i] = l.Body
	}
	return
}

func wideEventLogLines(timestamps []time.Time, severities []string, bodies []string) []WideEventLogLine {
	lines := make([]WideEventLogLine, len(timestamps))
	for i := range timestamps {
		lines[i] = WideEventLogLine{Timestamp: timestamps[i], Severity: severities[i], Body: bodies[i]}
	}
	return lines
}

func (s *clickhouseStore) InsertWideEvents(ctx context.Context, rows []WideEventRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx,
		"INSERT INTO wide_events (trace_id, span_id, parent_span_id, service_name, span_name, start_time, duration_ns, status_code, logs.timestamp, logs.severity, logs.body, attributes, business_attributes, runtime_attributes)",
	)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}
	for _, r := range rows {
		timestamps, severities, bodies := wideEventLogColumns(r.Logs)
		if err := batch.Append(
			r.TraceID,
			r.SpanID,
			r.ParentSpanID,
			r.ServiceName,
			r.SpanName,
			r.StartTime,
			r.DurationNs,
			r.StatusCode,
			timestamps,
			severities,
			bodies,
			r.Attributes,
			r.BusinessAttributes,
			r.RuntimeAttributes,
		); err != nil {
			return fmt.Errorf("append row: %w", err)
		}
	}
	return batch.Send()
}

func (s *clickhouseStore) QueryWideEvents(ctx context.Context, whereSQL string, args []any, limit int32, before string) ([]WideEventRow, error) {
	if limit <= 0 {
		limit = defaultWideEventLimit
	}
	if limit > maxWideEventLimit {
		limit = maxWideEventLimit
	}

	var (
		conditions []string
		queryArgs  []any
	)
	if whereSQL != "" {
		conditions = append(conditions, "("+whereSQL+")")
		queryArgs = append(queryArgs, args...)
	}
	if before != "" {
		v, err := parseCursorTimestamp("before", before)
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, "start_time < parseDateTime64BestEffort(?, 9)")
		queryArgs = append(queryArgs, v)
	}

	sql := "SELECT trace_id, span_id, parent_span_id, service_name, span_name, start_time, duration_ns, status_code, " +
		"logs.timestamp, logs.severity, logs.body, attributes, business_attributes, runtime_attributes FROM wide_events"
	if len(conditions) > 0 {
		sql += " WHERE " + strings.Join(conditions, " AND ")
	}
	sql += " ORDER BY start_time DESC LIMIT ?"
	queryArgs = append(queryArgs, limit)

	rows, err := s.conn.Query(ctx, sql, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("query wide_events: %w", err)
	}
	defer rows.Close()

	var result []WideEventRow
	for rows.Next() {
		var (
			r                        WideEventRow
			logTimestamps            []time.Time
			logSeverities, logBodies []string
		)
		if err := rows.Scan(
			&r.TraceID,
			&r.SpanID,
			&r.ParentSpanID,
			&r.ServiceName,
			&r.SpanName,
			&r.StartTime,
			&r.DurationNs,
			&r.StatusCode,
			&logTimestamps,
			&logSeverities,
			&logBodies,
			&r.Attributes,
			&r.BusinessAttributes,
			&r.RuntimeAttributes,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		r.Logs = wideEventLogLines(logTimestamps, logSeverities, logBodies)
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *clickhouseStore) GetWideEvent(ctx context.Context, spanID string) (WideEventRow, error) {
	sql := "SELECT trace_id, span_id, parent_span_id, service_name, span_name, start_time, duration_ns, status_code, " +
		"logs.timestamp, logs.severity, logs.body, attributes, business_attributes, runtime_attributes " +
		"FROM wide_events WHERE span_id = ? LIMIT 1"

	rows, err := s.conn.Query(ctx, sql, spanID)
	if err != nil {
		return WideEventRow{}, fmt.Errorf("query wide_events: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return WideEventRow{}, rows.Err()
	}

	var (
		r                        WideEventRow
		logTimestamps            []time.Time
		logSeverities, logBodies []string
	)
	if err := rows.Scan(
		&r.TraceID,
		&r.SpanID,
		&r.ParentSpanID,
		&r.ServiceName,
		&r.SpanName,
		&r.StartTime,
		&r.DurationNs,
		&r.StatusCode,
		&logTimestamps,
		&logSeverities,
		&logBodies,
		&r.Attributes,
		&r.BusinessAttributes,
		&r.RuntimeAttributes,
	); err != nil {
		return WideEventRow{}, fmt.Errorf("scan row: %w", err)
	}
	r.Logs = wideEventLogLines(logTimestamps, logSeverities, logBodies)
	return r, nil
}
