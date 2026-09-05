---
weight: 38
title: WideEvent Implementation Plan
description: Phased implementation plan for WideEvent — proto/schema, Beacon's consumer and query surface, the web-client insert RPC, and chassis's automatic per-span production
icon: checklist
draft: false
toc: true
---

This is the step-by-step implementation plan for [WideEvent — Correlated
Observability](/docs/architecture/wide-events), in the order work should be
done. Each phase produces a runnable, testable artifact. The order is
deliberate: the simpler, lower-risk surface (Beacon consuming and querying
a WideEvent someone else produced) comes first and is fully provable with a
hand-built CloudEvent before touching chassis at all; the riskiest piece —
automatic per-span capture, which needs new context-scoped log buffering
inside `pkg/chassis` — comes last, once everything downstream of it already
works.

File paths and existing code below are quoted directly from
`pkg/chassis/{otel_trace.go,otel_logger.go}`,
`services/core/beacon/{store/clickhouse.go,ingest/writer.go,query/{rpc.go,traces.go,beaconql.go},main.go}`,
`services/tooling/catalyst-produce/rpc.go`, and
`services/examples/consumer/main.go`.

---

## Phase 1 — Proto & Schema

Nothing downstream compiles until this exists. No behavior yet — this phase
is generated code plus a migration.

### 1.1 `api/core/observability/wide_events/v1/wide_events.proto`

```protobuf
syntax = "proto3";

package core.observability.wide_events.v1;

option go_package = "github.com/steady-bytes/draft/api/core/observability/wide_events/v1";

import "google/protobuf/timestamp.proto";

service WideEventsService {
    rpc SearchWideEvents(SearchWideEventsRequest) returns (SearchWideEventsResponse) {}
    rpc GetWideEvent(GetWideEventRequest) returns (GetWideEventResponse) {}
    rpc CreateWideEvent(CreateWideEventRequest) returns (CreateWideEventResponse) {}
}

message WideEvent {
    string                    trace_id       = 1;
    string                    span_id        = 2;
    string                    parent_span_id = 3;
    string                    service_name   = 4;
    string                    span_name      = 5;
    google.protobuf.Timestamp start_time     = 6;
    uint64                    duration_ns    = 7;
    string                    status_code    = 8;
    repeated LogLine          logs           = 9;
    map<string, string>       attributes           = 10;
    map<string, string>       business_attributes  = 11;
    map<string, string>       runtime_attributes   = 12;
}

message LogLine {
    google.protobuf.Timestamp timestamp = 1;
    string                    severity  = 2;
    string                    body      = 3;
}

message SearchWideEventsRequest {
    string filter = 1; // BeaconQL, same grammar as SearchTracesRequest.filter
    int32  limit  = 2;
    string before = 3;
}

message SearchWideEventsResponse {
    repeated WideEvent events = 1;
}

message GetWideEventRequest {
    string span_id = 1;
}

message GetWideEventResponse {
    WideEvent event = 1;
}

message CreateWideEventRequest {
    WideEvent event = 1;
}

message CreateWideEventResponse {
    string span_id = 1;
}
```

Run `dctl api build` (or `buf generate` directly) to regenerate Go, Rust,
and TypeScript bindings — same as every other `api/` change in this repo.
This alone should compile cleanly with zero other changes; nothing
references the new package yet.

### 1.2 ClickHouse table — `services/core/beacon/store/clickhouse.go`

Add alongside `createLogsTable`/`createSpansTable`/`createMetricPointsTable`
(same file, same constant-string-then-`migrate()`-call shape):

```go
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
TTL toDateTime(start_time) + INTERVAL 14 DAY;
`
```

```go
func (s *clickhouseStore) migrate(ctx context.Context) error {
	if err := s.conn.Exec(ctx, createLogsTable); err != nil {
		return err
	}
	if err := s.conn.Exec(ctx, createSpansTable); err != nil {
		return err
	}
	if err := s.conn.Exec(ctx, createTraceRootsView); err != nil {
		return err
	}
	if err := s.conn.Exec(ctx, createWideEventsTable); err != nil { // new
		return err
	}
	return s.conn.Exec(ctx, createMetricPointsTable)
}
```

### 1.3 `WideEventRow` and `Storer` interface

New row type, same shape as `LogRow`/`SpanRow`:

```go
WideEventRow struct {
	TraceID             string
	SpanID              string
	ParentSpanID        string
	ServiceName         string
	SpanName            string
	StartTime           time.Time
	DurationNS          uint64
	StatusCode          string
	Logs                []WideEventLogLine
	Attributes          map[string]string
	BusinessAttributes  map[string]string
	RuntimeAttributes   map[string]string
}

WideEventLogLine struct {
	Timestamp time.Time
	Severity  string
	Body      string
}
```

Add to the `Storer` interface (`clickhouse.go:83`), matching
`InsertSpans`/`QueryTraceRoots`'s own doc-comment convention:

```go
// InsertWideEvents writes a batch of rows. Called by ingest's WideEvent
// consumer (see Phase 2).
InsertWideEvents(ctx context.Context, rows []WideEventRow) error
// QueryWideEvents runs a bounded, parameterized query against the
// `wide_events` table. whereSQL is a fragment produced exclusively by
// query.CompileWideEvent (never raw user text) using `?` placeholders
// bound by args — see Phase 3.
QueryWideEvents(ctx context.Context, whereSQL string, args []any, limit int32, before string) ([]WideEventRow, error)
// GetWideEvent fetches one row by span_id, full logs/attributes included.
GetWideEvent(ctx context.Context, spanID string) (WideEventRow, error)
```

Implement `InsertWideEvents` following `InsertSpans`' exact
`PrepareBatch`/`Append`/`Send` shape (`clickhouse.go:437-465`) — the
`Nested` `logs` column takes parallel slices
(`logsTimestamp []time.Time, logsSeverity []string, logsBody []string`) on
`Append`, not a single composite value; build those three slices from
`WideEventRow.Logs` immediately before calling `Append`. Implement
`QueryWideEvents`/`GetWideEvent` following `QueryTraceRoots`/`GetTraceSpans`
(`clickhouse.go:466-524,580+`). Add the same three methods (no-op /
empty-result versions) to `noopStore` (`clickhouse.go:136-156`) so a
ClickHouse-disabled Beacon instance still compiles and runs, same as every
existing signal.

---

## Phase 2 — Beacon: Consuming WideEvents

Makes Beacon able to receive and store a WideEvent produced by *anyone* —
provable with a hand-built `CloudEvent` (e.g. `grpc-call`'d through Bench,
or a one-off test client) before Phase 4's `CreateWideEvent` or Phase 5's
chassis automation exist to produce one naturally.

### 2.1 `services/core/beacon/ingest/wide_events.go`

New file in the existing `ingest` package. `ingest`'s own doc comment
currently scopes it to "Beacon's OTLP log receiver and the batch writer" —
update that comment: this consumer's transport is Catalyst/CloudEvent, not
OTLP, but the package's actual charter ("gets an external signal batched
into ClickHouse") still fits; a separate package would just be the same
`Writer` shape under a different name.

The writer half is `ingest/writer.go`'s `Writer` verbatim, retyped to
`store.WideEventRow` (batch size/flush interval/channel capacity constants
unchanged — same volume-order-of-magnitude reasoning as `spans`).

The consumer half follows `services/examples/consumer/main.go`'s
`consumeType` exactly — one long-lived `Consumer.Consume` stream, filtered
to the `WideEvent` type, reconnecting on stream error rather than exiting:

```go
func ConsumeWideEvents(ctx context.Context, logger chassis.Logger, catalystAddr string, writer *WideEventWriter) {
	client := acConnect.NewConsumerClient(h2cClient(), catalystAddr, connect.WithGRPC())

	for {
		if ctx.Err() != nil {
			return
		}
		req := connect.NewRequest(&acv1.ConsumeRequest{
			Message: &acv1.CloudEvent{Source: "core-beacon", Type: wideEventType},
		})
		stream, err := client.Consume(ctx, req)
		if err != nil {
			logger.WithField("error", err.Error()).Error("wide_events: failed to open consume stream, retrying")
			time.Sleep(reconnectDelay)
			continue
		}
		for stream.Receive() {
			event := stream.Msg().GetMessage()
			if event == nil || event.Type != wideEventType {
				continue
			}
			var payload wideeventsv1.WideEvent
			if err := protojson.Unmarshal([]byte(event.GetTextData()), &payload); err != nil {
				logger.WithField("error", err.Error()).Error("wide_events: failed to unmarshal")
				continue
			}
			writer.Save(protoToRow(&payload))
		}
		if err := stream.Err(); err != nil && ctx.Err() == nil {
			logger.WithField("error", err.Error()).Error("wide_events: consume stream ended, reconnecting")
			time.Sleep(reconnectDelay)
		}
	}
}
```

`examples/consumer` doesn't need this reconnect loop (it's a fire-once demo
process); Beacon does, since a Catalyst restart shouldn't permanently stop
WideEvent ingestion until Beacon itself is also restarted.
`h2cClient()`/`reconnectDelay` are small enough to duplicate rather than
extract into `pkg/chassis` at this phase — revisit if a third service needs
the same consume-with-reconnect shape later.

### 2.2 Wire it up in `main.go`

`services/core/beacon/main.go` currently has no Catalyst dependency at all.
Add, alongside the existing `WithRunner` (blueprint's own
`registerBlueprintUIRoute` pattern, or bench's use of
`WithRunner`/scheduler):

```go
catalystAddr := cfg.GetString("catalyst.address")
if catalystAddr == "" {
	catalystAddr = "http://localhost:2220" // matches examples/consumer's own fallback
}
wideEventWriter := ingest.NewWideEventWriter(storer, logger)

// ... inside the chassis.New(...) builder chain:
WithRunner(func() {
	ingest.ConsumeWideEvents(chassis.Closer.Context() /* or equivalent ctx tied to shutdown */, logger, catalystAddr, wideEventWriter)
})
```

(Confirm the exact idiom for a shutdown-bound `context.Context` against
`chassis.Closer()`'s actual signature at implementation time —
`examples/consumer/main.go`'s `go func() { <-chassis.Closer(); cancel() }()`
pattern, reused as-is, is the safe default if there's no more direct
helper.)

### 2.3 Config

Add `catalyst.address` to `services/core/beacon/config.yaml` if it isn't
implied by a shared default — check whether other core services already
document this key centrally before duplicating it.

**How to test:** with Catalyst and Beacon both running, use `grpc-call`
(Bench) or a throwaway script to `Produce` a hand-built `WideEvent`
CloudEvent (`type: core.observability.wide_events.v1.WideEvent`, a
`TextData` JSON body). Confirm a row appears in `wide_events` via a direct
`clickhouse-client` query — no RPC surface needed yet to prove this phase.

---

## Phase 3 — Beacon: Querying WideEvents

### 3.1 BeaconQL — `query/beaconql.go`

Add `CompileWideEvent`, mirroring `CompileTraceRoot` (`beaconql.go:636`):
same `service_name`/`span_name`/`status_code`/`duration_ns` typed-column
handling, plus map-index support
(`business_attributes["key"]`/`runtime_attributes["key"]`/`attributes["key"]`)
extending whatever `CompileTraceRoot`/`Compile` already do for
`attributes["key"]` on `spans`/`logs`.

### 3.2 Controller — `query/wide_events.go`

New file, mirroring `query/traces.go`'s `TracesController` shape exactly:

```go
type (
	WideEventsController interface {
		SearchWideEvents(ctx context.Context, filter string, limit int32, before string) ([]store.WideEventRow, error)
		GetWideEvent(ctx context.Context, spanID string) (store.WideEventRow, error)
	}
	wideEventsController struct {
		logger chassis.Logger
		storer store.Storer
	}
)

func NewWideEventsController(logger chassis.Logger, storer store.Storer) WideEventsController {
	return &wideEventsController{logger: logger, storer: storer}
}

func (c *wideEventsController) SearchWideEvents(ctx context.Context, filter string, limit int32, before string) ([]store.WideEventRow, error) {
	whereSQL, args, err := CompileWideEvent(/* parsed filter expr, same flow as tracesController.SearchTraces */)
	if err != nil {
		return nil, err
	}
	return c.storer.QueryWideEvents(ctx, whereSQL, args, limit, before)
}

func (c *wideEventsController) GetWideEvent(ctx context.Context, spanID string) (store.WideEventRow, error) {
	return c.storer.GetWideEvent(ctx, spanID)
}
```

### 3.3 RPC — `query/rpc.go`

Add `wideeventsv1connect.WideEventsServiceHandler` to the `Rpc` interface,
`wideEventsController` to the `rpc` struct and `NewRPC`'s signature, and one
more `AddHandler` call in `RegisterRPC` (same `interceptor` reused, matching
the other three):

```go
wideEventsPattern, wideEventsHandler := wideeventsv1connect.NewWideEventsServiceHandler(h, interceptor)
server.AddHandler(wideEventsPattern, wideEventsHandler, true)
```

Implement `SearchWideEvents`/`GetWideEvent` following `SearchTraces`/`GetTrace`
(`query/traces.go:66-100`) exactly — call the controller, map
`store.WideEventRow` → `*wideeventsv1.WideEvent` (a `rowToProto`-shaped
function, including building the `LogLine` slice and all three attribute
maps), wrap in the response. `CreateWideEvent` is Phase 4, not this phase.

### 3.4 `main.go`

Pass the new controller into `query.NewRPC(...)` alongside the existing
three. No new `WithRoute` call needed — Beacon's existing
`Prefix: "/core.observability."` route (`main.go`'s first `WithRoute`) already
covers `core.observability.wide_events.v1.WideEventsService` as-is.

**How to test:** re-run Phase 2's manual produce-a-CloudEvent test, then
call `SearchWideEvents`/`GetWideEvent` (`curl` against the Connect-RPC JSON
endpoint, or Bench's `grpc-call`) and confirm the row comes back correctly
shaped, including a populated `logs` array and all three attribute maps.

---

## Phase 4 — `CreateWideEvent` (web-client submission)

The RPC signature already exists from Phase 1
(`WideEventsService.CreateWideEvent`) — this phase implements the handler.
Simpler than it looks: unlike Phase 5's automatic path, there's no
per-span log buffering to build here — the caller already hands over a
complete `WideEvent`, this just needs to produce it.

### 4.1 Beacon opens its own Producer stream

Same construction as `catalyst-produce/rpc.go`'s `NewHandler`
(`rpc.go:70-73`) — a long-lived stream opened once at startup, reused for
the process's lifetime:

```go
producerClient := acConnect.NewProducerClient(h2cClient(), catalystAddr, connect.WithGRPC())
producerStream := producerClient.Produce(ctx) // ctx tied to chassis.Closer(), same as Phase 2's consumer
```

### 4.2 Handler

```go
func (h *rpc) CreateWideEvent(ctx context.Context, req *connect.Request[wideeventsv1.CreateWideEventRequest]) (*connect.Response[wideeventsv1.CreateWideEventResponse], error) {
	event := req.Msg.GetEvent()
	if event.GetSpanId() == "" {
		event.SpanId = uuid.NewString() // CloudEvents requires a unique id; browsers can't be trusted to generate a collision-free one
	}
	if event.GetStartTime() == nil {
		event.StartTime = timestamppb.Now()
	}

	body, err := protojson.Marshal(event)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	cloudEvent := &acv1.CloudEvent{
		Id:          event.GetSpanId(),
		Source:      "/services/" + event.GetServiceName(),
		SpecVersion: "1.0",
		Type:        wideEventType,
		Data:        &acv1.CloudEvent_TextData{TextData: string(body)},
		Attributes: map[string]*acv1.CloudEvent_CloudEventAttributeValue{
			"time": {Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeTimestamp{CeTimestamp: event.GetStartTime()}},
		},
	}
	if err := h.produceWideEvent(cloudEvent); err != nil { // mutex-guarded Send, same as catalyst-produce's h.send
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&wideeventsv1.CreateWideEventResponse{SpanId: event.GetSpanId()}), nil
}
```

Matches `catalyst-produce/rpc.go:230-234`'s `send` for the mutex-around-Send
requirement — concurrent browser tabs can call `CreateWideEvent`
concurrently, and a Connect bidi stream's `Send` isn't safe for concurrent
use.

**How to test:** call `CreateWideEvent` directly (`curl`/Bench `grpc-call`)
with a minimal `WideEvent` (just `service_name`/`span_name`/`status_code`,
everything else empty — the client-submission case from the spec's own
"Client submission" section). Confirm it round-trips through Phase 2's
consumer into ClickHouse and is fetchable via Phase 3's `GetWideEvent`
using the `span_id` echoed back in the response. This proves the full
pipeline end to end using only Beacon-side code — nothing in `pkg/chassis`
needs to exist yet for this to pass.

### 4.3 Blueprint/Beacon web-client wiring

Once `dctl api build`'s TypeScript/Rust bindings exist (Phase 1), call
`CreateWideEvent` from wherever the web client wants to report a client-side
error — e.g. a top-level Dioxus error boundary, or explicit calls at known
failure points. Not further specced here: which web-client call sites
actually adopt this is a product decision for whoever owns that UI, not an
architecture question this plan needs to settle.

---

## Phase 5 — chassis: Automatic Per-Span Production

The most involved phase, exactly as flagged in the spec. Two chassis
subsystems need extending: `otel_trace.go`'s span lifecycle (both the
`NewTraceInterceptor` automatic path and the `StartSpan`/`Span.End` manual
path) and `otel_logger.go`'s `OTelLogger`, so a plain `logger.WithContext(ctx).Info(...)`
call made anywhere inside a traced span's lifetime gets captured into that
span's eventual `WideEvent` without the caller doing anything new.

### 5.1 Per-span log buffer

`spanContext` (`otel_trace.go:93-96`) currently carries only
`traceIDHex`/`spanIDHex` — two plain strings, no shared mutable state. Add a
pointer to a bounded, mutex-guarded buffer:

```go
// spanLogBuffer accumulates log lines emitted during one span's lifetime,
// for inclusion in that span's eventual WideEvent. Capped at maxBufferedLogs
// to bound memory for a pathological span that logs excessively or never
// ends (see the spec's own "Open Questions" — this is that cap).
type spanLogBuffer struct {
	mu       sync.Mutex
	lines    []wideEventLogLine
	overflow uint64
}

const maxBufferedLogs = 200

func (b *spanLogBuffer) append(line wideEventLogLine) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.lines) >= maxBufferedLogs {
		b.overflow++
		return
	}
	b.lines = append(b.lines, line)
}

func (b *spanLogBuffer) drain() ([]wideEventLogLine, uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lines, b.overflow
}

type spanContext struct {
	traceIDHex, spanIDHex string
	logBuf                *spanLogBuffer // new
}
```

`withSpanContext` (`otel_trace.go:102-107`) gains a `*spanLogBuffer`
parameter, allocated fresh by whichever of `WrapUnary`/`WrapStreamingHandler`
/`StartSpan` is creating this particular span — never inherited from a
parent span, so a child span's logs stay attributed to the child's own
`WideEvent`, matching the "one WideEvent per span, not per trace"
[decision](/docs/architecture/wide-events#decisions).

### 5.2 `OTelLogger` captures into the active buffer

`WithContext` (`otel_logger.go:790-796`) already reads `spanFromContext`;
extend it to also carry the buffer pointer onto the returned `*OTelLogger`
copy (`OTelLogger` gains an unexported `spanBuf *spanLogBuffer` field,
threaded through `WithFields`/`WithCallDepth`'s existing copy-construction
the same way `exporter`/`serviceName`/`level`/`fields` already are).
`emit` (`otel_logger.go:877-882`) appends to it, in addition to its existing
OTLP export — this is additive, not a replacement of the existing log path,
consistent with the spec's own additive framing:

```go
func (l *OTelLogger) emit(level LogLevel, msg string) {
	if l.spanBuf != nil {
		l.spanBuf.append(wideEventLogLine{
			timestamp: time.Now(),
			severity:  level.String(), // confirm LogLevel already has a String() matching severity_text's values
			body:      msg,
		})
	}
	if l.exporter == nil || !l.exporter.Enabled() || level > l.level {
		return
	}
	l.exporter.Export(logsExportProcedure, l.encodeRecord(level, msg))
}
```

A caller that never calls `WithContext(ctx)` gets `spanBuf == nil` and pays
zero cost beyond the nil check — matches every other telemetry path's
"free when unused" property in this file.

### 5.3 A chassis-internal WideEvent producer

New file, `pkg/chassis/wide_event.go`. Config-gated the same way
`OTelExporter.Enabled()` gates OTLP export (a `telemetry.wide_events.enabled`
key, or reuse `telemetry.enabled` if a separate toggle turns out to be
unwarranted — decide at implementation time based on whether anyone
actually wants OTLP tracing/logging without WideEvent, or vice versa).
Opens a `Producer.Produce` stream against `catalyst.address` lazily
(constructed once, on first use, via `sync.Once` — matching
`otel_trace.go:337-350`'s `manualExporter()` singleton-on-first-use shape
exactly, not eagerly at every service's startup regardless of whether it
ever emits a WideEvent) and fire-and-forget `Send`s, logging and dropping
on failure (this is an observability side-channel, not `catalyst-produce`'s
"publishing is the whole deliverable" case — a failed `Send` here must never
fail the caller's actual request).

### 5.4 Assemble and produce on span end

Both span-completion points call a new shared `buildAndProduceWideEvent`
helper:

- `otelInterceptor.reportSpan` (`otel_trace.go:301-313`) — after building
  and exporting the `Span` as it already does, also drain that request's
  `spanLogBuffer` (need to thread it through from `WrapUnary`/
  `WrapStreamingHandler`, which created it) and produce a `WideEvent`.
- `Span.End` (`otel_trace.go:402-418`) — same, for manually-started spans;
  `Span` itself needs a `logBuf *spanLogBuffer` field, set by `StartSpan`
  when it calls `withSpanContext`.

`business_attributes`/`runtime_attributes` need a way for calling code to
attach them — extend `Span.SetAttribute` (`otel_trace.go:391-396`) with
`SetBusinessAttribute`/`SetRuntimeAttribute` counterparts (the automatic
`NewTraceInterceptor` path has no equivalent caller-attaches-attributes
hook today, so it only ever populates `attributes`, i.e. OTel-semconv-style
data derived from the request itself — not a gap to fix in this phase,
just a scope boundary worth stating explicitly).

**How to test:** wrap a real RPC handler's `RegisterRPC` with
`NewTraceInterceptor` (already done for several services — pick one, e.g.
`services/examples/echo`), call it, and confirm a `WideEvent` lands in
`wide_events` with the request's own log lines (via
`logger.WithContext(ctx).Info(...)` calls inside the handler) correctly
attached. Separately, a manual-spans test: call `StartSpan`, log a few
lines through the returned context, call `End`, confirm the same. A third
test proves the cap: log more than `maxBufferedLogs` lines within one span
and confirm the resulting `WideEvent` has exactly `maxBufferedLogs` lines,
not a crash or unbounded growth.

---

## Phase 6 — Integration & Testing

### 6.1 Full round-trip test — done

Verified live against the running local stack, both automatic-production
paths, not just Phase 4's manual-CloudEvent proof:

- **`NewTraceInterceptor` path**: `services/examples/echo`'s `Speak` RPC
  (with `telemetry.wide_events.enabled: true` and its handler updated to
  `logger.WithContext(ctx)`, so its log line correlates) produced a
  `WideEvent` with the request's real OTel trace/span IDs, correct
  `duration_ns`, and the handler's own log line attached
  (`logs.body: ['received request']`) — queryable back out via
  `SearchWideEvents`/`GetWideEvent` through Fuse's existing Connect-RPC
  path, no new routing needed.
- **`StartSpan`/`Span.End` path**: `services/tooling/bench` (with the same
  config flag set) running its real `crud-e2e` workflow produced three
  correctly-correlated `WideEvent`s sharing one `trace_id` —
  `workflow:crud-e2e` (the run-level span) and its two children,
  `step:create-name`/`step:read-name` — confirming parent/child correlation
  survives the full pipeline (chassis → Catalyst → Beacon consumer →
  ClickHouse), not just the single-span case.
- **Log-buffer cap**: proven with a unit test
  (`pkg/chassis/otel_trace_test.go`) rather than a live overflow, since
  forcing 250 real log lines through one span is awkward to stage against a
  running service — `TestSpanLogBuffer_Cap` appends `maxBufferedSpanLogs +
  50` lines and asserts exactly `maxBufferedSpanLogs` are retained with
  `overflow == 50`; `TestSpanLogBuffer_UnderCap` confirms the common case
  (fewer lines than the cap) isn't truncated or padded. Both pass.

`echo`'s and `bench`'s `telemetry.wide_events.enabled: true` were left on
rather than reverted after testing — both are example/dev-tooling services
already, and having one of each production path (interceptor, manual)
demonstrably wired and working is a better default state for this repo than
reverting to an unproven one.

### 6.2 `CreateWideEvent` auth decision — resolved: no auth, matches existing status quo

Checked before deciding: **no route on Beacon has any `RouteAuth`/`AuthPolicy`
today** (`services/core/beacon/main.go` — neither the `core-beacon` prefix
route nor `core-beacon-ui` sets an `Auth` field). `LogsService`,
`TracesService`, and `MetricsService` are exposed at the same trust level
`CreateWideEvent` now is. This isn't a new gap `CreateWideEvent`
introduces — it's Beacon's existing, already-shipped posture, extended
consistently rather than made a one-off exception. Revisit if/when Beacon's
API surface gets an auth story at all; there's no reason for `CreateWideEvent`
to get one ahead of everything else Beacon already serves unauthenticated.

### 6.3 Volume/TTL sanity check — deferred, consistent with the spec's own Non-Goals

No real traffic exists yet to measure against — this is a local dev stack,
not a deployed cluster carrying production volume. Matches the spec's own
"No sampling... volume/cost implications are a follow-up once there's real
usage to measure against" Non-Goal exactly; revisit once a real deployment
has been running long enough for `wide_events`' 14-day TTL to matter.

---

## Milestone Summary

| Phase | Deliverable | Status |
|---|---|---|
| 1 | Proto generated, ClickHouse table, `Storer` interface extended | Done |
| 2 | Beacon consumes and stores any WideEvent CloudEvent | Done |
| 3 | `SearchWideEvents`/`GetWideEvent` queryable | Done |
| 4 | `CreateWideEvent` — web-client submission path | Done |
| 5 | Automatic per-span capture in `pkg/chassis` (both production paths) | Done |
| 6 | Auth: no-op, matches existing status quo. Volume/TTL: deferred, no real traffic yet | Done |

**Not done, out of scope for this plan** (per the spec's own Non-Goals and
Phase 4.3's explicit scope boundary): no Beacon web-client UI for
WideEvents, no web-client call sites actually wired up to `CreateWideEvent`
yet (a product decision for whoever owns that UI), and chassis's automatic
production remains opt-in per-service (`telemetry.wide_events.enabled`) —
nothing retroactively instruments every existing chassis service.
