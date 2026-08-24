---
weight: 30
title: 'End-to-End Testing — Overview'
description: 'Architecture for Bench (a declarative workflow engine) and Garage (its plugin repository), Draft''s answer to end-to-end testing.'
icon: 'fact_check'
draft: false
toc: true
---

This document proposes two new services for testing Draft-based systems end to end: **Bench**, a declarative workflow engine that runs YAML-defined test workflows and reports their results, and **Garage**, the plugin repository that supplies the reusable, versioned step executors Bench's workflows call into. Both are ordinary Draft applications — built on [Chassis](/docs/architecture/chassis-composability), registered with [Blueprint](/docs/architecture/core-services#blueprint), routed through [Fuse](/docs/architecture/core-services#fuse), eventing over [Catalyst](/docs/architecture/core-services#catalyst) — not a fifth core pillar. This document covers the prior art that shaped the design and the resulting architecture; [Bench — Workflow Engine](/docs/architecture/bench-workflow-engine) and [Garage — Plugin Repository](/docs/architecture/garage-plugin-repository) cover each service in depth, including its UI.

{{< alert context="warning" text="This is a design proposal, not a committed roadmap item or an implementation. Nothing described here exists yet." />}}

## The requirement

Testing a Draft-based system end to end usually means driving real gRPC calls against real services and asserting on the responses — sometimes against one service, sometimes across several in sequence (create a resource via one RPC, confirm a side effect via another). That's a workflow, not a unit test: an ordered sequence of steps, some of which need to know how to do something specific (make a gRPC call, poll a queue, check a database row) that the workflow engine itself shouldn't need to know how to do natively. The two primitives asked for — **workflow** and **step** — plus the requirement that a step can delegate to "some other service, or plugin," describe exactly this shape: an orchestrator that stays generic, and an open set of pluggable step executors that don't.

## Prior art

Five systems converge on variations of the same shape, plus one (Temporal) that deliberately doesn't. The comparison below is what shaped the decisions in this document.

| System | Primitives | Pluggable step unit | Trigger → status pattern |
|---|---|---|---|
| [Argo Workflows](https://argo-workflows.readthedocs.io/) | `Workflow` → `Template`s as `Steps` or `DAG` | `Template` = a container invocation; `WorkflowTemplate` for reuse | Argo Events: separate `EventSource` + `Sensor`, decoupling the webhook receiver from trigger logic |
| [Tekton Pipelines](https://tekton.dev/docs/pipelines/) | `Pipeline` (DAG) of `Task`s → `PipelineRun`/`TaskRun` | `Task` = named, versioned, parameterized unit with typed results; [Tekton Hub](https://hub.tekton.dev/) is a public Task registry | `EventListener` + `TriggerBinding` + `TriggerTemplate` create a real `PipelineRun` object synchronously — **the object's name is the run ID**, no separate ID-generation step |
| [GitHub Actions](https://docs.github.com/actions) | `workflow.yml`: `on:` → `jobs:` → `steps:` | `uses: owner/action@version`; the Actions Marketplace is the registry | `repository_dispatch` returns no run ID — callers must poll the list-runs API and correlate by branch/time. A known, frequently-worked-around pain point. |
| [Drone CI](https://docs.drone.io/) | `pipeline` → `steps`, each a shell command or a **plugin** | A "plugin" is a container image (`image: plugins/download`) taking settings as config; the drone-plugin-index is the community registry | Webhook-driven from the git provider; build object created synchronously with an ID |
| [Testkube](https://docs.testkube.io/) | `TestWorkflow`: `setup`/`steps`/`after`, each step `run`s inline or `execute`s a named, reusable **template** | Closest direct analog to this proposal, since it's specifically a *testing* system, not CI/CD | `events:` supports webhook and cron; execution gets an `id` synchronously, polled by that id — no correlation problem |
| [Concourse CI](https://concourse-ci.org/) | `pipeline` → `jobs` → `plan`; `resources` are typed, versioned external state | `resource_types`: a formal three-verb (`check`/`in`/`out`) container contract, not just "run this image" | Webhook resource checks trigger builds with an immediate ID |
| [Temporal](https://temporal.io/) (contrast) | Workflows/Activities are **code**, not YAML — durable execution via event-sourced replay | N/A — extensibility is "write a function," not "reference a plugin" | `WorkflowID`/`RunID` returned synchronously from the start call |

Two conclusions follow directly:

1. **Declarative YAML, not workflow-as-code.** Temporal's model earns its complexity when a workflow needs real branching, long human-in-the-loop waits, or activity selection decided at runtime. "Call this RPC, assert the response, maybe do it again with different input" — the actual shape of an E2E test — doesn't need that. Every purpose-built testing/CI system in the table is declarative for exactly this reason, and Testkube (the one of these actually built for testing rather than build/deploy) validates that this shape holds up for testing specifically, not just CI.
2. **The webhook must return a run ID synchronously.** Tekton, Testkube, Drone, and Concourse all do this; GitHub Actions' `repository_dispatch` doesn't, and it's a well-documented pain point precisely because of that gap. Bench's webhook contract (below, and detailed in the Bench doc) is deliberately the Tekton/Testkube shape, not the GitHub Actions one.

## Why plugins are registered services, not container images

Argo, Tekton, Drone, and Concourse all model a plugin as a container image invoked by a scheduler they own (Kubernetes, in four of five cases). Draft has no equivalent — it's a service-registration and RPC framework, not a container orchestrator; the closest thing it has to "run this pluggable unit somewhere" is a process registering itself with Blueprint and being reachable by RPC. Bolting a container scheduler onto Draft just to give Bench a plugin execution model would mean adopting an entire orchestration layer Draft doesn't have anywhere else, for the benefit of one service.

The design that actually fits Draft's existing primitives: **a plugin is an ordinary Draft service that implements a small, fixed gRPC contract (`StepExecutor`) and registers itself — with Blueprint for live discovery, and with Garage for versioned catalog metadata.** Bench resolves a step's `uses: name@version` reference by asking Garage which version satisfies it and Blueprint where a live instance of it currently is, then makes an ordinary RPC. Draft already composes at process/service granularity everywhere — that's what [service registration](/docs/architecture/core-services#process-registration) *is* — so Bench doesn't need a new composition model, it needs to use the one Draft already has.

One consequence worth stating plainly: a plugin process has to actually be running and registered for a step to execute, the same as any other Draft service dependency. There's no "spin up a container on demand" story here (that's what Argo/Tekton get from Kubernetes) — a plugin that needs elastic scaling would run behind a load-balanced pool of registered instances, same as any other Draft service under load, not via new machinery specific to Bench.

## Architecture at a glance

```
                        ┌─────────────┐
   external event ────▶ │    Fuse     │──ext_authz/route──▶ webhook received
   (CI, git provider)   └─────────────┘                          │
                                                                  ▼
                                                          ┌───────────────┐
                                                          │     Bench     │
                                                          │  (workflow    │
                                                          │   engine)     │
                                                          └───────────────┘
                                                          │       │      │
                                          register/query  │       │      │ RPC: Execute(step)
                                          ┌────────────────       │      │
                                          ▼                       ▼      ▼
                                    ┌───────────┐          ┌───────────┐  ┌──────────────┐
                                    │ Blueprint │          │ Catalyst  │  │ plugin        │
                                    │ (discover │          │ (run/step │  │ (registered   │
                                    │  plugins) │          │  status   │  │  service,     │
                                    └───────────┘          │  events)  │  │  implements   │
                                                            └───────────┘  │  StepExecutor)│
                                                                           └──────────────┘
                                                                                  ▲
                                                                    catalog       │
                                                                    metadata      │
                                                                           ┌───────────┐
                                                                           │  Garage   │
                                                                           │ (plugin   │
                                                                           │  catalog) │
                                                                           └───────────┘
```

- **Bench** owns workflow definitions, receives webhook triggers, executes steps by calling plugins over RPC, tracks run/step status, and publishes status-change events to Catalyst.
- **Garage** owns the plugin catalog: names, versions, config schemas, descriptions. It does not execute anything itself.
- **Plugins** are separate registered Draft services (or, for the common built-in cases, code shipped inside Bench itself — see the Bench doc) that implement `StepExecutor` and are discovered through Blueprint the same way any RPC dependency is.
- Both Bench's and Garage's UIs are server-side rendered, sharing one DaisyUI theme (detailed in each service's doc) — deliberately not the Dioxus/WASM approach used by [Blueprint's web client](/docs/architecture/blueprint-web-client-components), since the requirement here is explicitly SSR. The rationale for that split is covered in the Bench UI section.

## Decided

- **Naming**: **Bench** (as in a test bench) and **Garage** (where a chassis's parts are kept and serviced) — continuing the naming pattern of Blueprint/Catalyst/Fuse/Chassis without colliding with existing proto package names (a plugin *registry* would otherwise be easy to confuse with `api/core/registry/*`, which is Blueprint's KV/service-discovery domain).
- **Storage**: Postgres (via `pkg/repositories/postgres` + `bun`, the same pattern `golf-tracker`'s `course_creator` already uses) for both Bench's run history and Garage's plugin catalog — see each service's doc for the full reasoning.
- **Execution model**: Bench supports full DAG execution (`depends_on`, concurrent independent steps) from v1, not a sequential-only first cut.
- **Location**: `services/tooling/bench` and `services/tooling/garage` — a new top-level grouping, not `services/core/`, since (as stated above) neither is required cluster infrastructure the way Blueprint/Fuse/Catalyst are. This is the first thing to land under `services/tooling/`; nothing else lives there yet.

## Implementation plan

This is the cross-cutting plan: the proto work both services share, the order the two services get built in, and the one codegen change this whole effort actually requires. Each service's own doc — [Bench](/docs/architecture/bench-workflow-engine#implementation-plan), [Garage](/docs/architecture/garage-plugin-repository#implementation-plan) — has the detailed phase breakdown for that service specifically.

{{< alert context="info" text="**Tracking convention**, used on every checklist in this document and the two service docs: an unchecked box is not started; a checked box gets a completion timestamp appended in place, `- [x] ... _(completed 2026-08-23 14:00 UTC)_`. Timestamps are UTC so they're comparable across contributors regardless of local timezone. Don't remove or reword a completed item's original text — append the timestamp, so the history of what was planned stays intact." />}}

### Phase 0 — Proto definitions

Three new packages, following the `services/tooling/` split decided above rather than living under `api/core/`:

- [x] **`api/tooling/workflow/v1`** — `Workflow`/`Step`/`Run`/`StepResult` messages and `WorkflowService` (Bench's own API — `TriggerRun`, `GetRun`, `ListRuns`, `ListWorkflows`). Sketch in the [Bench doc](/docs/architecture/bench-workflow-engine#the-webhook-trigger-and-status-api). _(completed 2026-08-22 22:15 UTC — `api/tooling/workflow/v1/service.proto`, includes `Trigger`/`WebhookTrigger`/`RetryPolicy` and the `RunStatus`/`StepStatus`/`FailurePolicy` enums beyond the doc sketch, needed to make the messages concrete.)_
- [x] **`api/tooling/step_executor/v1`** — `StepRequest`/`StepResponse` and the `StepExecutor` service every plugin implements, called by Bench. Sketch in the [Garage doc](/docs/architecture/garage-plugin-repository#what-a-plugin-is). _(completed 2026-08-22 22:15 UTC — `api/tooling/step_executor/v1/service.proto`, matches the doc sketch exactly.)_
- [x] **`api/tooling/plugin_catalog/v1`** — `Plugin` manifest messages and `PluginCatalogService` (Garage's own API — `Publish`, `Get`, `List`, `Search`). Sketch in the [Garage doc](/docs/architecture/garage-plugin-repository#the-catalog). _(completed 2026-08-22 22:15 UTC — `api/tooling/plugin_catalog/v1/service.proto`; added a `Retract` RPC beyond the original sketch, since the doc's own "Publishing and discovery" section already described a retract-on-shutdown `chassis.Effect` that had no corresponding RPC defined until now.)_
- [x] Run `buf generate` against `buf.gen.go.yaml` and confirm the generated Go types compile. _(completed 2026-08-22 22:15 UTC — `buf lint` clean, `go build ./tooling/...` in `api/` passes, `go mod tidy` made no changes. TS/web codegen (`buf.gen.web.yaml`) needs plugins normally pulled by `dctl api build`'s Docker image — not runnable bare from this host, and not needed for Bench/Garage's own Go SSR UI, so left for whenever a TS consumer actually exists.)_

**The codegen change this requires** — the only one, and worth being precise about, because two of the three generation pipelines need nothing done to them:

- [x] **Go** (`buf.gen.go.yaml`, `buf.gen.gotag.yaml`) and **TypeScript/web** (`buf.gen.web.yaml`), both run via `dctl api build` → `buf generate`: `api/buf.yaml` has no hardcoded path list, only a `vendor/` exclude, so `buf` discovers `.proto` files anywhere under `api/` automatically. Dropping the three packages above under `api/tooling/` needs **no config change** for either pipeline. _(completed 2026-08-22 18:00 UTC — verified by reading `api/buf.yaml`; nothing to do here, confirmed as part of this doc's own research.)_
- [ ] **Rust** (`api/build.rs`, used by [Blueprint's Dioxus/WASM web client](/docs/architecture/blueprint-web-client-components)) is the one exception: it hardcodes an explicit `proto_dirs` array (`api/build.rs:8-13`) rather than discovering paths, and today lists only the four packages that client actually needs:
  ```rust
  let proto_dirs = [
      "./core/registry/key_value/v1/",
      "./core/registry/service_discovery/v1/",
      "./core/control_plane/networking/v1/",
      "./core/message_broker/actors/v1/",
  ];
  ```
  Since Bench's and Garage's own UIs are server-side rendered Go, not Rust/WASM, **none of the three new packages need an entry here for Bench or Garage themselves to work** — skip this in Phase 0. Add an entry only if/when a Rust consumer actually needs generated bindings for one of these packages (the most plausible future case: Blueprint's web client surfacing run status on its cluster view, echoing the `PluginInventoryGateway`-style read-only projection pattern from the DeepSeek harness research). Flagging this now so it isn't rediscovered as a mystery build failure later — a new proto package under a directory this array doesn't list will silently not get Rust bindings, not error. Left unchecked deliberately — there is no current Rust consumer, so there's nothing to do here yet; check it off only when one exists and the entry is actually added.

### Cross-service sequencing

| Phase | What | Depends on | Status |
|---|---|---|---|
| 0 | Proto definitions (above) | — | Done (2026-08-22) |
| 1 | Bench core loop: scaffolding → workflow loading → DAG scheduler against the built-in `grpc-call` executor only → webhook + status API | Phase 0 | Done (2026-08-22) |
| 2 | Garage: scaffolding → catalog RPCs → one real reference plugin (`slack-notify`) | Phase 0 (independent of Phase 1 — can build in parallel) | Done (2026-08-22) |
| 3 | Bench's `garage://` resolution (catalog lookup + Blueprint discovery + `Execute` call) | Phases 1 and 2 both complete | Not started |
| 4 | Both UIs | Phase 1 (Bench UI) / Phase 2 (Garage UI) — independent of Phase 3 and of each other | Garage done (2026-08-22); Bench not started |
| 5 (stretch) | Catalyst live-update wiring for the UI; `chassis.Effect`-based step teardown | Phase 4 / Phase 1 | Not started |

The one thing to protect in this ordering: **Bench's core loop (Phase 1) is fully provable without Garage existing at all**, since the built-in `grpc-call` executor covers the example workflow in the Bench doc end to end. Building Garage first, or blocking Bench on it, would be backwards — Garage only has to exist by the time Phase 3 needs a real plugin to resolve against.
