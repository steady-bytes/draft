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

This document is the system design and phased implementation plan. A companion visual design brief — wireframes for the four primary views, competitive positioning, and an architecture diagram — lives at [`assets/beacon-design-brief.html`](https://github.com/steady-bytes/draft/tree/main/assets/beacon-design-brief.html); open it directly in a browser to view.

### Why a new service instead of extending Catalyst

Catalyst already writes every `CloudEvent` to a ClickHouse `events` table (see `services/core/catalyst/broker/store.go`), which makes it tempting to route logs/traces/metrics through it too. Two things argue against that:

1. **Volume and shape mismatch.** Application events are business-domain messages, produced at business-domain rates. Telemetry — especially logs and traces — is produced continuously by every process at a rate one to three orders of magnitude higher, with a completely different schema (spans have parent/child relationships and durations; log lines have severity and free-form attributes; metrics are numeric time series). Forcing all three into the CloudEvent envelope means either a lossy encoding or a schema no consumer of the *event* bus actually wants to see.
2. **Catalyst's broadcast has an open, dated bug.** As documented in [Core Services § Known issues](/docs/architecture/core-services#known-issues), `Consume` currently fans every event out to exactly *one* random subscriber, not to all of them, once more than one stream is open for the same routing key. Telemetry ingestion cannot depend on that path being fixed or on being the single consumer.

Beacon therefore owns its own ingestion path, its own ClickHouse database, and its own query surface. It does not consume from or publish to Catalyst. The two services can be correlated at query time (a log line and a `CloudEvent` can share a `trace_id`), but they are architecturally independent — the same way Fuse and Catalyst are independent today.

---

## The Four Views

Beacon shipped with three views (Stream, Traces, Metrics); a fourth — Events — was added once `chassis.StartSpan`/WideEvent production (Phase 18, and the Fuse native-proxy work that consumes it) gave the cluster a canonical-log-line-style record worth its own browsing surface, distinct from raw log lines. All four share one visual language — DaisyUI semantic surfaces, a monospace query bar, `MetricCard`-style stat tiles — so Beacon reads as one product across signal types, not four bolted-together tools.

### Events — WideEvent browser

A searchable table of [WideEvents](/docs/architecture/wide-events) (`services/core/beacon/web-client/src/views/wide_events.rs`) — one row per span, with `business_attributes`/`runtime_attributes` alongside the standard OTel fields. A `WideEventQueryBuilder` (`components/wide_event_query_builder.rs`) builds BeaconQL-shaped filters (`business_attributes["run_id"] = "..."`) from toggles rather than hand-typed expressions. Selecting a row opens a detail panel; a List/FlameGraph `ViewMode` toggle switches between the searchable table and a full-width, single-trace flame graph rendering of the same underlying data (`FlameGraph`, shared with the Traces view) — two ways to look at one data set, not two data sets. A `WideEventHistogram` gives the same "spike shape before reading rows" glance Stream's `SeverityHistogram` does.

### Stream — live log tailing

A real-time log stream with line-by-line filtering (`views/stream.rs`). A query bar sits at the top of the page in both raw-expression and toggle/chip form (`QueryBuilder`, see [Query Language](#query-language) below), backed by a server-streaming RPC so new lines append live without polling. Filtering by `service_name`, `severity`, or any attribute added at log time narrows the stream without restarting it — the client re-issues the stream with the new filter and the server continues from the same cursor. A `TimeRangePicker` bounds historical queries, a `TracePill` cross-links a log line's `trace_id` into the Traces view, a `LogDetailDrawer` gives full-row detail (Overview/JSON/Context tabs) with click-to-filter on any attribute value, and a `SeverityHistogram` shows volume/severity shape above the table (Phases 12–17, all done — see [Implementation Plan](#implementation-plan)).

### Traces — flame graph

A query bar to locate traces (by service, span name, duration, status, or attribute), a result list of matching traces (root span, service, duration, span count — the SigNoz "trace list" pattern), and a flame graph for the selected trace: each span rendered as a horizontal bar, nested by parent/child relationship, width proportional to duration, color-coded by service. Clicking a span opens a detail panel with its attributes, events, and status.

### Metrics — system resources

Host and process resource usage (CPU, memory, disk, network, goroutine/GC stats for Go services) alongside the same throughput/latency style metrics Catalyst already derives for its own bus. Built from `MetricCard`-style stat tiles with sparklines — the same visual language already shipped in Blueprint's web client (`services/core/blueprint/web-client/src/components/metric_card.rs`) — plus a `TimeSeriesChart` for drill-down.

**Current state (v1, shipped in Phase 9) is deliberately minimal**: one raw-PromQL-subset text field (no click-to-build, unlike Stream's `QueryBuilder` or Events' `WideEventQueryBuilder`), one one-shot query per click of "run" (no auto-refresh), and one implicit chart shape (`TimeSeriesChart` renders every returned series as overlaid lines — there is no chart-type choice). This is the gap [Metrics Graph Builder](#metrics-graph-builder) below closes.

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

## Metrics Graph Builder

The [Metrics view](#metrics--system-resources) shipped in Phase 9 with exactly enough to prove `QueryMetrics`/the PromQL-subset parser end to end: one raw-expression query bar, one one-shot query, one implicit chart shape. That was the right scope for Phase 9 — it is not a graph-building tool. This section designs the gap closed: **chart-type selection, a click-to-build query bar (matching the pattern Stream and Events already established for their own grammars), and a configurable polling interval**, so a single query result can be shaped and kept live the way an operator actually wants to watch it.

### Scope for v1

- **One graph at a time, not a multi-panel dashboard.** The ask is a builder for *a* graph — chart type, query, polling — not a Grafana-style canvas of many saved panels. A dashboard of several saved graphs is a natural v2 (see below), but building the multi-panel layout, drag-to-resize, and cross-panel time-range sync *before* the single-graph experience is solid would be building the harder problem first for no immediate benefit.
- **Persisted, in Blueprint, as a typed proto message — written via a Catalyst event, not a direct KV call.** A graph's configuration (query, chart type, window, refresh interval) is worth surviving a page refresh and being nameable/reloadable — this is genuinely in scope, not deferred. It's *config*, not *data*: the same category as Fuse's route table, Bench's workflow schedules, and everything else this framework already keeps in Blueprint's KV rather than each service inventing its own settings store. See [Persisting a graph](#persisting-a-graph) below for the concrete design — a real proto message (`BeaconMetricsGraphConfiguration`), mutated through a new generic mechanism ([Type Mutation Events](/docs/architecture/core-services#type-mutation-events)) any process with a registered type can reuse, not something bespoke to this one feature.
- **Chart types: Line, Bar, Area, Single Stat.** Covers the shapes PromQL-subset results actually take — a rate/gauge over time (Line, today's only option), a counter-ish quantity better read as discrete bars (Bar), a stacked contribution view for multiple label sets of one metric (Area), and "I just want the current number, big" (Single Stat, which is a `MetricCard` alone with no chart beneath it — already half-built, since `MetricStatTile` in `metrics.rs` already adapts a `TimeSeries` into a `MetricCard`).

### Why the query builder needs a new backend capability

Stream's `QueryBuilder` and Events' `WideEventQueryBuilder` both build a filter by letting the operator click a value that's *already on screen* — a log line's attribute, a WideEvent's JSON body field (see the Blueprint client's equivalent for CloudEvents, `services/core/blueprint/web-client/src/views/store.rs`'s `JsonTree`). There's no equivalent "already loaded object" for metrics: PromQL selectors are built from a metric *name* and *label keys/values*, and until a query has already run, nothing has loaded any of those into the browser to click on. A metrics query builder therefore needs the one thing Prometheus's own HTTP API provides for exactly this reason — label/metric discovery endpoints — which `MetricsService` doesn't have yet:

```protobuf
// ListMetricNames returns every distinct metric_name currently in metric_points
// (bounded by a server-side cap + optional prefix filter, the same shape
// Prometheus's own /api/v1/label/__name__/values serves).
rpc ListMetricNames(ListMetricNamesRequest) returns (ListMetricNamesResponse) {}

// ListLabelValues returns every distinct value seen for `label_name`, optionally
// narrowed to rows matching `metric_name` -- the same shape Prometheus's
// /api/v1/label/<name>/values serves, and how Grafana's own PromQL query
// builder populates its label/value dropdowns.
rpc ListLabelValues(ListLabelValuesRequest) returns (ListLabelValuesResponse) {}
```

Both compile to a bounded `SELECT DISTINCT ... FROM metric_points [WHERE metric_name = ?] LIMIT N` — cheap, and the same query shape ClickHouse already serves well for `LowCardinality` columns. No new table, no new ingestion path.

### `MetricQueryBuilder`

A new component, `services/core/beacon/web-client/src/components/metric_query_builder.rs`, matching `WideEventQueryBuilder`'s shape (field/operator/value pickers + AND/OR connector chips + live fragment preview, see `wide_event_query_builder.rs`'s `combine`/`format_fragment`) but sourcing its choices from discovery instead of a loaded object:

1. A metric-name `<select>` (or type-to-filter combobox once the list is large), populated from `ListMetricNames` on mount.
2. Once a metric is chosen, zero or more label filters (`label = "value"`), each a key `<select>` (populated from `ListLabelValues` scoped to that metric — Beacon doesn't have a "list label *keys*" endpoint yet either; simplest v1 is folding key discovery into `ListLabelValues`'s response, returning `map<string, LabelValues>` keyed by label name rather than a flat list, one query covering both) and a value `<select>`.
3. An optional aggregation wrapper (`rate(...)`, `sum by (...)`, bare selector) — a `<select>` over the small fixed set the PromQL-subset parser actually supports (`query/promql.go`'s own grammar is the source of truth here, not a hand-maintained duplicate list).
4. The usual live fragment preview + "Add to query" button, wired to the same `on_add: EventHandler<String>` shape every other query builder in this codebase already uses.

### `ChartTypeSelector` + `Chart` dispatcher

A `ChartType` enum (`Line | Bar | Area | SingleStat`) and a `Chart { chart_type: ChartType, series: Vec<TimeSeries> }` component that dispatches to a per-type renderer:

- **Line** — today's `TimeSeriesChart`, unchanged, just renamed/wrapped as the `Line` arm.
- **Bar** — new, generalizing the bucket/bar-rendering approach `SeverityHistogram`/`WideEventHistogram` already implement (stacked, proportional-height bars in a flex row) from "count per time bucket, colored by severity" to "value per sample, colored by series."
- **Area** — new: `Line`'s existing point-to-path SVG logic, with the region under each series' path filled at low opacity instead of (or in addition to) stroked — the smallest addition of the three, since it reuses `Line`'s own coordinate math wholesale.
- **SingleStat** — new, but not really: it's `MetricStatTile`'s existing `MetricCard` adaptation, just used *alone* (no `TimeSeriesChart`/`Chart` beneath it) when there's exactly one series and the operator has picked this chart type.

`ChartTypeSelector` itself is a small icon-button group (matching `ViewMode`'s List/FlameGraph toggle in `wide_events.rs` for the interaction pattern), stored as a `ChartType` signal the parent view reads when choosing which `Chart` arm to render.

### `PollingIntervalSelector`

A `<select>` (Off / 5s / 10s / 30s / 1m / 5m) driving the same polling mechanism Blueprint's own Metrics view already uses live today (`services/core/blueprint/web-client/src/views/metrics.rs`): a `gloo_timers::callback::Interval`, held in a `use_signal` so it isn't dropped at the end of the render function, incrementing a `tick: Signal<u32>` that a `use_effect` depends on to re-issue the query. Beacon's web client doesn't have `gloo-timers` as a dependency yet (Blueprint's does — `web-client/Cargo.toml`'s `gloo-timers = { version = "0.3", features = ["futures"] }`); add the same line to Beacon's own `Cargo.toml` rather than inventing a second timer mechanism.

No backend change is needed for polling itself: `QueryMetricsRequest` already carries `start`/`end`/`step` (Phase 8), so each tick re-issues `QueryMetrics` with a sliding window (`end = now`, `start = now - window`) rather than the server needing any notion of a standing subscription — metrics are polled, not streamed, in every comparable system (Prometheus, Grafana) for the same reason: a range query is cheap and idempotent, so there's no product benefit to the added complexity of a push-based `StreamMetrics` RPC.

### Correlating a graph with Events

A worked, real example, not a hypothetical: Fuse's native proxy backend tags every WideEvent it produces with `attributes["http.path"]` and `attributes["route.name"]` (`services/core/fuse/control_plane/native/backend.go`, `span.SetAttribute("http.path", r.URL.Path)` — already shipped, see [Wide Events](/docs/architecture/wide-events)). Say the graph builder is watching `rate(http_requests_total{service="fuse"}[5m])` (still the illustrative metric name used throughout this doc — no service emits it yet, see [Query Language](#query-language)) and it spikes. `service="fuse"` is as specific as that metric's labels get; `http.path` isn't one of them, deliberately — a raw request path is exactly the kind of high-cardinality value Prometheus/OTel convention keeps *off* metric labels (one time series per distinct path, forever, is how a metrics store falls over), which is precisely why Beacon has a separate, per-request WideEvent stream in the first place. The question "which path is causing this" is a real one the metric alone cannot answer — it has to be answered by pivoting into Events, not by trying to make PromQL join across two ClickHouse tables at query time.

Two mechanisms, one already-real and one proposed, cover this without inventing a cross-table query language:

- **Exemplars (proposed, Phase 24).** OTLP's wire format already lets a histogram/sum data point carry one or more `Exemplar`s — a sample value plus the `trace_id`/`span_id` that produced it — precisely so a metrics backend can link an aggregate back to one concrete request. `services/core/beacon/ingest/metrics.go` doesn't capture this today; it flattens data points into `(metric_name, labels, timestamp, value)` and drops the exemplar. Add nullable `exemplar_trace_id`/`exemplar_span_id` columns to `metric_points` (empty when the exporter sends none, which is most exporters today — chassis's own `otel_metrics.go` reporter doesn't emit exemplars yet either, so this is groundwork more than an immediate payoff), and `QueryMetrics`'s `Sample` message gains the same two optional fields. When present, a chart point is clickable straight through to `GetWideEvent(trace_id)` — the same "jump to the exact request" interaction Grafana's own exemplar support gives Prometheus users.
- **"View correlated events" (v1, no exemplar needed).** Always available, regardless of whether the metric source ever emits exemplars: a button beside the graph that opens the Events view with `service_name = "<metric's service label>"` pre-filled as a BeaconQL fragment (translated from whichever label the PromQL selector already scoped by) and the time range set to the graph's current `start`/`end` window. `WideEventQueryBuilder` already supports filtering by any `attributes[...]` key — the operator adds `attributes["http.path"] = "..."` themselves once there, the same click-to-build motion described under [`MetricQueryBuilder`](#metricquerybuilder) above, just on the Events side. This is the same cross-view pivot Stream's `TracePill` already does today (log row → Traces view by `trace_id`) — one more instance of an established pattern, not a new one.

Exemplars answer "show me the *exact* request behind this one point"; "View correlated events" answers "show me *every* request in this window and let me slice by whatever attribute turns out to matter" — a graph without any exemplar data yet still gets the second one for free, since it only depends on the metric's own labels and time range, both already known to the graph.

### Putting it together — `GraphConfig`

The `Metrics` view's state grows from two signals (`expression`, `query_req`) to one struct:

```rust
#[derive(Clone, PartialEq, Default)]
struct GraphConfig {
    id: Option<String>,      // None until saved once -- see Persisting a graph below
    name: String,            // operator-given display name, empty until saved
    query: String,           // PromQL-subset expression, built by MetricQueryBuilder or typed directly
    chart_type: ChartType,   // Line | Bar | Area | SingleStat
    window: Duration,        // lookback window for each poll's start/end
    refresh: Option<Duration>, // None = off, matching PollingIntervalSelector's "Off" option
}
```

`Metrics`'s existing `run_query` closure becomes the tick handler: on a manual "run" click *or* every `refresh` interval (when set), rebuild `QueryMetricsRequest{ query, start: now - window, end: now, step: <window/N> }` and re-fetch, exactly as today's one-shot version already does — the only change is *what* triggers that rebuild, and that the query string can now arrive from a click instead of only the keyboard. `id`/`name` are the only fields not already covered above — they exist purely for the save/load flow next.

### Persisting a graph

A `GraphConfig` is worth saving under a name and reloading later — the same "worth surviving a refresh" bar every other piece of cluster *configuration* in this framework already clears by living in Blueprint's KV (Fuse's route table, Bench's workflow schedules, the KV browser's own UI state), rather than each service growing its own settings store. It is emphatically not a place for the queried *data* — `metric_points`/ClickHouse stays exactly where it is; only the graph's own definition (query, chart type, window, refresh) is config.

**Written via a Catalyst event, not a direct `KeyValueService` RPC.** Rather than Beacon calling `Set`/`Delete` on Blueprint's KV directly, Beacon produces a CloudEvent describing the mutation and Blueprint consumes it — a generic mechanism, [Type Mutation Events](/docs/architecture/core-services#type-mutation-events), that *any* process with a type already registered via `RegisterType`/`WithRegisteredType` can use, not something specific to this one type. Reads stay exactly as direct `Get`/`List` RPCs (see that doc for why: Catalyst is one-way, and reads need a response back to a specific caller); only mutation goes through the event.

**A real proto message, not an untyped blob** — `api/core/observability/metrics/v1/metrics.proto` gains:

```protobuf
enum ChartType {
    CHART_TYPE_UNSPECIFIED = 0;
    CHART_TYPE_LINE        = 1;
    CHART_TYPE_BAR         = 2;
    CHART_TYPE_AREA        = 3;
    CHART_TYPE_SINGLE_STAT = 4;
}

// BeaconMetricsGraphConfiguration is a Metrics Graph Builder configuration,
// persisted in Blueprint's Key/Value store the same way
// core.control_plane.networking.v1.Route is -- the type lives with its
// semantic owner (Beacon, here) even though Blueprint is where it's actually
// stored; Blueprint's KV store is deliberately schema-agnostic and doesn't
// define types for what it holds. Mutated via a TypeMutation CloudEvent (see
// core.registry.key_value.v1.TypeMutation), not a direct KeyValueService
// call -- see this doc's "Persisting a graph" section.
message BeaconMetricsGraphConfiguration {
    string id   = 1; // stable identity, generated at creation, used as the KV key
    string name = 2; // operator-given display name, e.g. "Fuse error rate"

    string query            = 3; // PromQL-subset expression
    ChartType chart_type    = 4;
    string window           = 5; // Go-duration string, e.g. "15m" -- same string-duration
    string refresh_interval = 6; // convention QueryMetricsRequest's own start/end/step already use;
                                  // empty string means refresh is off

    google.protobuf.Timestamp created_at = 7;
    google.protobuf.Timestamp updated_at = 8;
}
```

This mirrors `Route`'s own placement: a `Route` is defined in `core.control_plane.networking.v1` (Fuse's package, since Fuse owns what a route *means*) despite being stored generically in Blueprint's KV. `BeaconMetricsGraphConfiguration` follows the identical convention — defined in Beacon's own `metrics/v1` package, stored generically in Blueprint's KV, keyed by `"beacon_metrics_graph_configuration_" + id`.

**The generic event envelope** — new, in `api/core/registry/key_value/v1/service.proto` (alongside `TypeDescriptor`/`RegisterType`, the schema-registration mechanism this builds on):

```protobuf
message TypeMutation {
    enum Action {
        ACTION_UNSPECIFIED = 0;
        ACTION_CREATE       = 1;
        ACTION_UPDATE       = 2;
        ACTION_DELETE       = 3;
    }
    Action action = 1;
    // The KV key this mutation applies to -- same string Get/Set/Delete already take.
    string key = 2;
    // The typed value to store, for CREATE/UPDATE. Absent for DELETE -- only
    // `key` matters then. Blueprint decodes this using the same
    // RegisterType-backed descriptor DecodeValues already uses for
    // value.type_url; an unregistered type_url is logged and dropped, not
    // applied.
    google.protobuf.Any value = 3;
}
```

Produced as a CloudEvent of type `"core.registry.key_value.v1.TypeMutation"` (matching WideEvent's own CloudEvent-type-string convention). Blueprint runs exactly one consumer for this type, generic across every registered proto message — see [Type Mutation Events](/docs/architecture/core-services#type-mutation-events) for the consumer side; this doc covers Beacon's own use of it.

**A new chassis producer helper**, since this is a framework capability, not Beacon-specific plumbing — `pkg/chassis/type_mutation.go`:

```go
// PublishTypeMutation produces a TypeMutation CloudEvent for Blueprint's generic consumer to
// apply. Fire-and-forget, matching Catalyst's own broadcast nature (see Type Mutation Events) --
// a successful return means the event was handed to Catalyst, not that Blueprint has applied it
// yet. msg is nil for ACTION_DELETE.
func PublishTypeMutation(action kvv1.TypeMutation_Action, key string, msg proto.Message) error
```

Mirrors `wide_event.go`'s own shape almost exactly: a lazy, `sync.Once`-initialized `BidiStreamForClient` held for the process's lifetime (opening a fresh producer stream per call would be wasteful for something a UI action can trigger repeatedly), the same "stream doesn't reconnect if Catalyst restarts" caveat already documented for WideEvents applies here too, and the same fire-and-forget error-handling convention (log and drop, never propagate to the caller's own request path) — reusing a known, understood limitation rather than introducing a new one.

**Beacon's own wiring:**

```go
c := chassis.New(logger).
    WithRegisteredType(&metricsv1.BeaconMetricsGraphConfiguration{}).
    // ...
```

registers the schema once at startup (`KeyValueService.RegisterType`, purely so Blueprint's own KV browser UI can decode and display these values instead of showing an opaque byte count — has no effect on whether saving/loading actually works). Beacon's web client has never called another service's backend before (`API_DOMAIN`, `services/core/beacon/web-client/src/main.rs`, is same-origin only), so it gets one new unary RPC on Beacon's own backend, `MetricsService.SaveGraphConfiguration`/`DeleteGraphConfiguration` — the web client calls these (same-origin, no new cross-service web-client wiring needed at all), and the Go handler behind them is what actually calls `chassis.PublishTypeMutation`. Reads still go directly to Blueprint's `KeyValueService.List`/`Get`, which needs the same cross-origin path `CATALYST_DOMAIN` already established for Blueprint's own web client (a plain `option_env!`-overridable constant, cross-origin, no Fuse routing — every chassis service already wires in permissive CORS by default, `pkg/chassis/builder.go`'s `buildCors`): Beacon's web client gains `BLUEPRINT_DOMAIN` (default `http://localhost:2221`) for exactly this.

**Save/Load UI**, alongside `ChartTypeSelector`/`PollingIntervalSelector`: a "Save" button (prompts for a name if `GraphConfig.name` is empty, calls `SaveGraphConfiguration` with the current config — Beacon's backend fills in `id`/`created_at` on first save or preserves them on a re-save before publishing the mutation) and a "Load" dropdown (`List`s Blueprint directly by `BeaconMetricsGraphConfiguration`'s type URL on mount, matching how Fuse's own `listRawRoutes` lists routes by type URL) that replaces the current `GraphConfig` wholesale when a saved graph is picked. Per the fire-and-forget model, "Save" shows success as soon as Beacon's own RPC returns (confirming the event reached Catalyst) — not confirmation that Blueprint has applied it yet; a save that's somehow rejected (malformed payload, an unregistered type) surfaces only in Blueprint's own logs, not back to the button.

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
| 18 | `chassis.StartSpan` + cross-process `traceparent` propagation; Bench end-to-end workflow tracing | Background work outliving its inbound handler gets real spans; any two chassis services can hand a trace across a network call |
| 19 | `ListMetricNames`/`ListLabelValues` RPCs `[Go]` | Metrics gain the discovery surface a click-to-build query bar needs |
| 20 | Web client **Metrics** view: `MetricQueryBuilder` `[Rust/Dioxus]` | Building a PromQL-subset query is a series of clicks, not hand-typed syntax |
| 21 | Web client **Metrics** view: `ChartTypeSelector` + `Chart` dispatcher (Line/Bar/Area/SingleStat) `[Rust/Dioxus]` | A query result can be shaped as the chart that actually fits it |
| 22 | Web client **Metrics** view: `PollingIntervalSelector` `[Rust/Dioxus]` | A graph stays live at an operator-chosen cadence instead of one manual "run" per look |
| 23 | Web client **Metrics** view: "View correlated events" pivot `[Rust/Dioxus]` | A metric spike answers "which requests" by pivoting into Events, pre-filled by label and time window |
| 24 | OTLP exemplar capture + `QueryMetrics` exemplar fields `[Go]` | A single chart point can link straight to the exact WideEvent that produced it |
| 25 | Blueprint: generic `TypeMutation` Catalyst consumer `[Go]` | Any process with a registered type can mutate it over Catalyst instead of a direct KV RPC — a framework capability, not Beacon-specific |
| 26 | `BeaconMetricsGraphConfiguration` proto + `chassis.PublishTypeMutation` + Beacon's Save/Delete RPCs `[Go]` | A graph's config has a real, typed shape, and Beacon can persist one via Phase 25's new mechanism |
| 27 | Web client Metrics view: Save/Load a graph configuration `[Rust/Dioxus]` | A graph survives a page refresh and is nameable/reloadable, stored in Blueprint like every other piece of cluster config |

Phases 12&ndash;15 were a follow-up pass scoped to the three **high**-priority gaps identified against SigNoz's Logs Explorer (`assets/beacon-logs-redesign.html` has the full gap analysis and wireframes). Phase 16 begins the **medium**-priority items from that same analysis (click-to-filter, volume histogram, pause/resume, query-bar autocomplete), taken up one at a time rather than batched. Phases 19&ndash;27 are the [Metrics Graph Builder](#metrics-graph-builder) work: chart-type selection, a click-to-build query bar, configurable polling, [correlation with Events](#correlating-a-graph-with-events), and [persistence](#persisting-a-graph) for the Metrics view, mirroring the click-to-build/live-refresh/cross-view-pivot conveniences Stream and Events already have. Phase 25 is the one piece that isn't Metrics-specific at all — see [Type Mutation Events](/docs/architecture/core-services#type-mutation-events) — everything from Phase 26 on is Beacon's own use of it.

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

### Phase 18 — Manual spans (`chassis.StartSpan`) + Bench end-to-end workflow tracing `[Go]` — done

**Goal:** `NewTraceInterceptor` (Phase 10) only ever covers the synchronous body of one inbound RPC handler call — exactly right for a plain request/response endpoint, wrong for a handler that kicks off background work and returns before that work is done. Bench's `Scheduler.StartRun` is the motivating case: `TriggerRun`'s handler returns as soon as `beginRun`'s single DB write finishes (by design — see `scheduler.go`'s doc comments), while the workflow's steps keep running in a detached goroutine, on `context.Background()`, for the run's whole lifetime. The span the interceptor reported for that call covered a few milliseconds of bookkeeping, not the actual workflow — confirmed directly: three `TriggerRun` spans in `beacon.spans` measured 2–5ms each while the workflows they kicked off ran for hundreds of milliseconds, and zero spans existed anywhere for the individual steps. This phase gives any chassis service a way to create spans outside of an inbound-RPC-handler's synchronous scope, and uses it to give Bench real per-workflow/per-step tracing.

**New paths:** none — everything is added to the existing Phase 10 files.

**Changed paths:**
- `pkg/chassis/otel_trace.go`:
  - `chassis.StartSpan(ctx, name string) (context.Context, *Span)` — starts a new span and returns a `ctx` carrying it. If `ctx` already carries a span (from an inbound `NewTraceInterceptor`-wrapped handler, a propagated inbound `traceparent` header, or an outer `StartSpan` call), the new span is a **child** in that same trace; otherwise it starts a new trace. `(*Span).SetAttribute(key, value string)` attaches string attributes; `(*Span).End(err error)` reports the span to Beacon with an OK or ERROR status.
  - `encodeSpan` gained a `parentSpanID []byte` parameter (wire field 4, previously always omitted) so a span can declare its parent — needed for both nested `StartSpan` calls and continued cross-process traces.
  - **Cross-process propagation**, using the standard W3C [`traceparent`](https://www.w3.org/TR/trace-context/#traceparent-header) header (`00-{trace-id}-{parent-id}-{flags}`) rather than a bespoke format, so any two chassis services — or a future non-Go one — can hand a trace across a network call:
    - `chassis.TraceParentHeader(ctx) (string, bool)` — builds the header value for `ctx`'s current span, for a caller using a plain `net/http` request (no generated connect-go client to attach an interceptor to).
    - `chassis.NewTraceClientInterceptor()` — a connect-go client interceptor that sets the header automatically on every outbound unary/streaming call; pass it to `connect.NewXxxClient(httpClient, baseURL, connect.WithInterceptors(chassis.NewTraceClientInterceptor()))`.
    - `NewTraceInterceptor`'s `WrapUnary`/`WrapStreamingHandler` (the *receiving* side, unchanged call shape) now check the inbound request for a `traceparent` header first — if present and well-formed, the request continues that trace (its span becomes a child of the incoming one) instead of unconditionally minting a new, disconnected trace id the way it always did before this phase.
  - A process-wide `*OTelExporter` singleton (`sync.Once`) backs `StartSpan`/`Span.End` — `newOTelExporter` starts a background send-loop goroutine and its own HTTP client per call, so `StartSpan` must not repeat that per span the way `NewTraceInterceptor`/`NewOTelLogger` each do once at their own construction time.
- `services/tooling/bench/scheduler.go` — `Scheduler.execute` opens a root span (`workflow:<name>`, attributes `workflow_name`/`run_id`) around the whole DAG walk, ended OK/ERROR based on `runFailed` — this is the span `TriggerRun`'s own short-lived interceptor span was never long enough to be. `Scheduler.runStep` opens a child span (`step:<name>`, attributes `step_name`/`uses`) per step, ended OK/ERROR based on that step's pass/fail outcome, and passes its span-carrying `ctx` into `exec.Execute` so an executor can propagate the trace onward.
- `services/tooling/bench/grpc_call.go` — sets the `traceparent` header directly via `chassis.TraceParentHeader(ctx)` on its outbound `*http.Request` (a plain `net/http` call, not a connect-go client, so there's no interceptor chain to attach to).
- `services/tooling/bench/foundry_plugin.go` — adds `connect.WithInterceptors(chassis.NewTraceClientInterceptor())` to its `stepexecutorv1connect.NewStepExecutorClient` construction, so a `foundry://` step's call to the plugin also continues the trace.

**Usage — the pattern any chassis service can follow** for background work started from (but outliving) an inbound handler:

```go
func (s *Scheduler) execute(ctx context.Context, run *workflowv1.Run, w *workflowv1.Workflow) (*workflowv1.Run, error) {
    ctx, runSpan := chassis.StartSpan(ctx, "workflow:"+w.GetName())
    runSpan.SetAttribute("workflow_name", w.GetName())
    runSpan.SetAttribute("run_id", run.GetRunId())
    // ... do the work, using ctx for anything downstream ...
    if runFailed {
        runSpan.End(fmt.Errorf("workflow %q failed", w.GetName()))
    } else {
        runSpan.End(nil)
    }
}
```

Call `chassis.StartSpan(ctx, name)` again anywhere further down the same call path (another goroutine, a retry loop, a per-item iteration) to add a child span — nesting falls out automatically from whatever span `ctx` already carries, no explicit parent-passing needed. For an outbound call that should hand the trace to another chassis service: use `connect.WithInterceptors(chassis.NewTraceClientInterceptor())` on a connect-go client, or set the `chassis.TraceParentHeader(ctx)` header by hand on a raw `net/http` request — either way, the receiving service's own `NewTraceInterceptor` picks it up with no further wiring.

**How to test:** Trigger a workflow (`curl -X POST http://localhost:9300/tooling.workflow.v1.WorkflowService/TriggerRun -d '{"workflow_name":"crud-e2e"}'`) and query `beacon.spans` for the resulting `trace_id`. Verified live: a passing run produced `workflow:crud-e2e` (root) → `step:create-name` / `step:read-name` (children, correct `parent_span_id`) → `examples.crud.v1.CrudService/Create` / `.../Read` (crud's own spans, now children of the matching step span instead of disconnected new traces), all one `trace_id`, all `STATUS_CODE_OK`. Stopping `crud` mid-run and re-triggering produced the same shape with `STATUS_CODE_ERROR` on both `workflow:crud-e2e` and `step:create-name` (and no span at all for the skipped `read-name`, since no work happened for it). Confirmed this only took effect after rebuilding *both* services sharing the local chassis `replace` — bench and crud each needed a fresh build for the new `traceparent`-aware `NewTraceInterceptor` to take effect on their side.

### Phase 19 — `ListMetricNames`/`ListLabelValues` RPCs `[Go]`

**Goal:** Metrics gain the same "what can I even query" discovery surface Prometheus's own HTTP API provides — the precondition for any click-to-build query bar, per [Why the query builder needs a new backend capability](#why-the-query-builder-needs-a-new-backend-capability).

**New paths:**
- `api/core/observability/metrics/v1/metrics.proto` — `ListMetricNames(ListMetricNamesRequest{prefix, limit}) returns (ListMetricNamesResponse{names})`; `ListLabelValues(ListLabelValuesRequest{metric_name, limit}) returns (ListLabelValuesResponse{values: map<string, LabelValues>})`, `LabelValues{values: repeated string}` — keyed by label name so one call discovers both which label keys exist for a metric and each key's values, rather than needing a separate "list label keys" RPC.
- `services/core/beacon/query/metric_discovery.go` — `SELECT DISTINCT metric_name FROM metric_points [WHERE metric_name LIKE ?] LIMIT ?` for the first; `SELECT DISTINCT arrayJoin(mapKeys(labels)) AS k, arrayJoin(labels[k]) AS v FROM metric_points WHERE metric_name = ? GROUP BY k, v LIMIT ?`-shaped query for the second, grouped into the response's `map<string, LabelValues>` server-side.

**How to test:** With seeded `http_requests_total{route="/api", service="fuse"}` and `process_cpu_percent` points in `metric_points`, `ListMetricNames` (no prefix) returns both names; `ListMetricNames{prefix: "http_"}` returns only the first. `ListLabelValues{metric_name: "http_requests_total"}` returns `{"route": ["/api"], "service": ["fuse"]}`.

### Phase 20 — Web client Metrics view: `MetricQueryBuilder` `[Rust/Dioxus]`

**Goal:** Building a PromQL-subset query is a series of clicks, matching the convenience Stream's `QueryBuilder` and Events' `WideEventQueryBuilder` already give their own grammars.

**New paths:**
- `services/core/beacon/web-client/src/components/metric_query_builder.rs` — `MetricQueryBuilder(expression: Signal<String>, on_add: EventHandler<String>)`, per [`MetricQueryBuilder`](#metricquerybuilder) above: metric-name select (populated via `ListMetricNames` on mount) → label filter rows (key + value selects, populated via `ListLabelValues` scoped to the chosen metric) → optional aggregation wrapper select (`rate(...)` / `sum by (...)` / bare selector, sourced from `query/promql.go`'s own supported-construct list, not a hand-maintained duplicate) → live fragment preview + "Add to query".

**Changed paths:**
- `services/core/beacon/web-client/src/views/metrics.rs` — mounts `MetricQueryBuilder` above the existing `QueryBar`, wired to the same `expression` signal so a click-built fragment and hand-typed text compose the same way Stream's two input modes already do.

**How to test:** Selecting metric `http_requests_total`, adding a label filter `route = "/api"`, and choosing the `rate(...)` wrapper builds `rate(http_requests_total{route="/api"}[5m])` in the preview; clicking "Add to query" populates the query bar with exactly that string and running it returns the expected series.

### Phase 21 — Web client Metrics view: `ChartTypeSelector` + `Chart` dispatcher `[Rust/Dioxus]`

**Goal:** A query result renders as the chart shape that actually fits it, not always the same overlaid-lines view.

**New paths:**
- `services/core/beacon/web-client/src/components/chart_type_selector.rs` — `ChartType` enum (`Line | Bar | Area | SingleStat`) + a small icon-button group, matching `wide_events.rs`'s `ViewMode` List/FlameGraph toggle for the interaction shape.
- `services/core/beacon/web-client/src/components/chart.rs` — `Chart(chart_type: ChartType, series: Vec<TimeSeries>)`, dispatching to: today's `TimeSeriesChart` unchanged for `Line`; a new `Bar` renderer generalizing `SeverityHistogram`/`WideEventHistogram`'s stacked-proportional-bar approach from "count per time bucket" to "value per sample, colored by series"; a new `Area` renderer reusing `Line`'s coordinate math with each series' region filled at low opacity; `SingleStat` rendering only when exactly one series is present, reusing `MetricStatTile`'s existing `MetricCard` adaptation with no chart beneath it.

**Changed paths:**
- `services/core/beacon/web-client/src/views/metrics.rs` — a `chart_type: Signal<ChartType>` alongside the existing signals; the current unconditional `TimeSeriesChart { series: series_list.clone() }` call becomes `Chart { chart_type: chart_type(), series: series_list.clone() }`.

**How to test:** With a multi-series query result loaded, switching the selector between all four types re-renders the same data as overlaid lines, stacked bars, filled areas, and (once narrowed to a single series) one big `MetricCard` with no chart — no re-query needed, since it's the same `series_list` reshaped client-side.

### Phase 22 — Web client Metrics view: `PollingIntervalSelector` `[Rust/Dioxus]`

**Goal:** A graph stays live at an operator-chosen cadence, matching Blueprint's own Metrics view's existing (fixed 5s) polling and Stream's live tail, instead of Beacon's Metrics view remaining the one page in the product with no auto-refresh.

**New paths:**
- `services/core/beacon/web-client/src/components/polling_interval_selector.rs` — `PollingIntervalSelector(interval: Signal<Option<Duration>>)`, a `<select>` over Off/5s/10s/30s/1m/5m.

**Changed paths:**
- `services/core/beacon/web-client/Cargo.toml` — add `gloo-timers = { version = "0.3", features = ["futures"] }` (already a Blueprint web-client dependency; Beacon's doesn't have it yet).
- `services/core/beacon/web-client/src/views/metrics.rs` — a `refresh: Signal<Option<Duration>>` driven by `PollingIntervalSelector`; when `Some(d)`, a `gloo_timers::callback::Interval::new(d.as_millis(), ...)` held in its own `use_signal` (so it isn't dropped at the end of the render function — see Blueprint's `metrics.rs` for the exact pattern this mirrors) increments a `tick: Signal<u32>` that the existing query-refetch `use_effect` depends on; switching back to "Off" drops the `Interval` (setting the holding signal back to `None`), stopping the ticks. Each tick rebuilds `QueryMetricsRequest` with a sliding `start`/`end` window (`end = now`, `start = now - window`) rather than resending the exact same absolute range forever.

**How to test:** With a query already running, setting the interval to 5s and watching `metric_points` receive new rows (e.g. a chassis host-metrics reporter still running) shows the chart/tiles advance every ~5s with no manual "run" click; switching to "Off" stops further requests (confirm via network inspection — no new `QueryMetrics` calls after the switch) without clearing the last-fetched data off screen.

### Phase 23 — Web client Metrics view: "View correlated events" pivot `[Rust/Dioxus]`

**Goal:** A metric spike answers "which requests" by pivoting into Events, pre-filled by label and time window — see [Correlating a graph with Events](#correlating-a-graph-with-events).

**New paths:**
- `services/core/beacon/web-client/src/components/metric_events_link.rs` — `MetricEventsLink(series: TimeSeries, window_start: String, window_end: String)`: builds a BeaconQL fragment from `series.labels` (a label whose key matches one of WideEvent's own first-class fields — today just `service` → `service_name`, per `TimeSeries.labels`' typical OTel-convention keys — becomes `service_name = "..."`; every other label becomes `attributes["<key>"] = "<value>"`) and renders a link/button to the Events route carrying that fragment plus the window as query parameters.

**Changed paths:**
- `services/core/beacon/web-client/src/views/metrics.rs` — one `MetricEventsLink` per stat tile/series (reusing `series.labels` already in hand from the current `QueryMetrics` result — no new fetch, no PromQL-string parsing needed to recover the labels).
- `services/core/beacon/web-client/src/views/wide_events.rs` — reads an optional pre-fill (BeaconQL fragment + time range) from the navigation, seeding `expression`/the time range picker on mount, the same way a deep link into Traces from Stream's `TracePill` already seeds that view's own state today.

**How to test:** Querying `rate(http_requests_total{service="fuse"}[5m])`, clicking "View correlated events" on the resulting series opens Events with `service_name = "fuse"` already in the query bar and the time range matching the graph's current window; adding `attributes["http.path"] = "..."` by hand (or via `WideEventQueryBuilder`, once loaded) narrows to the specific route.

### Phase 24 — OTLP exemplar capture + `QueryMetrics` exemplar fields `[Go]`

**Goal:** A single chart point can link straight to the exact WideEvent that produced it, for exporters that actually send exemplars — see [Correlating a graph with Events](#correlating-a-graph-with-events) for why this is additive groundwork rather than the primary mechanism (most exporters, including chassis's own `otel_metrics.go` reporter today, don't emit exemplars).

**New paths:** none — extends Phase 7/8's existing files.

**Changed paths:**
- `services/core/beacon/ingest/metrics.go` — capture each OTLP data point's `Exemplars` (when present) instead of discarding them; add nullable `exemplar_trace_id String`, `exemplar_span_id String` columns to `metric_points` (empty string when the point carries none — ClickHouse has no natural `NULL` for `String` without `Nullable()`, and an empty string is an unambiguous "no exemplar" sentinel here since real trace/span ids are never empty).
- `api/core/observability/metrics/v1/metrics.proto` — `Sample` gains `optional string exemplar_trace_id = 3` / `optional string exemplar_span_id = 4`.
- `services/core/beacon/web-client/src/views/metrics.rs` — a chart point with a non-empty exemplar pair renders as clickable, opening the Events detail panel directly via `GetWideEvent(trace_id)` — no query-builder round trip needed, since there's exactly one event to show.

**How to test:** An OTel SDK configured to emit exemplars (most language SDKs support this for histograms) sends a data point with one attached; the resulting `Sample` in `QueryMetricsResponse` carries the matching `exemplar_trace_id`; clicking that point in the chart opens the exact WideEvent, confirmed by comparing its `trace_id` to the one the exemplar named.

### Phase 25 — Blueprint: generic `TypeMutation` Catalyst consumer `[Go]`

**Goal:** Any process with a registered type can mutate it over Catalyst instead of a direct KV RPC — a framework capability, not Beacon-specific. See [Type Mutation Events](/docs/architecture/core-services#type-mutation-events). This phase has no dependency on Beacon or Metrics at all; it's a prerequisite Phase 26 builds on.

**New paths:**
- `api/core/registry/key_value/v1/service.proto` — adds `TypeMutation` and its nested `Action` enum, per [Persisting a graph](#persisting-a-graph)'s exact definition.
- `services/core/blueprint/key_value/type_mutation_consumer.go` — opens a `ConsumerClient.Consume` stream (same shape `services/tooling/catalyst-consume/rpc.go` already uses) filtered to CloudEvent type `"core.registry.key_value.v1.TypeMutation"`, started as a background goroutine at Blueprint's own startup. For each event: decode the body into `TypeMutation`, look up `value.type_url` (or the message name for `ACTION_DELETE`, which carries none) against Blueprint's own already-existing registered-type descriptors (the same lookup `DecodeValues` uses), and apply `CREATE`/`UPDATE` as the internal equivalent of `KeyValueService.Set` or `DELETE` as the internal equivalent of `Delete` — calling the same underlying storage function the RPC handlers already call, not a second code path. An unregistered `type_url` or a decode failure is logged and dropped, never fatal to the consumer loop.

**Changed paths:**
- `services/core/blueprint/main.go` — starts the new consumer alongside Blueprint's other startup work. This is Blueprint's first-ever dependency on Catalyst (previously zero coupling between the two — Blueprint doesn't import Catalyst's API at all today); worth calling out plainly rather than treating as an incidental detail, even though it's a one-way "Blueprint reads from Catalyst" dependency, not a cycle (Catalyst doesn't depend on Blueprint's KV in return).

**How to test:** With a type registered via `RegisterType`, producing a `TypeMutation{action: ACTION_CREATE, key: "test-key", value: <the registered type>}` CloudEvent by hand (e.g. via `foundry://http-call@v1` or a small script hitting Catalyst's `Produce` RPC directly) results in `KeyValueService.Get{key: "test-key"}` returning that value shortly after — no direct `Set` call involved. The same for `ACTION_UPDATE` (value changes) and `ACTION_DELETE` (`Get` then returns not-found). A `TypeMutation` naming an unregistered `type_url` is logged by Blueprint and has no effect on the KV store — confirmed by checking nothing new appears under `ListKinds`.

### Phase 26 — `BeaconMetricsGraphConfiguration` proto + `chassis.PublishTypeMutation` + Beacon's Save/Delete RPCs `[Go]`

**Goal:** A graph's config has a real, typed shape, and Beacon can persist one via Phase 25's new mechanism — see [Persisting a graph](#persisting-a-graph).

**New paths:**
- `pkg/chassis/type_mutation.go` — `PublishTypeMutation(action, key, msg)`, the generic producer helper any chassis service (not just Beacon) can call, per [Persisting a graph](#persisting-a-graph)'s exact description (lazy `sync.Once`-held `BidiStreamForClient`, mirroring `wide_event.go`'s own shape and caveats).
- `services/core/beacon/query/graph_configuration.go` — `MetricsService.SaveGraphConfiguration`/`DeleteGraphConfiguration` RPC handlers: `SaveGraphConfiguration` fills in `id` (generated) and `created_at` on first save (an empty incoming `id`) or preserves both and bumps `updated_at` on a re-save (a non-empty incoming `id`), then calls `chassis.PublishTypeMutation(ACTION_CREATE_or_UPDATE, "beacon_metrics_graph_configuration_"+id, &config)`. `DeleteGraphConfiguration` calls it with `ACTION_DELETE` and no value.

**Changed paths:**
- `api/core/observability/metrics/v1/metrics.proto` — adds `ChartType`, `BeaconMetricsGraphConfiguration`, and the two new RPCs + their request/response messages.
- `services/core/beacon/main.go` — `chassis.New(logger).WithRegisteredType(&metricsv1.BeaconMetricsGraphConfiguration{})` added to the existing builder chain.

**How to test:** Calling `SaveGraphConfiguration` with an empty `id` returns one newly generated; querying Blueprint's `KeyValueService.Get` for `"beacon_metrics_graph_configuration_" + <that id>` shortly after returns the saved config, decoded as `BeaconMetricsGraphConfiguration` (not an opaque blob) in Blueprint's own KV browser UI — confirming both Phase 25's consumer and this phase's producer work together, end to end, before any UI exists to drive it.

### Phase 27 — Web client Metrics view: Save/Load a graph configuration `[Rust/Dioxus]`

**Goal:** A graph survives a page refresh and is nameable/reloadable, stored in Blueprint like every other piece of cluster config — see [Persisting a graph](#persisting-a-graph).

**New paths:**
- `services/core/beacon/web-client/src/components/saved_graphs.rs` — `BLUEPRINT_DOMAIN` (mirrors Blueprint's own `CATALYST_DOMAIN` shape exactly: `option_env!`-overridable, defaults to `http://localhost:2221`), used only for the *read* side (`KeyValueService.List`/`Get`, direct to Blueprint); a `SaveGraphButton(config: GraphConfig, on_saved: EventHandler<GraphConfig>)` (prompts for a name when `config.name` is empty, calls Beacon's own `SaveGraphConfiguration` RPC — same-origin, no cross-service client needed for this half) and a `LoadGraphDropdown(on_load: EventHandler<GraphConfig>)` (`List`s Blueprint directly by `BeaconMetricsGraphConfiguration`'s type URL on mount, populating a `<select>`).

**Changed paths:**
- `services/core/beacon/web-client/src/views/metrics.rs` — mounts both alongside `ChartTypeSelector`/`PollingIntervalSelector`; `LoadGraphDropdown`'s `on_load` replaces the view's `GraphConfig` signal wholesale (query, chart type, window, refresh interval all switch together, matching what was saved).

**How to test:** Building a graph, saving it as "Fuse error rate," navigating away (or refreshing the page) and back, then loading it from the dropdown reproduces the exact same query/chart-type/window/refresh-interval — confirmed by comparing the query bar's text and the chart-type toggle's selected state before and after the round trip. Matches Phase 26's own verification but now driven by the actual UI instead of a direct RPC call.
