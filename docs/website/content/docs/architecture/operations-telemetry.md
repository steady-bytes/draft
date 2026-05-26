---
weight: 10
title: "Operations & Telemetry"
description: "Design options for surfacing live event topology, service mesh topology, and runtime metrics in a Draft system"
icon: "monitoring"
draft: false
toc: true
---

## Overview

This document describes design options for making the operational state of a Draft cluster observable at runtime. It covers three related concerns:

- **Event topology** — which services are producing and consuming which event types through Catalyst
- **Service mesh topology** — which processes are registered in the cluster and how they are connected
- **Event metrics** — latency, throughput, and error rate derived from the event store

These concerns are addressed through a combination of existing APIs (Blueprint's `ServiceDiscoveryService`, Catalyst's `Query`), proposed new RPCs, and optional integration with the OpenTelemetry stack.

---

## Event Topology

### Current state

Catalyst stores every event in ClickHouse with `source` (the publishing service), `type` (the event kind), and a `forwarded_at` timestamp captured after the event is dispatched to consumers. The `Query` and `QueryStream` RPCs expose this history.

Routing in the broker uses `atomicMap`, keyed by the proto message descriptor name. Because every CloudEvent shares the same descriptor, **all active consumers receive all events** — routing is a pure broadcast at the broker level, with consumer-side filtering left to each service.

This means:
- **Producers** are fully derivable today: query ClickHouse for `DISTINCT source`.
- **Event types** are fully derivable today: query ClickHouse for `DISTINCT type`.
- **Consumers and their edge relationships are not currently queryable.** The `atomicMap` stores live gRPC stream handles only — no service identity.

### Option 1 — Query-only (producers and event types only)

Use the existing `Query` RPC with no filter expression and a high limit, then deduplicate `source` and `type` values client-side. This gives a partial topology — producers and the event types they emit — but cannot draw the consumer half of the graph.

**Suitable for**: a read-only producer view backed entirely by stored event history.

**What is needed today**: nothing — the API already supports this.

### Option 2 — Consumer identity convention + `GetTopology` RPC

This is the recommended path for a complete bidirectional topology.

#### Step 1: Consumers declare identity at subscription time

The `ConsumeRequest.message` is a full `CloudEvent` that is currently unused for routing. Adopt a convention: services populate it with their own identity and declared interest when they call `Consume`:

```
source = "notify-svc"      // the consumer's own service name
type   = "orders.created"  // the event type it intends to process
```

This is non-breaking — existing consumers continue to work unchanged and can adopt the convention incrementally.

#### Step 2: Track registrations in the broker

Add a `registrations` field to `atomicMap` populated from the subscription CloudEvent when a consumer connects:

```go
// maps event_type → []consumer_source_names
registrations map[string][]string
```

This gives the broker a live, in-memory view of which consumers are subscribed and what they care about.

#### Step 3: Add a `GetTopology` RPC to Catalyst

Add to the query proto (or a new `topology.proto`):

```protobuf
service Topology {
  rpc GetTopology(GetTopologyRequest) returns (GetTopologyResponse) {}
}

message GetTopologyRequest {}

message TopologyNode {
  string id   = 1;
  string name = 2;
}

message TopologyEdge {
  string producer_source = 1;
  string consumer_source = 2;
  string event_type      = 3;
}

message GetTopologyResponse {
  repeated TopologyNode producers = 1;
  repeated TopologyNode consumers = 2;
  repeated TopologyEdge edges     = 3;
}
```

The handler derives:
- **Producers**: `SELECT DISTINCT source FROM events LIMIT 200` run directly against ClickHouse
- **Consumers and edges**: read live from `atomicMap.registrations`

The response shape maps directly to the `TopologyData` type in the web client, allowing the `ArcSpine` (producers view) and `FullCircle` (consumers view) components to be fed real data with no structural change.

**Suitable for**: the primary production implementation of both topology views.

**Trade-off**: consumers that do not yet adopt the identity convention will not appear in the graph until they do.

---

## Service Mesh Topology

### Current state

Blueprint's `ServiceDiscoveryService` already provides a complete registry of every process in the cluster. The `Query` RPC returns all registered `Process` records with `name`, `group`, `ip_address`, `running_state`, `health_state`, `last_status_time`, and a `repeated Metadata` key-value bag. The `Watch` RPC streams process state changes in real time as services start, disconnect, or change health.

This means **every node in the service mesh is already enumerable at zero cost**. What is missing is the edge set: which service communicates with or depends on which other service.

### Option A — Metadata-declared dependencies (works today)

The `Process.metadata` field is already sent from every service via `ClientDetails.Metadata` on each `Synchronize` heartbeat. Services can declare their dependencies there using a convention:

```go
Metadata: []*sdv1.Metadata{
    {Key: "dep", Value: "blueprint"},
    {Key: "dep", Value: "catalyst"},
    {Key: "dep", Value: "fuse"},
}
```

The web client calls `ServiceDiscoveryService.Query`, receives all processes, reads `dep` metadata entries, and derives the edge list without any new server-side code. The `Watch` RPC keeps node health state live.

**Suitable for**: an immediate, lightweight service map that is accurate as long as services maintain their declarations.

**Trade-off**: declared intent, not observed behavior. Stale or missing declarations produce an inaccurate graph.

### Option B — OpenTelemetry service graph (observed behavior)

The Draft repository ships a local observability stack (`docker-otel-lgtm`) that includes [Grafana Tempo](https://grafana.com/oss/tempo/). Any service instrumented with the OpenTelemetry SDK emits distributed traces that carry the caller and callee service name on every RPC or HTTP call. Tempo's API can return this as a **service graph** — a live, automatically-derived adjacency list built from observed traffic.

The web client would query Tempo's HTTP API directly, or Blueprint could proxy the query and return it alongside `Process` data in an enriched response.

**Suitable for**: accurate, automatically maintained topology that requires no manual declaration and reflects actual traffic patterns.

**Trade-off**: requires OTel instrumentation in each service and a running Tempo instance. More infrastructure than Option A but more reliable long-term.

### Combining both sources

The two options are complementary. Blueprint's `Query` + `Watch` gives authoritative node identity and health state. The edge set comes from either metadata declarations (Option A) or OTel traces (Option B). A `GetServiceTopology` RPC on Blueprint could merge both: query all processes from the KV store, then enrich edges from whichever source is available.

---

## Event Metrics

### `forwarded_at` timestamp

The Catalyst broker captures a `forwarded_at` timestamp immediately after dispatching each event to consumers via `state.Broadcast()`. This timestamp is:

- Injected into the CloudEvent's `attributes` map before broadcasting, so live `QueryStream` subscribers receive it alongside the event payload
- Written to a dedicated `forwarded_at DateTime64(9)` column in the ClickHouse `events` table as part of the batch insert
- Back-filled dynamically for rows stored before this column existed: the `Query` handler reads both `raw` and `forwarded_at` from ClickHouse and injects the column value into the proto for any row where the attribute is absent and the column value is after epoch zero

### Derivable metrics

With `inserted_at` (event arrival time) and `forwarded_at` (dispatch time) both stored in ClickHouse, the following metrics can be computed purely from SQL:

| Metric | Query pattern |
|---|---|
| Throughput (messages/min) | `COUNT(*) / minutes` over a time window |
| Median dispatch latency | `quantile(0.5)(forwarded_at - inserted_at)` |
| P95 dispatch latency | `quantile(0.95)(forwarded_at - inserted_at)` |
| Error rate | Requires an `error` attribute convention on events |
| Producer volume by source | `COUNT(*) GROUP BY source` |
| Consumer load by type | `COUNT(*) GROUP BY type` |

### Surfacing metrics from the web client

The `Query` RPC accepts a CESQL expression and returns matching events. Aggregate metrics require either:

1. **Client-side aggregation** — `QueryStream` with a time-window filter and client-side bucketing. Simple to implement, suitable for small windows (< 10k events).

2. **A dedicated `GetMetrics` RPC** — a new endpoint on Catalyst that runs parameterised ClickHouse aggregate queries and returns pre-computed metric values. This is the correct path for production dashboards at any scale.

The current web client `Metrics` view uses placeholder sparkline data and is wired to accept real values once either approach above is implemented.

---

## Implementation Roadmap

The options above are ordered by implementation cost and dependency.

| Phase | What | Unlocks |
|---|---|---|
| 1 | Call `ServiceDiscoveryService.Query` + `Watch` in the web client | Live service node list with health state, no edges |
| 2 | Add `dep` metadata convention to core services | Service mesh edges appear in the topology view |
| 3 | Consumers set `source`/`type` on `ConsumeRequest.message` | Consumer identity is declared at the broker |
| 4 | Add `registrations` tracking to `atomicMap` | Broker holds live consumer-to-event-type mapping |
| 5 | Add `GetTopology` RPC to Catalyst | Full event topology (producers, consumers, edges) queryable over gRPC |
| 6 | Add `GetMetrics` RPC to Catalyst | Pre-computed throughput and latency from ClickHouse |
| 7 | Instrument services with OTel SDK | Observed service graph replaces or validates metadata declarations |

Phases 1–2 require no server-side changes and can be shipped from the web client alone. Phases 3–6 are additive server changes with no breaking impact on existing consumers. Phase 7 is an operational investment that pays off at scale.

---

## Implementation Plan

The phases below describe a concrete, ordered implementation path that takes the `FullCircle` and `ArcSpine` topology views from mock data to live operational telemetry. Each phase is independently shippable.

---

### Phase 1 — Query-only producers `[web client only]`

**Goal:** Replace mock data in the `Consumers` view with real producer topology derived from Catalyst's event history.

**Files changed:**
- `services/core/blueprint/web-client/src/views/consumers.rs`

**How it works:**
Call `QueryClient.query()` on mount with no expression filter. Walk the returned `CloudEvent` list, collect unique `source` field values, and build a `TopologyData` with producers only. Show mock data as a placeholder until the query returns. If the query fails (Catalyst unreachable), keep the mock data visible.

```rust
fn topology_from_events(events: &[CloudEvent]) -> TopologyData {
    let mut producers: Vec<TopologyNode> = Vec::new();
    for ev in events {
        if !ev.source.is_empty() && !producers.iter().any(|p| p.id == ev.source) {
            producers.push(TopologyNode { id: ev.source.clone(), name: ev.source.clone() });
        }
    }
    producers.sort_by(|a, b| a.id.cmp(&b.id));
    TopologyData { producers, consumers: vec![], edges: vec![] }
}
```

**Limitations:** The lower arc (consumers) and all bezier threads remain empty until Phase 5.

**How to test:** Open `/consumers` in a running cluster — the upper arc should show real service names from the event store. Open the route with Catalyst unreachable — mock data stays visible.

**Status:** ✅ Complete — `consumers.rs` calls `QueryClient.query()` on mount; `topology_from_events()` derives producers from unique `source` fields; mock data shown as initial placeholder while loading.

---

### Phase 2 — Live volume via QueryStream `[web client only]`

**Goal:** Keep the topology view's thread weights live by counting events from an open `QueryStream`.

**Files changed:**
- `services/core/blueprint/web-client/src/views/consumers.rs`

**How it works:**
After the initial snapshot query, open a `QueryStream` with no filter. For each arriving event, increment a `Signal<HashMap<(String, String), u32>>` keyed by `(source, event_type)`. When edges are present (Phase 5), merge the live counts into `TopologyData.edges[].vol` before each render so bezier thread weights update in real time.

**How to test:** Produce events through Catalyst while the consumers view is open. Thread widths should change in response to traffic.

**Status:** ✅ Complete — `consumers.rs` opens a `QueryStream` on mount alongside the Phase 1 snapshot query; each arriving event increments `vol_map: Signal<HashMap<(String, String), u32>>` keyed by `(source, event_type)`; `merge_vol()` splices the live counts into edge `vol` fields before each render. The `stream_task` handle is stored so the stream can be cancelled if the effect re-runs. Phase 5 edges will make the vol weights visually active; the counts are already flowing.

---

### Phase 3 — Consumer identity convention `[no server changes]`

**Goal:** Establish the convention that consumers set `source` and `type` on `ConsumeRequest.message` at subscription time so the broker can observe consumer identity.

**Files changed:**
- Any service that calls `Consume` (adopt the convention in each subscriber)

**Convention:**
```
ConsumeRequest.message.source = "<service-name>"       // the consumer's own name
ConsumeRequest.message.type   = "<event-type-pattern>" // the event type(s) it processes
```

Multiple `Consume` calls per service are fine — each maps to one `(consumer, event_type)` edge. The broker ignores `ConsumeRequest.message` today, so existing consumers continue to work unchanged; adopting the convention is opt-in.

**How to test:** No automated test at this phase — verify by reading `atomicMap.registrations` after Phase 4 is deployed.

**Status:** ✅ Complete — `services/examples/consumer/main.go` refactored to open one `Consume` stream per declared event type (`examples.crud.v1.DatabaseModelSaved`, `examples.user.v1.UserCreated`, `examples.user.v1.UserLoggedIn`), each setting `Message.Source = "examples-consumer"` and `Message.Type = <event-type>`; each goroutine filters to its own declared type so no event is double-processed. `tools/dctl/cmd/broker/consume.go` updated to set `Source = "dctl"` and `Type = ""` (subscribes to all events, appropriate for a debug CLI).

---

### Phase 4 — Registration tracking in broker `[Go server]`

**Goal:** Make the Catalyst broker track which consumers are subscribed and which event types they declared interest in, using the identity set in Phase 3.

**Files changed:**
- `services/core/catalyst/broker/atomicMap.go`
- `services/core/catalyst/broker/controller.go`
- `services/core/catalyst/broker/rpc.go`

**`atomicMap.go`** — add a `registrations` map with thread-safe accessors:
```go
type atomicMap struct {
    mu            sync.RWMutex
    m             map[string][]*connect.ServerStream[acv1.CloudEvent]
    n             map[string]chan *acv1.CloudEvent
    registrations map[string][]string // event_type → []consumer_source_names
}

func (a *atomicMap) AddRegistration(eventType, source string) { ... }
func (a *atomicMap) RemoveRegistration(eventType, source string) { ... }
func (a *atomicMap) Registrations() map[string][]string { ... }
```

**`controller.go`** — extend `register` struct with context for disconnect detection:
```go
type register struct {
    ctx     context.Context  // carries the client disconnect signal
    message *acv1.CloudEvent
    stream  *connect.ServerStream[acv1.CloudEvent]
}

// In consume() goroutine, after state.Broker(key, stream):
src := reg.message.GetSource()
typ := reg.message.GetType()
if src != "" && typ != "" {
    state.AddRegistration(typ, src)
    go func(ctx context.Context, src, typ string) {
        <-ctx.Done()
        state.RemoveRegistration(typ, src)
    }(reg.ctx, src, typ)
}
```

**`rpc.go`** — pass context through:
```go
// In Consume():
controller.Consume(ctx, &register{ctx: ctx, message: req.Msg.Message, stream: stream})
```

**How to test:** Start a consumer that sets `source`/`type` on its subscription CloudEvent. Inspect `GetTopology` (Phase 5). Disconnect the consumer — its entry should be removed within one connection timeout.

**Status:** ✅ Complete — `atomicMap` gains a `registrations map[string][]string` field with `AddRegistration`, `RemoveRegistration`, and `Registrations` (snapshot copy) methods, all guarded by the existing `mu` mutex. The `register` struct in `controller.go` gains a `ctx context.Context` field. `consumer.go` populates it from the RPC context passed into `Consume`. In `controller.consume`, after wiring the stream, `src`/`typ` are read from the CloudEvent; if both are non-empty, `AddRegistration` is called and a cleanup goroutine waits on `ctx.Done()` to call `RemoveRegistration`. `rpc.go` required no changes — `ctx` was already threaded through.

---

### Phase 5 — `GetTopology` and `WatchTopology` RPCs `[Go server + Rust client]` ✅

**Goal:** Add a `Topology` service to Catalyst so the web client can query and stream the full event topology (producers, consumers, edges) in real time.

**Files changed:**
- `api/core/message_broker/actors/v1/topology.proto` (new)
- `services/core/catalyst/broker/topology.go` (new)
- `services/core/catalyst/broker/rpc.go`
- `services/core/blueprint/web-client/src/views/consumers.rs`

**`topology.proto`:**
```protobuf
syntax = "proto3";
package core.message_broker.actors.v1;

service Topology {
  rpc GetTopology(GetTopologyRequest) returns (GetTopologyResponse) {}
  rpc WatchTopology(WatchTopologyRequest) returns (stream WatchTopologyResponse) {}
}

message GetTopologyRequest {}
message GetTopologyResponse {
  repeated TopologyNode producers = 1;
  repeated TopologyNode consumers = 2;
  repeated TopologyEdge edges     = 3;
}
message TopologyNode { string id = 1; string name = 2; }
message TopologyEdge {
  string producer_source = 1;
  string consumer_source = 2;
  string event_type      = 3;
  uint32 vol             = 4;  // events in the last 5-min window
}

message WatchTopologyRequest {}
message WatchTopologyResponse {
  oneof event {
    TopologyNode consumer_connected    = 1;
    TopologyNode consumer_disconnected = 2;
    TopologyNode producer_appeared     = 3;
  }
}
```

After editing the proto, run `dctl api build` to regenerate Go and Rust bindings.

**`topology.go`** — broker-side observer registry (mirrors `observerRegistry` pattern):
```go
type topologyObserver struct {
    stream *connect.ServerStream[acv1.WatchTopologyResponse]
}
type topologyRegistry struct {
    mu        sync.RWMutex
    observers []*topologyObserver
}
func (r *topologyRegistry) Add(obs *topologyObserver) { ... }
func (r *topologyRegistry) Remove(obs *topologyObserver) { ... }
func (r *topologyRegistry) Broadcast(resp *acv1.WatchTopologyResponse) { ... }
```

Call `Broadcast` from `AddRegistration` / `RemoveRegistration` in `atomicMap.go` after Phase 4.

**`rpc.go`** additions:
```go
func (r *Rpc) GetTopology(ctx context.Context, ...) (*connect.Response[acv1.GetTopologyResponse], error) {
    // producers: SELECT DISTINCT source FROM events via store.QueryDistinctSources()
    // consumers + edges: atomicMap.Registrations()
}
func (r *Rpc) WatchTopology(ctx context.Context, ..., stream *connect.ServerStream[acv1.WatchTopologyResponse]) error {
    // register observer, block on ctx.Done(), remove on disconnect
}
```

**`consumers.rs`** — three concurrent tasks:
```rust
// Task 1: TopologyClient.get_topology() snapshot on mount — populate full arc
// Task 2: TopologyClient.watch_topology() stream — apply_topology_delta() for live updates
// Task 3: QueryStream — update edge vol counts from live event traffic
```

**How to test:** Run Catalyst locally. Open `/consumers` — full arc (producers + consumers) and bezier threads should appear with real data. Start or stop a consumer service — the node should appear or disappear within seconds via the `WatchTopology` stream.

**Status:** ✅ Complete — `GetTopology` and `WatchTopology` implemented in `broker/rpc.go`, `broker/controller.go`, `broker/store.go` (via `QueryDistinctSources`), and `broker/topology.go`; proto generated in `api/core/message_broker/actors/v1/topology.proto`; `consumers.rs` uses all three tasks (snapshot, watch, query stream).

---

### Phase 6 — `GetMetrics` RPC `[Go server + Rust client]`

**Goal:** Replace client-side event counting with pre-computed per-edge volume from ClickHouse aggregates, and surface latency metrics.

**Files changed:**
- `api/core/message_broker/actors/v1/metrics.proto` (new)
- `services/core/catalyst/broker/store.go` — add `QueryVolumes()`
- `services/core/catalyst/broker/rpc.go` — add `MetricsHandler`
- `services/core/blueprint/web-client/src/views/consumers.rs` — replace sliding-window counter with `GetMetrics` call

**`metrics.proto`:**
```protobuf
service Metrics {
  rpc GetMetrics(GetMetricsRequest) returns (GetMetricsResponse) {}
}
message GetMetricsRequest { int32 window_seconds = 1; }
message EdgeVolume { string source = 1; string event_type = 2; uint32 count = 3; }
message GetMetricsResponse {
  repeated EdgeVolume edge_volumes = 1;
  double median_latency_ms         = 2;
  double p95_latency_ms            = 3;
}
```

**ClickHouse query in `store.go`:**
```sql
SELECT source, type, COUNT(*) AS cnt
FROM events
WHERE inserted_at >= now() - INTERVAL ? SECOND
GROUP BY source, type
ORDER BY cnt DESC
```

**How to test:** Send a burst of events via Catalyst. Call `GetMetrics`. Thread widths in the web client should reflect the burst with accurate ClickHouse-derived counts.

**Status:** ✅ Complete — `metrics.proto` added and Go/Rust bindings regenerated; `store.go` gains `QueryVolumes` (COUNT GROUP BY source, type) and `QueryLatency` (quantile aggregates on `forwarded_at - inserted_at`); `controller.go` exposes `GetMetrics`; `rpc.go` registers the `MetricsHandler`; `metrics.rs` calls `MetricsClient.get_metrics()` on mount and on time-range change, displaying real msgs/min, median latency, and P95 latency. Sparklines remain placeholder pending a time-series endpoint (Phase 7+).

---

### Phase 7 — OTel / Tempo service graph `[infrastructure + web client]`

**Goal:** Populate the `Cluster` view with the observed service graph from Grafana Tempo distributed traces, replacing or augmenting metadata-declared dependencies.

**Files changed:**
- `services/core/blueprint/web-client/src/views/cluster.rs`

**How it works:**
Query Tempo's HTTP API (service graph endpoint) from the web client. Map the returned `(caller, callee)` edge list to a node graph. `ServiceDiscoveryService.Query` + `Watch` from Blueprint provides node identity and health state; Tempo provides the observed edges.

**Prerequisites:** OTel SDK instrumentation in each service; Grafana Tempo running (the `docker-otel-lgtm` stack already ships it).

**How to test:** Run the full `docker-otel-lgtm` stack. Make RPCs between services. Open the Cluster view — nodes should appear connected by observed traffic edges.

**Status:** ⏳ Pending
