---
weight: 37
title: "WideEvent — Correlated Observability"
description: "Spec for WideEvent: a single CloudEvent that packs a span's trace context, logs, and processing attributes into one high-cardinality, efficiently-queryable record in Beacon"
icon: "merge_type"
draft: false
toc: true
---

## Overview

Beacon today stores logs, spans, and metrics as three separate signals (see
[Beacon — Observability](/docs/architecture/beacon-observability)), correlated
at query time via `trace_id`. That's the standard OTel shape, and it works —
but every cross-signal question ("show me everything that happened during
this failed request") costs a join across three tables.

**WideEvent** is a fourth, additive signal: one CloudEvent per span that
carries the span's own trace context *and* every log line emitted during it
*and* whatever business/runtime context the producing service wants to
attach — pre-joined, in a single record. This is the [wide event / canonical
log line](https://stripe.com/blog/canonical-log-lines) pattern: instead of
reconstructing "what happened" from many thin, separately-indexed rows,
emit one wide row per unit of work with every dimension you'll want to
slice by already sitting on it.

This document specs WideEvent's shape, its CloudEvent envelope, its
ClickHouse schema, and its production/consumption path. It does not yet
include a phased implementation plan — that follows once this spec is
approved, the same way [Beacon's own
doc](/docs/architecture/beacon-observability) separates system design from
its 18-phase build-out.

---

## Decisions

These were resolved before writing this spec; recorded here so the
rationale doesn't get re-litigated later.

**Grain: one WideEvent per span, not per trace.** Matches the granularity
Beacon's own `spans` table already uses — no buffering-until-trace-completes
problem, and correlation across child spans still works via
`trace_id`/`parent_span_id`, exactly like `spans` does today.

**Beacon writes it, not Catalyst.** Beacon subscribes as a Catalyst
`Consumer` of the `WideEvent` CloudEvent type and inserts into its own
ClickHouse database on receipt. This keeps a single-writer-per-database
invariant (only Beacon ever writes to the `beacon` database) and needs no
new machinery — it's the existing Producer/Consumer actor pattern, unchanged.

**Additive, not a replacement — for now.** The existing OTLP-based
logs/spans/metric_points ingestion keeps running untouched. WideEvent
duplicates a copy of the same trace/span/log content into one wide row
alongside it. This is intentional redundancy, not a migration; revisit once
there's real usage to justify collapsing the two paths.

**Reuse existing status/severity conventions.** No new "outcome" or
"exception" field. Consumers that want to react to failures filter on
`status_code` (same meaning as `Span.status_code` today) and per-log
`severity`, the same way any other Beacon consumer would.

**New `WideEventsService`, not an extension of `TracesService`.** A
WideEvent's shape (an embedded log array, three attribute maps) doesn't fit
`Span`/`TraceRoot` without changing messages every existing `TracesService`
consumer depends on. It gets its own Connect-RPC surface, mirroring
`LogsService`/`TracesService`/`MetricsService`'s own pattern.

**Attributes span both business and runtime context.** Both were in scope,
so they get separate columns/map fields (see [Data Model](#data-model))
rather than one flat map — keeps a query for "show me runtime memory
pressure during errors" from having to string-parse key prefixes to
distinguish it from "show me which user hit this."

**Web clients submit WideEvents through a dedicated RPC, which still
produces via Catalyst.** `chassis.Logger`'s span/log buffering (see
[Production path](#production-path)) is Go-only — a Dioxus/WASM web client
can never go through it, so it's the only way client-side telemetry
(browser errors, user-facing exceptions) enters this system at all. The new
RPC (see [Query surface](#query-surface)) does not write to ClickHouse
directly: it builds a `WideEvent` CloudEvent from the request and calls
`Producer.Produce` internally, the same as the chassis-instrumented path.
Skipping Catalyst here would make browser-submitted WideEvents invisible to
every other consumer built against this stream — defeating the reason this
uses the CloudEvent primitive in the first place.

---

## Data Model

### Proto

New package `core.observability.wide_events.v1`, sibling to
`logs`/`traces`/`metrics`:

```protobuf
// WideEvent is the payload of a WideEvent CloudEvent (see CloudEvent
// Envelope below) — one record per span, carrying its trace context, every
// log line emitted during it, and whatever attributes the producer attached.
message WideEvent {
    string                    trace_id        = 1;
    string                    span_id         = 2;
    string                    parent_span_id  = 3;
    string                    service_name    = 4;
    string                    span_name       = 5;
    google.protobuf.Timestamp start_time      = 6;
    uint64                    duration_ns     = 7;
    // Same meaning/values as Span.status_code (traces/v1) -- this is the
    // field exception-handling consumers filter on. No separate outcome
    // field.
    string                    status_code     = 8;
    repeated LogLine          logs            = 9;
    // OTel semantic-convention attributes -- same meaning as Span.attributes
    // in traces/v1, kept separate from the two attribute maps below so
    // standard OTel keys (http.method, etc.) don't mix with app-specific
    // context.
    map<string, string>       attributes            = 10;
    // Business/domain context the handling code knows about this unit of
    // work: user_id, workflow_run_id, request parameters, etc.
    map<string, string>       business_attributes   = 11;
    // Process/infra telemetry at the time of the span: goroutine count,
    // memory, host/pod identity, etc.
    map<string, string>       runtime_attributes    = 12;
}

message LogLine {
    google.protobuf.Timestamp timestamp = 1;
    string                    severity  = 2; // same values as LogRecord.severity_text (logs/v1)
    string                    body      = 3;
}
```

### CloudEvent envelope

Matches the existing convention every real producer in this repo already
uses (`services/tooling/catalyst-produce/rpc.go`'s `buildEvent`,
`services/tooling/bench/catalyst.go`'s `runEvent`/`stepEvent`) rather than
the CloudEvent proto's less-used alternatives:

| CloudEvent field | Value |
|---|---|
| `id` | `span_id` — CloudEvents wants `id` unique per event; `trace_id` is shared across many spans, `span_id` isn't. |
| `source` | `/services/<service_name>` — matches the path-like convention `catalyst-produce`'s own `defaultSource` (`/plugins/catalyst-produce`) uses. |
| `type` | `core.observability.wide_events.v1.WideEvent` — package + message name, matching `tooling.workflow.v1.RunStarted`'s own convention. |
| `data` | `TextData` — the `WideEvent` message JSON-marshaled via `protojson`, the same as every other typed payload in this repo (`examples/consumer/main.go` unmarshals `DatabaseModelSaved`/`UserCreated` the same way on the receiving end). Not `proto_data`/`Any` — nothing in this codebase uses that variant today. |
| `attributes["time"]` | A `ce_timestamp` extension attribute set to `WideEvent.start_time`, matching `buildEvent`'s own convention of surfacing time at the envelope level even though it's also in the payload — lets a consumer sort/filter without unmarshaling the body. |

### ClickHouse table (`beacon` database)

Same `MergeTree` + `PARTITION BY toYYYYMMDD(...)` + `TTL` shape every other
Beacon table already uses (see
[Beacon's Data Model](/docs/architecture/beacon-observability#data-model)),
`ORDER BY` low-cardinality → high-cardinality left to right so the sparse
primary index stays small and a `service_name` + time-range prefix (the
common case) prunes granules before anything else runs — `trace_id` isn't in
the sort key at all, same as `spans`, since an exact-match trace lookup
still runs fine within an already-pruned partition:

```sql
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
```

`Nested` (rather than `Array(Tuple(...))`) for `logs`: ClickHouse compiles a
`Nested` column into parallel arrays (`logs.timestamp`, `logs.severity`,
`logs.body`) that support `ARRAY JOIN` directly for flattening one
WideEvent's logs into queryable rows — more idiomatic here than a tuple
array for what's structurally "a sub-table per event."

`TTL` matches `spans`' own 14-day window as a starting point, since the
grain (one row per span) and expected volume are the same order of
magnitude — tune independently once there's real disk usage to look at,
same caveat [Beacon's own doc](/docs/architecture/beacon-observability#non-goals)
already makes about its TTLs.

### Query surface

```protobuf
service WideEventsService {
    // Same shape as TracesService.SearchTraces -- a bounded, paginated,
    // BeaconQL-filtered list, most recent first.
    rpc SearchWideEvents(SearchWideEventsRequest) returns (SearchWideEventsResponse) {}
    // Fetch one WideEvent by span_id, full logs/attributes included.
    rpc GetWideEvent(GetWideEventRequest) returns (GetWideEventResponse) {}
    // Submit a WideEvent directly -- the path for callers with no chassis
    // span to hang one off of. Primarily the Dioxus web clients (see
    // Production path: client submission), but open to any caller. Takes a
    // WideEvent message directly, not a raw CloudEvent -- the caller
    // shouldn't need to know about the envelope, Beacon builds it
    // internally and produces it via Catalyst like any other WideEvent.
    rpc CreateWideEvent(CreateWideEventRequest) returns (CreateWideEventResponse) {}
}

message CreateWideEventRequest {
    WideEvent event = 1;
}

message CreateWideEventResponse {
    string span_id = 1; // echoes WideEvent.span_id once accepted, or a server-generated one if the caller left it empty
}
```

BeaconQL filters against `wide_events`' typed columns
(`service_name`, `span_name`, `status_code`, `duration_ns`) plus map
indexing on all three attribute maps
(`business_attributes["user_id"] = "..."`), the same
`attributes["key"]` syntax `SearchTraces`/`QueryLogs` already support.

No new Fuse route needed for any of this: Beacon's existing route
(`Prefix: "/core.observability."`, see `services/core/beacon/main.go`)
already covers every service under the `core.observability.*` package
family, `WideEventsService` included — it's reachable the same way
`LogsService`/`TracesService`/`MetricsService` already are today.

---

## Production path

This is the piece with no existing chassis machinery to lean on, and the
most involved part of whatever implementation plan follows this spec:
building a `WideEvent` means knowing every log line emitted *during* a
specific span's lifetime, which means `chassis.Logger` needs a way to
attach an emitted log record to whatever span is active in the calling
context (via `context.Context`, the same way `chassis.StartSpan` already
threads trace context), buffered until that span ends. When the span's
`Finish`/`End` fires, chassis assembles the `WideEvent` from: the span's own
metadata, the buffered log lines, the span's existing OTel attributes, and
whatever `business_attributes`/`runtime_attributes` the caller attached —
then produces it as a CloudEvent via the existing `Producer.Produce` RPC.

Not specced further here on purpose — this is exactly the kind of thing
that belongs in the implementation plan, not the spec, once the shape above
is confirmed.

### Client submission

`CreateWideEvent` (see [Query surface](#query-surface)) is the second
production path, for callers with no chassis span to hang a WideEvent off
of — chiefly the Dioxus/WASM web clients, which never run chassis's
Go-native span/log buffering at all. A browser caller fills in whatever it
actually has — `service_name` as its own web-client identity
(`blueprint-web-client`, `beacon-web-client`, etc.), `span_name` for
whatever user-facing operation failed, `status_code`/`logs` for what went
wrong — and leaves the rest empty. `trace_id`/`parent_span_id` are only
populated when the client happens to have them (e.g. propagated from a
backend response header on the request that triggered the error); a
standalone client-side error report with no backend correlation is a valid
WideEvent with both fields empty, not a rejected one. If `span_id` is left
empty, Beacon generates one before producing the CloudEvent, since
CloudEvents requires `id` and nothing upstream of Beacon can be trusted to
provide a collision-free one from a browser.

---

## Non-Goals (v1)

- **No sampling.** Every span gets a WideEvent. Volume/cost implications of
  that are a follow-up once there's real usage to measure against, not a v1
  concern.
- **No controlled vocabulary for `business_attributes`/`runtime_attributes`
  keys.** Same posture as OTel semantic conventions generally: recommended
  naming, not enforced.
- **No replacing OTLP-based ingestion.** Explicitly additive (see
  [Decisions](#decisions)) — not attempted here.
- **No new Beacon web-client UI.** `WideEventsService` gets a query API in
  this spec; a dedicated view (if wanted) is separate follow-on work.

---

## Open Questions

Anything unresolved enough that it shouldn't block approving the shape
above, but should get an explicit answer before or during implementation:

- Does every chassis-instrumented span get a WideEvent automatically once a
  service opts in, or does opting in happen per-span (an explicit call
  alongside `StartSpan`)? Affects how big a "turn this on" change looks
  for an existing service.
- Log-buffering-per-span has a memory-growth failure mode for long-running
  or runaway spans (a span that never ends, or logs excessively, holds its
  buffered lines forever). Needs a cap or a max-span-duration cutoff before
  this ships.
- Does `Producer.Produce`'s existing broadcast-delivery model (per
  `CLAUDE.md`'s description of Catalyst) handle Beacon's consumption at the
  volume "every span" implies, or does this need its own delivery path?
- `CreateWideEvent` is a write RPC reachable from a browser, unlike
  everything else in this spec (query-only) — anyone who can reach Beacon's
  UI subdomain can call it. Does it need auth (`RouteAuth`/`AuthPolicy`
  already exists on Fuse's `Route` proto) or rate limiting before this
  ships, or is that over-engineering for a same-origin web-client call in
  v1? Worth a conscious answer either way, not a default-open oversight.
