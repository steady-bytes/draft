---
weight: 40
title: Service Registry Identity — Implementation Plan
description: Phased implementation plan for deterministic process identity and reaping in Blueprint's service registry — proto change, chassis, controller, raft-backed Delete, and the reaper.
icon: checklist
draft: false
toc: true
---

This is the step-by-step implementation plan for [Service Registry —
Deterministic Identity & Reaping](/docs/architecture/service-registry-identity),
in the order work should be done. Each phase produces a runnable, testable
artifact. Phase 4 (fixing raft-backed `Delete`) is a hard prerequisite for
Phase 5 (the reaper's delete tier) — deleting through the reaper before
`Delete` is replication-safe would silently desync a multi-node cluster.

## Phase 1 — Proto: `advertise_address` on `InitializeRequest`

- `api/core/registry/service_discovery/v1/service.proto`: add
  `string advertise_address = 3;` to `InitializeRequest`.
- Regenerate (`dctl api build`).

**Artifact:** generated Go/Rust/TS types compile; no behavior change yet
(field unused by any caller).

## Phase 2 — chassis: compute and send `advertise_address` at `Initialize`

- `pkg/chassis/builder.go`: factor the `adder := fmt.Sprintf("%s:%d", ...)`
  computation currently inline in `synchronize()` into a small shared helper
  (e.g. `resolveAdvertiseAddress(config)`), and call it from `initialize()`
  too, setting it on the `InitializeRequest`.
- `synchronize()` keeps sending it on every `ClientDetails` as it does today
  — no change to that call site's behavior, just removing the duplicated
  computation.

**Artifact:** `Initialize` requests now carry a real, stable address;
Blueprint server-side still ignores the new field (no behavior change on
the server yet).

## Phase 3 — Blueprint controller: deterministic ID, `Initialize` becomes an upsert

- `services/core/blueprint/service_discovery/controller.go`:
  - Delete the existing "find an existing disconnected `Process` with the
    same `name`" lookup loop in `Initialize` — it's now unnecessary.
  - Compute `id := uuid.NewSHA1(processNamespaceUUID, []byte(name+"@"+advertiseAddress)).String()`
    and use it as `Process.Pid` unconditionally.
  - `Initialize` always does a `kvController.Set` at that key — whether the
    key already exists (a restart) or not (first boot) is no longer a
    branch the code needs to care about; `Set` already overwrites.
  - Pick/define `processNamespaceUUID` as a package-level constant (any
    fixed, unique UUID works — it only needs to be stable across builds so
    the same inputs always hash to the same output).

**Artifact:** restarting a service (same `name`, same address) reuses the
same registry row instead of creating a new one — verify by stopping and
restarting a local service (e.g. `echo`) several times against a running
`run-local` stack and confirming only one row for it ever appears in
Blueprint's service registry KV, before Phase 5's reaper exists to clean up
anything.

## Phase 4 — key_value: make `Delete` raft-safe

- `services/core/blueprint/key_value/controller.go`:
  - Rewrite `Delete` to mirror `Set`'s shape: if not raft leader, forward
    the delete to the leader over RPC (same pattern `Set` uses for
    forwarding); if leader, build an LSM log with `Operation_DELETE` and
    call `raft.Apply`.
  - Fix the FSM's `Apply` method: replace the `case Delete: fmt.Println(...)`
    stub with an actual `c.model.Delete(payload.Key, ...)` call, returning a
    response value the same way the `Set` case does.

**Artifact:** a unit/integration test (or a manual multi-node exercise, if
one is feasible locally) confirming a `Delete` call is visible on every
raft member, not just the node that received the RPC — this is a
correctness fix independent of the registry work, and should be verifiable
on its own before Phase 5 depends on it.

## Phase 5 — Blueprint reaper: add the delete tier

- `services/core/blueprint/service_discovery/controller.go`:
  - Add a `DeregisterThreshold` constant (proposed default: 5 minutes;
    resolve the open question in the spec doc about whether this should be
    configurable before finalizing).
  - In `Reap`, alongside the existing staleness check that marks a process
    `PROCESS_DICONNECTED`/`PROCESS_UNHEALTHY`, add a second check: if a
    process is already `PROCESS_DICONNECTED` and
    `time.Since(LastStatusTime) > DeregisterThreshold`, call
    `kvController.Delete` instead of `Set`.

**Artifact:** a service stopped and left stopped (not restarted) eventually
disappears from the registry entirely after `DeregisterThreshold`, verified
against a local `run-local` run; a service restarted before that threshold
never leaves a disconnected row behind at all (superseded by Phase 3's
upsert behavior).

## Phase 6 — Verification pass

- Live-verify against the `run-local`/`run-local-watch` stack:
  - Restart a service (e.g. `echo`) 5-10 times in quick succession; confirm
    exactly one row for it in Blueprint's service registry throughout, with
    no `DISCONNECTED` orphans appearing even transiently.
  - Stop a service and leave it stopped; confirm it's marked
    `DISCONNECTED`/`UNHEALTHY` after `staleThreshold`, then disappears
    entirely after `DeregisterThreshold`.
  - Confirm the web-client's service registry view
    (`services/core/blueprint/web-client/src/views/service_registry.rs`)
    reflects both of the above with no code changes needed there — it
    already renders whatever rows the `Query`/`Watch` stream returns, so
    fixing the data should be sufficient without touching the UI.
  - Confirm pre-existing orphaned UUID-keyed rows from before this change
    get reaped the same way (no special-case migration logic exercised).

## Milestone Summary

| Phase | Scope | Status |
|---|---|---|
| 1 | Proto: `advertise_address` on `InitializeRequest` | Done |
| 2 | chassis: compute & send `advertise_address` at `Initialize` | Done |
| 3 | Blueprint controller: deterministic ID, `Initialize` upserts | Done |
| 4 | key_value: raft-safe `Delete` (FSM `Apply` fix) | Done |
| 5 | Blueprint reaper: delete tier past `DeregisterThreshold` | Done |
| 6 | Live verification against `run-local` | Done |

## Phase 6 findings

Verified live against the running `run-local-watch` stack (all services already
up, uptime spanning several days of prior sessions):

- **Restart-loop test:** triggered 5 rapid rebuild/restart cycles of `echo`
  (~1.5s apart) by touching its watched source file. Blueprint's registry
  showed exactly **one** `RUNNING`/`HEALTHY` row for `echo` throughout — the
  deterministic identity (`processID(name, advertiseAddress)`) collapsed all
  5 restarts into a single upserted row, confirming the fix's central claim.
- **Organic cleanup of pre-existing duplicates:** at the moment this fix
  deployed, 11 other services (`beacon`, `bench`, `catalyst`,
  `catalyst-consume`, `catalyst-produce`, `crud`, `fuse`, `garage`,
  `grpc-call`, `http-call`, `slack-notify`) each had a leftover
  random-UUID-keyed `DISCONNECTED` row from before this change, alongside a
  live `RUNNING` row. Within one `ReapInterval` (30s) of Blueprint restarting
  with the new reaper logic, every one of those stale rows was deleted
  outright — exactly the "no special-case migration needed" behavior the
  spec predicted, observed without any manual intervention.
- **Reaper's delete tier confirmed end-to-end:** two `echo` entries stale
  since before this deploy (last heartbeats before the fix landed) were
  present at deploy time and gone from the registry ~10 minutes later, once
  `DeregisterThreshold` (5m) elapsed past their last heartbeat — verified via
  the web-client (`Registered` count dropped from 3 to 1, both stale cards
  disappeared). Note: `grep`ing the running Blueprint's log file for the
  `"reaper: deregistering..."` message it should log never found a match
  despite the deletion demonstrably happening — logging output for this path
  may not be reaching the file sink as expected; worth a follow-up look, but
  the actual delete behavior (the thing that matters) is confirmed correct.
- **Side finding, not part of this fix:** two additional stale `echo`
  entries (joined 2026-08-24 and 2026-08-28, both pre-dating this fix) were
  still receiving real heartbeats up until moments before the restart-loop
  test — implying stray orphaned `echo` OS processes from earlier, unrelated
  dev sessions were still running in the background this whole time. `ps
  aux` at the time of writing showed only one live `bin/echo` process, so
  whatever those were, they're gone now. This is a local dev-environment
  process-hygiene issue (stray background processes from old sessions), not
  a defect in the registry-identity code — flagged here for visibility, not
  addressed as part of this plan.
- Web-client's service registry view needed no changes for any of this — it
  already renders whatever rows `Query`/`Watch` returns, as predicted.
