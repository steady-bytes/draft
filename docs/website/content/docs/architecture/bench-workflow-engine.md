---
weight: 31
title: 'Bench — Workflow Engine'
description: 'A declarative, YAML-defined workflow engine for end-to-end testing Draft services, with a server-side rendered UI.'
icon: 'timeline'
draft: false
toc: true
---

{{< alert context="warning" text="This is a design proposal, not a committed roadmap item or an implementation. Nothing described here exists yet. See the Overview doc for the prior-art research and architecture context this design draws from." />}}

Bench is the workflow engine described in [End-to-End Testing — Overview](/docs/architecture/e2e-testing-overview). This document covers its two primitives (`Workflow` and `Step`), the YAML schema, how a run executes, the webhook trigger and status-polling contract, and its server-side rendered UI.

## Primitives

A **Workflow** is a named, versioned definition: metadata, a trigger configuration, and an ordered list of steps. A **Step** is one unit of work within a workflow: a reference to a plugin (or a built-in executor), the configuration to pass it, and the assertions to check against its result.

```yaml
# workflows/course-creation.yaml
apiVersion: bench/v1
kind: Workflow
metadata:
  name: course-creation-e2e
  description: Create a course via CourseCreator, then confirm it's queryable.
trigger:
  webhook:
    # Bench derives the actual endpoint from this slug: POST /webhooks/course-creation-e2e
    slug: course-creation-e2e
    # HMAC-SHA256 over the raw request body, checked against this header.
    signature:
      header: X-Bench-Signature
      secret_ref: blueprint://secrets/bench/course-creation-webhook
steps:
  - name: create-course
    uses: bench://grpc-call@v1          # built-in executor, ships with Bench — see below
    with:
      service: golf-app.app.v1.CourseCreator
      method: CreateCourse
      request:
        name: "Pebble Beach"
        holes: 18
    expect:
      status: OK
      body:
        id: { exists: true }

  - name: query-course
    uses: bench://grpc-call@v1
    depends_on: [create-course]         # optional; defaults to the previous step
    with:
      service: golf-app.app.v1.CourseCreator
      method: GetCourse
      request:
        id: "{{ steps.create-course.result.id }}"   # data flow between steps
    expect:
      status: OK
      body:
        name: { equals: "Pebble Beach" }

  - name: notify-slack-on-seed-data
    uses: garage://slack-notify@v2      # a plugin resolved through Garage + Blueprint
    with:
      channel: "#e2e-results"
      message: "Seeded a test course: {{ steps.create-course.result.id }}"
    on_failure: continue                # a failing notification shouldn't fail the run
```

A few things worth calling out explicitly:

- **`depends_on` defaults to the previous step**, so the common case (a straight-line sequence, which is what most E2E tests actually are) reads top-to-bottom with no extra ceremony — the same convenience Argo's `Steps` template offers over its more general `DAG` template. A step can name multiple dependencies to fan-in, and independent steps with no shared dependency run concurrently, which is what makes this a DAG rather than strictly a list.
- **`uses:`** is a plugin reference, `scheme://name@version`. `bench://` addresses executors built into Bench itself (see below); `garage://` addresses a plugin resolved through the Garage catalog. This mirrors GitHub Actions' `uses: owner/action@version` and Drone's `image: plugins/name` — the same shape three independent systems converged on for "delegate this step to a named, versioned, external unit."
- **`expect:`** is assertion configuration passed to the executor, not interpreted by Bench itself — the built-in `grpc-call` executor understands `status`/`body` assertions with `equals`/`exists`/`matches` operators; a different executor could define entirely different assertion vocabulary appropriate to what it does. Bench's only job is: call the executor, and record whether it reported success or failure.
- **`{{ steps.<name>.result.<field> }}`** templating lets later steps reference earlier steps' outputs — necessary for anything beyond single-call smoke tests, and present in some form in every system surveyed (Tekton's `results`, Argo's parameter passing, GitHub Actions' `steps.<id>.outputs`).

## Built-in executors vs. Garage plugins

The single most common step — "make a gRPC call, check the response" — shouldn't require standing up a separate registered service just to run a test. Bench ships a small set of built-in executors compiled directly into it (`bench://grpc-call@v1` above; a `bench://http-call@v1` for testing anything fronted by Fuse over plain HTTP would be the other obvious one). Anything more specific — posting to Slack, seeding a database with fixture data, polling an external system — is a Garage plugin: a separately deployed, separately versioned, separately owned service. This mirrors Drone's split between its handful of built-in step kinds and its community plugin index, and keeps Bench itself from accumulating integrations for every system a test might ever need to touch.

## Execution model

A **Run** is one execution of a Workflow, created either by its webhook firing or by a manual trigger from the UI. A run's steps execute according to the DAG implied by `depends_on`: steps with no unmet dependency start immediately (concurrently, if there's more than one), and completion of a step unblocks its dependents. Full DAG execution — not just the sequential common case — is in scope from v1, so the scheduler has to validate the `depends_on` graph at workflow-authoring time (create/update, not first run) and reject cycles then: a cycle is knowable statically from the graph alone, and should never be discovered as a hang once a run is already in flight. This is exactly the ordering guarantee LIFO revertible effects [give a single component's teardown](/docs/architecture/chassis-composability) — later work assuming earlier work is done — applied here to a graph of steps instead of a stack of effects within one process.

That connection isn't just an analogy worth drawing — it's a real implementation option. A step that provisions something (the `grpc-call` step above creates a course; a fixture-seeding plugin might create rows in a database) is a natural fit for [`chassis.Effect`](/docs/architecture/chassis-composability): the step's `Execute` call is the setup, and if the executor also returns a teardown, Bench can track it the same way chassis tracks any other effect, and revert it — in reverse order — once the run finishes, regardless of whether later steps passed or failed. This gives every workflow author automatic, ordered cleanup of whatever their test created, without hand-writing an `after:` block for it (though an explicit `after:` remains available for cleanup that isn't naturally expressed as a step's own inverse). Whether this is worth building for v1 or is a later enhancement is an open question — see the end of this document.

**Retries and failure policy** are per-step: `retry: { attempts: 3, backoff: 2s }` and `on_failure: fail | continue | retry`, matching the pattern in the YAML example above. A step's failure fails the run by default; `on_failure: continue` (used above for the Slack notification) lets a non-critical step fail without failing the whole workflow — necessary, or every workflow author ends up wrapping optional steps in awkward try/catch-shaped workarounds.

## The webhook trigger and status API

The Overview doc's research is decisive here: **the trigger must return a run ID synchronously**, not force the caller to poll and correlate (the GitHub Actions problem). Concretely:

```
POST /webhooks/course-creation-e2e
X-Bench-Signature: <hmac-sha256 of body>
{ "inputs": { ... } }                       → 202 Accepted
                                               { "run_id": "run_01hz...", "status": "pending" }

GET /runs/run_01hz...                        → 200 OK
                                               {
                                                 "run_id": "run_01hz...",
                                                 "workflow": "course-creation-e2e",
                                                 "status": "running",   // pending|running|passed|failed
                                                 "started_at": "...",
                                                 "steps": [
                                                   { "name": "create-course", "status": "passed", "duration_ms": 42 },
                                                   { "name": "query-course",  "status": "running" },
                                                   { "name": "notify-slack-on-seed-data", "status": "pending" }
                                                 ]
                                               }
```

This is exposed two ways, matching how [`auth`](https://github.com/steady-bytes/draft/blob/main/services/core/auth/main.go) already sits at the boundary between plain HTTP and Draft's RPC-everywhere convention: the webhook receiver is a raw HTTP endpoint (external systems — CI providers, git hosts — post arbitrary JSON, not Connect-framed requests), signature-verified via HMAC before anything else happens; everything else — `GetRun`, `ListWorkflows`, `ListRuns`, and the manual-trigger RPC the UI itself uses — is an ordinary Connect/gRPC service, sketched below.

```protobuf
// api/tooling/workflow/v1/service.proto (sketch)
service WorkflowService {
  rpc TriggerRun(TriggerRunRequest) returns (TriggerRunResponse);   // used by the UI's "run now" button
  rpc GetRun(GetRunRequest) returns (Run);
  rpc ListRuns(ListRunsRequest) returns (ListRunsResponse);
  rpc ListWorkflows(ListWorkflowsRequest) returns (ListWorkflowsResponse);
  // CreateWorkflow/UpdateWorkflow back "Authoring workflows without a file"
  // below (Phase 10) -- used by the UI's New/Edit workflow pages, and by
  // anything else that'd rather not touch workflows_dir directly.
  rpc CreateWorkflow(CreateWorkflowRequest) returns (CreateWorkflowResponse);
  rpc UpdateWorkflow(UpdateWorkflowRequest) returns (UpdateWorkflowResponse);
  // DeleteWorkflow removes a definition; run history is untouched (runs are
  // keyed by workflow_name as a plain string, not a foreign key).
  rpc DeleteWorkflow(DeleteWorkflowRequest) returns (DeleteWorkflowResponse);
}

// CreateWorkflow/UpdateWorkflow are YAML-in, YAML-validated-server-side --
// not a structured message built field-by-field -- so both reuse the exact
// parse-and-validate path workflows_dir files already go through
// (loader.go's ParseWorkflow), rather than a second workflow-authoring code
// path that could drift from the file-based one.
message CreateWorkflowRequest {
  // yaml is a complete workflow document, the same shape as a workflows_dir
  // file's contents. Rejected (ALREADY_EXISTS) if metadata.name collides
  // with an existing workflow -- see "Authoring workflows without a file"
  // for why Create and Update are deliberately not the same upsert.
  string yaml = 1;
}
message CreateWorkflowResponse {
  Workflow workflow = 1;
}
message UpdateWorkflowRequest {
  // name identifies the workflow being replaced; yaml is its full new
  // definition. name must equal yaml's own metadata.name -- a mismatch is
  // rejected as INVALID_ARGUMENT, not treated as a rename (see below).
  string name = 1;
  string yaml = 2;
}
message UpdateWorkflowResponse {
  Workflow workflow = 1;
}
message DeleteWorkflowRequest {
  string name = 1;
}
message DeleteWorkflowResponse {}

message Run {
  string run_id = 1;
  string workflow_name = 2;
  RunStatus status = 3;
  google.protobuf.Timestamp started_at = 4;
  google.protobuf.Timestamp finished_at = 5;
  repeated StepResult steps = 6;
}

enum RunStatus {
  RUN_STATUS_UNSPECIFIED = 0;
  RUN_STATUS_PENDING = 1;
  RUN_STATUS_RUNNING = 2;
  RUN_STATUS_PASSED = 3;
  RUN_STATUS_FAILED = 4;
}
```

Every status transition — a run starting, a step completing, a run finishing — is also published as a CloudEvent on [Catalyst](/docs/architecture/core-services#catalyst). Polling `GetRun` remains the source of truth (a client that missed an event can always ask), but Catalyst is what lets Bench's own UI update live instead of re-polling on a timer — covered next.

Three CloudEvent types are published, one per transition named above, each carrying a purpose-built payload rather than the full `Run`/`StepResult` (no nested steps, no `request`/`result`/`detail` — a subscriber that needs that level of detail calls `GetRun`, which stays the source of truth):

```protobuf
// RunEvent backs RunStarted and RunFinished.
message RunEvent {
  string run_id = 1;
  string workflow_name = 2;
  RunStatus status = 3;
  google.protobuf.Timestamp started_at = 4;
  google.protobuf.Timestamp finished_at = 5;
}

// StepEvent backs StepCompleted.
message StepEvent {
  string run_id = 1;
  string workflow_name = 2;
  string step_name = 3;
  StepStatus status = 4;
  string error = 5;
  google.protobuf.Timestamp started_at = 6;
  google.protobuf.Timestamp finished_at = 7;
}
```

| CloudEvent `type` | Payload | Fires when |
|---|---|---|
| `tooling.workflow.v1.RunStarted` | `RunEvent` (`status = RUN_STATUS_RUNNING`, `finished_at` unset) | A run moves PENDING → RUNNING, at the top of `execute` (scheduler.go) |
| `tooling.workflow.v1.StepCompleted` | `StepEvent` | Every step reaches a terminal status — PASSED, FAILED, or SKIPPED (a step blocked by an upstream hard failure "completes" as skipped too) |
| `tooling.workflow.v1.RunFinished` | `RunEvent` (`status` = the run's final PASSED/FAILED) | The run itself reaches a terminal status, at the end of `execute` |

Each event's CloudEvent envelope sets `source = "/services/bench"` and the `subject` attribute to the run's `run_id` (including on `StepCompleted`, so a subscriber can group a run's step events without parsing the payload). Published over a single long-lived Connect bidi-stream to Catalyst's `ProducerService` (`catalyst.go`) — the same raw-client pattern `services/examples/producer` uses, since Catalyst's `chassis.Broker` interface is for broker *implementations*, not for services that just want to publish. Publishing is best-effort: a failure (Catalyst unreachable, stream broken) is logged and otherwise ignored, the same "logged, not fatal" convention this package already uses for persistence — a run's correctness never depends on whether its events made it onto Catalyst.

**Known limitation, in Catalyst itself, not Bench**: with more than one `Consume` subscriber connected at the same time, Catalyst currently delivers each event to only one of them rather than to every interested subscriber — see [Catalyst's "Known issues"](/docs/architecture/core-services#catalyst) for the root cause. A single subscriber (e.g. Bench's own UI, once it consumes these events for live updates) sees every event correctly; this only matters once a second concurrent subscriber is added.

## The UI

Bench's UI is server-side rendered, using [DaisyUI](https://daisyui.com/) (a Tailwind-based component library) for every page and element, as required. This is a deliberate departure from [Blueprint's web client](/docs/architecture/blueprint-web-client-components), which is a Dioxus/Rust-compiled-to-WASM single-page app — worth being explicit about why, since it's the more unusual choice for a Draft service today:

- A test-run dashboard is fundamentally document-shaped (tables, status badges, a timeline) rather than app-shaped (no complex client-side state machine, no offline-first requirements) — the case where SSR's simplicity wins over a SPA's is exactly "render a page, occasionally patch a fragment of it," not "manage a rich interactive canvas" (which is what justifies Blueprint's WASM-rendered graph view).
- SSR keeps the whole stack in Go, next to the service logic it's rendering — no separate build toolchain, no WASM bundle, no client-server type duplication to keep in sync (Blueprint's web client pays that cost deliberately, for a payoff — an interactive graph — that doesn't apply here).

**Proposed stack**: [`templ`](https://templ.guide/) (compile-time-checked Go HTML templates — a good fit for a codebase that already leans on protobuf's compile-time contracts everywhere else) for markup, [htmx](https://htmx.org/) for the interactive parts (polling a run's status, swapping in updated step rows) without a client-side framework, and DaisyUI/Tailwind for styling. Go's standard `html/template` is a viable zero-dependency fallback if the team would rather not add `templ`'s code-generation step; the page inventory and DaisyUI mapping below hold either way.

| Page | Purpose | Key DaisyUI components |
|---|---|---|
| Dashboard | Pass rate, currently-running count, recent failures at a glance | `stats`, `badge`, `alert` for recent failures |
| Workflow list | Every configured workflow, its webhook URL, last-run status | `table`, `badge` (status color-coded) |
| Workflow detail | The YAML source, webhook config, run history for this workflow | a plain styled `<pre>` block (rendered YAML — see [note](#implementation-plan) on why `mockup-code` was dropped), `table` |
| Run detail | Per-step status, timing, and assertion results for one run | `steps`/`timeline` — DaisyUI's timeline component maps almost exactly onto "a workflow run's step sequence," including in-progress vs. completed states |
| New/edit workflow | Author or edit a workflow's YAML; delete from the detail page; search and insert a step from the configured plugin registry | `textarea` for the form; `alert alert-error` for a validation failure; a right-hand search sidebar (`input`, plugin result cards) |
| Settings | Add, edit, or remove the garage-compatible plugin registries the search sidebar and `garage://` steps resolve against, seeded from `config.yaml` at startup | `table` for the registry list; `input`/`alert` for a failed connection check |

The **Run detail** page is the one that benefits most from live updates: while a run is `running`, its DaisyUI `steps` timeline is wired via htmx's SSE extension to a Catalyst-backed event stream, so step rows flip from pending → running → passed/failed in place as the corresponding CloudEvents arrive — no polling loop in the browser, and no client-side state beyond what htmx swaps into the DOM directly from server-rendered fragments.

### Authoring workflows without a file

Every workflow today comes from a `.yaml` file in `bench.workflows_dir`, loaded once at startup (`workflows.go`'s `loadWorkflowsDir`) — there's no way to create or edit one without touching disk and restarting the process. This section designs the missing piece: a New/Edit workflow page backed by real `CreateWorkflow`/`UpdateWorkflow` RPCs (sketched above), so a workflow can be authored entirely from the browser. Phase 10 in the [Implementation plan](#implementation-plan) tracks building it; nothing in this section is built yet.

**Reuse, not a parallel authoring path.** `loader.go`'s `ParseWorkflow(data []byte) (*workflowv1.Workflow, error)` already does exactly what a Create/Update call needs — parse YAML, convert to the proto type, validate (required fields, a recognized `uses:` scheme, an acyclic `depends_on` graph) — and returns the first error found, in the same terms a file author already sees from `-validate`. Both new RPCs call it verbatim. Persistence is already there too: `store.go`'s `UpsertWorkflow`/`GetWorkflow`/`ListWorkflows` exist today only because `loadWorkflowsDir` calls them at startup — the RPC handlers call the same three methods, nothing new to write at that layer.

**Create vs. Update, deliberately not one upsert.** The store's `UpsertWorkflow` is unconditional (insert-or-replace on the `name` primary key), but the RPC layer enforces real create/update semantics on top of it: `CreateWorkflow` does a `GetWorkflow` existence check first and fails `ALREADY_EXISTS` if the name is taken (steering a "New workflow" submission that collides toward Update instead of silently clobbering something); `UpdateWorkflow` fails `NOT_FOUND` if the name doesn't exist yet, and rejects (`INVALID_ARGUMENT`) if the YAML's own `metadata.name` doesn't match the identity being updated — a rename isn't exposed as an Update; it isn't exposed at all in this design (see Open questions).

**A real gap this feature has to close, not just UI plumbing: webhook routing is static today.** `main.go` builds `slugRouting` (slug → workflow name) once from whatever `loadWorkflowsDir` returned at startup, and `webhook.go`'s handler reads that same map for the life of the process. A workflow created or edited through the UI — including changing its own webhook slug — wouldn't be reachable by webhook until Bench restarts, which defeats the point of "no file, no restart" for exactly the trigger path most workflows actually use. Phase 10 includes making this dynamic (recomputing from `store.ListWorkflows` per webhook request, or updating the in-memory map on every successful Create/Update) — without it, this feature would only be half-true.

**No new validation UX beyond what `ParseWorkflow` already gives.** The New/Edit page is a plain DaisyUI `textarea` (matching the doc's own page-table sketch) and a Save button doing a normal form POST — no live/inline-as-you-type validation, no YAML syntax highlighting beyond `font-mono`. A failed Create/Update re-renders the same form with the request body preserved and `ParseWorkflow`'s error shown in an `alert alert-error` banner, the same visual language `run_detail.html` already uses for a failed step. This matches the rest of Bench's UI: server does the real work, the browser gets an SSR page back, no client-side framework.

**File-based and UI-authored workflows coexist in the same table without conflict, by design — not "mostly."** An earlier draft of this section had `loadWorkflowsDir` unconditionally re-upserting every file on every restart, which meant a file added or edited after a workflow was first seeded would silently clobber whatever the UI had since done to it. Asked directly, the actual intent turned out to be simpler and stronger: **files seed, once — after that, a workflow is DB-owned.** `loadWorkflowsDir` now only persists a name it hasn't seen in the DB before; once a row exists (from a file *or* the UI), later restarts skip re-loading its file entirely, even if that file's contents changed. This closes the collision case outright rather than just documenting around it: there's no longer a write path that can revert a DB-side edit, so "don't reuse a name across a file and a UI-created workflow" isn't even a rule that needs stating anymore.

### Connecting a plugin registry

Authoring a `garage://` step today means knowing a plugin's exact name, version, and `with:` shape ahead of time — in practice, browsing to Garage's own catalog UI in a separate tab, reading its detail page, and hand-copying the `uses:` snippet it renders there. And *which* registry a `garage://` step even resolves against is a single `garage.address` value in `config.yaml`, read once at startup — no way to point Bench at a different (or additional) garage-compatible registry without editing a file and restarting. This section designs three things closing that gap: **plugin registries declared in `config.yaml`** and loaded into the running service, a **Registries page** to add, edit, or remove them at runtime without a restart, and a **search sidebar** on the New/Edit workflow page that turns "browse Garage, copy a snippet, switch tabs, paste it" into one page. Phase 11 in the [Implementation plan](#implementation-plan) tracks building it; nothing in this section is built yet.

**"Garage-compatible" is already true, not a new requirement.** `garage_plugin.go`'s executor has only ever depended on the `tooling.plugin_catalog.v1.PluginCatalogService` proto contract — never anything Garage-specific — so "connect to a garage-compatible registry" doesn't change what Bench assumes about the other end, only that the set of addresses it checks is now configurable and plural instead of fixed and singular.

**Registries are seeded from config the same way workflows are, and for the same reason.** A new `bench.plugin_registries` config key — a list, not a single address:

```yaml
bench:
  plugin_registries:
    - name: garage
      address: http://localhost:9301
    - name: internal-plugins
      address: http://internal-registry.example:9301
```

At startup, each entry is seeded into a new `plugin_registries` table (`name` primary key, `address`) — but only the *first* time its name is seen, exactly matching `loadWorkflowsDir`'s seed-once behavior (see "File-based and UI-authored workflows coexist..." above): a registry later added, renamed, or re-pointed through the UI is DB-owned from then on, and re-editing `config.yaml` and restarting does nothing further to it. This is a direct reuse of an already-proven pattern, not a new one. **Backward compatible with what's already shipped**: if `bench.plugin_registries` is unset, Bench seeds one implicit registry named `default` from the existing static `garage.address` value — every already-written workflow (including `catalyst-consume-e2e.yaml`) and every already-deployed `config.yaml` keeps working with zero changes required.

**Registries get a real RPC surface, the same shape Create/Update/Delete Workflow already established.** A new `SettingsService` (its own small proto package, `api/tooling/settings/v1` — matching Garage's `plugin_catalog`/`step_executor` each getting a dedicated package rather than being folded into `workflow/v1`), with list-shaped CRUD rather than a singleton getter/setter, since there can be more than one registry:

```protobuf
// api/tooling/settings/v1/service.proto (sketch)
service SettingsService {
  rpc ListPluginRegistries(ListPluginRegistriesRequest) returns (ListPluginRegistriesResponse) {}
  // AddPluginRegistry/UpdatePluginRegistry live-check address (a real
  // PluginCatalogService.List call, page_size 1) before persisting --
  // see "Live-checked, not blindly saved" below.
  rpc AddPluginRegistry(AddPluginRegistryRequest) returns (AddPluginRegistryResponse) {}
  rpc UpdatePluginRegistry(UpdatePluginRegistryRequest) returns (UpdatePluginRegistryResponse) {}
  rpc DeletePluginRegistry(DeletePluginRegistryRequest) returns (DeletePluginRegistryResponse) {}
}

message PluginRegistry {
  string name = 1;
  string address = 2;
}

message ListPluginRegistriesRequest {}
message ListPluginRegistriesResponse {
  repeated PluginRegistry registries = 1;
}

message AddPluginRegistryRequest {
  string name = 1;
  string address = 2;
}
message AddPluginRegistryResponse {
  PluginRegistry registry = 1;
}

message UpdatePluginRegistryRequest {
  string name = 1;
  string address = 2;
}
message UpdatePluginRegistryResponse {
  PluginRegistry registry = 1;
}

message DeletePluginRegistryRequest {
  string name = 1;
}
message DeletePluginRegistryResponse {}
```

**Live-checked, not blindly saved.** `AddPluginRegistry`/`UpdatePluginRegistry` make a real `PluginCatalogService.List` call (`page_size: 1`) against the submitted address before persisting it, and fail clearly if nothing garage-compatible answers there — the same "reject bad input at the boundary" discipline `ParseWorkflow` already applies to workflow YAML, rather than silently saving an address that then breaks every search and every `garage://` step that happens to resolve against it.

**Federated search, first-match execution.** Two different operations need "all the configured registries," and they need it differently:

- The **search sidebar** queries every registry's `Search`/`List` and merges the results, each one tagged with which registry it came from — necessary once two registries can plausibly publish a plugin with the same name, so a human picking one isn't guessing.
- **`garage://name@version` resolution** (`garagePluginExecutor`) deliberately keeps the `uses:` string's shape unqualified — no registry name in it, so nothing already written has to change — by checking configured registries in a fixed order (insertion order, oldest first) and using the first one whose `Get(name, version)` succeeds. If two registries both publish the same `(name, version)`, the earlier-added one silently wins; see Open questions.

Worth being explicit about a subtlety this doesn't change: a registry only stores manifests (`Get`/`List`/`Search`) — it never tells Bench *where a live instance is running*. That part is exactly Phase 6's existing `PluginResolver.ResolveByProcessName` via Blueprint, completely unaffected by which registry's catalog happened to list the plugin.

**Dynamic, like everything else this doc's "no restart" thread already insists on.** `garagePluginExecutor` currently builds one `PluginCatalogServiceClient` at `Scheduler` construction time, pointed at whatever `garage.address` said at process startup. That has to change to resolve the *current* list of registries fresh on every `Execute` call — otherwise adding, editing, or removing a registry through the UI wouldn't actually change what a `garage://` step can resolve against without a restart, exactly the gap Phase 10c closed for webhook routing. Same principle, same fix shape, now over a list instead of one address.

**The search sidebar needs no new Bench-side RPC for the search itself — Bench is just another client of an API that already exists, N times over instead of once.** New/Edit workflow's layout gains a right-hand panel: a search input and a scrollable result list, calling Garage's existing `Search`/`List` RPCs directly from `ui.go` against every currently-configured registry, the same way `garagePluginExecutor` already does for execution. htmx handles the fetch — a debounced `hx-trigger` on the search input swaps a results fragment — no new client-side framework. Each result card shows a small badge naming which registry it came from.

**Appending, not inserting at the cursor.** Clicking a result's "Add step" button has to put a step block into the workflow `textarea`. Inserting at an arbitrary cursor position risks landing mid-indentation and producing invalid YAML; appending a correctly-indented block to the end of whatever's already there is simpler and can't corrupt existing content. htmx can fetch and swap HTML, but it can't reach into a `<textarea>`'s value — this needs a small amount of real client-side JavaScript, the same kind of narrowly-scoped, non-framework exception `workflow_detail.html`'s delete-confirm `onsubmit` already is.

**The snippet itself is a small, deliberate duplication, not a new cross-service call.** Garage's own `ui.go` already computes a ready-to-paste `uses:` snippet from a plugin's `config_schema` (`usesSnippet`, rendered on its own plugin detail page) — but that's private page-rendering logic, not exposed over RPC, and adding a dedicated RPC to Garage's `PluginCatalogService` just to externalize a string-formatting helper isn't worth the cross-service surface it would add. Bench reimplements the same small walk (required `config_schema` properties → placeholder values) itself, matching this repo's existing precedent of small, deliberate duplication between tooling services (e.g. both services' entire "workshop" theme CSS is duplicated verbatim, not shared) over inventing a package neither service's own Go module can actually import from the other.

## Decided

- **Execution model**: full DAG from v1, not sequential-first (see Execution model above).
- **Storage**: Postgres, via the same [`pkg/repositories/postgres`](https://github.com/steady-bytes/draft/tree/main/pkg/repositories) + `bun` pattern already proven in `golf-tracker`'s `course_creator` service, registered with Chassis the ordinary way (`WithRepository`). Chosen over reusing the in-progress `pkg/repositories/sqlite` work specifically to avoid making Bench's first real usage depend on unfinished, moving-target infra, and over Blueprint's KV store because "show me the last N runs of this workflow" is a query pattern a KV store answers poorly at volume. Because `Repository` is already a swappable Chassis plugin, this doesn't foreclose an sqlite-backed mode later if that work matures.
- **Location**: `services/tooling/bench`, not `services/core/bench` — see [End-to-End Testing — Overview](/docs/architecture/e2e-testing-overview) for why Bench isn't core cluster infrastructure the way Blueprint/Fuse/Catalyst are.

## Implementation plan

Proto definitions (`api/tooling/workflow/v1`, `api/tooling/step_executor/v1`) are Phase 0 of the [cross-cutting plan](/docs/architecture/e2e-testing-overview#implementation-plan) and a prerequisite for everything below. Each phase here produces something runnable and testable before the next one starts — none of Phases 1–5 need Garage to exist. See the [tracking convention](/docs/architecture/e2e-testing-overview#implementation-plan) for how checked items are annotated.

- [x] **1. Scaffolding.** `services/tooling/bench`: `go.mod`, `main.go`, `config.yaml` — following `services/core/auth`'s shape (it's the smallest existing example of the same builder pattern). Wire the Postgres repository via `chassis.WithRepository` with an initial schema for `workflows` and `runs`/`step_results`, and register with Blueprint. Deliverable: a service that starts, registers, opens its database connection, and does nothing else yet — proves the Chassis/Postgres/Blueprint wiring before any workflow logic exists. _(completed 2026-08-22 22:24 UTC — `go build`/`go vet`/`gofmt` all clean; `workflowRow`/`runRow`/`stepResultRow` bun models with `jsonb` columns for the nested proto fields, per the "don't use the generated types directly as bun models" guidance above; schema creation smoke-tested against a real local Postgres container. `go.mod` needs local `replace` directives for `pkg/chassis` and `pkg/loggers` — no published release of either contains `chassis.Effect` yet, which `WithRepository` is now built on. **Correction from Phase 3**: the `Definition`/`Result` jsonb columns were originally typed `[]byte`, which silently double-encodes already-serialized JSON through bun's jsonb marshaling — fixed to `json.RawMessage` when Phase 3 found it against a live round-trip; see Phase 3's note below.)_
- [x] **2. Workflow loading.** A loader that parses the `Workflow`/`Step` YAML shape from the [Primitives](#primitives) section into the Phase 0 proto types, validates it (required fields, a `uses:` scheme of either `bench://` or `garage://`, and — since DAG is in scope from v1 — a `depends_on` graph with no cycles and no reference to an undeclared step), and rejects anything invalid at load time rather than at run time. Deliverable: a CLI or RPC that loads a workflow file and reports valid/invalid with a specific reason, no execution yet. _(completed 2026-08-22 22:31 UTC — `loader.go` parses into an intermediate plain-Go-typed struct first, not directly into the generated proto types: `Step.With`/`Step.Expect` (`*structpb.Struct`) and `Step.OnFailure` (an int32-backed enum) can't be `yaml.Unmarshal`'d into by reflection, verified before implementation. 3-color DFS cycle detection over the graph with the "empty `depends_on` defaults to the previous step" rule already applied, reporting the actual cycle path. `-validate <path>` CLI flag plus 12 passing table-driven tests, including the doc's full worked example as a fixture. `go build`/`go vet`/`gofmt`/`go test` all clean.)_
- [x] **3. The built-in `grpc-call` executor and the DAG scheduler.** The actual execution engine: given a loaded, valid workflow, walk the `depends_on` graph, run unblocked steps (concurrently where more than one is unblocked at once), resolve `{{ steps.X.result }}` templating from completed steps into dependents' `with:` blocks, and evaluate `expect:` assertions against each `grpc-call` response. Deliverable: the example workflow from the [Primitives](#primitives) section — minus its `garage://` step — runs end to end via a manual trigger (a CLI command or an unexposed RPC is fine here) and produces a correct pass/fail `Run` record with per-step results in Postgres. _(completed 2026-08-22 23:00 UTC — `scheduler.go` (goroutine-per-step, mutex-protected shared state, `-race`-clean), `discovery.go` (resolves a step's `with.service` to a live address via Blueprint's `Query`, exploiting the same ignored-`Filter` behavior noted in the cross-cutting plan), `grpc_call.go` (plain HTTP+JSON Connect calls, no reflection needed; recursive `expect.body` assertions including a bonus `matches` regex operator), `templating.go`. **Also fixed a real bug found in Phase 1's `model.go`**: bun's jsonb column type re-marshals a field's Go value through JSON before writing, and a bare `[]byte` gets base64-encoded by that pass — silently double-encoding the already-serialized `protojson` output Phase 1 was storing, breaking every future read. Fixed by switching `Definition`/`Result` from `[]byte` to `json.RawMessage` (which marshals itself verbatim); verified against `bun`'s actual source and a live Postgres round-trip, not just asserted. Proved end to end against real running Blueprint + Postgres + `services/examples/crud` (temporarily patched to drop its Fuse dependency for this proof, then `git checkout`-reverted — confirmed clean afterward): a real create→query workflow ran, passed, and persisted correctly, including templated data flowing between steps; the failure/skip path was exercised live too. 29 tests pass including `go test -race`.)_
- [x] **4. The webhook receiver.** The raw HTTP endpoint from [the webhook trigger and status API](#the-webhook-trigger-and-status-api): HMAC-SHA256 signature verification against the configured secret, synchronous `Run` creation, `202 { run_id }` response, and handing the run to Phase 3's scheduler asynchronously. Deliverable: `POST /webhooks/{slug}` behaves exactly as documented, including rejecting a bad signature before touching anything else. _(completed 2026-08-22 23:20 UTC — mounted on chassis's own mux via `AddHandler`, not a second listener; uses `hmac.Equal` for constant-time signature comparison (not `==`), a 1MiB body cap, and uniform 401s that don't leak which specific check failed. `Scheduler.Run` was split into `beginRun` (single synchronous write, returns immediately) + `execute` (the DAG walk) + a new `StartRun` (async) — `Run` itself is unchanged and still backs the `-run` CLI flag and every Phase 3 test. Live-proved: a correctly-signed webhook call returned `202` with a `run_id` in ~30ms, long before the workflow could have finished; bad/missing signatures returned 401 and never inserted a run row, confirmed via direct SQL count before/after.)_
- [x] **5. `WorkflowService` RPCs.** `TriggerRun` (the UI's manual-trigger path — same execution path as the webhook, different entry point), `GetRun`, `ListRuns`, `ListWorkflows`. Deliverable: the full poll contract (`GET /runs/{id}` equivalent) works, which is everything a UI or an external caller needs regardless of whether Bench has a UI yet. _(completed 2026-08-22 23:20 UTC — also added `workflows.go`: loads/validates every YAML file in a configured `workflows_dir` at startup and persists it, since nothing previously loaded workflow definitions into Postgres outside of Phase 2/3's CLI flags. **Second real bug found and fixed**, this time in `stepResultRow.toProto`: a step with no `Result` (any failed/skipped step) stores as the literal 4-byte JSON value `null` (confirmed live via `result::text` — not SQL `NULL`, since `json.RawMessage(nil).MarshalJSON()` returns `"null"` by its documented behavior), which `protojson.Unmarshal` correctly rejects as not a valid `Struct` — this was latent since Phase 3 because nothing had read a `Result` back until `GetRun` existed to do it. Fixed by treating both zero-length and literal `"null"` as "no result." All 47 tests pass including `go test -race`.)_
- [x] **6. Garage integration.** Resolving a `uses: garage://name@version` reference — query Garage's `PluginCatalogService` for the manifest, find a live instance via Blueprint, call `StepExecutor.Execute`. **Depends on [Garage's Phase 1–3](/docs/architecture/garage-plugin-repository#implementation-plan)** being done, specifically a reference plugin — this is the first point where the two services' work actually has to meet. _(completed 2026-08-23 03:53 UTC. `garage_plugin.go`'s `garagePluginExecutor`: confirms `(name, version)` is actually published via Garage's `Get` RPC (Garage never tracks *where* a plugin runs — its catalog is purely descriptive manifests — so this is an existence check, not address resolution), then finds a live instance via a new `discovery.go` addition, `PluginResolver.ResolveByProcessName`. **Real design finding, not assumed**: resolving by the RPC service name every plugin shares (`tooling.step_executor.v1.StepExecutor`, the way `ServiceResolver.Resolve` already works for `grpc-call` steps) can't disambiguate *which* plugin a `garage://` reference means — every plugin registers the identical RPC service name with Blueprint. Traced through `pkg/chassis/builder.go`'s `synchronize` and `services/core/blueprint/service_discovery/`'s `Process` message to find the fix: `Process.name` (the process's own `service.name` config value, e.g. `"catalyst-consume"`, distinct from the RPC-service-keyed `Metadata`) is what disambiguates, and `Query` already returns it — no Blueprint change needed, just a second cache keyed off it, built from the same single `Query` call `Resolve` already pays for. Filtered to `PROCESS_RUNNING` only (stricter than the existing `Resolve`'s unfiltered lookup — deliberate, since a plugin restarted a few times during local development is far likelier to leave multiple stale same-`Name` entries than a `grpc-call` step's single targeted service ever is). **Known, deliberate simplification**: resolution matches on plugin name only, not version — nothing in this codebase's registration path advertises which version a running instance is, so two different published versions running simultaneously can't be disambiguated at the network layer; not hit in practice since nothing here runs multiple versions of one plugin side by side, but a real limitation, not an oversight. `Executor`'s existing interface (`Execute(ctx, step, with)`) was reused as-is — `StepRequest.Context` (prior-steps snapshot, beyond what `with:` already carries post-templating) is left unset, since threading a live snapshot through would touch every `Executor` implementation including every fake one `scheduler_test.go` defines, for a capability nothing built so far needs. Built and live-verified together with a real new plugin, `services/tooling/catalyst-consume` (waits for a Catalyst CloudEvent of a configured type and extracts named fields from its payload — see [Garage's own Phase 3 note](/docs/architecture/garage-plugin-repository#implementation-plan) for the plugin's own details) — a `catalyst-consume-e2e.yaml` workflow's `garage://catalyst-consume@v1` step genuinely waited for and correctly extracted fields from a real `crud-e2e` run's `RunFinished` event, `Detail` confirming resolution actually hit `catalyst-consume`'s own address (`localhost:9303`) and not some other plugin's.)_
- [x] **7. Catalyst event publishing.** Publish a CloudEvent on every run/step status transition, as described in [the webhook trigger and status API](#the-webhook-trigger-and-status-api) section. Deliverable: an external subscriber can observe a run's progress without polling `GetRun`. _(completed 2026-08-23 00:55 UTC — new `RunEvent`/`StepEvent` proto messages (api/tooling/workflow/v1), deliberately lighter than `Run`/`StepResult` themselves (no steps, no request/result/detail jsonb payloads) since `GetRun` remains the source of truth for full detail. `catalyst.go`: a `catalystPublisher` backed by a single long-lived `Produce` bidi-stream to Catalyst (the same raw Connect-client pattern `services/examples/producer/main.go` already demonstrates — Catalyst's `chassis.Broker` interface is what a broker *implementation* backs, not something an ordinary publishing client goes through), opened once at startup and reused for the process's life, mutex-guarded since a bidi stream's `Send` isn't safe for concurrent use and the scheduler runs steps concurrently. `EventPublisher` is a small interface (`RunStarted`/`RunFinished`/`StepCompleted`) so `scheduler.go`'s `execute`/`runStep` don't depend on the concrete Catalyst client directly — `newSchedulerWithExecutors` (used by every `scheduler_test.go` case) defaults to a `noopEventPublisher{}`, so no test needed a fake Catalyst connection. Publishing is best-effort/non-fatal, matching every other persistence call in this package. Three event types: `tooling.workflow.v1.RunStarted` (on the PENDING→RUNNING transition), `tooling.workflow.v1.StepCompleted` (every step reaching PASSED/FAILED/SKIPPED — skipped counts as "completing" too, per the doc's own "a step completing" language), `tooling.workflow.v1.RunFinished` (the run's own terminal PASSED/FAILED transition). **Verified genuinely end-to-end, not just "Send didn't error"**: a throwaway standalone Consume subscriber (not committed) confirmed real delivery through a running Catalyst against two live `crud-e2e` runs, receiving all 4 events per run (`RunStarted`, 2×`StepCompleted`, `RunFinished`) with correct `run_id`/timestamps/status. That verification surfaced a real, pre-existing quirk in Catalyst itself (`services/core/catalyst/broker/atomicMap.go`): `Broadcast`'s routing key is computed from the Go `*acv1.CloudEvent` struct's own proto descriptor name (constant for every event, regardless of the event's `.Type` field), and delivery goes through one shared unbuffered channel that every open `Consume` stream reads from — so with N concurrent `Consume` streams open, each produced event is fanned out to exactly one of them (a work-queue, effectively), not broadcast to all N, despite the method's name and every documented client (`examples/consumer`'s own comment: "the broker broadcasts all events to all streams") assuming true broadcast. Confirmed by reading the source, not guessed. **Not fixed** — it's pre-existing `services/core/catalyst` code, out of scope for a Bench-side phase, flagged here the same way `discovery.go`'s resolver-caching bug was flagged in Phase 8's note above rather than silently worked around or ignored. Practical implication for any future multi-subscriber consumer of these events: don't assume every subscriber sees every event under concurrent subscriptions.)_
- [x] **8. The UI.** The pages and DaisyUI components from [The UI](#the-ui) section, built with `templ` + htmx + DaisyUI; the Run detail page's live-updating timeline consumes Phase 7's Catalyst events over htmx SSE. Deliverable: everything in the page table above, working against real data from Phases 1–7. _(completed 2026-08-23 00:09 UTC, with two deliberate scope departures from the original description — both because their prerequisites don't exist yet, not oversights: **no "New/edit workflow" page** (`WorkflowService` has no Create/Update RPC — workflows only ever load from `workflows_dir` files — so no editor was built rather than shipping a form that looks functional and silently does nothing; Workflow detail is a read-only reconstructed-YAML view instead); **no Catalyst/SSE** (Phase 7 isn't built) — the Run detail page instead wraps itself in an htmx poll (`hx-trigger="every 2s"`) that **stops itself**: each polled fragment only carries the poll trigger while the run is non-terminal, so a terminal response's fragment simply has nothing left to re-trigger. Built with `html/template`, not `templ`, matching Garage's Phase 4 choice and for the same reason. Shares Garage's exact "workshop" theme byte-for-byte (this document's and Garage's UI section both call for one consistent identity across both products). Proved against real end-to-end runs, not fixtures: triggered `crud-e2e` from the UI against a real running `examples/crud` service, watched it fail cleanly when the target wasn't registered yet, watched it pass once it was, and confirmed the dashboard's pass-rate math and "needs attention" list against the real resulting run history. **Found and fixed a real bug this pass surfaced**: `notFoundPageData` lacked the `Nav` field `base.html`'s nav-highlighting now requires, which isn't a silent no-op in Go's `html/template` — a struct missing a referenced field fails the whole template execution, so the not-found page was serving a `404` status with almost no body until this was caught by actually curling it, not by `go vet`/`go build`. **Found, but did not fix (out of scope for a UI pass)**: `discovery.go`'s `blueprintResolver` caches Blueprint's registry for the resolver's entire process lifetime, not per-run as originally intended — a service that registers with Blueprint *after* Bench has already started is invisible to every run for the rest of that Bench process's life. Surfaced live while trying to demo a passing run against a freshly-started `crud`; worked around by restarting Bench, not by fixing the cache — flagging here since it's a real correctness gap in already-shipped Phase 3 code, not a UI concern.)_

  **Addendum, 2026-08-23 00:40 UTC — request/response/detail troubleshooting on the Run detail page.** `StepResult` gained two new `google.protobuf.Struct` fields, `request` (7) and `detail` (8), alongside the existing `result`: `request` is the step's `with:` block after template resolution — what was actually sent, captured in `scheduler.go`'s `runStep` regardless of whether the step then failed for some other reason — and `detail` is optional executor-specific diagnostic info (`bench://grpc-call@v1` reports the resolved address, exact URL, and HTTP status). Both flow through `model.go`'s `stepResultRow` (new `request`/`detail` jsonb columns, `structToJSON`/`jsonToStruct` helpers shared with `result` and preserving the Phase 5 "null"-literal-vs-empty handling) and render on the Run detail page as a per-step collapsible "troubleshooting detail" section (`run_detail.html`, DaisyUI `collapse-arrow`) — auto-open when the step failed, collapsed when it passed, so the page stays scannable but a failure's full request/response/detail is immediately visible without a click. `createSchema` now also runs idempotent `ALTER TABLE step_results ADD COLUMN IF NOT EXISTS` statements on every startup, needed because this was added after `step_results` already existed in local dev Postgres and `CREATE TABLE IF NOT EXISTS` alone doesn't retrofit new columns onto an existing table. **Found and fixed a real bug this pass surfaced**: `store.go`'s `UpsertStepResult` builds an `INSERT ... ON CONFLICT (run_id, step_name) DO UPDATE` with an explicit `Set(...)` column list — `runStep` always inserts a `RUNNING` row first, so a step's terminal write always lands on the `DO UPDATE` path, and `request`/`detail` were left out of that `Set` list. They round-tripped fine through `stepResultRowFromProto`/`toProto` and even survived the *first* insert, but the terminal upsert silently discarded them — `GetRun` came back with `result` populated but `request`/`detail` always absent, live-verified via a temporary always-failing fixture workflow (`zz-tmp-verify-fail.yaml`, removed after use, along with its run rows) both before and after the fix. Fixed by adding `request = EXCLUDED.request` and `detail = EXCLUDED.detail` to the `Set` chain. This is the same shape of bug as `notFoundPageData`'s missing `Nav` field above — a change that compiled and passed `go vet`/`go test` cleanly but was wrong at runtime, only caught by actually curling `GetRun` and checking the JSON, not by static checks.)_
  **Addendum, 2026-08-23 08:14 UTC — dropped `mockup-code` from the Workflow detail page's Definition block.** User report: "there are three dots in the top left corner of the configuration display... let's remove them and make sure to render the yaml file correctly." DaisyUI's `mockup-code` component (used since Phase 8) ships two `::before` pseudo-elements on its own class, neither of which had been noticed before: `.mockup-code:before` draws the three-dot terminal-window decoration via a `box-shadow` trick, and — the actual rendering bug, confirmed by fetching and grepping DaisyUI 4.12.24's real stylesheet rather than guessing — `.mockup-code pre:before{content:"";margin-right:2ch}` inserts an empty pseudo-element before the `<pre>`'s content that only visibly shifts the *first* line right by ~2 characters, since the whole YAML document was rendered as one single `<pre>` block rather than the one-`<pre>`-per-line-with-`data-prefix` shape `mockup-code` actually expects. Confirmed live via a zoomed screenshot: `apiVersion: bench/v1` sat indented relative to every other line below it. Fixed by dropping `mockup-code` entirely in favor of a plain styled `<pre>` (`font-mono text-sm bg-base-300 rounded p-4 overflow-x-auto whitespace-pre-wrap break-words`), matching the convention `run_detail.html`'s request/response/detail blocks already established — removes both the dots and the misaligned first line in one change, no CSS override needed. Verified live: dots gone, `apiVersion:` now flush with `kind:`/`metadata:`/`steps:`. Also found and cleaned up an unrelated stray `my-workflow` test fixture left over from earlier Phase 11 browser testing, discovered only because a restart's `workflow_count=3` didn't match the 2 real workflows expected.
- [ ] **9. (Stretch, deferred)** `chassis.Effect`-based automatic step teardown — see [Open questions](#open-questions) below; not scheduled until a concrete workflow needs it.
- [x] **10. Workflow authoring via the UI.** Design in [Authoring workflows without a file](#authoring-workflows-without-a-file) above. Deliverable: a workflow can be created, edited, and run entirely from the browser — no `workflows_dir` file, no restart — verified against a real webhook delivery to a workflow whose slug was set entirely through the UI, not by adding a file. _(completed 2026-08-23 04:40 UTC. All four sub-phases below shipped together, live-verified end to end via curl and the browser, not just unit tests. Also added `DeleteWorkflow` (RPC, store method, and a Delete button on the detail page) beyond the original design — decided in the same round of scoping questions that pinned down the seed-once behavior below.)_
  - [x] **10a. Proto.** `CreateWorkflow`/`UpdateWorkflow`/`DeleteWorkflow` RPCs and their request/response messages added to `api/tooling/workflow/v1/service.proto`, regenerated.
  - [x] **10b. RPC handlers.** `workflow_write.go` (new): `createWorkflow`/`updateWorkflow`, shared verbatim between `rpc.go`'s RPCs and `ui.go`'s form handlers — both call `loader.go`'s existing `ParseWorkflow` and `store.go`'s existing `GetWorkflow`/`UpsertWorkflow`, no new parsing/storage logic. `store.go` gained `DeleteWorkflow` (run history untouched — `runRow.WorkflowName` is a plain string column, not a foreign key). `rpc.go` maps `ErrWorkflowAlreadyExists`/`ErrWorkflowNotFound`/`ErrWorkflowNameMismatch` to `AlreadyExists`/`NotFound`/`InvalidArgument`.
  - [x] **10c. Dynamic webhook routing, plus a real seed-once change to `workflows.go` beyond the original design.** Clarifying what "seed the db, then never modified" (the user's own framing) actually meant surfaced a bigger, better decision than the original sketch: `loadWorkflowsDir` no longer unconditionally upserts every file on every restart — it now only seeds a name the *first* time it's seen (checked against `store.ListWorkflows` once, not a per-file query), and skips it silently thereafter, logging that it's DB-owned now. This is what actually makes a UI edit permanent: previously, even the "leave the file/UI collision as a documented footgun" answer would still have meant *every* restart re-clobbers a UI edit to a file-seeded workflow, not just a same-name collision. `webhook.go`: the static `slugToWorkflow` map and its `main.go`-built `slugRouting` argument are gone; `workflowForSlug` scans `store.ListWorkflows` per webhook request instead (same "stays small" scale assumption every other list view in this package already makes) — verified live: created a workflow with a webhook trigger through the UI and delivered a correctly-signed webhook to it with **zero restart** in between, `202` + real run that passed.
  - [x] **10d. UI.** New-workflow page (`GET /workflows/new`) and an Edit affordance (`GET /workflows/{name}/edit`) on the workflow detail page, both a plain `textarea` form POST; a failed Create/Update re-renders the same form with the submitted YAML preserved and the error in an `alert alert-error` banner (verified live for all three failure modes: duplicate name, mismatched rename attempt, invalid YAML, each showing the exact right message); success redirects to the detail page. A Delete button (red, `onsubmit="return confirm(...)"`) rounds out the detail page's action row alongside the existing Edit and Run now.

  **Scoping decisions from the user, asked directly rather than assumed** (since this phase had several genuine forks): no rename support (`UpdateWorkflow` rejects a `metadata.name` mismatch, exactly as designed) — file/UI name collisions left as a documented footgun rather than adding `origin` tracking (moot in practice once seed-once shipped, since a file can no longer re-clobber an existing row at all) — Delete included in this pass rather than deferred — and all of Phase 10 built together in one pass rather than RPCs-first.
- [x] **11. Plugin registry settings + a step-search sidebar.** Design in [Connecting a plugin registry](#connecting-a-plugin-registry) above. Deliverable: registries declared in `config.yaml` are seeded at startup; adding, editing, or removing one on the Settings page is live-checked, saved, and immediately changes what `garage://` steps and the search sidebar can see — no restart; the New/Edit workflow page's sidebar lets a real plugin from any configured registry be searched and appended into the textarea as a correctly-formed step, no manual copy-paste from Garage's own UI required anymore. _(completed 2026-08-23 07:39 UTC — all five sub-phases built and live-verified together, not separately: added a second registry through the real Settings form (live-checked against real running Garage), confirmed the search sidebar federated both registries' catalogs (`slack-notify`/`catalyst-consume`, each tagged by source), re-ran `catalyst-consume-e2e` and confirmed `garage://` resolution still passed with `Detail.registry` now naming which registry matched, and confirmed seed-once on a real restart (`seeded_this_run=0` for both the pre-existing workflow and registry rows). 69 tests pass (`-race` clean, up from 61), including a fake-registry-backed suite (`registry_write_test.go`, `garage_plugin_test.go`) that directly proves first-match-wins federation and the documented silent-collision behavior, not just the pure-Go helpers in isolation.)_
  - [x] **11a. Proto + schema.** New `api/tooling/settings/v1/service.proto` (`SettingsService`: `ListPluginRegistries`/`AddPluginRegistry`/`UpdatePluginRegistry`/`DeletePluginRegistry`); a new `plugin_registries` table (`name` primary key, `address`) in `model.go`/`createSchema`.
  - [x] **11b. Config seeding.** New `bench.plugin_registries` config key (a list of `{name, address}`); `plugin_registries.go`'s `loadPluginRegistries` mirrors `loadWorkflowsDir`'s seed-once behavior exactly. Unset falls back to one implicit `default` registry from the existing static `garage.address` value — verified live: the real running stack (never given a `bench.plugin_registries` config) seeded `default` → `http://localhost:9301` (Garage's real address) with zero config changes needed.
  - [x] **11c. Registry RPC + page.** `AddPluginRegistry`/`UpdatePluginRegistry` (`registry_write.go`) live-check the submitted address via a real `PluginCatalogService.List` call before persisting. `GET`/`POST /settings` UI: a table of configured registries plus an add form, wired to the same shared logic the RPCs use — proven live by adding a real second registry through the actual HTML form, not just the RPC directly.
  - [x] **11d. Federated resolution for execution.** `garage_plugin.go`'s `garagePluginExecutor` no longer caches one `PluginCatalogServiceClient` at construction; every `Execute` call fetches the current registry list and tries each in insertion order via the new `findPluginInRegistries` helper, using the first match. `Detail` now includes which registry resolved the plugin — confirmed live (`"registry": "default"` on a real passing run) and by two dedicated tests proving both first-match-wins across distinct registries and the documented silent-collision behavior when two registries publish the same `(name, version)`.
  - [x] **11e. Search sidebar.** New/Edit workflow's layout is now a two-column grid: the existing `textarea` plus a right-hand panel (search input, htmx-debounced, `hx-trigger="load"` on first render) that federates `Search`/`List` across every registry and renders each result tagged by source, with an "Add step" button carrying a server-computed snippet (`pluginUsesSnippet`, Bench's own small reimplementation of Garage's `usesSnippet`). **Real bug found and fixed during live verification**: the snippet button's `onclick="appendStep({{.Snippet | js}})"` double-escaped the value — `html/template` already auto-detects the JS-attribute context around `onclick="..."` and escapes accordingly, so the explicit `| js` pipe escaped it a second time, corrupting every newline/angle-bracket into literal escape-sequence text instead of real characters. Caught by actually clicking "Add step" and inspecting the resulting textarea value, not by any static check — fixed by removing the redundant manual `| js` pipe and trusting `html/template`'s own contextual auto-escaping, exactly as its documentation specifies. Verified fixed the same way: a real click now appends a correctly-formed, immediately-usable step block. **Second real bug, found by the user after this phase first shipped**: `pluginUsesSnippet` built its block at column 0 (`- name: ...`, fields at 2 spaces), but every `steps:` list in this codebase — the existing textarea content, `defaultWorkflowTemplate`, every real workflow file — indents list items 2 spaces (`  - name: ...`) with their fields at 4 and any nested `with:` fields at 6. Appending the mismatched block made the new step read as a sibling of `steps:` itself rather than a sibling of the existing step, producing structurally-wrong YAML. Fixed by shifting every line `pluginUsesSnippet` emits by 2 spaces (`  - name:` / `    uses:` / `    with:` / `      <field>:`), matching the established convention. Verified live: added a step from the search sidebar via a real click, confirmed the appended block aligned exactly with the existing step, then saved the workflow through the real form and confirmed it parsed with no validation error before deleting the test fixture.

## Open questions

- Whether step-level automatic teardown via `chassis.Effect` (discussed above) ships in v1 or is deferred behind an explicit `after:` block first, with the Effect-based version added once there's a concrete workflow that needs it — the same "don't build ahead of need" discipline applied to [Chassis's own roadmap](/docs/architecture/chassis-composability).
- **Renaming a UI-authored workflow.** Decided: no rename in v1 — `UpdateWorkflow` rejects a `metadata.name` change (confirmed with the user directly rather than assumed). A real rename (new primary key, same run history?) still isn't designed at all; revisit if it's ever actually asked for.
- **No optimistic concurrency on Update.** Two browser tabs editing the same workflow both succeed, last write wins, with no version check or conflict warning. Plausible to ignore for an internal dev tool; worth revisiting if Bench ever gets more than one concurrent author.
- **Silent first-match collisions across registries.** If two configured registries both publish the same `(name, version)`, `garage://` resolution picks whichever registry was added first with no warning that the other one was shadowed — the search sidebar shows both (tagged by registry), so a human would notice, but execution doesn't surface the ambiguity at all. A stricter design might fail loudly on a collision instead of silently picking one; not decided here.
- **Appending, not inserting at the cursor.** The search sidebar always appends a new step to the end of the textarea, regardless of where the cursor is — simple and can't corrupt existing YAML, but means a step added mid-edit never lands where you were actually looking. Revisit if that turns out to matter in practice.
- **No version grouping in search results.** `Search`/`List` return every published version of a plugin as its own row, matching Garage's own catalog page today, rather than one row per name with a version picker. Fine for a catalog that stays small; worth revisiting if it grows.
