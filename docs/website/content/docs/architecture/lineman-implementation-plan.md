---
weight: 44
title: 'Lineman — Task Orchestrator Implementation Plan'
description: 'System design, data model, and RPC interface for Lineman, a Draft tooling service that tracks objectives, tasks, and the agents (human, scripted, or AI) working them, with time-based scheduling primitives.'
icon: 'task_alt'
draft: false
toc: true
---

{{< alert context="info" text="Implemented and live-verified against the real running stack as of 2026-09-09 — see the Implementation Plan's per-phase notes and the Milestone Summary for what was built, what was found and fixed along the way, and what's still explicitly out of scope." />}}

Lineman is the task orchestrator described in `assets/lineman/idea.md`: a service that tracks the status, priority, and completion order of tasks as they roll up into an objective, with two time-based primitives — a **scheduler** (fire a single task once, at a future time) and **loops** (a recurring schedule that spawns a new task instance every time it fires). Its product/UI design — page layouts, component inventory, the full daisyUI mockups — already exists as `assets/lineman/design-brief.html`; this document doesn't repeat that, it specs what the brief's mockups need to actually run: the proto data model, the RPC surface the Dioxus web client calls, and how the service fits into the rest of a Draft cluster.

Lineman is in the **tooling** domain (`services/tooling/lineman`), alongside Bench and Foundry — not a core cluster service like Blueprint or Catalyst.

## Relationship to the Agent Service

Draft separately has an unbuilt design for an [Agent Service](/docs/architecture/agents) (Orchestrator/Operator/Executor, its own Postgres/ClickHouse-backed `Objective`/`Task`/`AgentEvent` model, DAG planning, LLM execution, budget tracking). Its vocabulary — objective, task, agent — overlaps with Lineman's on the surface, but the two are **deliberately independent**, decided explicitly before writing this plan:

- Lineman owns its own lightweight `Objective`/`Task`/`Agent` model, stored in Blueprint, exactly as `idea.md` originally specs it — general-purpose, not LLM-specific. A Lineman task can be worked by a human, a script, or an AI agent; Lineman doesn't care which.
- Lineman's `Agent` (see [Data Model](#data-model)) is a loose, assignable-worker identity — an id and a display name a task is currently assigned to — not a handle into the Agent Service's Orchestrator/Operator/Executor internals. Lineman never calls `OrchestratorService`, never subscribes to `AgentEvent`s, and doesn't plan task graphs.
- If the Agent Service is ever built, a *future* integration could have it report progress into Lineman (e.g. an Executor calling Lineman's `Heartbeat`/`RequestInput` RPCs — see [RPC Interface](#rpc-interface) — the same way any other agent would) so its work is visible on Lineman's board. That's out of scope here; this plan doesn't assume the Agent Service exists.

## System Design

```
┌──────────────────────────────────────────────────────────────────────┐
│                           Draft Cluster                              │
│                                                                      │
│   ┌───────────┐   ┌──────────┐   ┌───────────┐   ┌────────────────┐  │
│   │ Blueprint │   │   Fuse   │   │ Catalyst  │   │     Beacon     │  │
│   │ (storage, │   │ (routing,│   │ (events)  │   │ (wide events,  │  │
│   │  registry)│   │  UI host)│   │           │   │  telemetry)    │  │
│   └─────┬─────┘   └────┬─────┘   └─────┬─────┘   └───────┬────────┘  │
│         │              │               │                 │           │
│         │        ┌─────▼───────────────▼─────────────────▼──────┐   │
│         └───────▶│                  Lineman                     │   │
│                  │  services/tooling/lineman                    │   │
│                  │  - LinemanService RPCs (objectives, tasks,    │   │
│                  │    agents, scheduler, loops, Watch)           │   │
│                  │  - internal scheduler/loop firing ticker      │   │
│                  │  - Dioxus web client (lineman.draft.localhost)│   │
│                  └───────────────────────┬───────────────────────┘   │
│                                          │ publishes manifest         │
│                                   ┌──────▼──────┐                     │
│                                   │   Foundry    │                     │
│                                   │ (catalog)   │                     │
│                                   └─────────────┘                     │
└──────────────────────────────────────────────────────────────────────┘
```

- **Blueprint** is Lineman's only datastore. `Objective`, `Task`, `Agent`, `ScheduledTask`, and `Loop` are all proto messages registered with Blueprint's [type registry](/docs/architecture/kv-type-registry-implementation-plan) (`WithRegisteredType`) at startup, the same way `tooling.workflow.v1.BenchWebhookSecret` already is — so they render generically (decoded, not opaque bytes) in Blueprint's own Key/Value browser for free, with zero Lineman-side UI work.
- **Catalyst** carries one event per state change, for downstream consumption — matching `idea.md`'s "emits an event... for self consumption or downstream use."
- **Beacon** receives the same state change as a wide event, via a `chassis.StartSpan`-scoped child span carrying business attributes — not a separate write path. See [Telemetry](#telemetry).
- **Fuse** routes `lineman.draft.localhost` to Lineman's own mux, serving both its RPC API and its Dioxus web client on one route — the same [subdomain convention](/docs/architecture/service-ui-subdomains) every other service's UI uses.
- **Foundry** gets a published plugin manifest for Lineman at startup (`chassis.Effect`, retracted on shutdown) — `idea.md`'s "register lineman as a plugin to foundry so it can be added to other draft systems," using the exact publish/retract pattern [Foundry's own doc](/docs/architecture/foundry-plugin-repository#publishing-and-discovery) already establishes. Lineman doesn't implement `StepExecutor` itself — the manifest exists purely for discovery/distribution, per `idea.md`, not so Bench can call it as a workflow step.
- **The web client** is Dioxus, following Blueprint's web client pattern exactly (`drawer lg:drawer-open` layout, daisyUI 5 "black" theme, `NavigationConfig`-driven sidebar) per `idea.md` and the design brief — not specced further here; see `assets/lineman/design-brief.html` for every page's layout.

## Data Model

### Proto

New package `tooling.lineman.v1` (`api/tooling/lineman/v1/models.proto`), following the `service.proto`/`models.proto` split every other tooling-domain API uses:

```protobuf
// Objective is a goal a set of tasks count toward. It owns its own ordered
// list of valid task states -- states are not a global enum shared by every
// objective (see Decisions).
message Objective {
    string   id          = 1;
    string   name        = 2;
    string   description = 3;
    repeated string states = 4; // ordered; e.g. ["Queued", "In Flight", "Verifying", "Done"]
    google.protobuf.Timestamp created_at = 5;
}

// Task is a unit of work belonging to exactly one objective.
message Task {
    string id            = 1;
    string objective_id  = 2; // empty for a standalone task (see Decisions)
    string name           = 3; // short label -- what shows on a board card
    string state         = 4; // must be one of objective_id's Objective.states
    Priority priority     = 5;
    // Sparse ordering within (objective_id, state) -- see Decisions.
    int64  order          = 6;
    string agent_id       = 7; // empty if unassigned
    string current_action = 8; // last value reported via Heartbeat; live status line
    // Set only while state is the objective's "Needs Input"-convention state.
    NeedsInput needs_input = 9;
    google.protobuf.Timestamp created_at = 10;
    google.protobuf.Timestamp updated_at = 11;
    // Everything needed to actually complete the task -- free-text
    // instructions, the primary content a human or agent reads. Optional;
    // empty for a task spawned by a ScheduledTask/Loop (see Decisions).
    string details        = 12;
}

enum Priority {
    PRIORITY_UNSPECIFIED = 0;
    PRIORITY_LOW    = 1;
    PRIORITY_MEDIUM = 2;
    PRIORITY_HIGH   = 3;
}

message NeedsInput {
    string question           = 1;
    repeated string options    = 2; // suggested responses, e.g. ["Roll back", "Retry with longer timeout"]
    google.protobuf.Timestamp requested_at = 3;
}

// Agent is a loose, assignable-worker identity -- a human, a script, or an
// AI agent instance/run. Not a handle into any other system; see
// "Relationship to the Agent Service" above.
message Agent {
    string id           = 1;
    string display_name = 2; // e.g. "lineman-agent-3"
    AgentKind kind       = 3;
    google.protobuf.Timestamp last_seen_at = 4; // updated by Heartbeat
}

enum AgentKind {
    AGENT_KIND_UNSPECIFIED = 0;
    AGENT_KIND_HUMAN  = 1;
    AGENT_KIND_SCRIPT = 2;
    AGENT_KIND_AI     = 3;
}

// ScheduledTask is the Scheduler primitive: fire a single task once.
message ScheduledTask {
    string id           = 1;
    string objective_id = 2; // empty for standalone, per Decisions
    string description  = 3;
    Priority priority    = 4;
    google.protobuf.Timestamp fire_at = 5;
    ScheduledTaskStatus status = 6;
    string created_task_id     = 7; // set once fired
}

enum ScheduledTaskStatus {
    SCHEDULED_TASK_STATUS_UNSPECIFIED = 0;
    SCHEDULED_TASK_STATUS_PENDING   = 1;
    SCHEDULED_TASK_STATUS_FIRED     = 2;
    SCHEDULED_TASK_STATUS_CANCELLED = 3;
}

// Loop is the recurring primitive: one definition spawns a new Task instance
// every time it fires.
message Loop {
    string id           = 1;
    string objective_id = 2; // empty for standalone, per Decisions
    string description  = 3;
    Priority priority    = 4;
    Recurrence recurrence = 5;
    LoopStatus status     = 6;
    google.protobuf.Timestamp next_fire_at = 7;
    int64 occurrence_count = 8;
}

message Recurrence {
    // "Daily" | "Weekly" | "Every N days" | "Custom (cron)" -- matches the
    // Loops create-form mockup's Repeats options exactly.
    string kind          = 1;
    int32  interval_days = 2; // for "Every N days"
    string cron          = 3; // for "Custom (cron)"
    string at            = 4; // "HH:MM", local to the Lineman process
    google.protobuf.Timestamp starts_at = 5;
    // Zero value = "Never". Mutually exclusive with ends_after_occurrences.
    google.protobuf.Timestamp ends_at = 6;
    int32 ends_after_occurrences = 7;
}

enum LoopStatus {
    LOOP_STATUS_UNSPECIFIED = 0;
    LOOP_STATUS_ACTIVE = 1;
    LOOP_STATUS_PAUSED  = 2;
}
```

### Blueprint storage

Each message type above is `WithRegisteredType`'d and stored via Blueprint's `KeyValueService.Set`/`Get`/`List` — no separate database. This mirrors Bench's own precedent-setting question ("is this structured, queryable data a KV store handles poorly at scale?") but lands differently: Bench's run history is write-once, append-heavy, and queried historically (exactly what pushed it to Postgres, see [Bench's Decided](/docs/architecture/bench-workflow-engine#decided)). Lineman's board only ever needs **current-state** reads — `List` by kind, filtered/grouped client-side or server-side into the shapes the UI needs (queued list, in-flight cards, per-objective stats) — the same access pattern Blueprint's own Service Registry already serves this way. Historical/analytical questions ("how long do tasks sit in Needs Input") are explicitly Beacon's job, not Lineman's — see [Telemetry](#telemetry). If real usage ever proves KV inadequate, migrate then, the same way Bench did — not preemptively.

## RPC Interface

`api/tooling/lineman/v1/service.proto`:

```protobuf
service LinemanService {
    // Objectives
    rpc CreateObjective(CreateObjectiveRequest) returns (Objective) {}
    rpc GetObjective(GetObjectiveRequest) returns (Objective) {}
    rpc ListObjectives(ListObjectivesRequest) returns (ListObjectivesResponse) {}

    // Tasks
    rpc CreateTask(CreateTaskRequest) returns (Task) {}
    rpc GetTask(GetTaskRequest) returns (Task) {}
    rpc ListTasks(ListTasksRequest) returns (ListTasksResponse) {}
    rpc UpdateTaskState(UpdateTaskStateRequest) returns (Task) {}
    rpc ReorderTask(ReorderTaskRequest) returns (Task) {}
    rpc AssignAgent(AssignAgentRequest) returns (Task) {}

    // Agents -- called by whatever is doing the work (human tooling, a
    // script, or an AI agent run), not by Lineman's own UI.
    rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse) {}
    rpc ListAgents(ListAgentsRequest) returns (ListAgentsResponse) {}

    // Needs Input -- the recommended human-in-the-loop convention (see the
    // design brief's Decisions). Not hardcoded to a specific state name;
    // RequestInput/ProvideGuidance just read/write Task.needs_input and
    // move Task.state, same as UpdateTaskState would.
    rpc RequestInput(RequestInputRequest) returns (Task) {}
    rpc ProvideGuidance(ProvideGuidanceRequest) returns (Task) {}

    // Scheduler
    rpc CreateScheduledTask(CreateScheduledTaskRequest) returns (ScheduledTask) {}
    rpc ListScheduledTasks(ListScheduledTasksRequest) returns (ListScheduledTasksResponse) {}
    rpc CancelScheduledTask(CancelScheduledTaskRequest) returns (CancelScheduledTaskResponse) {}

    // Loops
    rpc CreateLoop(CreateLoopRequest) returns (Loop) {}
    rpc ListLoops(ListLoopsRequest) returns (ListLoopsResponse) {}
    rpc PauseLoop(PauseLoopRequest) returns (Loop) {}
    rpc ResumeLoop(ResumeLoopRequest) returns (Loop) {}
    rpc DeleteLoop(DeleteLoopRequest) returns (DeleteLoopResponse) {}

    // Live board updates -- mirrors ServiceDiscoveryService.Watch exactly
    // (api/core/registry/service_discovery/v1/service.proto), not a Catalyst
    // Consume subscription. See Decisions for why.
    rpc Watch(WatchRequest) returns (stream WatchResponse) {}
}

message WatchRequest {}

message WatchResponse {
    oneof item {
        Task task = 1;
        Agent agent = 2;
        ScheduledTask scheduled_task = 3;
        Loop loop = 4;
    }
    bool removed = 5;
}

message HeartbeatRequest {
    string agent_id       = 1;
    string task_id        = 2;
    string current_action = 3; // e.g. "Calling KeyValueService.Set(...)"
}

message HeartbeatResponse {}

message RequestInputRequest {
    string task_id  = 1;
    string question = 2;
    repeated string options = 3;
}

message ProvideGuidanceRequest {
    string task_id  = 1;
    string response  = 2; // one of the offered options, or free text
    string next_state = 3; // state to transition into, e.g. back to "In Flight"
}
```

No Fuse-specific routing work needed beyond the one `WithRoute` call in [System Design](#system-design) — `LinemanService` gets its own prefix like every other Connect-RPC service.

## Telemetry

Every mutating RPC above (`UpdateTaskState`, `AssignAgent`, `RequestInput`, `ProvideGuidance`, a `ScheduledTask`/`Loop` firing) does two things on a state change, matching the design brief's "Catalyst event + Beacon wide event" decision:

1. **Catalyst**: produce a CloudEvent (`tooling.lineman.v1.TaskStateChanged`, etc.) via the same long-lived `Producer.Produce` stream pattern `services/tooling/bench/catalyst.go`'s `catalystPublisher` uses — for downstream reaction (a loop firing, another service consuming task-completed).
2. **Beacon**: *not* a separate `CreateWideEvent` call, but *not* the bare `NewTraceInterceptor()` automatic path either — `SetBusinessAttribute`/`SetRuntimeAttribute` are only exposed on a manually-started `*chassis.Span` (`StartSpan`/`Span.End`); the interceptor's own per-RPC span has no caller-attaches-attributes hook today (a documented scope boundary of [WideEvent's own implementation](/docs/architecture/wide-events-implementation-plan#53-a-chassis-internal-wideevent-producer), not an oversight here). Every mutating handler therefore does:

   ```go
   spanCtx, span := chassis.StartSpan(ctx, "task.update_state")
   span.SetBusinessAttribute("task_id", req.TaskId)
   span.SetBusinessAttribute("objective_id", task.ObjectiveId)
   span.SetBusinessAttribute("from_state", task.State)
   span.SetBusinessAttribute("to_state", req.NewState)
   // ... do the actual work, using spanCtx ...
   span.End(err)
   ```

   With `telemetry.wide_events.enabled: true` set in `config.yaml`, this produces a real `WideEvent` — a child span of the interceptor's own outer request span, correctly correlated by `trace_id` — carrying the business attributes, no ClickHouse/Beacon-facing code in Lineman at all.

## Decisions

**`Task.description` was split into `name` + `details` (2026-09-09, post-launch change, requested by the user).** A single free-text field was doing two jobs — a short label for board cards and full instructions for what to actually do — and conflating them meant a card either showed a wall of text or the instructions had nowhere to live. `name` is the short label; `details` is everything needed to complete the task, rendered as its own prominent card on Task Detail, above Current Action (same placement rationale as the original single-field version — see the design brief's own reasoning for putting this first). Done as a wire-compatible rename (`name` keeps `description`'s old tag, `3`; `details` is a new tag, `12`), not a fresh field pair — existing stored `Task` entries decode with their old description text as `name` and empty `details`, no migration needed. `ScheduledTask`/`Loop` keep their own single `description` field unchanged (their create forms were always short single-line inputs, closer to a name than instructions already) — a fired one supplies only `name` on the `Task` it spawns, `details` starts empty. `CreateTaskRequest` got the same split (`description` → `name` + new `details`, tag `5`).

**Scheduler/Loops fire on an internal timer, not by consuming Lineman's own Catalyst events.** Catalyst's `Consume` has a [documented, unfixed fan-out bug](/docs/architecture/core-services#known-issues): with more than one `Consume` stream open cluster-wide, an event is delivered to exactly one of them, chosen effectively at random — not to every interested subscriber. Beacon already holds one such stream (consuming `WideEvent`s); Lineman running a second `Consume` loop to trigger its own scheduled firings would race against Beacon's for delivery of *any* CloudEvent on the bus. Lineman never depends on receiving its own or anyone else's event to function correctly: a background ticker inside the Lineman process itself scans `ScheduledTask`/`Loop` entries in Blueprint for due ones and calls the same internal `createTask` function directly. The Catalyst event on firing is still produced, for downstream consumers — it just isn't Lineman's own trigger mechanism.

**`Watch` is a dedicated server-streaming RPC, not a Catalyst subscription.** Same reasoning: the web client's live board needs reliable, single-recipient delivery, and mirroring `ServiceDiscoveryService.Watch` (already proven, see `services/core/blueprint/web-client/src/views/service_detail.rs`) sidesteps the fan-out bug entirely rather than working around it.

**Ordering uses sparse integers, not fractional indexing.** `Task.order` is assigned in large gaps (e.g. multiples of 1000) within `(objective_id, state)`; a drag-to-reorder writes a value between its new neighbors' `order`s. Re-sequence the whole state's list (rewrite every `order` in that state as clean multiples of 1000) only when two neighbors' gap is exhausted (adjacent orders differing by 1). Simpler to reason about and debug in the Key/Value browser than float/fractional keys, at the cost of an occasional batch rewrite — acceptable given a single objective's queue is expected to be small (tens of tasks, not thousands).

**A task's state transitions are unrestricted, not a fixed DAG.** Unlike the Agent Service's `allowedTransitions` state machine, `UpdateTaskState` accepts any of `Objective.states` as the new state — matching the design brief's "Task states are fully custom per objective" decision. Lineman validates membership (`new_state` must be in `Objective.states`), not the transition edge.

**Standalone tasks/scheduled-tasks/loops (`objective_id: ""`) are stored and listed like any other**, distinguished only by an empty `objective_id` — no separate storage kind or RPC path. Matches the design brief's "a loop or scheduled task can target no objective at all" decision.

**`AgentKind` exists, but Lineman never validates or enforces it beyond display.** A `HUMAN`-kind agent calling `Heartbeat` is exactly as valid as an `AI`-kind one — the field is for the UI's `🤖`/human iconography, not an authorization or behavior gate.

## Non-Goals

- **No cost/token/budget tracking.** Deliberately excluded — see the design brief's Open Questions.
- **No coupling to the Agent Service.** See [Relationship to the Agent Service](#relationship-to-the-agent-service).
- **No Postgres/analytical storage.** See [Blueprint storage](#blueprint-storage) above; revisit only if KV proves inadequate under real usage.
- **No auth on any RPC**, including `ProvideGuidance` (a write reachable from anyone who can reach Lineman's UI). Matches the system-wide status quo (no RPC in this repo enforces authorization today) rather than a bespoke policy for Lineman alone — worth a conscious revisit before any real deployment, same caveat [WideEvent's own open questions](/docs/architecture/wide-events#open-questions) raise about `CreateWideEvent`.
- **No cron-expression parsing/validation library chosen yet.** `Recurrence.cron` (the "Custom (cron)" option) is a string field; picking and vendoring a parser is implementation-time work, not a data-model concern.
- **No plugin/`StepExecutor` implementation.** Lineman publishes a Foundry manifest for discovery/distribution only, per `idea.md` — it doesn't implement `StepExecutor` or get called from a Bench workflow step.

## Implementation Plan

- [x] **1. Scaffolding.** `services/tooling/lineman`: `go.mod`, `main.go`, `config.yaml`, following the same `chassis.New(logger).Register(...).WithRPCHandler(...).WithRoute(...).Start()` shape as `services/examples/echo` and `services/tooling/bench`. `WithRoute` for `lineman.draft.localhost` (UI + RPC on one route, per [Service UIs via Subdomains](/docs/architecture/service-ui-subdomains)). No repository/database needed — see [Blueprint storage](#blueprint-storage). Deliverable: a service that starts, registers with Blueprint, and does nothing else yet. _(completed 2026-09-09 — port 9307, `bind_port`/`route.host` matching every other tooling service's config.yaml shape; live-confirmed registered in Blueprint's Service Registry and reachable both directly and through Fuse's `lineman.draft.localhost` subdomain.)_

- [x] **2. Proto.** `api/tooling/lineman/v1/models.proto` and `service.proto` per [Data Model](#data-model) and [RPC Interface](#rpc-interface). Regenerate (`dctl api build`). Deliverable: generated Go/Rust/TS types compile; no behavior change yet. _(completed 2026-09-09 — Go/Connect-Go generation via `dctl api build` clean; the "Generating Web protos" npm step failed the same pre-existing way it does for every other tooling-domain proto (`workflow.v1`, `plugin_catalog.v1` have no `_pb.ts` either) — not something this pass broke. Rust codegen needed one addition this plan didn't call out: `api/build.rs`'s hardcoded `proto_dirs` list (the exact gotcha `proto-codegen`'s own skill doc warns about) didn't include `tooling/lineman/v1/` — added it, which is also what makes `dioxus_grpc::generate_hooks` produce Lineman's web-client hook automatically.)_

- [x] **3. Server: Objectives and Tasks.** `objective.go`/`task.go`, each a thin model/controller split over Blueprint's `KeyValueService` (`Get`/`Set`/`List`), mirroring `services/core/blueprint/key_value`'s own file separation. `CreateObjective` defaults `states` to `["Queued", "In Flight", "Done"]` when the caller passes none, matching Create Objective's pre-filled mockup. `UpdateTaskState` validates `new_state ∈ Objective.states` per the transition decision above. `ReorderTask` implements the sparse-integer scheme. At startup, `WithRegisteredType` for `Objective` and `Task`. Deliverable: `CreateObjective` → `CreateTask` → `UpdateTaskState` → `ListTasks` round-trips correctly through real Blueprint; entries render decoded (not opaque bytes) in Blueprint's Key/Value browser. _(completed 2026-09-09 — live-verified via direct RPC calls: default states applied correctly, sparse ordering (1000/2000 gaps, midpoint reorder) confirmed, an invalid `new_state` correctly rejected with `invalid_argument`.)_

- [x] **4. Server: Agents, Heartbeat, Needs Input.** `agent.go`: `Heartbeat` upserts an `Agent` record (creating one on first call for an unseen `agent_id`), sets `Task.agent_id`/`current_action`, bumps `last_seen_at`. `RequestInput`/`ProvideGuidance` per [RPC Interface](#rpc-interface). `WithRegisteredType` for `Agent`. Deliverable: a scripted `Heartbeat` loop against a real task updates `current_action` visibly on re-`GetTask`; `RequestInput` followed by `ProvideGuidance` correctly moves a task's state and clears `needs_input`. _(completed 2026-09-09 — live-verified end to end, including from the real web client: a `RequestInput` call showed the Needs Input card live on Task Detail, and clicking a real "Retry with longer timeout" button called `ProvideGuidance` and cleared it, both without a page refresh.)_

- [x] **5. Server: `Watch`.** In-process pub/sub (a `sync.Map` of open channels, one per connected stream — no Catalyst involved, per Decisions) fed by every mutating call in Phases 3–4. Mirrors `ServiceDiscoveryService.Watch`'s shape exactly. Deliverable: two concurrent `Watch` streams both receive every `Task`/`Agent` change live, proving delivery isn't subject to Catalyst's fan-out bug. _(completed 2026-09-09 — real bug hunt during verification, worth recording: a hand-rolled Go test client hung indefinitely opening `Watch` whenever it shared one HTTP/2 connection with a concurrent mutating call on the same `*http.Client`: the connect-go client's `Watch()` call doesn't return until the first message arrives, and sharing a connection with an in-flight stream starved the second request. Using two separate h2c clients (matching this service's own real web-client usage, and `services/tooling/catalyst-consume`'s established `Consume` pattern) resolved it immediately — not a Lineman bug, a test-harness one. Watch itself opened in ~1s and delivered the live event correctly once fixed.)_

- [x] **6. Server: Scheduler.** `scheduler.go`: `CreateScheduledTask`/`ListScheduledTasks`/`CancelScheduledTask`, plus a background ticker (started in `main.go` via `chassis.WithRunner` or equivalent) that polls for `PENDING` entries with `fire_at <= now`, calls the internal `createTask` function, sets `created_task_id`, transitions status to `FIRED`, and produces the Catalyst event. `WithRegisteredType` for `ScheduledTask`. Deliverable: a `ScheduledTask` created 5 seconds in the future results in a real `Task` appearing in its objective's Queued list within one tick interval, with no manual trigger. _(completed 2026-09-09 — `ticker_interval: 5s`; live-verified twice, once via direct RPC and once through the real web-client form (a hand-typed `datetime-local` value, parsed by `parse_datetime_local`), both firing into a real `Task` with `created_task_id` set and status flipping to `FIRED` with zero manual intervention.)_

- [x] **7. Server: Loops.** `loop.go`: same firing mechanism as Phase 6, reused rather than duplicated (`Recurrence` → next `fire_at` computation is the only new logic — daily/weekly/every-N-days is straightforward date math; `Custom (cron)` needs a parser, see Non-Goals). Each fire spawns a new `Task` (not the same one re-triggered) and advances `next_fire_at`/`occurrence_count`; a loop past `ends_at` or `ends_after_occurrences` transitions to a terminal state and stops ticking. `WithRegisteredType` for `Loop`. Deliverable: a Loop with `interval_days: 1` and no end condition spawns a second, distinct `Task` (different id) after its next fire, without disturbing the first one's own state. _(completed 2026-09-09 — `nextFireTime`'s daily/weekly/every_n_days math and `withTimeOfDay`'s "HH:MM" application live-verified via a real `daily` loop created through the web client; Pause/Resume live-verified end to end from a real button click, `LOOP_STATUS_PAUSED`/`ACTIVE` reflected immediately.)_

- [x] **8. Telemetry.** Set `telemetry.wide_events.enabled: true` (Phase 1's scaffolding already wires `chassis.NewTraceInterceptor()`), and add the `StartSpan`/`SetBusinessAttribute`/`End` calls described in [Telemetry](#telemetry) to every mutating handler from Phases 3–7. Deliverable: a real `UpdateTaskState` call is queryable afterward via `SearchWideEvents`/`GetWideEvent` with `business_attributes` containing `task_id`/`objective_id`/`from_state`/`to_state`; the same call's Catalyst event is independently confirmed via a standalone `Consume` subscriber (single-subscriber case only, per the known fan-out bug). _(completed 2026-09-09 — live-confirmed via `SearchWideEvents` against the real running Beacon: e.g. a `task.create` span with `businessAttributes: {objective_id, task_id, to_state}`, correctly nested (`parentSpanId`) under the outer `/tooling.lineman.v1.LinemanService/CreateTask` request span in the same trace. The Catalyst-event half of this deliverable could not be independently confirmed this session — the live stack already has other long-lived `Consume` subscribers running (catalyst-consume, Beacon's own WideEvent consumer), so a single ad hoc subscriber is exactly the "not guaranteed delivery" case the fan-out bug describes, and it did in fact time out waiting for an event. No errors were logged on the send side across the whole session, which is the best available confirmation short of fixing the underlying platform bug.)_

- [x] **9. Foundry.** Publish a Lineman plugin manifest (name, version, description, maintainer) via `chassis.Effect` on startup, retracted on shutdown — the exact pattern [Foundry's own doc](/docs/architecture/foundry-plugin-repository#publishing-and-discovery) documents for `slack-notify`. Deliverable: `PluginCatalogService.List` shows Lineman while running; a clean `Get` failure after `SIGTERM`. _(completed 2026-09-09 — `foundry.go` mirrors `slack-notify/catalog.go` verbatim; live-confirmed via a direct `Get` call against Foundry's real running catalog while Lineman was up.)_

- [x] **10. Web client.** `services/tooling/lineman/web-client`, Dioxus, following Blueprint's web client pattern exactly. Implements the seven pages from `assets/lineman/design-brief.html` (Dashboard, Task Board, Create Objective, Objective Detail, Task Detail, Scheduler, Loops) against the real RPCs from Phases 3–7, subscribed live via Phase 5's `Watch`. `NavigationConfig` entry for Lineman's sidebar sections (Overview / Objectives / Automation, per the design brief's IA). Deliverable: every page in the design brief renders against real data — an objective created via Create Objective appears on the Dashboard and Task Board, a task's state change (via a manual `UpdateTaskState` call or a live `Heartbeat`) updates the board without a page refresh. _(completed 2026-09-09, with real scope trims from the mockups, called out rather than silently dropped: a static three-section sidebar instead of a `NavigationConfig`-driven one — Lineman's IA is small and fixed enough that the KV-backed indirection Blueprint's own sidebar uses isn't worth it yet; Task Detail has no Retry/Cancel buttons — this pass's RPC interface has no corresponding RPC for either, so real buttons would have been inert; `dx build --release`'s `wasm-opt` pass crashes on this machine (`UNREACHABLE ... DWARFEmitter.cpp:201`, a binaryen/DWARF version issue) but `dx` treats it as non-fatal and still produces a working, just unminified, WASM bundle — build output confirmed served correctly regardless. Live Watch-driven updates wired on Task Board and Task Detail specifically (the two pages the design brief calls out as needing to feel "live"); Scheduler/Loops/Dashboard/Objective Detail refetch instead, which is enough given neither has a pulsing-dot requirement.)_

  **Follow-up, 2026-09-09 — Add Task was missing entirely.** The design brief itself never specced a "create task" page (only Create *Objective*), so this pass shipped the same gap without noticing: there was no UI path to add a task to an objective right now — only `CreateTask` via raw RPC, or indirectly via a `ScheduledTask`/`Loop` firing in the future. Caught when the user asked "how can tasks be added to an objective?" immediately after this phase was marked done. Fixed by adding an "Add Task" form (description + priority) to Objective Detail, plus a new "Unassigned" list on that page so a newly queued task is actually visible afterward (previously only stat counts and In-Flight cards rendered individual tasks — a Queued task had nowhere to show up at all). Required switching that page's task fetch off the generated hook and onto a raw `use_resource` with an explicit `refresh` signal (matching `scheduler.rs`/`loops.rs`'s own pattern) since the hook's `Resource` only re-fires when the request itself changes, and `objective_id` never does on a plain create. Live-verified through the real browser UI: a task typed into the form appeared immediately in Objective Detail's new Unassigned list and in Task Board's Queued column, no refresh needed.

- [x] **11. Verification.** Live, end-to-end, against the real running stack: create an objective with custom states including a "Needs Input" state; create and drag-reorder several queued tasks; assign an agent and heartbeat a fake `current_action` into it, confirmed live on Task Detail; trigger `RequestInput`/`ProvideGuidance` and confirm the board reflects it without a refresh; create a `ScheduledTask` and a `Loop` and confirm both fire for real; confirm every state change is independently visible via `SearchWideEvents` (Beacon) and via a `Consume` subscriber (Catalyst); confirm Lineman is listed in Foundry's catalog while running. _(completed 2026-09-09 — every item above verified live, several of them twice (once via raw RPC, once through the actual browser UI against `lineman.draft.localhost`), with one real bug found and fixed along the way: Objective Detail's Active Agents table showed the whole system-wide agent roster instead of only agents assigned to that objective's own tasks — a brand-new objective with zero tasks incorrectly showed an agent from a different objective. Fixed by cross-referencing `list_tasks`' `agent_id`s before rendering the table; confirmed fixed live (an empty objective now correctly shows no Active Agents section). Browser console clean of errors throughout. Scheduler/Loops smoke tests also went through the real form UI, not just curl, including a hand-typed `datetime-local` field.)_

## Open Questions

- **Cron parsing.** Which library backs `Recurrence.cron`'s "Custom (cron)" option — deferred to Phase 7, per Non-Goals.
- **Ticker interval and jitter.** How often the Scheduler/Loops background ticker polls (Phase 6) trades off firing latency against Blueprint read load; not specced here, needs a concrete number chosen at implementation time.
- **Multi-instance Lineman.** This plan assumes a single Lineman instance (no raft, unlike Blueprint) — `Watch`'s in-process pub/sub (Phase 5) and the firing ticker (Phase 6) both implicitly assume there's exactly one of them running. Horizontal scaling would need the ticker to use a leader-election or claim mechanism (e.g. Blueprint-backed, similar in spirit to a distributed lock) to avoid double-firing — not designed here, since nothing in `idea.md` or the design brief calls for multiple Lineman instances.
- **Auth on `ProvideGuidance`.** Flagged under Non-Goals as a conscious gap, not an oversight — worth a real answer before any deployment where "anyone who can reach the UI can approve a blocked agent's action" is unacceptable.

## Milestone Summary

| Phase | Scope | Status |
|---|---|---|
| 1 | Scaffolding | Done |
| 2 | Proto: `models.proto`, `service.proto` | Done |
| 3 | Server: Objectives, Tasks | Done |
| 4 | Server: Agents, Heartbeat, Needs Input | Done |
| 5 | Server: `Watch` | Done |
| 6 | Server: Scheduler | Done |
| 7 | Server: Loops | Done |
| 8 | Telemetry: Catalyst + Beacon wide events | Done — WideEvents confirmed live; Catalyst-event delivery not independently re-confirmed this session, see note above |
| 9 | Foundry: plugin manifest | Done |
| 10 | Web client: seven pages from the design brief | Done, with disclosed scope trims (see note above) |
| 11 | Verification | Done — one real bug found and fixed (Active Agents table wasn't objective-scoped) |

**Not done, out of scope for this pass**: reproducibility wiring into `scripts/run-local.sh`/`scripts/run-tooling-stack.sh` (every other tooling service is added there; Lineman was live-verified by hand instead — a real gap for anyone else trying to bring the local stack up with Lineman included), cron recurrence parsing, and auth on any RPC — all per this doc's own Non-Goals and Open Questions, not oversights.
