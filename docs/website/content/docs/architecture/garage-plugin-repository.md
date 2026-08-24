---
weight: 32
title: 'Garage — Plugin Repository'
description: 'The catalog of versioned, pluggable step executors Bench workflows call into, with a server-side rendered browsing UI.'
icon: 'inventory_2'
draft: false
toc: true
---

{{< alert context="warning" text="This is a design proposal, not a committed roadmap item or an implementation. Nothing described here exists yet. See the Overview doc for the prior-art research and architecture context this design draws from." />}}

Garage is the plugin repository described in [End-to-End Testing — Overview](/docs/architecture/e2e-testing-overview) — the catalog [Bench](/docs/architecture/bench-workflow-engine) resolves a step's `uses: garage://name@version` reference against. This document covers what a plugin actually is, the catalog it lives in, how a plugin publishes itself, how Bench resolves and calls one, and Garage's own UI.

## What a plugin is

A plugin is an ordinary Draft service — built on Chassis, like any other — that implements one small, fixed gRPC contract:

```protobuf
// api/tooling/step_executor/v1/service.proto (sketch)
service StepExecutor {
  rpc Execute(StepRequest) returns (StepResponse);
}

message StepRequest {
  string step_name = 1;
  google.protobuf.Struct config = 2;     // the step's `with:` block, passed through opaquely
  google.protobuf.Struct context = 3;    // resolved {{ steps.X.result }} references from earlier steps
}

message StepResponse {
  bool success = 1;
  google.protobuf.Struct result = 2;     // available to later steps via {{ steps.<name>.result }}
  string error = 3;
}
```

That's the entire contract a plugin author has to implement. Everything else — what `config` actually means, what it does with it, what it puts in `result` — is up to the plugin. This mirrors Concourse's `resource_types` idea (a plugin is a formal, minimal protocol, not an arbitrary integration point) while staying at the granularity Draft already operates at: a registered RPC service, not a container image implementing a shell-script-shaped contract.

## The catalog

Garage doesn't run plugins — it's metadata about them. A plugin publishes a manifest describing itself; Garage stores and serves that manifest; Bench queries it to resolve a `uses:` reference and to validate a step's `with:` block before ever calling the plugin.

```yaml
# published by the plugin at startup, or via garage-cli publish
apiVersion: garage/v1
kind: Plugin
metadata:
  name: slack-notify
  version: v2
  description: Posts a message to a Slack channel via an incoming webhook.
  maintainer: platform-team
  source: https://github.com/steady-bytes/draft-plugins/tree/main/slack-notify
config_schema:            # JSON Schema, used to validate a step's `with:` block at
                           # workflow-authoring time, before any run ever calls this plugin
  type: object
  required: [channel, message]
  properties:
    channel: { type: string, pattern: "^#" }
    message: { type: string }
result_schema:
  type: object
  properties:
    message_ts: { type: string }
```

```protobuf
// api/tooling/plugin_catalog/v1/service.proto (sketch)
service PluginCatalogService {
  rpc Publish(PublishRequest) returns (PublishResponse);
  rpc Get(GetPluginRequest) returns (Plugin);
  rpc List(ListPluginsRequest) returns (ListPluginsResponse);
  rpc Search(SearchPluginsRequest) returns (SearchPluginsResponse);
}
```

Validating a step's `with:` block against `config_schema` at the point a workflow is authored or edited — not the first time it runs — is worth calling out on its own: it catches a mismatch (a workflow author fat-fingering a field name in a plugin's config) before it costs a failed run instead of after.

## Publishing and discovery

A plugin's own `main.go` looks like any other Draft service, plus two registrations at startup instead of the usual one:

```go
c := chassis.New(logger).
    WithRPCHandler(stepExecutorRPC).      // implements StepExecutor — Bench calls this directly
    Register(chassis.RegistrationOptions{Namespace: "plugins"})  // Blueprint: live discovery

// publishing to Garage's catalog is exactly the "ad hoc effect with an inverse" shape
// from chassis.Effect — publish on startup, retract on graceful shutdown, so a plugin
// that's no longer running doesn't linger in the catalog as if it were.
c.Effect("garage-catalog-entry", func() (func(context.Context) error, error) {
    if err := publishToGarage(manifest); err != nil {
        return nil, err
    }
    return retractFromGarage, nil
})

defer c.Start()
```

This is the same pattern this session already landed in Chassis for [`auth`'s service-address registration](/docs/architecture/chassis-composability#worked-example-giving-auth-a-real-inverse) — a plugin's presence in Garage's catalog is exactly the kind of ad hoc effect `chassis.Effect` exists for, tracked so it's automatically retracted on graceful shutdown instead of left stale.

Two separate registrations, two separate purposes: **Blueprint** answers "where is a live instance of `slack-notify` right now" (ordinary service discovery, the same as any RPC dependency); **Garage** answers "what versions of `slack-notify` exist, and what does `v2`'s config look like" (catalog metadata, versioned independently of any particular running instance). Bench needs both to resolve a step: Garage to validate the reference and its config, Blueprint to find somewhere to actually send the `Execute` call.

## The UI

Garage's UI shares Bench's stack and DaisyUI theme (`templ` + htmx + DaisyUI, or `html/template` as the zero-dependency fallback — see [Bench's UI section](/docs/architecture/bench-workflow-engine#the-ui) for the rationale), since a developer moving between "which plugins are available" and "what did my last test run do" should not feel like they've left one product for another.

| Page | Purpose | Key DaisyUI components |
|---|---|---|
| Catalog | Browse/search all published plugins | `card` grid (one card per plugin: name, description, latest version, maintainer), search `input` |
| Plugin detail | All published versions, config schema, example usage | `tabs` (one per version), rendered `config_schema` as a `table`, a copy-pasteable YAML `uses:` snippet in `mockup-code` |
| Version diff *(stretch)* | What changed in `config_schema`/`result_schema` between two versions | side-by-side `table` |

The plugin detail page's rendered config-schema table and copy-pasteable snippet exist to answer the question a workflow author actually has — "what do I put in `with:` for this plugin, concretely" — directly from the schema Garage already validates against, rather than requiring a plugin author to separately maintain human-readable docs that can drift from the schema itself.

## Decided

- **Storage**: Postgres, the same choice and reasoning as [Bench's run history](/docs/architecture/bench-workflow-engine#decided) — a plugin catalog is exactly the kind of structured, queryable data a KV store handles poorly at any real scale, and reusing the in-progress sqlite work here would make Garage's first real usage depend on infra that isn't finished yet.
- **Location**: `services/tooling/garage`, not `services/core/garage` — see [End-to-End Testing — Overview](/docs/architecture/e2e-testing-overview) for why Garage isn't core cluster infrastructure the way Blueprint/Fuse/Catalyst are.

## Implementation plan

Proto definitions (`api/tooling/plugin_catalog/v1`, and `api/tooling/step_executor/v1` shared with Bench) are Phase 0 of the [cross-cutting plan](/docs/architecture/e2e-testing-overview#implementation-plan). Garage's own phases are independent of Bench's — see the [cross-service sequencing table](/docs/architecture/e2e-testing-overview#cross-service-sequencing) — and can be built in parallel with [Bench's Phases 1–5](/docs/architecture/bench-workflow-engine#implementation-plan); the two only have to meet at Bench's Phase 6. See the [tracking convention](/docs/architecture/e2e-testing-overview#implementation-plan) for how checked items are annotated.

- [x] **1. Scaffolding.** `services/tooling/garage`: `go.mod`, `main.go`, `config.yaml`, following the same `services/core/auth`-derived shape as Bench's Phase 1. Wire the Postgres repository via `chassis.WithRepository`, with a schema for `plugins` (name, version, description, maintainer, source, `config_schema` and `result_schema` as `jsonb` columns — one row per published version, not per plugin name). Deliverable: a service that starts, registers with Blueprint, opens its database connection, and does nothing else yet. _(completed 2026-08-22 22:24 UTC — `go build`/`go vet`/`gofmt` all clean; `pluginRow` bun model with a composite `unique:plugins_name_version` constraint on `(name, version)`, `jsonb` columns for `config_schema`/`result_schema`. Same local `replace` requirement for `pkg/chassis`/`pkg/loggers` as Bench, for the same reason — see Bench's Phase 1 note.)_
- [x] **2. `PluginCatalogService` RPCs.** `Publish` (validates the manifest shape itself, and that `config_schema`/`result_schema` are well-formed JSON Schema — not that any particular step config satisfies them, that check happens on the Bench side against a specific `with:` block), `Get`, `List`, `Search`. Deliverable: a manifest like the `slack-notify` example in [The catalog](#the-catalog) can be published and retrieved by name+version through the real RPCs, backed by Postgres. _(completed 2026-08-22 22:34 UTC — `store.go` (persistence: keyset pagination on `(published_at, id)`, SQLSTATE-23505-based duplicate detection) and `rpc.go` (all five RPCs incl. `Retract`, Connect typed error codes, a deliberately minimal JSON-Schema-shape check on publish). `Retract` on an already-gone `(name, version)` is a documented no-op, not an error — a shutdown-path `chassis.Effect` inverse shouldn't fail because the entry is already gone. 9 tests pass against a real Postgres container the suite spins up and tears down itself. `go build`/`go vet`/`gofmt`/`go test` all clean.)_
- [x] **3. The reference plugin.** Build `slack-notify` for real: a small separate service implementing `StepExecutor.Execute`, registering with Blueprint the ordinary way, and publishing itself to Garage on startup via the `chassis.Effect` pattern shown in [Publishing and discovery](#publishing-and-discovery) — retracting the catalog entry on graceful shutdown. This is deliberately not a throwaway example: it's what proves Phase 2's `Publish` flow against a real caller, and it's the plugin [Bench's Phase 6](/docs/architecture/bench-workflow-engine#implementation-plan) resolves and calls. Deliverable: `slack-notify` is discoverable via Blueprint and listed in Garage's catalog while running; both disappear cleanly on shutdown. _(completed 2026-08-22 22:46 UTC — `services/tooling/slack-notify`, a genuinely separate service, port 9302, `garage.address` as static config (not Blueprint-resolved, avoiding a bootstrapping chicken-and-egg problem). Proved end to end against real running Blueprint/Fuse/Garage/Postgres, not just tests: startup published the manifest (confirmed via a live `Get` RPC matching the doc's example exactly), `SIGTERM` retracted it (confirmed `Get` afterward returned not-found). `Execute` correctly handles Slack's actual webhook behavior — a plain-text `"ok"` response, not JSON, so `message_ts` is typically absent outside of test doubles that return JSON. 9 unit tests + 1 real-Garage integration test (gated behind an env var, skips by default) all pass; `go build`/`go vet`/`gofmt` clean; all test infra torn down and verified gone.)_

  **Second reference plugin, 2026-08-23 03:53 UTC — `catalyst-consume`** (user: "I want to create a new plugin that will consume events from catalyst, grab the fields from the event payload and pass them along to the next step"), built alongside [Bench's Phase 6](/docs/architecture/bench-workflow-engine#implementation-plan) to prove real `garage://` resolution end to end, not just a second isolated plugin. `services/tooling/catalyst-consume`, port 9303, mirrors `slack-notify`'s shape exactly (same `main.go`/`catalog.go` structure, same static `garage.address` config). What it does: `Execute` opens a Consume stream against Catalyst (the same raw Connect client `services/examples/consumer` and `services/tooling/bench/catalyst.go` both use — Catalyst's `chassis.Broker` interface is for broker *implementations*, not publishing/consuming clients), waits for the next CloudEvent matching a configured `event_type` (client-side filtering — Catalyst's `Consume` doesn't filter server-side, a fact this doc's Catalyst section documents), decodes its JSON payload, and copies caller-specified dot-paths (`config.fields: {output_key: "json.path"}`) into the step's result alongside `_event` metadata and the full raw payload. Config/result schemas published to the catalog accordingly. 9 tests, including 5 that exercise `Execute` against a real fake-Catalyst `httptest`/h2c server (a genuine success case, a case proving non-matching event types before the match are correctly skipped, a real-clock timeout case, and both StepResponse-level and RPC-level error cases) — not just the pure `parseConfig`/`extractPath` helpers in isolation. `go build`/`go vet`/`go test -race`/`gofmt` all clean.

  **Live-verified together with Bench's new `garage://` resolution**, not separately: a `catalyst-consume-e2e.yaml` workflow (`services/tooling/bench/workflows/`) was triggered, then — while its single step was actively waiting — a real `crud-e2e` run was triggered a second later; the first workflow's step correctly picked up `crud-e2e`'s real `RunFinished` CloudEvent and extracted `run_id`/`workflow`/`status` from it, with `Detail` confirming Bench's resolver had found `catalyst-consume`'s own address (`localhost:9303`) specifically, not some other plugin's — the exact ambiguity [Bench's Phase 6 note](/docs/architecture/bench-workflow-engine#implementation-plan) explains was the real design problem this phase had to solve. Both `scripts/run-local.sh` and `scripts/run-tooling-stack.sh` updated to build and start `catalyst-consume` alongside `slack-notify`.
  **Third, fourth, and fifth reference plugins, 2026-08-23** (user: "I'd like to add three new plugins to garage. One to make http calls like curl, and one to configure and make gRPC calls, and finally one to produce events in catalyst"). All three mirror `slack-notify`/`catalyst-consume`'s exact shape (same `main.go`/`catalog.go`/`manifest.go` structure, static `garage.address` config, no `WithRoute` — Bench finds them via Blueprint, not Fuse) and each ships with real, `-race`-clean unit tests, `go vet`/`gofmt` clean.

  - **`http-call`** (`services/tooling/http-call`, port 9304): makes a plain HTTP request (`with.method`/`url`/`headers`/`body`/`timeout`) and returns `status`/`headers`/`body` as the step's result. A generic HTTP-call plugin is only as useful as its ability to make a test actually fail — so it also evaluates `with.expect` (`status`, `body`) using the same `exists`/`equals`/`matches` grammar `bench://grpc-call@v1` already established, generalized to accept either `"OK"` (any 2xx, matching `grpc-call`'s convention) or an exact status code (e.g. `404`, for testing error paths). **Deliberate placement decision, asked and confirmed with the user before writing any code**: this lives under `with.expect`, not the workflow's top-level `expect:` keyword `bench://grpc-call@v1` uses — `StepRequest` (the fixed, minimal contract every plugin implements, see [What a plugin is](#what-a-plugin-is)) carries only `config`/`context`, no `expect` field, and extending that shared contract for one plugin's benefit would compromise its documented "fixed and minimal" framing. The assertion logic itself is a deliberate duplicate of `grpc_call.go`'s (confirmed with the user as the preferred trade-off over extending the contract) — this repo's established precedent for small cross-service logic (see `pluginUsesSnippet` in [Bench's Connecting a plugin registry](/docs/architecture/bench-workflow-engine#connecting-a-plugin-registry)) is to duplicate rather than invent a shared package two separate Go modules can't import from each other anyway.

  - **`grpc-call`** (`services/tooling/grpc-call`, port 9305): makes a unary call to *any* gRPC server that exposes standard server reflection — not limited to Draft/Connect services the way Bench's own built-in `bench://grpc-call@v1` is (that executor only speaks plain HTTP+JSON to services resolved by name through Blueprint; it has no reflection or generated stubs because every Draft service already speaks that exact shape). This plugin dials `with.address` directly, resolves `with.service`/`with.method` at runtime via the target's reflection service, and builds/decodes the request/response as dynamic protobuf messages from `with.request`'s JSON — no generated Go client for the target service required or assumed. Depends on `github.com/jhump/protoreflect` (`grpcreflect` + `dynamic` + `dynamic/grpcdynamic`, the same library `grpcurl` itself is built on) — hand-rolling correct `FileDescriptor` dependency resolution over the raw reflection RPC (transitive imports, well-known types, diamond deps) is a solved problem that library already gets right; this is the one plugin in this repo that carries that dependency, and the reason is documented in `rpc.go`'s own file comment. `with.expect` follows the same placement reasoning as `http-call`'s (`status`: a grpc status code name, e.g. `"NotFound"`; `response`: the same recursive assertion grammar against the decoded response message). **Verified against a real, unrelated chassis-based Draft service, not a purpose-built test double**: a live workflow step pointed `grpc-call` at Garage's own already-running `PluginCatalogService.List` (`localhost:9301`) with zero prior knowledge of its shape baked into the plugin, and it came back with the full, correctly-decoded catalog — genuine proof server reflection works against this repo's real chassis-based services (every `chassis.Rpcer` handler already exposes it via `connectrpc.com/grpcreflect`, confirmed by reading `pkg/chassis/builder.go` before relying on it), not just against a hand-built fixture server.

  - **`catalyst-produce`** (`services/tooling/catalyst-produce`, port 9306): the publish-side mirror of `catalyst-consume` — builds a CloudEvent from `with.event_type`/`source`/`subject`/`data` and sends it on a long-lived Produce stream opened once at startup (the same stream-reuse shape `services/tooling/bench/catalyst.go`'s `catalystPublisher` and `services/examples/producer` both use). **Deliberately not best-effort**, unlike Bench's own event publishing (which is a secondary observability channel — `GetRun` stays authoritative regardless): a `Send` failure here fails the step outright, since publishing the configured event *is* this plugin's entire deliverable, not a side effect of something else succeeding.

  All three published to Garage's catalog cleanly on startup against the real running stack (confirmed via `PluginCatalogService.List`, alongside the pre-existing `slack-notify`/`catalyst-consume`), and were exercised together in one real Bench workflow, not three separate throwaway checks: `http-call` against a genuine local HTTP server with a real `expect.body` assertion, `grpc-call` against Garage's live `PluginCatalogService.List` as described above, and `catalyst-produce` publishing a real event whose delivery was independently confirmed by a standalone Consume subscriber (not just that `Send` returned without error) — the whole run reported `PASSED` with all three steps' real `Detail`/`Result` data intact. Also confirmed discoverable through Bench's search sidebar (`/workflows/plugins/search`). `scripts/run-local.sh` and `scripts/run-tooling-stack.sh` both updated to build and start all three alongside the existing two reference plugins. Test workflow and its ad hoc HTTP server torn down afterward.

  **Addendum, 2026-08-23 — `catalyst-produce`'s `config.delay` and a checked-in produce/consume round-trip workflow** (user: "create a workflow that will emit an event using catalyst produce, and then the next step will consume that same event"). Real design problem surfaced before writing any YAML, not discovered by trial and error: Catalyst's `Consume` has no event replay (this doc's own note on `catalyst-consume`, above), so a naive two-step `depends_on`-sequenced workflow (produce, then consume) is not merely racy — it is **guaranteed to fail every time**, since Bench's DAG scheduler only starts a dependent step's `runStep` after its dependency's goroutine has already returned (`scheduler.go`'s `waitForDependencies` blocks on `<-done[dep]`), by which point the single-shot event has already been sent and lost. The two steps have to run as **concurrent siblings** instead — but `bench-workflow-engine.md`'s own "empty `depends_on` defaults to the previous step in declaration order" rule means any two-step workflow's second step always defaults to depending on the first; genuine independence needs a third, shared step both explicitly `depends_on: [ready]`, making them true siblings the scheduler starts together (`services/tooling/bench/workflows/catalyst-produce-consume-e2e.yaml`, the anchor step reusing `bench://grpc-call@v1` against Garage's own `PluginCatalogService.List` — cheap, real, and independent of any of these three new plugins being healthy).

  Even as concurrent siblings, the two steps aren't symmetric: `catalyst-produce`'s stream is already open (opened once at plugin startup), so its `Send` is a single fast write, while `catalyst-consume`'s `Execute` has to open a brand-new stream to Catalyst per call before it can even start waiting — a real, structural latency asymmetry favoring the producer. Added `config.delay` (a Go duration string, e.g. `"300ms"`) to `catalyst-produce` for exactly this: it sleeps (respecting context cancellation) before sending, giving the sibling consumer's stream time to actually open, turning a latency-biased race into a deterministic sequencing. **Verified the asymmetry was real, not a defensive fiction**: before committing to the fix, ran the identical workflow shape with `delay` removed — 4 of 5 runs failed (`consume-event` timed out, having opened its stream after the event was already gone); with `delay: 300ms` restored, 6 of 6 runs passed, `_event.id` on the consumed side matching `publish-event`'s own `event_id` exactly each time, confirming it's genuinely the same event, not some other stray one. `catalyst-produce` gained 3 new tests (`TestExecute_DelayWaitsBeforePublishing`, `TestExecute_DelayRespectsContextCancellation`, `TestExecute_InvalidDelay` — 12 total, `-race` clean). The workflow itself is a real, checked-in file (`services/tooling/bench/workflows/`), not a throwaway fixture, seeded once via Bench's existing seed-once `loadWorkflowsDir` — matching `catalyst-consume-e2e.yaml`'s own precedent exactly, including its "no `expect:` block" note (`garage_plugin.go` never reads `step.GetExpect()`).
- [x] **4. The UI.** The Catalog and Plugin detail pages from [The UI](#the-ui) section, sharing Bench's `templ` + htmx + DaisyUI stack, rendering real data from Phases 2–3. _(completed 2026-08-22 23:15 UTC — built with `html/template`, not `templ`: the `templ` CLI isn't installed in this environment, and introducing a new codegen toolchain wasn't a call to make unilaterally mid-implementation; `html/template` was the doc's own documented fallback. htmx vendored locally (48KB, reviewable); DaisyUI linked from a CDN rather than vendoring its ~2.9MB standalone build. RPC and UI share one mux/port (`chassis.Rpcer.AddHandler`), not a second listener. Catalog page groups rows to one card per plugin *name* (latest version); detail page shows a version-tabs strip and a config-schema-derived, copy-pasteable `uses:` snippet with placeholder values per required field — directly answering "what do I put in `with:` for this plugin" from the schema itself. Proved against two real, distinct published plugins (`slack-notify`, published for real by the actual Phase 3 service, plus a second fixture), not one. 18 tests pass (9 from Phase 2 plus 9 new template/schema tests); `go build`/`go vet`/`gofmt` clean.)_ **Design pass, 2026-08-23 00:09 UTC**: the shipped page loaded DaisyUI's CSS but never loaded Tailwind itself, so every layout/spacing/typography utility class in the templates (`flex`, `grid`, `gap-6`, `container`, `text-3xl`, ...) resolved to nothing — DaisyUI is a Tailwind plugin, not a replacement for it. Confirmed via the network tab (DaisyUI's CSS *did* load, 200) and by reading the raw HTML's class list against what `full.min.css` actually contains. Fixed by adding Tailwind's Play CDN script (DaisyUI's own documented no-build setup) and replaced the stock `dracula` theme with a custom `workshop` theme — OKLCH tokens (verified against the actual loaded stylesheet's format, not assumed), drawing on Draft's own naming (a "blueprint" cyanotype blue as the primary accent) rather than a generic dashboard palette. Verified visually via browser screenshots at each step, not just curl'd HTML.
- [ ] **5. (Stretch, deferred)** Version range constraints and plugin sandboxing — see [Open questions](#open-questions) below; neither is scheduled until there's a concrete need (a second, differently-trusted plugin author, or an actual version-compatibility complaint).

## Open questions

- **Version compatibility**: today a workflow pins an exact version (`slack-notify@v2`); whether Garage should support range constraints (`^v2`) the way a package manager would is deliberately left open — it's a nominal-vs-structural-linking tension, the same shape as resolving a dependency by name alone versus by verified interface compatibility, and probably deserves a "start with exact pins, revisit if it's actually painful" answer rather than solving it upfront.
- **Plugin sandboxing / trust**: a plugin is an ordinary registered service today, with no isolation beyond whatever the deploying team gives it — appropriate for internally-authored plugins, less obviously appropriate for anything resembling a community contribution. Not designed here.
