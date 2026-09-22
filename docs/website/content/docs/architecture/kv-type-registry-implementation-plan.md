---
weight: 43
title: Key/Value — Type Registry Implementation Plan
description: Letting a service explicitly register its custom proto type's descriptor with Blueprint so the Key/Value web UI can safely decode and render it generically, instead of only ever showing type name and byte count.
icon: schema
draft: false
toc: true
---

This is the step-by-step implementation plan for a **type registry** in
Blueprint: a way for a service to explicitly tell Blueprint "here is the
schema for this proto type I store in your Key/Value store," so the Key/Value
web UI can decode and render values of that type generically — without ever
being recompiled to know about it.

## Current state

Following the [Discoverable Kinds]({{< ref "kv-list-kinds-implementation-plan" >}})
work, the Key/Value page (`views/key_value.rs`) can already discover and
filter by every distinct kind stored in Blueprint. But only one kind actually
gets decoded: `core.registry.key_value.v1.Value`, hand-coded directly into
the web client. Every other kind — `Route`, `Process`, and any future
kind a plugin or tooling service stores — renders as `"<TypeName> — N
bytes"`. That's a deliberate, documented non-goal of the previous plan
("no generic Any→JSON decoding for arbitrary kinds"), not an oversight — but
it means the Key/Value page is only ever really useful for one kind of data,
and a new kind stored by a new service is invisible in practice even once
it's discoverable by name and count.

The fundamental obstacle: the web client is a compiled Rust/WASM binary.
`prost` (the crate generating its proto types) only knows how to decode a
message it was compiled against — it has no reflection API for "decode this
byte string, I'll tell you the type_url at runtime," the way Go's
`google.golang.org/protobuf/reflect/protoreflect` and
`google.golang.org/protobuf/types/dynamicpb` do. Shipping a real fix means
either recompiling the web client every time a new kind is added anywhere in
the system (untenable — Blueprint would need to know about every plugin's
types at its own build time), or moving the "does anyone know how to decode
this type_url" question and the decoding itself somewhere that already has
full reflection: Blueprint's own Go process.

## Decisions

These were worked through directly before writing this plan — each answers a
question that had more than one reasonable answer.

### Decoding happens server-side, in Blueprint (Go)

Blueprint already has full `protoreflect`/`dynamicpb` support in its own
runtime and is already the thing every service already talks to for KV
reads. Given a type's descriptor, it can build a `dynamicpb.Message`,
`proto.Unmarshal` the raw bytes into it, and `protojson.Marshal` the result
to a plain JSON string — no bespoke per-type code, and no new dependency in
the WASM build. The alternative (client-side decoding via something like
`prost-reflect` compiled into the web client) would work too, but means a
reflective protobuf decoder running in WASM against attacker-adjacent bytes
(anything written to the KV store), and gets none of the "just a returned Go
error, not a browser panic" safety property below for free.

### Registration is explicit, not automatic-on-write

A service that wants one of its custom types to be viewable calls a new
chassis helper once, deliberately, naming the type. Blueprint does not try to
infer or auto-register a descriptor the first time it sees an unfamiliar
`type_url` written to the KV store. This trades a small bit of friction (an
extra line in a service's `main.go`) for values never appearing "spontaneously
decodable" the moment a schema happens to exist somewhere in the binary that
wrote them, and for there being one obvious place to look
(`WithRegisteredType` calls) when auditing what's decodable.

### "Safely" means "never crashes the UI," not new sanitization

Decoded values render as plain text through Dioxus's normal RSX text
interpolation (`"{value}"`), which is escaped by default — the same way every
other piece of user- or network-sourced text already rendered in this web
client is handled today (route names, process ids, etc.). No new escaping
work is needed for that. What *is* new: a malformed payload, a stale/wrong
descriptor, or a decode-path panic must never take down the page. Every
per-value decode attempt on the server is wrapped so a single bad entry
degrades to "not decodable" for *that entry only* — it never fails the whole
request, and never reaches the client as anything other than "no JSON for
this key." The existing `views/key_value_detail.rs` infinite-loop incident
earlier in this project (a signal read-then-cleared directly in a component
body) is exactly the class of self-inflicted frontend crash this design
avoids by keeping the client dumb: it renders whatever JSON string it's
handed, or falls back, and never runs new decode logic of its own.

### Scope: unknown/third-party kinds only — `Route`, `Process`, `Value` are untouched

`Value` keeps its existing hand-written decode. `Route` and `Process` are
*not* registered through this mechanism — they stay `"<TypeName> — N bytes"`
in the Key/Value browser exactly as today, same as any other kind nobody has
registered. Both already have dedicated, purpose-built pages elsewhere
(Gateway, Service Registry) that are strictly better than a generic JSON tree
for their shape of data; this plan's target audience is a plugin or tooling
service storing its *own* structured configuration with nowhere else to show
it. If a first-party kind ever wants generic decoding too, it can call
`WithRegisteredType` like anyone else — nothing about this design privileges
or excludes first-party types structurally, this is purely about not
bothering with it for the three that already have a better home.

### The list page shows a real truncated preview, not just a "(decoded)" badge

A row for a decodable kind shows a short, single-line snippet of the actual
decoded JSON (eg. the first ~40 characters) directly in the list, the same
way `Value` already shows real text inline today — not just an indicator
that a preview exists somewhere else. The full `JsonTree` still only renders
on the detail page (a whole nested tree doesn't fit in a packed row), but the
list itself should look and feel like every other kind's row, not like a
second-class placeholder pointing elsewhere.

### `TypeDescriptor` is not special-cased out of the kind dropdown

It shows up in the Key/Value page's kind-filter dropdown like any other
kind — `TypeDescriptor (2)` alongside `Value`, `Route`, `NavigationConfig`,
etc. It renders as `"TypeDescriptor — N bytes"` in the list (nobody registers
a decoder for the registry's own storage type in this pass), which is an
honest, unsurprising outcome given the rest of this design, not a bug to
special-case around. Filtering kinds out of that dropdown is a precedent
worth being wary of in general — it's exactly the "hide the entry no matter
what its kind is" instinct that made `NavigationConfig` invisible for the
whole first pass of the Key/Value page.

## Non-goals

- **No client-side (WASM) protobuf reflection.** All decoding happens in
  Blueprint. The web client never parses raw proto bytes for anything but
  `Value`, which it already does today.
- **No automatic/implicit registration.** Writing a value of a new type_url
  to the KV store does not register it. Only an explicit
  `WithRegisteredType` (or a direct `RegisterType` RPC call) does.
- **No registry admin UI.** No page to browse, search, or unregister
  registered types in this pass. The only visible surface is that
  previously-opaque kinds now render decoded where a descriptor exists.
- **No authorization/ownership model for registration.** Any service can
  register (or overwrite) the descriptor for any `type_url`, the same
  no-ownership-check posture every other KV write in this system already
  has (eg. `AddRoute`).
- **No schema versioning beyond what protobuf's own wire compatibility
  already provides.** Registering a new descriptor for a `type_url`
  overwrites the previous one outright. A non-additive schema change
  (renumbered/retyped field) is the caller's own problem, same as it already
  is for every proto in this repo (append new fields, never renumber/reuse
  one) — this plan doesn't add new tracking on top of that.
- **No decoding for `Route`/`Process`/`Value`.** Covered under Decisions
  above; called out again here because it's a real, deliberate limitation of
  this pass, not a gap to be discovered later.

## Phase 1 — Proto: registration and decode RPCs

`api/core/registry/key_value/v1/service.proto`:

```proto
rpc RegisterType(RegisterTypeRequest) returns (RegisterTypeResponse) {}
rpc DecodeValues(DecodeValuesRequest) returns (DecodeValuesResponse) {}

// TypeDescriptor is both the request payload for RegisterType and the shape
// persisted in the KV store under its own type_url, mirroring how every
// other registration message in this repo (eg. networking's Route) is used
// both as a wire request and as the thing that gets stored.
message TypeDescriptor {
    // The type_url values of this type are stored under (eg.
    // "type.googleapis.com/tooling.foundry.v1.PluginManifest").
    string type_url = 1;
    // A serialized google.protobuf.FileDescriptorSet containing the message
    // named by type_url and every file it transitively depends on.
    bytes file_descriptor_set = 2;
}

message RegisterTypeRequest {
    TypeDescriptor descriptor = 1;
}

message RegisterTypeResponse {}

message DecodeValuesRequest {
    string type_url = 1;
    // Caller-chosen keys (eg. the same keys already returned by List for
    // this type_url) mapped to the raw message bytes to decode.
    map<string, bytes> values = 2;
}

message DecodeValuesResponse {
    // Same keys as the request. A key is absent from this map, or maps to
    // an empty string, whenever that specific entry couldn't be decoded —
    // no descriptor registered for type_url, corrupt bytes, or anything
    // else going wrong for that one entry. Never fails the whole call.
    map<string, string> json = 1;
}
```

Regenerate (`dctl api build`).

**Artifact:** generated Go/Rust types compile; no behavior change yet.

## Phase 2 — Server: descriptor registration

New file `services/core/blueprint/key_value/type_registry.go`, kept separate
from `model.go` (pure Badger IO) and `controller.go` (KV business logic) since
this is a distinct concern: proto reflection, not key/value storage.

- An in-memory cache, `map[string]protoreflect.MessageDescriptor` keyed by
  `type_url`, guarded by a mutex (or `sync.Map`) — every Blueprint raft node
  needs its own copy, since Fuse now load-balances traffic (including
  Blueprint's own UI route) across every registered instance, so any node in
  a multi-node Blueprint cluster might be the one that ends up answering a
  given `DecodeValues` call.
- `RegisterType(descriptor *kvv1.TypeDescriptor) error`:
  1. Unmarshal `file_descriptor_set` into a `descriptorpb.FileDescriptorSet`.
  2. `protodesc.NewFiles(set)` to build a `*protoregistry.Files`.
  3. `files.FindDescriptorByName(protoreflect.FullName(strings.TrimPrefix(type_url, "type.googleapis.com/")))`
     — **fail fast** here if this doesn't resolve to a message descriptor;
     a `RegisterType` call with a descriptor set that doesn't actually
     contain its own claimed type_url is caller error, and should be
     rejected up front rather than accepted and silently never decoding
     anything later.
  4. Persist via the existing `model.Set(type_url, &kvv1.TypeDescriptor{...})`
     — reuses the exact same KV write path every other kind already goes
     through; `TypeDescriptor` is just another kind, replicated via raft for
     free like everything else.
  5. Update the in-memory cache with the resolved descriptor so it's
     immediately usable without waiting for a restart.
- **Startup rehydration:** on Blueprint boot, after the model's underlying
  Badger store opens, list every existing `TypeDescriptor` entry
  (`model.List(&kvv1.TypeDescriptor{})`) and repeat steps 2–3/5 above for
  each, rebuilding the in-memory cache from what's already persisted. Without
  this, a freshly (re)started node — or one of the four followers in a
  multi-node Blueprint cluster that never personally handled the original
  `RegisterType` call — would have an empty cache despite the descriptor
  being right there in its own replicated KV store.
- `controller.go` / `rpc.go`: thin `RegisterType` passthrough, matching the
  existing shape of every other RPC in this package.

**Artifact:** a raw RPC call registering `NavigationConfig`'s descriptor
(built via the Phase 4 helper against that real, already-existing type — see
Phase 7) succeeds; the entry is visible via `ListKinds` as
`core.registry.key_value.v1.TypeDescriptor` (count 1); restarting Blueprint
and re-querying confirms the in-memory cache survives via rehydration, not
just the original in-process registration.

## Phase 3 — Server: `DecodeValues`

Also in `type_registry.go`:

- Look up the cached descriptor for `type_url`. If none exists, return an
  empty `json` map immediately — cheap, and exactly the outcome the client
  needs to fall back to today's byte-count display for every entry.
- Otherwise, for each `(key, bytes)` in the request:
  1. `dynamicpb.NewMessage(descriptor)`.
  2. `proto.Unmarshal(bytes, msg)` — on error, skip this key (leave it out of
     the response map) and continue with the rest of the batch.
  3. `protojson.Marshal(msg)` — on error, same as above.
  4. Wrap steps 1–3 in a `recover()`, matching the "never crash" decision:
     a hostile or corrupt payload causing a panic somewhere in `dynamicpb`'s
     decode path (not expected in normal operation, but not something to
     bet the whole RPC — or the process — on never happening) degrades that
     one key to "not decodable" exactly like a normal unmarshal error would.
- One bad entry never affects any other entry in the same batch, and a
  batch for an unregistered type_url never errors at all — it just comes
  back empty.

**Artifact:** `DecodeValues` against real `NavigationConfig` bytes returns
correct JSON matching the source data; the same call with the last byte of
the value truncated returns an empty map entry for that key rather than an
RPC error; a call for a `type_url` nothing has registered returns an
all-empty map.

## Phase 4 — Chassis: `WithRegisteredType`

New file `pkg/chassis/type_registry.go`, alongside `networking.go`'s existing
single-concern-per-file pattern:

- `func (c *Runtime) WithRegisteredType(msg proto.Message) *Runtime`. Unlike
  `WithRoute`, this needs no `WithRunner`-after-`Start()` dance — it's an
  ordinary outbound call to Blueprint's `KeyValueService`, the same kind
  every other direct KV call in this codebase already makes synchronously
  from within the builder chain (via `c.config.Entrypoint()`). There's no
  "can't reach myself before I'm listening" problem here because the target
  (Blueprint) is a separate, already-running process for every caller this
  plan targets — per the scope decision, no first-party (Blueprint-about-
  itself) type is being registered, so that harder case doesn't need solving
  now.
- Implementation:
  1. `md := msg.ProtoReflect().Descriptor()`.
  2. Walk `md.ParentFile()` and its `Imports()` recursively, collecting each
     into a `descriptorpb.FileDescriptorProto` via
     `protodesc.ToFileDescriptorProto`, deduplicated by file path, into one
     `descriptorpb.FileDescriptorSet`. (Go's linked-in proto registry
     already has the full dependency graph in memory for any message
     compiled into the binary — this is a pure in-memory walk, no I/O.)
  3. `type_url := "type.googleapis.com/" + string(md.FullName())`.
  4. Call `RegisterType`. Panic on failure — a service that explicitly asked
     for this and can't get it is misconfigured, matching the existing
     fail-loud convention for other builder-chain registration calls
     (`Register()`, `WithRoute`) rather than silently degrading.

**Artifact:** a one-line addition to a test service's `main.go`
(`.WithRegisteredType(&kvv1.NavigationConfig{})`, used only for Phase 7
verification, not shipped as a real feature of that service) results in the
type being queryable via `DecodeValues` afterward, exercising the real
builder-chain path rather than a hand-built RPC call.

## Phase 5 — Web client: generic decoded-value rendering

- New component `services/core/blueprint/web-client/src/components/json_tree.rs`:
  a small recursive renderer over a parsed `serde_json::Value` — object keys
  as labels, arrays as indexed children, scalars as leaf text. Registered in
  `components/mod.rs` alongside the other shared components.
- `views/key_value.rs`: when the selected kind isn't `Value`, after `List`
  resolves, call `DecodeValues` once with the current page's raw byte values
  keyed by their existing KV keys, and merge the result in. Per row: if
  `DecodeValues` returned non-empty JSON for that key, show a truncated
  single-line preview of it (eg. the first ~40 characters, matching how
  `Value` already shows real decoded text inline) instead of the byte count;
  otherwise keep today's `"<TypeName> — N bytes"` fallback unchanged. The
  full tree still only renders on the detail page — a whole nested structure
  doesn't fit in a packed row — but the row itself should read like every
  other kind's row, not like a placeholder pointing elsewhere.
- `views/key_value_detail.rs`: same `DecodeValues` call, scoped to the single
  entry being viewed. On success, render the full `JsonTree` in place of the
  existing "No preview available for this kind yet" placeholder; on failure
  (empty result), that placeholder is exactly what's already there today —
  no change needed for the fallback path itself.
- Client-side, additionally guard `serde_json::from_str` on whatever
  `DecodeValues` returns: if the server ever returned something that isn't
  valid JSON (shouldn't happen, given Phase 3, but costs nothing to check),
  treat it identically to "no JSON for this key" rather than trusting it
  blindly.

**Artifact:** browsing the Key/Value page with `NavigationConfig` selected
shows real entries (once one exists — see Phase 7) with a truncated JSON
preview instead of a byte count; opening one shows a real nested tree
(`sections[].items[].{label,path}`) instead of the placeholder; every other
existing kind (`Value`, `Route`, `Process`, `TypeDescriptor`) renders exactly
as it would without this plan.

## Phase 6 — Bench: a real caller, `BenchWebhookSecret`

Every other piece of this plan is infrastructure with nothing permanent
using it. `services/tooling/bench` is the one real, if narrow, opening: its
webhook signature verification (`webhook.go`'s `blueprintSecretResolver.
Resolve`) already resolves a `secret_ref` (`blueprint://secrets/<key>`, eg.
`bench/course-creation-webhook`) by calling Blueprint's `KeyValueService.Get`
and reading `value.GetData()` off a generic `core.registry.key_value.v1.
Value` — the exact "everything looks like an opaque string" gap this whole
plan exists to move past. It's the only real (non-`Value`, non-framework)
KV consumer anywhere in `services/tooling/*` today.

`api/tooling/workflow/v1/service.proto` (already imports
`google/protobuf/timestamp.proto` for an unrelated field, so no new import to
add):

```proto
// BenchWebhookSecret is the KV-stored value a WebhookTrigger's secret_ref
// resolves to. Replaces the generic Value{data: "<secret>"} this used to be
// stored as -- registered with Blueprint's type registry so it renders
// properly in the Key/Value browser instead of as an opaque byte count.
message BenchWebhookSecret {
    string secret = 1;
    // When this secret was last rotated. Optional -- zero/unset for a
    // secret that predates rotation tracking. Also exercises the type
    // registry's transitive-dependency handling for real: this is the only
    // registered type in this plan that isn't self-contained in one file.
    google.protobuf.Timestamp rotated_at = 2;
}
```

- `webhook.go`: change the witness/response type from `&kvv1.Value{}` to
  `&workflowv1.BenchWebhookSecret{}`, and `value.GetData()` to
  `value.GetSecret()`. Everything else about `Resolve` (the key derivation,
  the empty-secret error) is unchanged.
- `services/tooling/bench/main.go`: add
  `.WithRegisteredType(&workflowv1.BenchWebhookSecret{})` to the builder
  chain — the one genuine, permanent caller this plan ships with.
- **Migration note, not a bug:** `Get`/`Set` key physical storage is
  `<type_url>-<key>` (`key_value/model.go`'s `makeKey`), so changing the
  requested type changes which physical key is read — any secret
  hand-seeded today under the old `Value` type_url becomes unreachable
  through this new code path, not silently misread. This is a local-dev
  framework with no writer for this value anywhere in code today (per
  investigation, it's seeded by hand for testing); re-add it after this
  change via a direct `Set` RPC call under the new type. The Key/Value
  page's own "Add Entry" form only ever creates `Value`-typed entries
  (unchanged by this plan — building a generic typed-entry editor is real,
  separate work, not something this pass does), so that's the only way to
  create one until such an editor exists.

**Artifact:** a `BenchWebhookSecret` value written via direct `Set` RPC
(`{secret: "...", rotated_at: <now>}`) is resolved correctly by
`blueprintSecretResolver.Resolve` (an actual webhook call with a valid HMAC
signature succeeds end-to-end); the same entry renders decoded — including
`rotated_at` as a real timestamp, not an opaque nested blob — in the
Key/Value browser.

## Phase 7 — Verification

Two real registered types, deliberately different shapes, carry
verification:

- **`NavigationConfig`** (`api/core/registry/key_value/v1/nav_config.proto`):
  nested repeated sub-messages (`sections[].items[]`), no imports of its
  own — exercises the single-file case. Currently invisible in the Key/Value
  browser (per the previous plan's own findings — nothing has ever written
  to `ui/navigation`). Register its descriptor (a temporary
  `WithRegisteredType` call from a throwaway/test binary, or a direct
  `RegisterType` RPC — whichever is faster against the live local stack) and
  write a real value to `ui/navigation` (a couple of sections, a couple of
  items each) via a direct `Set` call. It remains a stand-in for "some
  third-party service's custom type," not a first-party kind this plan is
  adopting for real — per the scope decision, `Route`/`Process`/`Value`
  remain untouched regardless of what this phase demonstrates.
- **`BenchWebhookSecret`** (Phase 6): flat structure, but depends on
  `google.protobuf.Timestamp` from a different file — exercises the
  transitive-dependency-collection path in `WithRegisteredType` for real,
  not just in a synthetic fixture, and is registered by Bench's actual
  startup path rather than a throwaway call.

Live checks:

- The Key/Value page's kind dropdown shows `NavigationConfig (1)` and
  `BenchWebhookSecret (1)` (once seeded); selecting either shows a truncated
  JSON preview instead of a byte count, and the detail page renders the real
  tree for each (including `rotated_at` decoding as an actual timestamp for
  `BenchWebhookSecret`, proving the imported-type path resolved correctly).
- The kind dropdown also shows `TypeDescriptor` itself like any other
  kind — per the scope decision, no special-casing to verify *didn't*
  accidentally get added.
- `DecodeValues` called against a `Route` or `Process` entry (neither ever
  registered) returns empty, and both continue to render exactly as they did
  before this plan — this pass must not change anything about kinds that
  were already working.
- A corrupted/truncated value under a registered type_url falls back to the
  byte-count display rather than erroring the page or the whole list.
- A real webhook POST against Bench, signed with the `BenchWebhookSecret`
  written above, still verifies and triggers its workflow — confirming the
  migration in Phase 6 didn't just render nicely, it still actually works.

## Resolved during implementation

- **The plan's own Phase 2 text was wrong about how to persist a `TypeDescriptor`.**
  It said to call `model.Set` directly. That bypasses raft entirely — this store's
  `KeyValue.Set` (the controller-level method, not the model) is what checks
  `c.raft.State() != raft.Leader` and forwards to the actual leader when this node
  isn't it; `model.Set` is a raw local Badger write with no replication awareness at
  all. `RegisterType` calls the controller's own `Set` instead, exactly like every
  other write in this store, so it forwards correctly when a `RegisterType` RPC
  lands on a follower — which, now that Fuse load-balances traffic across every
  Blueprint node, it routinely will.
- **A related gap the plan didn't call out at all: the in-memory descriptor cache
  needs to stay current on every node, not just whichever one handled the RPC.**
  Solved by hooking `Apply` — the one FSM method raft guarantees runs on every
  node for every committed write, leader and followers alike — to update the
  cache whenever the applied value's type_url is `TypeDescriptor`. `register` is
  idempotent, so this doubles harmlessly with the synchronous validate-and-cache
  `RegisterType` already does for fail-fast error reporting on the originating
  node.
- **No separate `JsonTree` component was built.** `key_value_detail.rs` already
  had `pretty_print` — parse as JSON, pretty-print with indentation if it is,
  fall back to raw text if not — built for the `Value` kind and rendered in a
  `<pre>` block with a JSON/TEXT badge. Decoded values are just plain JSON
  strings, so they go through that exact same function and render in that exact
  same block — nested structure (confirmed live against `NavigationConfig`'s
  `sections[].items[]`) already reads perfectly well as indented JSON, and reusing
  it means a newly-decodable kind's detail page looks identical to `Value`'s
  rather than introducing a second, different-looking widget. Simpler, and a
  better match for the plan's own "should look like it belongs" framing than
  what was originally spec'd.
- **`RegisterTypeRequest.descriptor`'s generated Go accessor is `GetDescriptor_()`**
  (trailing underscore) — protoc-gen-go renames it to avoid colliding with the
  standard `Descriptor()` method every generated message already has.
- **Read-after-write timing, not a bug:** immediately after a `Set`/`RegisterType`
  forwarded to a different node's leader, a `Get`/`DecodeValues` against the
  *originating* node can briefly miss before that write replicates back down to
  it locally — the same class of eventual-consistency window seen before with
  Fuse route registration. Verification scripts just needed a short retry, not a
  code change.
- **A temporary verification script placed inside `services/tooling/bench/`
  briefly broke Bench's real build** (`go build .` compiles every `.go` file in
  the directory, including a scratch one with its own `func main`) and took the
  live service down until it was deleted. Fixed by moving verification scripts
  outside any watched service directory entirely for the rest of this pass.

## Phase 7 findings

- Live-verified end-to-end against the real running stack: `BenchWebhookSecret`
  (real caller, `google.protobuf.Timestamp` dependency) and `NavigationConfig`
  (nested `sections[].items[]`, no dependencies) both registered, decoded, and
  rendered correctly — list-page truncated previews and full detail-page trees
  alike.
- Confirmed the two negative cases live, not just in isolation: `DecodeValues`
  against an unregistered type_url returns an empty map (no error); against a
  registered type_url with corrupted bytes, the corrupted key is simply absent
  from the result (no error, no partial-batch failure) while other keys in the
  same batch still decode correctly.
- Confirmed `Route`/`Process` — never registered — render exactly as they did
  before this plan, byte counts unchanged, and `TypeDescriptor` itself shows in
  the kind dropdown like any other kind, per the scope decision.
- Confirmed the real webhook flow, not just the KV round-trip: POST
  `/webhooks/crud-e2e` signed with the `BenchWebhookSecret`-stored secret
  returned `202` with a real `run_id`; the same request with a wrong signature
  returned `401`. The migration didn't just make the secret render nicely, it's
  still the exact same secret Bench's HMAC verification actually checks.

## Milestone Summary

| Phase | Scope | Status |
|---|---|---|
| 1 | Proto: `RegisterType`, `DecodeValues`, `TypeDescriptor` | Done |
| 2 | Server: descriptor registration + startup rehydration | Done |
| 3 | Server: `DecodeValues` (panic-safe, per-entry fallback) | Done |
| 4 | Chassis: `WithRegisteredType` builder helper | Done |
| 5 | Web client: reused `pretty_print` + wiring into list/detail pages | Done |
| 6 | Bench: real caller (`BenchWebhookSecret` replaces generic `Value`) | Done |
| 7 | Live verification (`NavigationConfig` + `BenchWebhookSecret`) | Done |
