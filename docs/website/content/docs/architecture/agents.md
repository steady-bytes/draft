---
weight: 10
title: Agent Service
description: An overview of the agent orchestration service and its integration with the Draft cluster
icon: smart_toy
draft: false
toc: true
---

The Agent Service is a Draft domain that provides infrastructure for orchestrating AI agents at scale. It is composed of three cooperating processes — an **Orchestrator**, an **Operator**, and one or more **Executors** — that communicate exclusively through [Catalyst](/docs/architecture/core-services#catalyst) using a typed event system built on top of CloudEvents.

The Agent Service is designed around four principles:

- **Event sourcing** — every meaningful state change is recorded as an immutable event, giving a full audit trail and the ability to replay or resume any objective
- **Separation of concerns** — the Orchestrator manages state and the external API; the Operator manages task graph planning and DAG execution; the Executor manages LLM calls and tool use; none does another's job
- **Draft-native** — all three processes are standard Draft processes registered with Blueprint, routing through Fuse, and communicating through Catalyst
- **Durable by default** — objectives survive crashes and can resume from their last checkpoint without replaying from the beginning

See below for an overview of the agent service within a Draft cluster.

```
┌──────────────────────────────────────────────────────────────────────┐
│                           Draft Cluster                              │
│                                                                      │
│   ┌───────────┐      ┌──────────┐      ┌──────────────────────────┐  │
│   │ Blueprint │      │   Fuse   │      │        Catalyst          │  │
│   │ (registry)│      │ (routing)│      │        (events)          │  │
│   └───────────┘      └──────────┘      └────────────┬─────────────┘  │
│                                                     │                │
│         ┌───────────────────────────────────────────┤                │
│         │                       │                   │                │
│  ┌──────▼───────┐      ┌────────▼──────┐      ┌─────▼────────────┐   │
│  │ Orchestrator │      │   Operator    │      │   Executor(s)    │   │
│  │              │      │               │      │                  │   │
│  │ - Objective  │      │ - Task graph  │      │ - LLM calls      │   │
│  │   state      │      │ - DAG routing │      │ - Tool execution │   │
│  │ - API layer  │      │ - Planning    │      │ - Working memory  │   │
│  │ - Snapshots  │      │ - Dispatch    │      │                  │   │
│  │ - Event log  │      │               │      │                  │   │
│  └──────────────┘      └───────────────┘      └──────────────────┘   │
└──────────────────────────────────────────────────────────────────────┘
```

---

## Orchestrator

[#orchestrator](#orchestrator)

The Orchestrator is the control plane of the agent service. It owns the lifecycle of every objective and is the single source of truth for objective state. It does not plan task graphs, call LLMs, or execute tools — it accepts objectives from callers, records all events, and keeps queryable state current.

Its responsibilities are:

- Accepting new objectives via its `OrchestratorService` RPC
- Maintaining an **objective snapshot** — a materialized view of the current objective state, queryable without replaying the full event log
- Maintaining the **event log** — the append-only record of every `AgentEvent` emitted during an objective, backed by Catalyst's ClickHouse store
- Enforcing the **state machine** — only legal objective status transitions are permitted; illegal transitions are rejected before any event is emitted
- Surfacing **human-in-the-loop** decisions — when a task requires approval the Orchestrator transitions the objective to `paused` and exposes an `ApproveTask` RPC for external actors

### State Machine

An objective moves through the following states. Terminal states (`completed`, `failed`, `cancelled`) accept no further transitions.

```
created ──▶ pending ──▶ running ──▶ completed
                           │
                           ├──▶ paused ──▶ running
                           ├──▶ failed
                           └──▶ cancelled
```

### Objective Snapshot

The Orchestrator maintains a flat, queryable snapshot of each objective updated after every event is processed. This allows the current state of any objective to be read instantly without replaying the event log. The snapshot tracks identity, status, task graph progress, budget consumption, and timestamps.

When an objective must be resumed after a crash, the Orchestrator loads the saved snapshot, then replays only the events that occurred after the snapshot's `last_event_sequence` cursor — not the full history.

---

## Operator

[#operator](#operator)

The Operator is the planning and execution engine of the agent service. It receives an objective from Catalyst, decomposes it into a `TaskGraph`, and drives that graph to completion by dispatching tasks to Executors and reacting to their results.

Its responsibilities are:

- Subscribing to `AGENT_EVENT_TYPE_OBJECTIVE_STARTED` events on Catalyst
- Calling the planner (LLM or rule-based) to decompose the objective into a `TaskGraph` — a directed acyclic graph of `Task` nodes with explicit dependency edges
- Emitting `AGENT_EVENT_TYPE_TASK_GRAPH_CREATED` and one `AGENT_EVENT_TYPE_TASK_CREATED` event per task
- Identifying all tasks with no unmet dependencies (in-degree zero) and dispatching them in parallel via `AGENT_EVENT_TYPE_TASK_ASSIGNED`
- Subscribing to `AGENT_EVENT_TYPE_TASK_COMPLETED` and `AGENT_EVENT_TYPE_TASK_FAILED` events
- On each `TASK_COMPLETED`: decrement the in-degree of dependent tasks; dispatch any task whose in-degree reaches zero
- On `TASK_FAILED`: apply the configured failure policy (retry, skip, or fail the objective)
- Emitting `AGENT_EVENT_TYPE_OBJECTIVE_COMPLETED` or `AGENT_EVENT_TYPE_OBJECTIVE_FAILED` when the graph reaches a terminal state

### DAG Execution Loop

```
OBJECTIVE_STARTED received
         │
    Build TaskGraph via planner
         │
    Emit TASK_GRAPH_CREATED + TASK_CREATED (×N)
         │
    Find all tasks with in-degree = 0
         │
    Emit TASK_ASSIGNED in parallel ──────────────────┐
                                                      │
    TASK_COMPLETED received                           │
         │                                            │
    Mark task done, store result                      │
    Decrement in-degree of dependents                 │
    Any dependent reaches 0? → emit TASK_ASSIGNED ───┘
         │
    No tasks remaining → emit OBJECTIVE_COMPLETED
```

The Operator is stateless between events. All durable task graph state is persisted in PostgreSQL so any Operator instance can pick up after a crash by reading the stored graph and finding tasks that are ready but unassigned.

---

## Executor

[#executor](#executor)

The Executor is the data plane of the agent service. It receives task assignments from Catalyst, runs the agent logic (LLM calls, tool use, scratchpad updates), and publishes results back as `AgentEvent`s.

Its responsibilities are:

- Subscribing to `AGENT_EVENT_TYPE_TASK_ASSIGNED` events on Catalyst, load-balanced across all running Executor instances via a consumer group
- Loading working memory for the task at the start of execution
- Executing the agent: constructing the prompt, calling the LLM, parsing tool calls, and running tools across one or more internal steps
- Emitting `AGENT_EVENT_TYPE_STEP_STARTED`, `AGENT_EVENT_TYPE_STEP_COMPLETED`, `AGENT_EVENT_TYPE_TOOL_CALLED`, and `AGENT_EVENT_TYPE_TOOL_RETURNED` for each iteration
- Publishing `AGENT_EVENT_TYPE_TASK_COMPLETED` or `AGENT_EVENT_TYPE_TASK_FAILED` when the task finishes
- Committing updated working memory after each step

Executors are stateless between tasks. All durable state lives in the Orchestrator and Operator. Multiple Executor instances can run simultaneously, each picking up tasks from the Catalyst consumer group, giving horizontal scalability at the execution layer.

---

## Agent Events

[#agent-events](#agent-events)

All communication between the three services flows through `AgentEvent` messages published on Catalyst. An `AgentEvent` is a typed, versioned proto message carried inside a standard Catalyst `CloudEvent`.

### Envelope

Every `AgentEvent` shares a common envelope defined in `api/agent/v1/events.proto`:

```protobuf
message AgentEvent {
  string         id           = 1;  // unique event ID
  AgentEventType type         = 2;  // typed enum, see below
  string         objective_id = 3;  // which objective this event belongs to
  string         task_id      = 4;  // which task, if applicable
  int64          sequence     = 5;  // monotonic per-objective ordering
  google.protobuf.Timestamp occurred_at = 6;

  oneof payload { ... }  // typed payload, one per event type
}
```

The `sequence` field provides a monotonic per-objective cursor used by the snapshot to know which events have already been applied, enabling safe incremental replay.

### CloudEvent Mapping

`AgentEvent`s are packed into Catalyst `CloudEvent`s using the following field mapping:

| CloudEvent field | Value | Purpose |
|---|---|---|
| `id` | `AgentEvent.id` | Unique event identity |
| `source` | `agent.AgentEvent/{objective_id}` | Enables per-objective subscription and prefix filtering |
| `type` | `AgentEventType` enum name | Enables event-type routing without deserializing payload |
| `binary_data` | proto-marshaled `AgentEvent` bytes | Full typed event, deserialized by consumers |

The `source` format `agent.AgentEvent/{objective_id}` is intentional. Catalyst consumers can filter at two levels: prefix-match on `agent.AgentEvent/` to receive all agent events across all objectives, or exact-match on `agent.AgentEvent/{objective_id}` to receive events for a specific objective only.

```go
// Subscribe to all events for a specific objective
broker.Subscribe(ctx, chassis.SubscribeOptions{
    Event: &acv1.CloudEvent{
        Type:   agentv1.AgentEventType_AGENT_EVENT_TYPE_OBJECTIVE_STARTED.String(),
        Source: "agent.AgentEvent/" + objectiveID,
    },
    Consumer: orchestrator,
})

// Subscribe to task assignments across all objectives, load-balanced
broker.Subscribe(ctx, chassis.SubscribeOptions{
    Event: &acv1.CloudEvent{Type: agentv1.AgentEventType_AGENT_EVENT_TYPE_TASK_ASSIGNED.String()},
    Group: "agent-executors",
    Consumer: executor,
})
```

### Event Types

`AgentEventType` is a proto enum. Adding a new event type requires a recompile, which is intentional — it ensures all consumers are aware of new event types at compile time.

```protobuf
enum AgentEventType {
  AGENT_EVENT_TYPE_UNSPECIFIED = 0;

  // Objective lifecycle (10–19)
  AGENT_EVENT_TYPE_OBJECTIVE_CREATED   = 10;
  AGENT_EVENT_TYPE_OBJECTIVE_STARTED   = 11;
  AGENT_EVENT_TYPE_OBJECTIVE_PAUSED    = 12;
  AGENT_EVENT_TYPE_OBJECTIVE_RESUMED   = 13;
  AGENT_EVENT_TYPE_OBJECTIVE_CANCELLED = 14;
  AGENT_EVENT_TYPE_OBJECTIVE_COMPLETED = 15;
  AGENT_EVENT_TYPE_OBJECTIVE_FAILED    = 16;

  // Task graph (20–29)
  AGENT_EVENT_TYPE_TASK_GRAPH_CREATED = 20;
  AGENT_EVENT_TYPE_TASK_GRAPH_UPDATED = 21;  // replanning

  // Task lifecycle (30–39)
  AGENT_EVENT_TYPE_TASK_CREATED   = 30;
  AGENT_EVENT_TYPE_TASK_ASSIGNED  = 31;
  AGENT_EVENT_TYPE_TASK_STARTED   = 32;
  AGENT_EVENT_TYPE_TASK_COMPLETED = 33;
  AGENT_EVENT_TYPE_TASK_FAILED    = 34;
  AGENT_EVENT_TYPE_TASK_RETRYING  = 35;
  AGENT_EVENT_TYPE_TASK_BLOCKED   = 36;
  AGENT_EVENT_TYPE_TASK_UNBLOCKED = 37;

  // Step lifecycle (40–49)
  AGENT_EVENT_TYPE_STEP_STARTED   = 40;
  AGENT_EVENT_TYPE_STEP_COMPLETED = 41;
  AGENT_EVENT_TYPE_STEP_FAILED    = 42;
  AGENT_EVENT_TYPE_STEP_RETRYING  = 43;

  // Tool calls (50–59)
  AGENT_EVENT_TYPE_TOOL_CALLED   = 50;
  AGENT_EVENT_TYPE_TOOL_RETURNED = 51;
  AGENT_EVENT_TYPE_TOOL_FAILED   = 52;

  // Human-in-the-loop (60–69)
  AGENT_EVENT_TYPE_APPROVAL_REQUESTED = 60;
  AGENT_EVENT_TYPE_APPROVAL_RECEIVED  = 61;

  // Budget / safety (70–79)
  AGENT_EVENT_TYPE_BUDGET_WARNING  = 70;
  AGENT_EVENT_TYPE_BUDGET_EXCEEDED = 71;
}
```

Event type numbers are grouped by category with gaps between groups, leaving room to add new types within a category without breaking ordering.

### Event Groups

**Objective lifecycle** events mark top-level transitions through the state machine. `OBJECTIVE_CREATED` is always the first event for any objective. `OBJECTIVE_COMPLETED`, `OBJECTIVE_FAILED`, and `OBJECTIVE_CANCELLED` are terminal — no further events are emitted after them.

**Task graph** events are emitted by the Operator when it plans or replans the work for an objective. `TASK_GRAPH_CREATED` carries the full DAG structure. `TASK_GRAPH_UPDATED` is emitted if the Operator revises the plan mid-execution based on task results.

**Task lifecycle** events track each node in the task graph from creation through assignment, execution, and completion. `TASK_BLOCKED` is emitted when a task cannot start because a dependency has not yet completed. `TASK_UNBLOCKED` is emitted by the Operator when the last blocking dependency resolves and the task becomes eligible for dispatch.

**Step lifecycle** events mark individual LLM iterations within a single task execution. A task may require multiple steps if the agent needs to reason across several LLM calls before producing a final result. `STEP_RETRYING` is emitted between retry attempts and carries the attempt number and backoff duration for observability.

**Tool call** events are emitted by the Executor during step execution for every external tool interaction. They provide a complete record of what the agent called, with what arguments, and what was returned — essential for debugging and cost attribution.

**Human-in-the-loop** events pause an objective at a task boundary and surface a decision to an external actor. The objective transitions to `paused` status on `APPROVAL_REQUESTED` and resumes on `APPROVAL_RECEIVED`.

**Budget and safety** events are emitted by the Orchestrator when an objective approaches or exceeds a configured limit (token count, cost, or step count). `BUDGET_EXCEEDED` causes the objective to transition to `failed`.

---

## Service Interaction

[#service-interaction](#service-interaction)

Each service has a defined set of events it emits and subscribes to. No service calls another directly — all coordination flows through Catalyst.

### Orchestrator

| Direction | Event | Trigger |
|---|---|---|
| Emits | `OBJECTIVE_CREATED` | Client calls `CreateObjective` RPC |
| Emits | `OBJECTIVE_STARTED` | Immediately after `OBJECTIVE_CREATED` |
| Emits | `OBJECTIVE_PAUSED` | `APPROVAL_REQUESTED` received; transitions state |
| Emits | `OBJECTIVE_RESUMED` | Client calls `ApproveTask` RPC |
| Emits | `OBJECTIVE_CANCELLED` | Client calls `CancelObjective` RPC |
| Emits | `BUDGET_WARNING` | Budget threshold crossed |
| Emits | `BUDGET_EXCEEDED` | Budget limit exceeded |
| Subscribes | `TASK_GRAPH_CREATED` | Records graph structure in snapshot |
| Subscribes | `TASK_COMPLETED` | Updates per-task progress in snapshot |
| Subscribes | `TASK_FAILED` | Updates failure state in snapshot |
| Subscribes | `OBJECTIVE_COMPLETED` | Transitions objective to terminal state |
| Subscribes | `OBJECTIVE_FAILED` | Transitions objective to terminal state |
| Subscribes | `APPROVAL_REQUESTED` | Transitions objective to `paused` |

### Operator

| Direction | Event | Trigger |
|---|---|---|
| Subscribes | `OBJECTIVE_STARTED` | Begins planning: calls the planner, builds TaskGraph |
| Subscribes | `TASK_COMPLETED` | Decrements dependent in-degrees; dispatches newly ready tasks |
| Subscribes | `TASK_FAILED` | Applies failure policy: retry, skip, or fail the objective |
| Subscribes | `APPROVAL_RECEIVED` | Resumes the task that was waiting on approval |
| Subscribes | `OBJECTIVE_CANCELLED` | Stops processing; marks in-flight tasks cancelled |
| Emits | `TASK_GRAPH_CREATED` | Planning complete; carries full DAG |
| Emits | `TASK_GRAPH_UPDATED` | Replanning triggered by a task result |
| Emits | `TASK_CREATED` | One per task node in the graph |
| Emits | `TASK_ASSIGNED` | Task is ready and dispatched to an Executor |
| Emits | `TASK_BLOCKED` | Task is waiting on an unresolved dependency |
| Emits | `TASK_UNBLOCKED` | Last dependency resolved; task is now ready |
| Emits | `APPROVAL_REQUESTED` | Task requires human approval before proceeding |
| Emits | `OBJECTIVE_COMPLETED` | All tasks in the graph have completed successfully |
| Emits | `OBJECTIVE_FAILED` | An unrecoverable task failure has terminated the objective |

### Executor

| Direction | Event | Trigger |
|---|---|---|
| Subscribes | `TASK_ASSIGNED` | Picks up a task from the consumer group |
| Emits | `TASK_STARTED` | Task execution begins |
| Emits | `STEP_STARTED` | Each LLM iteration begins |
| Emits | `STEP_COMPLETED` | LLM iteration produces a result |
| Emits | `STEP_FAILED` | LLM iteration fails |
| Emits | `STEP_RETRYING` | Step is being retried after failure |
| Emits | `TOOL_CALLED` | Agent invokes an external tool |
| Emits | `TOOL_RETURNED` | Tool call returns a result |
| Emits | `TOOL_FAILED` | Tool call fails |
| Emits | `TASK_COMPLETED` | Task execution finishes successfully |
| Emits | `TASK_FAILED` | Task execution fails after all retries |
| Emits | `TASK_RETRYING` | Task is being retried at the task level |

---

## Event Flow

[#event-flow](#event-flow)

Below is the sequence of events for a two-task objective where the tasks have no dependency between them and execute in parallel:

```
Client          Orchestrator         Catalyst          Operator           Executor
  │                  │                  │                 │                  │
  │─ CreateObjective▶│                  │                 │                  │
  │                  │─ OBJ_CREATED ───▶│                 │                  │
  │                  │─ OBJ_STARTED ───▶│────────────────▶│                  │
  │                  │                  │                 │─ [plan tasks]    │
  │                  │                  │◀─ TASK_GRAPH_CREATED ──────────────│
  │                  │◀─ TASK_GRAPH_CREATED ──────────────│                  │
  │                  │                  │◀─ TASK_CREATED (×2) ───────────────│
  │                  │                  │◀─ TASK_ASSIGNED ───────────────────│──────────────▶│
  │                  │                  │◀─ TASK_ASSIGNED ───────────────────│──────────────▶│
  │                  │                  │                 │                  │─ TASK_STARTED  │
  │                  │                  │                 │                  │─ STEP_STARTED  │
  │                  │                  │◀─ TOOL_CALLED ─────────────────────────────────────│
  │                  │                  │◀─ TOOL_RETURNED ───────────────────────────────────│
  │                  │                  │◀─ STEP_COMPLETED ──────────────────────────────────│
  │                  │                  │◀─ TASK_COMPLETED ──────────────────│◀──────────────│
  │                  │◀─ TASK_COMPLETED ─│                │                  │               │
  │                  │                  │◀─ TASK_COMPLETED ──────────────────│◀──────────────│
  │                  │◀─ TASK_COMPLETED ─│                │                  │               │
  │                  │                  │                 │─ [all done]      │               │
  │                  │                  │◀─ OBJ_COMPLETED ────────────────────               │
  │                  │◀─ OBJ_COMPLETED ──│                │                                  │
  │                  │ [update snapshot] │                │                                  │
  │◀─ GetObjective ──│                  │                 │                                  │
```

---

## Storage

[#storage](#storage)

The Orchestrator uses PostgreSQL as its storage layer with the following logical tables:

- **`objectives`** — the objective snapshot, one row per objective, updated after each event
- **`task_graphs`** — the DAG structure for each objective: task nodes and dependency edges
- **`tasks`** — individual task records with status, assignment, and result
- **`steps`** — individual step records within a task, with timing and token usage
- **`working_memory`** — a JSON blob per task holding the agent's message history and scratchpad
- **`task_queue`** — pending work claimed by the Operator using `SELECT ... FOR UPDATE SKIP LOCKED`, ensuring each ready task is dispatched exactly once even under concurrent Operator instances

The **event log** is backed by Catalyst's ClickHouse store. `AgentEvent`s are stored with `source = agent.AgentEvent/{objective_id}` and ordered by `(source, sequence)`, making per-objective replay efficient. The Orchestrator's snapshot tracks a `last_event_sequence` cursor so it can replay only new events after a crash rather than the full history.

---

## Proto Definitions

[#proto-definitions](#proto-definitions)

All agent service types are defined under `api/agent/v1/`.

| File | Contents |
|---|---|
| `events.proto` | `AgentEvent`, `AgentEventType` enum, all payload message types |
| `objective.proto` | `ObjectiveStatus`, `ObjectiveConfig`, `ObjectiveSnapshot`, `OrchestratorService` RPC |
| `task_graph.proto` | `TaskGraph`, `Task`, `TaskStatus`, `OperatorService` RPC |
| `step.proto` | `StepStatus`, `Step`, `AgentExecutorService` RPC |
| `memory.proto` | `WorkingMemory`, `Scratchpad`, `Message` |
| `queue.proto` | `QueuedTask` |

{{< alert context="info" text="The AgentEvent envelope is packed into a Catalyst CloudEvent using binary_data. The CloudEvent type and source fields are populated from the AgentEvent directly, so Catalyst can route by event type and objective ID without deserializing the payload." />}}
