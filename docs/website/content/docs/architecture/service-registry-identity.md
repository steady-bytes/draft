---
weight: 39
title: "Service Registry — Deterministic Identity & Reaping"
description: "Root cause and fix for duplicate/orphaned Process entries in Blueprint's service registry: a content-addressed identity instead of a random per-restart UUID, and real deletion for entries that never come back."
icon: "fingerprint"
draft: false
toc: true
---

## Overview

Blueprint's service registry (`service_discovery`) shows many rows with the
same `name` and the same `ip_address`/advertise address, but different
`pid`, some `RUNNING`/`HEALTHY` and others `DISCONNECTED`/`UNHEALTHY`. These
are not concurrent instances — they're the accumulated history of one
logical service being restarted over and over, because the registry's
identity scheme mints a brand-new random ID on almost every restart instead
of recognizing "this is the same service coming back."

Two independent gaps cause this, and both need to be fixed:

1. **Identity is random, not derived from anything stable.** `Process.pid`
   (the field name predates this design and is not the OS PID — see its own
   proto comment) is `uuid.NewString()`, generated fresh on every
   `Initialize` call
   (`services/core/blueprint/service_discovery/controller.go`). There is a
   reuse heuristic — look for an existing entry with the same `name` already
   flagged `PROCESS_DICONNECTED` and reuse its ID — but it's racy: it only
   works if the old entry has already been marked disconnected by the time
   the new process calls `Initialize`. Disconnection is only detected when
   the old process's `Synchronize` stream closes server-side
   (`services/core/blueprint/service_discovery/rpc.go`), and there's no
   handshake between "old socket actually closed" and "new instance starts."
   On a fast restart or crash-loop, the new process almost always wins that
   race, finds nothing marked disconnected yet, and mints a fresh UUID — a
   brand-new registry row with the same `name` and (once it starts
   heartbeating) the same address as the old one.

2. **Stale entries are flagged, never removed.** The reaper (`Reap`,
   `services/core/blueprint/service_discovery/controller.go`) only flips
   `RunningState`/`HealthState` to disconnected/unhealthy once a process
   misses heartbeats past `staleThreshold` (15s). It never deletes the KV
   entry. So every lost race in (1) becomes a *permanent* duplicate, and the
   web-client's service registry view
   (`services/core/blueprint/web-client/src/views/service_registry.rs`)
   renders one row per `pid` forever, with no dedup by `name`/address.

Compare this to Consul: a Consul registration is keyed by a caller-supplied
service ID, so re-registering under the same ID is an idempotent upsert, and
`DeregisterCriticalServiceAfter` actually removes an entry once it's been
critical long enough. Blueprint has neither piece today.

## Decisions

### 1. Content-addressed process identity

Replace `uuid.NewString()` with a deterministic UUIDv5 derived from the
process's own `name` and `advertise_address`:

```go
id := uuid.NewSHA1(processNamespaceUUID, []byte(name+"@"+advertiseAddress)).String()
```

The same logical instance — same `name`, same reachable address — always
computes the same ID. `Initialize` becomes a pure upsert keyed by that ID:
write (or overwrite) the row at that key and return it. This removes the
race in root cause (1) entirely rather than narrowing it — there is nothing
left to race, because there's no lookup step to lose. The existing "find a
disconnected process with the same `name`" heuristic in `controller.go` is
deleted, not patched; it's made obsolete by determinism.

Two replicas of the same service (horizontal scaling) still get distinct
IDs, because their `advertise_address` differs per instance — confirmed via
every service's own `config.yaml` (e.g. `services/examples/echo/config.yaml`):
`internal.host`/`internal.port` is documented as "advertised to peers ...
and must be reachable from other host-native processes," i.e. it's the
per-instance reachable address, not a shared bind-all address.

### 2. `advertise_address` has to move earlier

Today `advertise_address` is only known to Blueprint via `ClientDetails` in
the `Synchronize` stream (`api/core/registry/service_discovery/v1/service.proto`),
sent *after* `Initialize` already assigned an ID. To compute the ID
deterministically at `Initialize` time, the client needs to send its address
up front. `chassis`'s `synchronize()` already computes this exact string
(`pkg/chassis/builder.go`, `adder := fmt.Sprintf("%s:%d", ...)`) — `initialize()`
needs to compute the same value earlier and send it in the request.

### 3. Two-tier lifecycle: mark, then delete

Keep the existing tier — heartbeat missed past `staleThreshold` (15s) →
`PROCESS_DICONNECTED` / `PROCESS_UNHEALTHY` — and add a second, longer tier:
heartbeat still missing past a new `DeregisterThreshold` while already
disconnected → delete the KV row outright. This is the direct analog of
Consul's `DeregisterCriticalServiceAfter`, and runs as a second check in the
same `Reap` loop/ticker that already exists.

### 4. Prerequisite: `Delete` has to actually go through raft

`key_value.Controller.Delete` exists today and is even exposed over RPC, but
it writes straight to the local Badger model
(`services/core/blueprint/key_value/controller.go`, `func (c *controller) Delete`),
skipping the leader-forwarding `Set` does when the current node isn't raft
leader, and skipping `raft.Apply` entirely. The FSM's `Apply` method has a
`Delete` case, but it's an unimplemented stub:

```go
case Delete:
    fmt.Println("TODO: make sure to call the `Apply` command with the `Delete` operations so it's committed to all nodes")
```

Calling `Delete` as it stands today only removes the key on whichever single
node happens to handle the call — other raft members never see it, and a
future leader election could resurrect the "deleted" key from a node that
never got the delete. This is a pre-existing bug, independent of the
registry work, but the reaper's new delete tier is the first caller that
would actually exercise it in a multi-node cluster, so it has to be fixed
first: build an LSM log with a `Operation_DELETE` payload and `raft.Apply`
it, the same shape `Set` already uses, and make the FSM's `Delete` case call
`c.model.Delete(payload.Key, ...)` instead of printing a TODO.

## API / data model changes

- `api/core/registry/service_discovery/v1/service.proto`: add
  `string advertise_address = 3;` to `InitializeRequest`.
- No changes to `Process`, `ClientDetails`, or the `KeyValueService` RPC
  surface — `Delete` already exists; only its internal replication path
  changes.

## Migration

No explicit migration step or backfill script. Every pre-existing
UUID-keyed row from the old random-ID scheme simply stops receiving
heartbeats the moment its owning process restarts (the restarted process
now computes a new deterministic ID and heartbeats under that key instead).
The orphaned old row ages past `staleThreshold`, then past
`DeregisterThreshold`, and the reaper's new delete tier removes it — the
same path that will clean up genuinely dead future entries. Old garbage and
new garbage are collected the same way.

## Non-goals

- **No client-initiated graceful deregister call on shutdown.** Not
  required for correctness: because identity is now content-addressed, even
  a "lost race" (new instance re-registers before the old one is marked
  disconnected) just re-upserts the same row instead of creating a
  duplicate. A graceful deregister would only shrink the visible
  `DISCONNECTED` window, which is a UX nicety, not a correctness fix — worth
  a future phase, not this one.
- **No change to raft leader election or cluster topology** beyond fixing
  `Delete`'s replication path.
- **No change to `staleThreshold`'s value** (still 15s) — only
  `DeregisterThreshold` is new.

## Resolved during implementation

- **`DeregisterThreshold` = 5 minutes, fixed (not configurable).** Shipped as
  a single package-level constant in
  `services/core/blueprint/service_discovery/controller.go`, same as
  `staleThreshold`. Per-service or cluster-wide configurability was left out
  as unnecessary scope — nothing in the design depends on tuning it per
  deployment, and it's a one-line change later if that changes.
- **`JoinedTime` resets on every `Initialize`.** A restart is treated as a
  fresh join; `Initialize` always builds a new `Process` (with a fresh
  `JoinedTime: timestamppb.Now()`) and lets the deterministic id's `Set`
  overwrite whatever was there before, rather than reading back and
  preserving the old value. Simpler code, and consistent with "a restart is
  a legitimate new join" from the Decisions section above.
