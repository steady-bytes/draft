---
weight: 33
title: "Beacon — Observability"
description: "System design and implementation plan for Beacon, Draft's core observability service: logs, traces, and metrics"
icon: "sensors"
draft: false
toc: true
---

## Overview

Draft currently has no first-party story for logs, traces, or metrics. `pkg/chassis` gives every service a structured `Logger`, and [Operations & Telemetry](/docs/architecture/operations-telemetry) describes deriving event-bus topology from data Catalyst already stores — but there is no place logs go, no trace collection, and no system-resource metrics. Operators fall back to `docker-otel-lgtm`, an external stack bundled in the monorepo but not integrated with Draft's registration, routing, or UI conventions.

**Beacon** is a new core service that closes this gap: an [OpenTelemetry](https://opentelemetry.io/docs/)-native ingestion point for logs, traces, and metrics, backed by [ClickHouse](https://github.com/ClickHouse/ClickHouse), with a Dioxus web client served the same way Blueprint serves its own UI. It joins Blueprint, Catalyst, and Fuse as a fourth core service — see [Core Services](/docs/architecture/core-services).

This document is the system design and phased implementation plan. A companion visual design brief — wireframes for the three primary views, competitive positioning, and an architecture diagram — lives at [`assets/beacon-design-brief.html`](https://github.com/steady-bytes/draft/tree/main/assets/beacon-design-brief.html); open it directly in a browser to view.

### Why a new service instead of extending Catalyst

Catalyst already writes every `CloudEvent` to a ClickHouse `events` table (see `services/core/catalyst/broker/store.go`), which makes it tempting to route logs/traces/metrics through it too. Two things argue against that:

1. **Volume and shape mismatch.** Application events are business-domain messages, produced at business-domain rates. Telemetry — especially logs and traces — is produced continuously by every process at a rate one to three orders of magnitude higher, with a completely different schema (spans have parent/child relationships and durations; log lines have severity and free-form attributes; metrics are numeric time series). Forcing all three into the CloudEvent envelope means either a lossy encoding or a schema no consumer of the *event* bus actually wants to see.
2. **Catalyst's broadcast has an open, dated bug.** As documented in [Core Services § Known issues](/docs/architecture/core-services#known-issues), `Consume` currently fans every event out to exactly *one* random subscriber, not to all of them, once more than one stream is open for the same routing key. Telemetry ingestion cannot depend on that path being fixed or on being the single consumer.

Beacon therefore owns its own ingestion path, its own ClickHouse database, and its own query surface. It does not consume from or publish to Catalyst. The two services can be correlated at query time (a log line and a `CloudEvent` can share a `trace_id`), but they are architecturally independent — the same way Fuse and Catalyst are independent today.

---

## The Three Views

### Stream — live log tailing

A real-time log stream with line-by-line filtering. A query bar sits at the top of the page in both raw-expression and toggle/chip form (see [Query Language](#query-language) below), backed by a server-streaming RPC so new lines append live without polling. Filtering by `service_name`, `severity`, or any attribute added at log time narrows the stream without restarting it — the client re-issues the stream with the new filter and the server continues from the same cursor.

### Traces — flame graph

A query bar to locate traces (by service, span name, duration, status, or attribute), a result list of matching traces (root span, service, duration, span count — the SigNoz "trace list" pattern), and a flame graph for the selected trace: each span rendered as a horizontal bar, nested by parent/child relationship, width proportional to duration, color-coded by service. Clicking a span opens a detail panel with its attributes, events, and status.

### Metrics — system resources

Host and process resource usage (CPU, memory, disk, network, goroutine/GC stats for Go services) alongside the same throughput/latency style metrics Catalyst already derives for its own bus. Built from `MetricCard`-style stat tiles with sparklines — the same visual language already shipped in Blueprint's web client (`services/core/blueprint/web-client/src/components/metric_card.rs`) — plus larger time-series charts for drill-down.

---

## Query Language

The product notes flagged this as open: adopt PromQL wholesale, or go SQL-native given ClickHouse is the backing store? Neither, exactly — the right answer differs by signal type, and Draft already has a relevant precedent.

**Precedent already in the codebase:** Blueprint's web client queries Catalyst via a small filter-expression language exposed through `CesqlBar` (raw text input) and `QueryBuilder`/`FilterChips` (toggle construction) — see `services/core/blueprint/web-client/src/components/{cesql_bar,query_builder,filter_chips}.rs`. Users type expressions like `type = 'com.shop.order.created'` or build them from chips. This is the UX pattern Beacon should extend, not replace.

**Decision — two constrained languages, one shared UI shape:**

- **Logs & Traces: BeaconQL**, a WHERE-clause-style filter grammar (`service_name = "beacon" AND severity = "error" AND duration_ms > 200 AND attributes["route"] LIKE "/api/%"`). It is parsed server-side into a small AST and compiled to a **parameterized** ClickHouse query — user input is never string-interpolated into SQL. This keeps the grammar deliberately small (comparisons, `AND`/`OR`, `LIKE`, `IN`, attribute map indexing) rather than exposing ClickHouse's full SQL surface, which would both be an injection risk in a browser-facing tool and a much larger thing to parse safely.
- **Metrics: a scoped PromQL subset.** Metrics are where an industry-standard query language earns its keep — engineers already know PromQL, and Draft may eventually want Grafana or Alertmanager compatibility. Support range-vector selectors, `rate()`, `sum by(...)`, `avg`, `max`/`min`, and simple binary operators, translated to ClickHouse aggregate queries over `metric_points` (see [Data Model](#data-model)). This mirrors how SigNoz and Grafana Mimir front a non-Prometheus store with a PromQL parser.
- **Shared UI:** both signal types use the same query-bar shape — raw-expression mode plus toggle chips built from the parsed AST — so `Stream`, `Traces`, and `Metrics` feel like one product rather than three. `CesqlBar` is generalized into a `QueryBar` component parameterized by grammar (CESQL for Catalyst stays as-is; BeaconQL and PromQL-subset are new grammars behind the same component shape).

**Rejected alternatives:**
- *Raw ClickHouse SQL passthrough* — fastest to build, but a full SQL surface exposed to a browser client is a large injection and resource-exhaustion surface (unbounded joins, no query-cost limits) for very little product benefit over a constrained grammar.
- *PromQL for everything* — PromQL has no natural way to express "find me traces where status = error and duration > 200ms and service = X"; forcing log/trace search through range-vector semantics fights the data model.

---

## Data Model

Beacon owns a dedicated ClickHouse database (`beacon`, separate from Catalyst's `events` database) with three tables, following the same `MergeTree` + `PARTITION BY toYYYYMMDD(...)` shape Catalyst already uses in `store.go`, plus a `TTL` clause for retention (which Catalyst's `events` table does not currently have — Beacon needs it from day one given telemetry volume).

```sql
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
TTL toDateTime(timestamp) + INTERVAL 30 DAY;

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
TTL toDateTime(start_time) + INTERVAL 14 DAY;

CREATE TABLE IF NOT EXISTS metric_points (
    metric_name  LowCardinality(String),
    labels       Map(String, String),
    timestamp    DateTime64(3),
    value        Float64
) ENGINE = MergeTree()
ORDER BY (metric_name, timestamp)
PARTITION BY toYYYYMMDD(timestamp)
TTL toDateTime(timestamp) + INTERVAL 15 DAY;
```

`attributes`/`resource_attributes`/`labels` follow [OTel semantic conventions](https://opentelemetry.io/docs/specs/semconv/) directly — `service_name`, `severity_text`, etc. are lifted straight from the OTLP proto rather than invented, so Beacon interoperates with any standard OTel SDK or Collector out of the box.

A trace list needs its root span fast, without scanning every span of every trace. Add a materialized view (the same technique SigNoz uses) that maintains one row per `trace_id`:

```sql
CREATE MATERIALIZED VIEW IF NOT EXISTS trace_roots
ENGINE = ReplacingMergeTree()
ORDER BY trace_id
AS SELECT trace_id, service_name, span_name, start_time, duration_ns, status_code
FROM spans WHERE parent_span_id = '';
```

---

## Service Architecture

<div class="beacon-arch" role="img" aria-label="OTel SDKs and the chassis Logger send OTLP into Beacon's ingest batch writer. Ingest inserts into ClickHouse, and Beacon's Query/Stream RPCs read the same ClickHouse database back out, serving the Dioxus web client over gRPC-web routed through Fuse. Beacon registers itself with Blueprint.">
<style>
.beacon-arch{background:#0a0d10;border:1px solid #1c232b;border-radius:10px;padding:26px 22px;margin:24px 0;font-family:'JetBrains Mono',ui-monospace,'SFMono-Regular',Menlo,Consolas,monospace;color:#d7e0e8;overflow-x:auto;}
.beacon-arch *{box-sizing:border-box;}
.beacon-arch .ba-eyebrow{color:#4db380;font-size:11px;letter-spacing:.18em;margin-bottom:22px;white-space:nowrap;}
.beacon-arch .ba-grid{display:grid;grid-template-columns:200px 90px 250px 150px 220px;align-items:center;gap:0 4px;min-width:960px;}
.beacon-arch .ba-col{display:flex;flex-direction:column;gap:16px;}
.beacon-arch .ba-box{position:relative;background:#0e1318;border:1px solid currentColor;border-radius:2px;padding:10px 12px;}
.beacon-arch .ba-box::before,.beacon-arch .ba-box::after{content:'';position:absolute;width:6px;height:6px;border-color:currentColor;}
.beacon-arch .ba-box::before{top:-1px;left:-1px;border-top:1px solid;border-left:1px solid;}
.beacon-arch .ba-box::after{bottom:-1px;right:-1px;border-bottom:1px solid;border-right:1px solid;}
.beacon-arch .ba-row{display:flex;align-items:center;gap:8px;}
.beacon-arch .ba-badge{display:flex;align-items:center;justify-content:center;width:24px;height:24px;flex:0 0 24px;border:1px solid currentColor;font-size:10px;font-weight:700;}
.beacon-arch .ba-name{font-size:12px;letter-spacing:.05em;color:#d7e0e8;}
.beacon-arch .ba-sub{font-size:9.5px;color:#5d6a76;letter-spacing:.03em;margin-top:3px;}
.beacon-arch .ba-dim{color:#5d6a76;}
.beacon-arch .ba-green{color:#4db380;}
.beacon-arch .ba-blue{color:#58a6ff;}
.beacon-arch .ba-amber{color:#ffb454;}
.beacon-arch .ba-purple{color:#b48cff;}
.beacon-arch .ba-conn{display:flex;flex-direction:column;align-items:center;justify-content:center;gap:3px;height:100%;}
.beacon-arch .ba-conn-split{display:flex;flex-direction:column;justify-content:space-between;height:100%;padding:10px 0;}
.beacon-arch .ba-conn-item{display:flex;flex-direction:column;align-items:center;gap:3px;}
.beacon-arch .ba-arrow{font-size:20px;line-height:1;}
.beacon-arch .ba-conn-label{font-size:8.5px;letter-spacing:.1em;text-align:center;white-space:nowrap;}
.beacon-arch .ba-frame{position:relative;border:1.6px solid currentColor;border-radius:2px;padding:16px 14px;display:flex;flex-direction:column;gap:12px;}
.beacon-arch .ba-frame::before,.beacon-arch .ba-frame::after{content:'';position:absolute;width:8px;height:8px;border-color:currentColor;}
.beacon-arch .ba-frame::before{top:-2px;left:-2px;border-top:1.6px solid;border-left:1.6px solid;}
.beacon-arch .ba-frame::after{bottom:-2px;right:-2px;border-bottom:1.6px solid;border-right:1.6px solid;}
.beacon-arch .ba-io-note{font-size:8.5px;color:#5d6a76;text-align:center;letter-spacing:.03em;border-top:1px dashed #232a34;border-bottom:1px dashed #232a34;padding:6px 2px;}
.beacon-arch .ba-register{display:flex;flex-direction:column;align-items:center;gap:4px;grid-column:3;grid-row:2;justify-self:center;margin-top:24px;width:200px;}
.beacon-arch .ba-box-sm{width:100%;}
.beacon-arch .ba-legend{display:flex;flex-wrap:wrap;gap:8px 22px;margin-top:26px;padding-top:16px;border-top:1px solid #1c232b;font-size:9.5px;letter-spacing:.08em;}
.beacon-arch .ba-leg{color:#5d6a76;}
.beacon-arch .ba-leg b{margin-right:6px;}
</style>
<div class="ba-eyebrow">— SERVICE ARCHITECTURE / DATA FLOW</div>
<div class="ba-grid">
<div class="ba-col">
<div class="ba-box ba-dim"><div class="ba-row"><div class="ba-badge">Sd</div><div><div class="ba-name">OTel SDKs</div><div class="ba-sub">otel-lgtm collector</div></div></div></div>
<div class="ba-box ba-dim"><div class="ba-row"><div class="ba-badge">Lg</div><div><div class="ba-name">chassis.Logger</div><div class="ba-sub">every Draft service</div></div></div></div>
</div>
<div class="ba-conn"><div class="ba-arrow ba-green">→</div><div class="ba-conn-label ba-green">OTLP</div></div>
<div class="ba-col">
<div class="ba-frame ba-green">
<div class="ba-row"><div class="ba-badge">Bn</div><div><div class="ba-name">BEACON</div><div class="ba-sub">core service · 04</div></div></div>
<div class="ba-box ba-blue"><div class="ba-name">INGEST</div><div class="ba-sub">batch writer</div></div>
<div class="ba-io-note">↕ shared ClickHouse client — Ingest writes, Query/Stream reads</div>
<div class="ba-box ba-green"><div class="ba-name">QUERY / STREAM</div><div class="ba-sub">connect-rpc</div></div>
</div>
</div>
<div class="ba-conn-split">
<div class="ba-conn-item"><div class="ba-arrow ba-blue">→</div><div class="ba-conn-label ba-blue">INSERT</div></div>
<div class="ba-conn-item"><div class="ba-arrow ba-amber">→</div><div class="ba-conn-label ba-amber">gRPC-WEB<br>/ FUSE</div></div>
</div>
<div class="ba-col">
<div class="ba-box ba-blue"><div class="ba-row"><div class="ba-badge">Ch</div><div><div class="ba-name">CLICKHOUSE</div><div class="ba-sub">db: beacon</div></div></div></div>
<div class="ba-box ba-amber"><div class="ba-row"><div class="ba-badge">Wc</div><div><div class="ba-name">DIOXUS WEB CLIENT</div><div class="ba-sub">served by Beacon</div></div></div></div>
</div>
<div class="ba-register">
<div class="ba-arrow ba-purple">↓</div>
<div class="ba-conn-label ba-purple">REGISTERS</div>
<div class="ba-box ba-box-sm ba-purple"><div class="ba-row"><div class="ba-badge">Bp</div><div><div class="ba-name">BLUEPRINT</div></div></div></div>
</div>
</div>
<div class="ba-legend">
<div class="ba-leg ba-green"><b>■</b>OTLP</div>
<div class="ba-leg ba-blue"><b>■</b>INSERT</div>
<div class="ba-leg ba-amber"><b>■</b>gRPC-WEB / FUSE</div>
<div class="ba-leg ba-purple"><b>■</b>REGISTER</div>
</div>
</div>

*Beacon owns one ClickHouse database: Ingest writes to it, Query/Stream reads from it — the shared client is called out inside the frame rather than drawn as a separate trace.*

- **Ingestion is standard OTLP**, not a Draft-specific protocol. Beacon implements the OTLP gRPC receiver (`opentelemetry-proto`'s `LogsService`/`TraceService`/`MetricsService`) directly, so any existing OTel SDK — or the `docker-otel-lgtm` Collector already in this monorepo — can export straight to it with only an endpoint change. This is deliberate: it means Beacon can absorb or sit alongside the existing local observability stack rather than competing with it, and non-Go services (the Rust/Dioxus clients, `dioxus-grpc`) get an ingestion path for free via any language's OTel SDK.
- **Ingest → ClickHouse follows Catalyst's proven pattern**: a bounded channel, a ticker-driven batch flusher (`batchSize` / `flushInterval`), non-blocking `Save` that drops under sustained overload rather than back-pressuring producers. Reuse `services/core/catalyst/broker/store.go`'s shape almost verbatim, parameterized per table.
- **Query/Stream is Connect-RPC**, defined in `.proto` under `api/core/observability/{logs,traces,metrics}/v1/`, generated for Go and Rust exactly like every other Draft service (`dctl api build`). The Dioxus web client talks gRPC-web to Beacon the same way Blueprint's web client talks to itself and to Catalyst.
- **Web client is served by Beacon itself**, following Blueprint's pattern of a single process serving both the API and the compiled Dioxus/WASM bundle — not a separate static host.
- **Registration and routing follow every other core service**: Beacon registers with Blueprint at boot and its UI/API routes are published to Fuse. Suggested startup order: Blueprint → Catalyst → Fuse → **Beacon** → application services. Application services may start before Beacon comes up; OTLP export is fire-and-forget with local buffering/retry in the exporter, so a slow or not-yet-ready Beacon never blocks an application service's own startup.
- **chassis integration**: add an OTLP-exporting `Logger` implementation behind the existing `chassis.Logger` interface (`pkg/chassis/logger.go`) so switching a service's log output to Beacon is a config change, not a code change. Add a thin tracing/metrics helper alongside it (span-wrapping middleware for RPC handlers, a background reporter for Go runtime metrics) so instrumenting a service is a few lines, not a full OTel SDK integration exercise per service. The exporter finds Beacon's OTLP address the same way every other Draft service finds Catalyst or Fuse — a Blueprint service-discovery lookup at startup, not a hardcoded endpoint in `config.yaml`.
- **Non-Draft services need no chassis at all.** Because ingestion is plain OTLP (previous bullet), any microservice in any language ships logs/traces/metrics to Beacon by pointing its own OTel SDK's exporter at Beacon's address (e.g. the standard `OTEL_EXPORTER_OTLP_ENDPOINT` env var) — chassis only exists to make that automatic for Go services already using `chassis.Logger`, not to gate access to Beacon.

---

## Non-Goals (v1)

- **Not a full APM.** No anomaly detection, no synthetic monitoring, no real-user-monitoring (RUM). Beacon stores and lets you query what you send it; it does not decide what's anomalous.
- **Not a Collector reimplementation.** Beacon speaks the OTLP receiver protocol but does not build a general transform/processor pipeline. Sampling in v1 is simple head-based sampling at the SDK/exporter, not adaptive/tail sampling in Beacon.
- **Not a Catalyst replacement.** Catalyst remains the CloudEvent/application-event bus. Beacon is telemetry only, with an independent database and ingestion path (see [Why a new service](#why-a-new-service-instead-of-extending-catalyst)).
- **No cold-storage tiering in v1.** Retention is TTL-based deletion (30/14/15 days above); S3/object-storage tiering is future work.
- **No alerting/paging in v1.** Query and visualize only. Alertmanager-style rule evaluation is a plausible v2, once the PromQL-subset engine exists to evaluate against.

---

## Competitive Landscape (brief)

| | SigNoz | Grafana Tempo/Mimir/Loki | Graphite | **Beacon** |
|---|---|---|---|---|
| Storage | ClickHouse | Object storage + local | Whisper/Ceres | ClickHouse |
| Query | PromQL + ClickHouse SQL builder | PromQL/TraceQL/LogQL (3 languages) | Graphite functions | BeaconQL (logs/traces) + PromQL subset (metrics) |
| Ingestion | OTLP | OTLP (per-signal collectors) | Custom protocol | OTLP |
| Deployment | Separate product/cluster | Separate products per signal | Metrics-only | One core service, registered in your Draft cluster |
| UI | Standalone web app | Standalone (Grafana) | Standalone (Graphite-web) | Embedded in the Draft cluster's own UI conventions (Dioxus, gRPC-web, served like Blueprint) |

The differentiator is not the storage engine (ClickHouse is a well-worn choice, shared with SigNoz) — it's that Beacon is a Draft-native core service: it registers with Blueprint, routes through Fuse, and its UI shares Blueprint's component library and visual language, rather than being a third-party product bolted alongside the cluster.

---

## Implementation Plan

Phases are ordered so each is independently shippable and the earliest phases deliver a usable Stream view before Traces or Metrics exist at all.

| Phase | What | Unlocks |
|---|---|---|
| 0 | Proto contracts (`api/core/observability/...`) + `services/core/beacon` scaffold, chassis registration, ClickHouse `logs` table + migrate | Beacon exists as a registered core service with an empty log store |
| 1 | OTLP/gRPC log receiver + batch writer into `logs` | Any OTel SDK can send logs to Beacon |
| 2 | `QueryLogs`/`StreamLogs` Connect-RPCs + BeaconQL parser (logs grammar only) | Logs are queryable and streamable over gRPC-web |
| 3 | Web client **Stream** view: `QueryBar` (generalized from `CesqlBar`) + live tail | First visible product surface |
| 4 | OTLP/gRPC trace receiver, `spans` table, `trace_roots` materialized view | Trace data is captured |
| 5 | `SearchTraces`/`GetTrace` RPCs (BeaconQL extended to spans) | Traces are queryable |
| 6 | Web client **Traces** view: search list + flame graph rendering | Second visible product surface |
| 7 | OTLP/gRPC metrics receiver, `metric_points` table | Metric data is captured |
| 8 | PromQL-subset parser + `QueryMetrics` RPC (range vectors, `rate`, `sum by`) | Metrics are queryable |
| 9 | Web client **Metrics** view: `MetricCard` stat tiles + time-series drill-down | Third visible product surface |
| 10 | chassis `Logger` → OTLP exporter plugin + tracing/metrics helpers | Every Draft service can ship telemetry to Beacon with a config change |
| 11 | Fuse routing + retention/TTL tuning + `docker-otel-lgtm` interop pass | Beacon is a drop-in for or alongside the existing local stack |
| 12 | `before` cursor on `QueryLogs` (shared backend for 13 & 15) | Logs are queryable over a bounded time window, not just "recent N rows" |
| 13 | Web client **Logs** view: time range picker (relative presets + custom range) | Historical windows are queryable from the UI, not just live tail |
| 14 | Web client **Logs** view: Trace column + cross-view link into **Traces** | `trace_id` (already captured since Phase 1) is finally reachable from a log row |
| 15 | Web client **Logs** view: log detail panel (Overview / JSON / Context tabs) | Full row detail, `resource_attributes`, and chronological context are visible without leaving the row |
| 16 | Web client **Logs** view: click-to-filter on attribute values | Turning a value into a filter is a click, not hand-typed BeaconQL |
| 17 | Web client **Logs** view: volume/severity histogram | A quiet visual sense of spike shape before reading rows |

Phases 12&ndash;15 were a follow-up pass scoped to the three **high**-priority gaps identified against SigNoz's Logs Explorer (`assets/beacon-logs-redesign.html` has the full gap analysis and wireframes). Phase 16 begins the **medium**-priority items from that same analysis (click-to-filter, volume histogram, pause/resume, query-bar autocomplete), taken up one at a time rather than batched.

### Phase 0 — Scaffold `[Go]`

**Goal:** Beacon exists as a registered, empty core service.

**New paths:**
- `api/core/observability/logs/v1/logs.proto` (service + message shells, filled in Phase 2)
- `services/core/beacon/main.go`, `config.yaml`, `go.mod` — mirror `services/core/catalyst`'s `main.go` chassis bootstrap
- `services/core/beacon/store/clickhouse.go` — `logs` table `CREATE TABLE`/migrate, adapted from `catalyst/broker/store.go`

**How to test:** `dctl run` starts Beacon; it registers with Blueprint (visible in Blueprint's Service Registry view) and connects to ClickHouse with no ingestion traffic yet.

### Phase 1 — OTLP log ingestion `[Go]`

**Goal:** Beacon accepts standard OTLP log exports.

**New paths:**
- `services/core/beacon/ingest/logs.go` — implements `opentelemetry.proto.collector.logs.v1.LogsService/Export`, using `go.opentelemetry.io/proto/otlp` message types directly (no reinvented wire format)
- `services/core/beacon/ingest/writer.go` — bounded channel + ticker batch flush into `logs`, structurally identical to `clickhouseStore.flusher`/`insertBatch` in Catalyst

**How to test:** Point any OTel SDK's OTLP log exporter (or the `docker-otel-lgtm` Collector) at Beacon's gRPC port; rows appear in ClickHouse `logs`.

### Phase 2 — Query/Stream RPCs + BeaconQL (logs) `[Go]`

**Goal:** Logs are queryable through Beacon's own API, not raw ClickHouse access.

**New paths:**
- `api/core/observability/logs/v1/logs.proto` — `QueryLogs` (bounded result) + `StreamLogs` (server-streaming, cursor-based) filled in
- `services/core/beacon/query/beaconql.go` — parser producing a small AST (comparisons, `AND`/`OR`, `LIKE`, `IN`, map-index), compiled to parameterized ClickHouse SQL — never string concatenation

**How to test:** `QueryLogs` with an empty filter returns recent rows; a BeaconQL expression like `severity = "error" AND service_name = "beacon"` returns only matching rows; malformed expressions return a clear parse error, not a ClickHouse error.

### Phase 3 — Web client Stream view `[Rust/Dioxus]`

**Goal:** First visible page.

**New paths:**
- `services/core/beacon/web-client/` — new Dioxus crate, scaffolded like `blueprint/web-client`
- `services/core/beacon/web-client/src/components/query_bar.rs` — generalized from Blueprint's `cesql_bar.rs`, parameterized by grammar
- `services/core/beacon/web-client/src/views/stream.rs` — opens `StreamLogs`, appends lines live, applies `QueryBar` filter by re-issuing the stream

**How to test:** Open `/stream`, produce logs from a running service via the Phase 1 exporter, confirm lines appear live and filtering narrows the stream without a page reload.

### Phase 4 — OTLP trace ingestion `[Go]`

**Goal:** Span data is captured.

**New paths:**
- `api/core/observability/traces/v1/traces.proto`
- `services/core/beacon/ingest/traces.go` — `TraceService/Export`, batch writer into `spans`
- ClickHouse migration adds `spans` table + `trace_roots` materialized view

**How to test:** An OTel-instrumented service (or the OTLP trace exporter test fixture) sends spans; rows appear in `spans`, and `trace_roots` reflects one row per trace.

### Phase 5 — Trace query RPCs `[Go]`

**Goal:** Traces are searchable and individually fetchable.

**New paths:**
- `services/core/beacon/query/traces.go` — `SearchTraces` (BeaconQL over `trace_roots`, paginated) + `GetTrace` (all spans for one `trace_id`, ordered for flame-graph assembly)

**How to test:** `SearchTraces` with `duration_ms > 200` returns only slow traces; `GetTrace` for a known `trace_id` returns every span with correct `parent_span_id` relationships.

### Phase 6 — Web client Traces view `[Rust/Dioxus]`

**Goal:** Second visible page — flame graph.

**New paths:**
- `services/core/beacon/web-client/src/views/traces.rs` — search list + selected-trace detail
- `services/core/beacon/web-client/src/components/flame_graph.rs` — SVG rendering, spans as nested horizontal bars keyed by `start_time`/`duration_ns`, colored by `service_name`

**How to test:** Search for a known slow trace, select it, confirm the flame graph's nesting and widths match the span durations from `GetTrace`.

### Phase 7 — OTLP metrics ingestion `[Go]`

**Goal:** Metric points are captured.

**New paths:**
- `api/core/observability/metrics/v1/metrics.proto`
- `services/core/beacon/ingest/metrics.go` — `MetricsService/Export`, batch writer into `metric_points` (data points flattened from OTLP's gauge/sum/histogram shapes into `(metric_name, labels, timestamp, value)` rows; histograms flattened into `_bucket`/`_sum`/`_count` rows, matching Prometheus exposition convention so the PromQL-subset layer needs no special-casing)

**How to test:** An OTel metrics exporter (or a chassis host-metrics reporter from Phase 10) sends a gauge and a counter; both appear in `metric_points` with correct labels.

### Phase 8 — PromQL-subset + `QueryMetrics` `[Go]`

**Goal:** Metrics are queryable with a familiar syntax.

**New paths:**
- `services/core/beacon/query/promql.go` — parser for the supported subset (range-vector selectors, `rate()`, `sum by(...)`, `avg`/`max`/`min`, simple binary ops), compiled to ClickHouse aggregate queries over `metric_points`

**How to test:** `rate(http_requests_total[5m])` against seeded counter data returns the expected per-second rate; an unsupported PromQL construct returns a clear "unsupported" error rather than silently misinterpreting it.

### Phase 9 — Web client Metrics view `[Rust/Dioxus]`

**Goal:** Third visible page.

**New paths:**
- `services/core/beacon/web-client/src/views/metrics.rs` — stat-tile grid using Blueprint's existing `MetricCard` shape (ported into Beacon's client) + a time-series chart component for drill-down

**How to test:** Open `/metrics` on a cluster with a chassis host-metrics reporter (Phase 10) running; CPU/memory tiles show live, correct values.

### Phase 10 — chassis integration `[Go]`

**Goal:** Every Draft service can ship telemetry to Beacon with configuration, not code changes. (Non-Draft services don't need this phase at all — they already have a path: any OTel SDK exporting OTLP straight to Beacon's address, exactly like exporting to a Collector or Tempo today. This phase is purely the convenience layer for services already built on `chassis`.)

**New paths:**
- `pkg/chassis/otel_logger.go` — a `Logger` implementation that satisfies the existing interface in `logger.go` and exports via OTLP
- `pkg/chassis/otel_trace.go` — RPC-handler middleware that opens a span per request
- `pkg/chassis/otel_metrics.go` — background reporter for Go runtime metrics (goroutines, GC pause, heap) exported as OTLP gauges

**How it works:** A service doesn't hardcode Beacon's address. At startup, `otel_logger.go` resolves it via the same Blueprint service-discovery lookup every service already uses to find Catalyst and Fuse (`ServiceDiscoveryService.Query` for the registered `beacon` process, keyed off its Blueprint registration from Phase 0). This means Beacon can move, restart on a new address, or run in a different topology per environment without any service's `config.yaml` naming a specific host — only a boolean-ish "ship telemetry: on" toggle plus the log level, matching how other chassis plugins (Postgres, NATS) are configured. If the lookup fails or Beacon is unreachable, the exporter buffers briefly and drops rather than blocking the service's own request path — consistent with the fire-and-forget behavior described under [Service Architecture](#service-architecture).

**How to test:** Flip a service's `config.yaml` to enable the OTLP logger; its logs appear in the Stream view with no code change, resolved via Blueprint with no address configured by hand. Add the tracing middleware to one RPC handler; a trace for that call appears in the Traces view. Stop Beacon, restart the service — it starts cleanly rather than blocking on the discovery lookup, and begins shipping telemetry once Beacon is back and re-registered.

### Phase 11 — Fuse routing + retention + interop `[Go/infra]`

**Goal:** Beacon behaves like a first-class core service end-to-end and coexists with the existing `docker-otel-lgtm` stack.

**How it works:** Register Beacon's RPC routes with Fuse at startup via `chassis.WithRoute` — the same pattern `services/examples/{crud,echo,auth,file_host}/main.go` already use (a `Route{Match: &RouteMatch{Prefix: "..."}}` per service, matched against `/core.observability.{logs,traces,metrics}.v1.`). Note this is *not* the pattern Blueprint follows: Blueprint is Fuse's own bootstrap dependency (Fuse resolves its own address by querying Blueprint), so Blueprint's UI/API are reachable directly rather than Fuse-routed — Beacon has no such chicken-and-egg problem and should mirror the examples instead. Serve Beacon's web client from its own Go binary in production the same way `services/core/blueprint/main.go` does — a `//go:embed` over the Dioxus release build's output directory plus `chassis.WithClientApplication(files, ...)` — rather than relying on `dx serve`, which remains dev-only. Tune the `TTL` intervals in [Data Model](#data-model) against real disk usage. Document how to point `docker-otel-lgtm`'s Collector at Beacon instead of (or in addition to) its bundled Tempo/Loki/Prometheus, so existing local setups can migrate incrementally rather than needing a hard cutover.

**How to test:** A fresh `dctl infra start` cluster routes `/beacon` through Fuse/Envoy correctly; a cluster already running `docker-otel-lgtm` can be repointed at Beacon without losing existing signal.

#### `docker-otel-lgtm` interop

`docker-otel-lgtm`'s Collector (`docker-otel-lgtm/docker/otelcol-config.yaml`) already terminates OTLP on the same standard ports Beacon uses (`0.0.0.0:4317` gRPC, `:4318` HTTP) and fans each signal out to its bundled backends via `otlphttp/*` exporters — `otlphttp/traces` → Tempo, `otlphttp/metrics` → Prometheus's OTLP endpoint, `otlphttp/logs` → Loki's OTLP endpoint. Beacon speaks plain OTLP/gRPC directly (not `otlphttp`), so it slots in as one more exporter in that same Collector config rather than requiring any change to how services *send* telemetry:

```yaml
exporters:
  # ...existing otlphttp/{traces,metrics,logs} exporters...
  otlp/beacon:
    endpoint: <beacon-host>:4317
    tls:
      insecure: true

service:
  pipelines:
    traces:
      exporters: [otlphttp/traces, otlp/beacon]   # add alongside Tempo
    metrics:
      exporters: [otlphttp/metrics, otlp/beacon]  # add alongside Prometheus
    logs:
      exporters: [otlphttp/logs, otlp/beacon]     # add alongside Loki
```

Adding `otlp/beacon` alongside the existing exporters (rather than replacing them) dual-writes every signal to both stacks — an incremental migration with zero risk to whatever Grafana dashboards or alerts already point at the bundled LGTM stack. Dropping the original exporters once Beacon covers a team's needs is a one-line config change, not a cutover event. Services that export straight to `:4317` without going through this Collector (bypassing it entirely) can repoint at Beacon the same way — change the exporter endpoint, nothing else, since both speak the same unmodified OTLP wire protocol.

### Phase 12 — `before` cursor on `QueryLogs` `[Go]` — done

**Goal:** `QueryLogs` can bound a query on both ends of the timestamp axis, not just a lower bound. This single addition is the shared foundation both Phase 13 (a custom time range has a fixed end) and Phase 15 (the Context tab's "10 rows immediately *before* the selected row") build on — neither needs a new RPC, just this one field threaded through the existing one.

**Changed paths:**
- `api/core/observability/logs/v1/logs.proto` — added `string before = 4` (RFC3339, exclusive upper bound, empty means unbounded — doc-commented the same way as `after`) and `bool ascending = 5` (default `false`/descending, matching existing behavior; `true` needed for "the rows immediately *after* a cursor," which descending-with-`after` cannot express) to `QueryLogsRequest`. `StreamLogsRequest` is unchanged — it stays the open-ended live-tail path; any query with a fixed end goes through `QueryLogs` instead (see Phase 13).
- `services/core/beacon/store/clickhouse.go` — `Storer.QueryLogs`/`clickhouseStore.QueryLogs` gained a `before string` parameter (mirrors `after`); `noopStore.QueryLogs` updated to match.
- `services/core/beacon/query/controller.go` — `Controller.QueryLogs` gained `before string, ascending bool`, threaded straight through; `StreamLogs`'s internal historical-replay call updated for the new signature (`before` always empty there — unbounded above, as before).
- `services/core/beacon/query/rpc.go` — `QueryLogs` handler passes `req.Msg.GetBefore()`/`req.Msg.GetAscending()`.

**How it works:** `logs` is partitioned `PARTITION BY toYYYYMMDD(timestamp)` (see [Data Model](#data-model)), so a bounded `WHERE timestamp > ? AND timestamp < ?` prunes whole-day partitions outside the range before scanning — a custom range query stays cheap even without a timestamp-leading sort key. No migration needed; this was a query-path change only.

**Bug found and fixed along the way (not originally scoped, but the same file):** `clickhouse-go` v2's driver silently truncates a bare positional `time.Time` argument to whole-second precision — bound via a plain `?` placeholder, the driver has no way to know the destination is `DateTime64(9)` and defaults to `DateTime` (second precision, no fractional component). Verified directly: `timestamp < ?` bound this way against a cursor with 500 real matching rows returned exactly 1. This is a *pre-existing* bug — it silently affected the `after` cursor `StreamLogs`/`QueryLogs` already shipped with (Phase 2), and the identical pattern in `QueryTraceRoots`'s `before` cursor (Phase 5) and `QueryMetricPoints`'s `start`/`end` range (Phase 8), not something Phase 12 introduced. Fixed in all three places the same way: bind the timestamp as a string (`t.UTC().Format(time.RFC3339Nano)`) and cast it in SQL via `parseDateTime64BestEffort(?, 9)` instead of binding `time.Time` directly — verified against real ClickHouse data pre/post fix (500 rows recovered from the 1-row-truncated result; live tail confirmed still streaming correctly afterward).

**How to test:** `QueryLogs` with only `after` set behaves exactly as before (regression check — verified live-tailing still works end-to-end in the browser after the fix). `QueryLogs` with both `after` and `before` set returns only rows strictly between the two timestamps. `QueryLogs` with only `before` set (default `ascending=false`) returns the most recent rows older than that timestamp — the exact shape Phase 15's Context tab needs for "load more before." `QueryLogs` with `after` set and `ascending=true` returns the earliest rows immediately following the cursor, not the current tail — needed for Context's "load more after."

### Phase 13 — Web client Logs view: time range picker `[Rust/Dioxus]` — done

**Goal:** Query an explicit historical window instead of only ever tailing from "now."

**New paths:**
- `services/core/beacon/web-client/src/components/time_range.rs` — a `TimeRangePicker` component: the `Last 15m ▾` pill plus its dropdown (relative presets — 5m/15m/1h/6h/24h/7d — and a custom start/end pair), emitting a small `TimeRange` enum (`Live(Duration)` or `Custom { start, end }`) via `on_change: EventHandler<TimeRange>`.

**Changed paths:**
- `services/core/beacon/web-client/src/views/stream.rs` — the existing always-`StreamLogs` behavior becomes conditional on the active `TimeRange`:
  - `TimeRange::Live(d)` (the default, `Last 15m`) — same `StreamLogs` hand-rolled loop as today, but `after` is now seeded from `(now - d).to_rfc3339()` instead of an empty cursor, so switching presets actually changes what's shown rather than just labeling the same "recent N rows" replay. Live tailing continues exactly as it does today.
  - `TimeRange::Custom { start, end }` — a one-shot call through the already-generated `use_logs_service_service().query_logs(...)` hook (the same pattern `traces.rs` uses for `SearchTraces`/`GetTrace` — no hand-rolled client needed here, `QueryLogs` already has a codegen'd hook, only `StreamLogs` doesn't) with `after: start, before: end`. No live tail is opened; the connection badge shows "historical" instead of "live"/"connecting".

**How to test:** Selecting `Last 1h` after `Last 15m` shows strictly more rows going back further, still live-updating. Selecting a custom range in the past shows only rows inside that window, the live badge changes to "historical," and no new rows appear even while log traffic continues elsewhere in the cluster. Selecting `Last 15m` again returns to live tailing. Verified all of the above live in the browser.

**Known v1 limitation (flagged, not fixed here):** a custom range wider than one `QueryLogs` page (`maxLimit` rows) shows only the most recent page within that window — no "load older" pagination inside a fixed range. Worth a follow-up once this phase is in use; not blocking for the wireframe's scope.

**Bug found and fixed along the way:** the custom-range dropdown initially rendered cut off past the right edge of the viewport (only visible by scrolling). Root cause: this app loads Tailwind via the `@tailwindcss/browser` CDN runtime, which generates utility CSS by scanning/observing the DOM rather than from a precompiled stylesheet — a class appearing for the first time only when the dropdown first opens (`right-0`, `w-72`, etc.) isn't guaranteed to be styled on that first render. Fixed by moving the dropdown's layout-critical properties (position, right/top, width, z-index) to inline `style`, which applies immediately with no JIT dependency; cosmetic classes (colors, borders, shadow) were left as Tailwind classes.

### Phase 14 — Web client Logs view: Trace correlation link `[Rust/Dioxus]` — done

**Goal:** `trace_id` has been on every `LogRecord` since Phase 1 and is captured for every row, but is rendered nowhere in the UI. Surface it and make it a live link into the Traces view — no backend change needed at all: `SearchTraces`/`GetTrace` already accept a `trace_id`-scoped BeaconQL filter (Phase 5).

**New paths:**
- `services/core/beacon/web-client/src/components/trace_pill.rs` — a small `TracePill` component: renders a shortened `trace_id` (e.g. `a91d4f…9c206c`), `onclick` hands off to Traces (below). Renders nothing for an empty `trace_id`.

**Changed paths:**
- `services/core/beacon/web-client/src/main.rs` — added `PENDING_TRACE_ID: GlobalSignal<Option<String>>`, the same `Signal::global` pattern Blueprint's navbar uses for `BLUEPRINT_NAME`, as the hand-off channel between views (no query-string route param — this codebase has no existing precedent for Dioxus Router query segments, and a global signal is simpler for exactly this kind of one-shot cross-view state).
- `services/core/beacon/web-client/src/views/stream.rs` — added a Trace column rendering `TracePill`; `onclick` sets `PENDING_TRACE_ID` to the row's `trace_id` and calls `use_navigator().push(Route::Traces {})`.
- `services/core/beacon/web-client/src/views/traces.rs` — on mount, a `use_effect` checks `PENDING_TRACE_ID`: if set, seeds `search_req`/`expression` with `filter: trace_id = "<id>"`, immediately sets `selected_trace_id`/`get_req` so the flame graph opens without a second click, then clears the signal so a plain visit to `/traces` isn't affected.

**How to test:** Click a Trace pill on a log row with a real `trace_id`; the Traces view opens with that trace's flame graph already rendered, not just the search list filtered. Visiting `/traces` directly (nav link, not via a log row) behaves exactly as it does today — empty search, no trace pre-selected. Verified end-to-end in the browser using a synthetic log row (`INSERT`ed directly, then deleted afterward) paired with a real `trace_id` already present in `spans` — real RPC-triggered log rows currently have no `trace_id` to click at all (see below), so this was the only way to exercise the click-through honestly rather than just trusting the code.

**Gap found, then fixed (chassis, not Logs-view):** no row in `beacon.logs` had ever had a non-empty `trace_id` — confirmed directly (`SELECT count() FROM beacon.logs WHERE trace_id != ''` returned `0`). Phase 14's premise ("`trace_id` has been on every `LogRecord` since Phase 1") assumed the OTel logger correlates a log emitted during a traced RPC call with that call's trace; it didn't. Root cause: `NewTraceInterceptor()` (`pkg/chassis/otel_trace.go`) generated a `traceID`/`spanID` per request but never attached them to the request `context.Context` before calling the handler — and `OTelLogger.WithContext(ctx)` (`pkg/chassis/otel_logger.go`) was a documented, deliberate no-op ("v1 has no context-propagated trace correlation"). Fixed by closing that exact gap: `otel_trace.go` now has `withSpanContext`/`spanFromContext` (hex-encoded, an unexported context key), called in both `WrapUnary` and `WrapStreamingHandler` before invoking `next`; `WithContext` now reads them back and sets `"trace_id"`/`"span_id"` fields the same way an explicit `WithField` call already did (`encodeRecord` already looked for exactly those keys — no change needed there). Every existing `logger.WithContext(ctx)` call site in Blueprint's RPC handlers started correlating automatically the moment chassis rebuilt — zero caller changes. Verified against real traffic post-fix: `SELECT count() FROM beacon.logs WHERE trace_id != ''` went from `0` to `476` within a minute of restarting Blueprint, an `INNER JOIN` between `logs` and `spans` on `trace_id` returns matching rows with sane span names/durations, and clicking a real (non-synthetic) Trace pill in the browser opens the Traces view with the correct flame graph pre-rendered.

### Phase 15 — Web client Logs view: log detail panel `[Rust/Dioxus]` — done

**Goal:** A row can be inspected in full — untruncated body, both attribute maps (`resource_attributes` currently renders nowhere at all), and its immediate chronological neighbors — without leaving the Logs view.

**New paths:**
- `services/core/beacon/web-client/src/components/log_detail.rs` — a `LogDetailDrawer` component: right-side panel (`w-[420px]`), three tabs.
  - **Overview** — full `body` (with a wrap/truncate toggle, local component state) and two read-only attribute tables (`attributes`, `resource_attributes`, both skipped entirely when empty); a `TracePill` (Phase 14) if `trace_id` is non-empty. No click-to-filter affordances here — that interaction is a **medium**-priority item from the gap analysis and deliberately out of scope for this phase.
  - **JSON** — the selected `LogRecord`'s fields hand-formatted as pretty-printed JSON (matching this codebase's existing preference for hand-rolled formatting over pulling in `serde_json` as a new dependency purely for a debug view — `LogRecord`'s prost-generated struct doesn't derive `Serialize`; a small hand-rolled `json_string`/`json_object` pair handles escaping). No copy-to-clipboard button in this pass — cut for scope, not blocking.
  - **Context** — 10 rows immediately before and after the selected row, ignoring the active BeaconQL filter (matching SigNoz's Context tab behavior): "before" via `QueryLogs { before: selected.timestamp, ascending: false, limit: 10 }` (Phase 12), "after" via `QueryLogs { after: selected.timestamp, ascending: true, limit: 10 }`; "load 10 more before/after" repeats the call using the newly-earliest/latest displayed row's timestamp as the next cursor. The selected row itself renders inline, highlighted, between the two lists.
  - Given a `key` derived from the selected record's timestamp+body when rendered (see stream.rs below), Dioxus remounts a fresh `LogDetailDrawer` instance on every new row selection — resets `tab`/`wrap`/the Context lists/pagination state for free, no hand-rolled prop-change tracking needed.

**Changed paths:**
- `services/core/beacon/web-client/src/views/stream.rs` — rows are clickable (`onclick` sets a `selected: Signal<Option<LogRecord>>`, highlighted via `bg-base-300` while open); the table's container becomes a `flex` row with `LogDetailDrawer` as a sibling, narrowing the table rather than overlaying it.

**How to test:** Clicking a row opens the drawer with the full body visible (no ellipsis), both attribute maps populated (confirmed live: a row with `resource_attributes.service.name` set — silently dropped everywhere before this phase — now actually shows it), and a valid Trace link if the row has one. The JSON tab's output matches Overview. The Context tab shows the selected row highlighted among 10 neighbors on each side; "load 10 more before" extends the list further back in correct chronological order without duplicating the boundary row. Verified all of the above live in the browser against real streaming data (not a mock).

### Phase 16 — Web client Logs view: click-to-filter on attribute values `[Rust]` — done

**Goal:** Turn a value in the log detail panel's attribute tables into a BeaconQL clause with a click, instead of hand-typing it into the query bar.

**Changed paths:**
- `services/core/beacon/web-client/src/components/log_detail.rs` — `AttributeTable` (Phase 15) gained a `namespace: &'static str` prop (`"attributes"` or `"resource_attributes"`, since BeaconQL needs to know which map to index) and an `on_filter: EventHandler<String>` prop; each row now renders `+`/`−` buttons that call it with a ready-to-use clause (`attributes["key"] = "value"` / `attributes["key"] != "value"`). A small `beaconql_string` helper backslash-escapes `\`/`"` the same way `query/beaconql.go`'s `lexString` unescapes them on the way back in. Threaded up through `OverviewTab` and `LogDetailDrawer`'s own new `on_filter` prop. "Group by" from the original wireframe is intentionally not built — BeaconQL has no aggregation functions, so it would have nothing to do (see the gap table's "Aggregation / Group By" row, still low-priority/unstarted).
- `services/core/beacon/web-client/src/views/stream.rs` — `LogDetailDrawer`'s `on_filter` handler AND-combines the clause into the active `expression` and re-runs. The existing filter is parenthesized before combining (`(existing) AND clause`) — BeaconQL's `AND` binds tighter than `OR`, so appending a bare `AND clause` to a filter that already has a top-level `OR` would silently scope the new clause to only the `OR`'s last operand instead of the whole existing filter.

**Bug found and fixed along the way (pre-existing, not introduced by this phase):** a filter matching zero historical rows — with no live traffic arriving afterward either — left the Logs view stuck on "connecting…" forever, discovered while testing a click-to-filter selection narrow enough to match nothing. Root cause: connect-go's `ServerStream` doesn't flush response headers to the client until the first `Send` call; `query/rpc.go`'s `StreamLogs` handler only ever called `Send` for an actual row, so an empty result left the client's initial `stream_logs().await` hanging with nothing to resolve it, even though the RPC was healthy. The same gap could also delay a malformed-filter error from surfacing. Fixed by sending one empty `StreamLogsResponse{}` heartbeat immediately, before replaying history or waiting on live rows — `record` is optional and the web client's stream loop already no-ops on `record: None`, so it's invisible in the UI; it exists purely to force the header flush. Verified directly: before the fix, a zero-match filter hung 15+ seconds with no sign of resolving; after, the badge reads "live" within the same render.

**How to test:** Open a row's detail panel, click `+` next to an attribute value — the query bar updates to `attributes["key"] = "value"` and the list re-runs to match. Click `−` next to a different value with an existing filter already active — the bar shows `(existing) AND attributes["key"] != "value"`, correctly AND-combined regardless of whether the existing filter contains its own `OR`. A filter narrow enough to match nothing still flips the badge to "live" immediately rather than hanging on "connecting…".

### Phase 17 — Web client Logs view: volume/severity histogram `[Rust]` — done

**Goal:** A quiet visual sense of spike shape above the table before reading individual rows.

**New paths:**
- `services/core/beacon/web-client/src/components/severity_histogram.rs` — a `SeverityHistogram` component: 120 thin stacked bars (each `flex:1`, so raising the bucket count is purely a matter of trading individual bar width for time resolution — no layout changes needed), ~56px tall, framed as a labeled card (`bg-base-200`/`border-base-300`/`rounded-lg`, padded) rather than a bare strip — a chart with no title, no key, and no axis reads as decoration, not data, so all three were added in a follow-up polish pass:
    - An eyebrow title ("Volume by severity") and a color legend (error/warn/info/other swatches, top-right) so the stacked colors don't rely on the reader already knowing the row-marker palette by heart.
    - A start/end time axis along the bottom (`HH:MM:SS UTC`, from the same `min_ns`/`max_ns` the bucketing itself uses) so the strip reads as a chart *of* something instead of an abstract sparkline.
  - Buckets whatever rows are *currently displayed* client-side (the same `lines` buffer the table itself renders from, capped at `MAX_LINES`) rather than issuing a new aggregation query — BeaconQL has no aggregate functions (see the gap table's still-unstarted "Aggregation / Group By" row), and bucketing the exact rows already on screen guarantees the histogram and the table underneath it never disagree about what they're showing. Being a plain function of its `lines` prop, it re-buckets and re-renders for free on every new row the live tail receives.
  - Severities collapse into four stacked segments per bucket: `error`/`fatal` (`var(--color-error)`), `warn`/`warning` (`var(--color-warning)`), `info` (`var(--color-info)`) — the same theme-token colors `severity_stripe_color` already uses for the row markers, so the histogram and the rows read as the same visual language — and everything else (`debug`/`trace`/empty) as a faint `var(--color-base-content)` sliver so it doesn't compete for attention.
  - Segment heights are relative to the tallest bucket's total (`(count * CHART_HEIGHT_PX) / max_total`), and each bucket carries a `title` tooltip with its per-severity breakdown on hover. Renders nothing (not an empty chart) when fewer than two distinct timestamps are loaded — nothing meaningful to bucket yet.

**Changed paths:**
- `services/core/beacon/web-client/src/views/stream.rs` — mounted between the status-badge row and the table, passed `lines: displayed.clone()`.

**Known v1 limitation (flagged, not fixed here):** no click/drag-to-select a bucket range to narrow the time picker, unlike the original wireframe — a real interaction to build later, not just a styling pass; the histogram today is read-only.

**How to test:** With mixed-severity traffic in view, bars show visibly different heights and stacked colors matching what's in the table below (verified live: amber `warn` bars of varying height from the reaper's bursty cadence, plus a blue `info` segment for a real startup log line and a real `error` segment, colors and proportions consistent with the underlying rows). The title, legend, and `HH:MM:SS UTC` start/end labels render and match the visible time range. An idle window with only one or two rows loaded renders no card at all rather than a degenerate single-pixel chart.
