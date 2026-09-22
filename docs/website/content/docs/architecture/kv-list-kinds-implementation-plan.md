---
weight: 42
title: Key/Value — Discoverable Kinds Implementation Plan
description: Adding a ListKinds RPC to Blueprint's KeyValueService so the web client can discover and filter by every distinct value type stored, instead of only ever showing the one hardcoded kind it happens to ask for.
icon: category
draft: false
toc: true
---

This is the step-by-step implementation plan for letting Blueprint's
Key/Value store answer "what kinds of values exist in here?" — something it
genuinely cannot do today — and for using that in the web client to let a
user filter the Key/Value page by kind instead of only ever seeing one
hardcoded type.

## Current state

`KeyValueService.List` only ever returns entries matching a *specific*
`Any.type_url` the caller already knows and passes in — the proto's own
comment says as much: *"List accepts a type to search the key_value store
for all keys matching that type."* Server-side
(`services/core/blueprint/key_value/model.go`), the physical Badger key is
literally `<type_url>-<key>`, and `List` prefix-scans seeded by the
type_url you supply. There is no RPC to enumerate the distinct type_urls
actually present.

**Concrete, currently-live consequence:** the web client's Key/Value page
(`views/key_value.rs`) only ever calls `List` with one hardcoded type —
`core.registry.key_value.v1.Value` (the generic string wrapper most entries
use, including `leader`, `fuse_address`, and `cluster/layout`). But
`ui/navigation` is stored as a `core.registry.key_value.v1.NavigationConfig`
— a different type_url — so it exists in the store (`Get` reads it fine on
app boot) and yet **never appears in the Key/Value list page at all**,
because that page's `List` call is filtered to a kind that isn't
`NavigationConfig`. A user browsing that page has no way to know this other
kind of data even exists.

## Decisions

### A full scan is the only way to discover kinds, and that's fine here

Since type URLs never contain a hyphen (they're dotted proto package/message
names — `type.googleapis.com/core.registry.key_value.v1.Value`), the
physical key format `<type_url>-<key>` can be split unambiguously on its
*first* `-` to recover the type_url, with no schema knowledge needed ahead
of time. `ListKinds` does an unprefixed Badger scan, splits every key this
way, and returns the distinct type_urls found, each with a count. Blueprint's
KV store is a small, cluster-configuration-scale store (dozens to low
hundreds of keys, not a general-purpose database) — an unprefixed scan here
is the same cost class `List` already pays for any single kind today, just
without the prefix filter.

### `ListKinds` returns counts, not just names

`repeated KindSummary kinds`, each `{ type_url, count }`, rather than a bare
`repeated string`. The web client's dropdown wants to show "Value (12)" /
"NavigationConfig (1)" rather than an unlabeled list, and the count falls
out of the same scan for free.

### The web client filters by kind; it does not attempt full generic decoding of every kind

The Key/Value list page gets a kind-filter dropdown (populated from
`ListKinds`, defaulting to the current hardcoded `Value` kind so existing
behavior is unchanged until a user picks something else) and re-runs `List`
against whichever kind is selected. For the **value preview column**, only
the already-understood `Value{data: string}` shape gets decoded and shown
as text (as today); any other kind shows its type name and raw byte count
rather than attempting to decode/pretty-print its actual structure. Building
a generic reflection-based Any→JSON renderer (so *any* proto kind
pretty-prints automatically, the way `protojson` does server-side in Go) is
real, separable work — Rust's `prost` doesn't ship that out of the box the
way Go's `protojson` does — and is called out as a non-goal below rather
than bundled in here.

## Non-goals

- **No generic Any→JSON decoding for arbitrary kinds.** Only `Value` gets a
  real preview; other kinds show type + size. A future phase could add
  per-kind decoders one at a time (`NavigationConfig` would be the obvious
  first one, given it's the only other kind that exists in practice today).
- **No change to `List`'s or `Get`'s existing single-kind-at-a-time
  contract.** `ListKinds` is additive.
- **No change to read consistency.** `ListKinds` reads local Badger state
  the same way `List`/`Get` already do (no raft leader-forwarding) — same
  potential-staleness-on-a-follower characteristics those already have,
  not a new concern this plan introduces.

## Phase 1 — Proto: `ListKinds`

- `api/core/registry/key_value/v1/service.proto`:
  ```proto
  rpc ListKinds(ListKindsRequest) returns (ListKindsResponse) {}

  message ListKindsRequest {}

  message KindSummary {
      string type_url = 1;
      uint32 count = 2;
  }

  message ListKindsResponse {
      repeated KindSummary kinds = 1;
  }
  ```
- Regenerate (`dctl api build`).

**Artifact:** generated Go/Rust types compile; no behavior change yet.

## Phase 2 — Server: model, controller, RPC

- `services/core/blueprint/key_value/model.go`: add `ListKinds() ([]KindSummary, error)`
  (or an equivalent internal type) — an unprefixed `badger` iterator over
  every key, splitting each on its first `-` to recover the type_url,
  accumulating counts in a `map[string]uint32`, returned as a sorted slice
  (by type_url) for deterministic output. Skip (and log, don't fail) any
  key that doesn't contain a `-` at all — malformed/legacy data shouldn't
  crash discovery.
- `services/core/blueprint/key_value/controller.go`: thin passthrough,
  matching `List`'s existing shape (no raft involvement, per the Decisions
  section).
- `services/core/blueprint/key_value/rpc.go`: `ListKinds` handler wrapping
  the controller call.

**Artifact:** a manual RPC call (e.g. via `buf curl` or a quick Go test)
against the running local store returns every kind currently present —
verify it includes both `core.registry.key_value.v1.Value` and
`core.registry.key_value.v1.NavigationConfig` with correct counts.

## Phase 3 — Web client: kind-filter dropdown

- `services/core/blueprint/web-client/src/views/key_value.rs`:
  - Fetch `ListKinds` on mount into a signal alongside the existing `list_result`.
  - Add a `<select>` populated from that signal, each option labeled with a
    short name (strip the `type.googleapis.com/` prefix — "core.registry.key_value.v1.Value"
    is still fully unambiguous, or trim to just the trailing message name
    "Value"/"NavigationConfig" for brevity) and the count, e.g. "Value (12)".
  - Defaults to `core.registry.key_value.v1.Value` (current hardcoded
    behavior) so nothing changes for an existing user until they interact
    with the dropdown.
  - Changing the selection re-runs `List` with the newly selected type_url
    (mirroring the existing `use_resource`/`list_result.restart()` pattern
    already used after Set/Delete).

**Artifact:** selecting "NavigationConfig (1)" in the dropdown shows the
`ui/navigation` row — the exact entry that's invisible on this page today —
with its key, type_url, and a value column showing something honest (see
Phase 4) rather than the page silently omitting it.

## Phase 4 — Value-column fallback for non-`Value` kinds

- In the row-mapping logic (currently `Value::decode(any.value.as_slice())`
  unconditionally), only attempt that decode when the selected kind's
  type_url is `core.registry.key_value.v1.Value`. For any other kind, show
  `"<TypeName> — N bytes"` instead of attempting a decode that would either
  error or (worse, per the Decisions section) silently produce garbage from
  a wire-format coincidence.

**Artifact:** switching between kinds never shows decode garbage — either a
real decoded string (`Value` kind) or an honest "can't preview this kind
yet" placeholder.

## Phase 5 — Verification

- Live-verify against the running stack: confirm `ListKinds` reports the
  real current kinds and counts; confirm the dropdown defaults match
  today's behavior with no selection made; confirm selecting
  `NavigationConfig` surfaces `ui/navigation`; confirm the detail page
  (`/kv/:..kv_key_parts`, from the earlier pretty-print work) still works
  unchanged for `Value`-kind entries reached through the filtered list.

## Resolved during implementation

- **The detail page does take the kind into account** — the first open
  question above. Implemented in the same pass as Phase 4 rather than left
  as a follow-up: the list page hands the selected kind to the detail page
  via a `PENDING_KV_KIND` global-signal hand-off (mirroring Beacon's
  existing `PENDING_TRACE_ID` pattern), and `Get` now fetches under the
  actual kind rather than always assuming `Value` — needed for correctness,
  not just preview quality, since `Get`'s `value.type_url` determines the
  physical key looked up (`model.go`'s `makeKey`); fetching a non-`Value`
  entry under the wrong type_url wouldn't just preview it wrong, it
  wouldn't find it at all.
- **`NavigationConfig` decoder:** not built — no entry of that kind
  actually exists in the live store right now (nothing has ever written to
  `ui/navigation`; the app just falls back to `default_nav_config()` every
  time), so there was nothing to decode against. The dropdown will surface
  it correctly by kind/count whenever one is eventually saved; a real
  decoder remains future work if that becomes worth doing.

## Phase 5 findings

- Live-verified against the real running store: `ListKinds` correctly
  reported the three kinds actually present — `Route` (11, Fuse's
  routes, persisted via Blueprint KV), `Value` (3), and `Process` (12,
  the Service Registry) — with accurate counts.
- **Caught and fixed a real bug during verification, not anticipated by
  this plan:** `ListKinds`'s `defer txn.Discard()` was registered *before*
  `defer it.Close()`, and Go defers run LIFO — so `Discard` fired first,
  and Badger panics ("Unclosed iterator at time of Txn.Discard") if the
  transaction is discarded while its iterator is still open. Every call
  crashed that request (recovered per-request by Go's http2 server, not a
  process crash) until the defer order was swapped.
- **Caught and fixed a second real bug, more serious:** the detail page's
  first version read `PENDING_KV_KIND` and immediately wrote `None` back to
  clear it directly in the component body (not inside `use_effect`).
  Reading a signal during render subscribes that render to it; writing to
  the same signal right after — even just to clear it — re-triggers the
  same component, forever. This froze the browser tab solid (confirmed via
  a fresh tab working fine against the exact same route once fixed).
  Wrapping the read-then-clear in `use_effect`, matching Beacon's own
  already-shipped `PENDING_TRACE_ID` pattern in `views/traces.rs`, fixed
  it — the existing precedent turned out to be load-bearing, not just a
  style choice.
- Confirmed the non-`Value` fallback (`"<Kind> — N bytes"`) renders
  correctly in both the list and detail pages for real `Process` entries,
  with no decode-garbage.

## Milestone Summary

| Phase | Scope | Status |
|---|---|---|
| 1 | Proto: `ListKinds` RPC | Done |
| 2 | Server: model/controller/RPC (full scan, split-on-first-`-`) | Done |
| 3 | Web client: kind-filter dropdown | Done |
| 4 | Value-column fallback for non-`Value` kinds | Done |
| 5 | Live verification | Done |
