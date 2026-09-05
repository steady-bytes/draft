---
weight: 41
title: Cluster View — Live Topology Implementation Plan
description: Replacing the Cluster view's hardcoded seed diagram with a live graph derived from Blueprint's service registry, Fuse's gateway routes, and Catalyst's event topology — while keeping the existing PCB-style canvas, drag/pan/zoom, and manual-node editing.
icon: hub
draft: false
toc: true
---

This is the step-by-step implementation plan for turning the Blueprint web
client's Cluster view from a hand-authored mock diagram into a live model of
the actual running Draft cluster, built from three data sources that already
exist and are already used elsewhere in this exact crate. Each phase produces
a runnable, testable artifact, and the plan is ordered so the rendering layer
(already solid) never has to be touched until the data feeding it is real.

## Current state

`views/cluster.rs` (~1,170 lines) is a fully manual PCB-style diagram editor:
`seed_nodes()`/`seed_traces()` hardcode six fictional nodes (`auth-svc`,
`billing-svc`, `ledger-svc`, ...) and six wires, held in plain
`use_signal`/`Vec` state with no fetch, no watch, and no persistence —
`docs/architecture/cluster-graph-persistence.md`'s localStorage/KV plan was
never actually implemented against this (later, pure-Dioxus) version of the
view. Drag-to-reposition, pan/zoom, the context menu, and the config drawer
all work well against this fake data; none of that rendering machinery needs
to change for this plan.

Three RPCs already give real data, and are already called from other views in
this same crate — this plan wires `cluster.rs` to them, it doesn't add any
new proto or client:

- **Registry** — `ServiceDiscoveryService.Query`/`Watch` (via `API_DOMAIN`),
  already used by `views/service_registry.rs`. Gives every real `Process`:
  name, `ip_address`, `running_state`, `health_state`.
- **Gateway** — `NetworkingService.ListRoutes` (via `FUSE_DOMAIN`), already
  used by `views/gateway.rs`. Gives every real ingress `Route`: match
  host/prefix, target `Endpoint{host, port}`, auth policy.
- **Event topology** — `TopologyService.GetTopology`/`WatchTopology` (via
  `CATALYST_DOMAIN`), already used by `views/topology.rs` and
  `views/metrics.rs`. Gives a server-computed producer→consumer graph with
  real event types and volumes.

## Decisions

### Identity join across the three sources

The three sources key nodes differently, and none share a key directly:

| Source | Key | Example |
|---|---|---|
| Registry | `Process.name` | `bench` |
| Gateway | `Route.endpoint.host:port` | `host.docker.internal:9300` |
| Topology | CloudEvent `source` | `/services/bench` |

- **Registry ↔ Topology:** strip the leading `/services/` or `/plugins/`
  segment from a topology node's id (the same source convention documented
  in `docs/architecture/wide-events.md` and used by every real producer in
  this repo) to recover the logical name, then match against `Process.name`.
- **Registry ↔ Gateway:** match `Route.endpoint.host:port` against
  `Process.ip_address` (which is exactly `advertise_address`, host:port —
  see `docs/architecture/service-registry-identity.md`) to resolve which
  process a route actually targets.

A topology node or route endpoint that doesn't resolve to any registered
process (network hiccup, a process that just crashed, an external caller) is
still shown — as an unresolved node labeled with its raw source/host string
— rather than silently dropped, so the graph never hides real event/route
activity just because the join failed.

### Node kind inference

Reuse the existing four `NodeKind` variants (`Catalyst`/`Blueprint`/`Fuse`/
`Service`) — no new kind needed for this pass. Kind is derived by name:
`"blueprint"` → `Blueprint`, `"catalyst"` → `Catalyst`, `"fuse"` → `Fuse`,
everything else → `Service`. Finer-grained kinds (e.g. distinguishing
tooling plugins like `bench`/`garage` from example services) are a future
enhancement, not required for a correct live model.

### Edge derivation

- **Wires** (static, non-animated): one per Gateway `Route`, from the `Fuse`
  node to the route's resolved target node — this is exactly what a gateway
  route *is*, an ingress wire. Whether to also always draw a `Blueprint` →
  every-registered-process wire (since every process heartbeats to Blueprint
  universally, not selectively) is an open question below — it's accurate,
  but risks turning into visual noise on a 13-service cluster.
- **Events** (animated, per the existing `cv-flow` keyframe): two hops per
  Topology edge — `producer → Catalyst` and `Catalyst → consumer` — carrying
  `event_type` and `vol` as trace metadata, matching the seed data's own
  existing shape (`ClTrace::event()` already models a broker hop this way;
  the seed's `auth-svc → catalyst-00 → ledger-svc` chain is exactly this
  pattern by hand). This is correct by construction: every Topology edge, by
  definition, is two CloudEvents through Catalyst, never a direct call.

### Live vs. manual nodes

Add `origin: NodeOrigin::Live | NodeOrigin::Manual` to `ClNode`. A live node
represents a real registered process or resolved route/topology endpoint —
its existence isn't user-editable (deleting the row on screen doesn't stop
the process, so the "Delete" context-menu action doesn't apply to it), but
its *position* still is. A manual node (added via the existing "+ Add Node"
context-menu flow) behaves exactly as it does today — fully editable,
deletable, never touched by a live-data refresh.

### Position persistence

Finish what `cluster-graph-persistence.md` started but never wired up against
the current renderer: on drag-end, write `{node_id (or name, for live
nodes): (x, y)}` plus pan/zoom to `localStorage` immediately (fast local
cache, survives a reload even if Blueprint is down) and to Blueprint KV under
`cluster/layout` (source of truth across browsers/sessions) via the same
`KeyValueServiceClient` `main.rs`'s nav-config fetch already uses. On mount,
apply saved positions for any node whose id/name matches; auto-place anything
new (see Phase 4). Live nodes are keyed by `name` (stable across restarts,
now that service identity itself is deterministic — see
`docs/architecture/service-registry-identity.md`); manual nodes keep their
existing `next_id()`-generated id.

## Non-goals

- No new proto messages or RPCs — this plan only adds client-side calls to
  RPCs that already exist and are already used elsewhere in this crate.
- No change to the PCB rendering, drag/pan/zoom, context menu, or config
  drawer — only what populates `nodes`/`traces` changes.
- No attempt to watch Gateway routes live — `ListRoutes` has no `Watch` RPC
  today (Gateway's own list view doesn't live-update either); this plan
  polls it on an interval instead, which is no worse than existing behavior
  elsewhere in this crate.

## Phase 1 — Wire up the three data fetches

- `views/cluster.rs`: add the same `use_resource`/`use_signal` + background
  `use_coroutine` Watch pattern `service_registry.rs` already uses for
  `ServiceDiscoveryService`, store the result as
  `processes: Signal<HashMap<String, Process>>`.
- Add a `use_resource` for `NetworkingService.ListRoutes` (mirroring
  `gateway.rs`) on a `gloo_timers`/`use_future` interval (e.g. 10s) rather
  than one-shot, storing `routes: Signal<Vec<Route>>`.
- Add the `TopologyService.GetTopology` + `WatchTopology` pattern
  `topology.rs` already uses, storing `topology: Signal<TopologyData>`.

**Artifact:** three live signals populated and visibly changing (log/print
their contents) with nothing else in the view changed yet — `seed_nodes()`/
`seed_traces()` still drive the actual render.

## Phase 2 — Node derivation

- New pure functions (unit-testable without Dioxus): `service_name_from_source(source: &str) -> Option<&str>`
  (strips `/services/`/`/plugins/`), and `derive_nodes(processes, routes, topology, existing_positions) -> Vec<ClNode>`
  implementing the identity join and kind inference from the Decisions
  section above.
- Replace `seed_nodes()`'s call site with `derive_nodes(...)`, keeping
  `seed_nodes()` itself around (renamed `demo_nodes()`) for reference/tests
  only.

**Artifact:** loading the Cluster page against the real `run-local` stack
shows a node for every actually-running service (`blueprint`, `catalyst`,
`fuse`, `beacon`, `bench`, `garage`, `echo`, `crud`, ...) with correct kind
coloring — verify by cross-checking against the Service Registry page's
grouped list.

## Phase 3 — Edge derivation

- New pure function `derive_traces(nodes, routes, topology) -> Vec<ClTrace>`
  implementing the wire/event derivation from the Decisions section.
- Wire it in alongside Phase 2's node derivation.

**Artifact:** wires match Fuse's actual route table (cross-check against the
Gateway page) and animated event traces match real event flow (cross-check
event types/volume against Beacon's Events/Query page).

## Phase 4 — Auto-layout for newly-discovered nodes

- Simple deterministic placement for a node with no saved/known position:
  lane by `NodeKind` (e.g. `Blueprint`/`Fuse`/`Catalyst` each in a fixed
  column, `Service` nodes filling a grid), not a force-directed layout — a
  full graph-layout algorithm is more machinery than a ~15-node cluster
  diagram needs. Revisit only if this looks bad in practice against the real
  cluster.
- A node with a saved position (drag history or a live node keyed by a
  `name` seen before) keeps it; only genuinely new nodes get auto-placed.

**Artifact:** restarting the stack (or a fresh browser with no saved layout)
produces a readable, non-overlapping default layout.

## Phase 5 — Position persistence

- Implement the `localStorage` + Blueprint KV (`cluster/layout`) round trip
  described in the Decisions section — write on drag-end, read on mount.
- Manual nodes and their existing edit/delete flow are unaffected; only the
  position-storage keying changes (now name-keyed for live nodes).

**Artifact:** dragging a node, reloading the page, and reloading in a
different browser profile all show the node in its last-dragged position.

## Phase 6 — Live refresh

- Confirm the Watch-driven signals (Registry, Topology) from Phase 1 flow
  through to `derive_nodes`/`derive_traces` reactively (a `use_memo` keyed on
  the three signals, replacing a one-shot derivation) — a service
  registering/disconnecting or a new event type appearing should update the
  diagram without a page reload, the same way `service_registry.rs`'s table
  already does.

**Artifact:** stop a watched service (e.g. `echo`) via the same restart-loop
technique used to verify `docs/architecture/service-registry-identity.md`;
watch its Cluster node's health badge flip to disconnected live, with no
manual refresh.

## Phase 7 — Verification pass

- Live-verify against `run-local-watch`: every real service present with
  correct kind/health; wires match Gateway; event edges match real
  type/volume; drag-persist round-trips through both localStorage and
  Blueprint KV; a manually-added custom node survives a live-data refresh
  untouched; restarting a service updates its node's health live without
  creating a duplicate (this plan reads the same deterministic `Process.name`
  identity the registry fix already guarantees, so no new dedup logic is
  needed here).

## Open questions

- **Blueprint→every-process wires:** technically accurate (every process
  really does heartbeat to Blueprint) but potentially cluttering on a
  larger cluster. Proposing to leave these out of v1 (Blueprint's
  relationship to services is implicit, shown by the Registry data feeding
  health badges, not drawn as N extra wires) and revisit if the diagram
  reads as incomplete without them.
- **Visual distinction for manual vs. live nodes** (e.g. a dashed border on
  manual nodes) — proposing yes, so it's obvious at a glance which nodes on
  screen represent real running processes vs. hand-added annotations, but
  the exact treatment is a small design pass during implementation, not
  decided here.
- **Gateway poll interval** — proposing 10s as a starting point (routes
  change far less often than process health or event volume); open to
  tightening or loosening once this is running against the real stack.

## Milestone Summary

| Phase | Scope | Status |
|---|---|---|
| 1 | Wire up the three data fetches (Registry, Gateway, Topology) | Not started |
| 2 | Node derivation (identity join + kind inference) | Not started |
| 3 | Edge derivation (wires from Gateway, events from Topology) | Not started |
| 4 | Auto-layout for newly-discovered nodes | Not started |
| 5 | Position persistence (localStorage + Blueprint KV) | Not started |
| 6 | Live refresh (Watch-driven reactive derivation) | Not started |
| 7 | Live verification against `run-local` | Not started |
