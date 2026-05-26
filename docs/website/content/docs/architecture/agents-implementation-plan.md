---
weight: 11
title: Agent Service — Implementation Plan
description: Phased implementation plan for the Orchestrator and Operator services
icon: checklist
draft: false
toc: true
---

This document describes the implementation plan for the Agent Service in the order work should be done. Each phase produces a runnable, testable artifact that unblocks the next.

---

## Phase 1 — Proto Definitions

All service contracts are defined before any Go code is written. This ensures Orchestrator, Operator, and Executor can be developed in parallel against a stable API surface.

### 1.1 Rename `run` → `objective` in existing protos

- `api/agent/v1/run.proto` → `api/agent/v1/objective.proto`
  - `RunStatus` → `ObjectiveStatus`
  - `RunConfig` → `ObjectiveConfig`
  - `RunSnapshot` → `ObjectiveSnapshot`
  - `OrchestratorService.CreateRun` → `OrchestratorService.CreateObjective`
  - `OrchestratorService.GetRun` → `OrchestratorService.GetObjective`
  - All `run_id` fields → `objective_id`
- `api/agent/v1/events.proto`
  - Rename all `AGENT_EVENT_TYPE_RUN_*` → `AGENT_EVENT_TYPE_OBJECTIVE_*`
  - Replace `AGENT_EVENT_TYPE_SUBAGENT_*` with `AGENT_EVENT_TYPE_TASK_*`
  - Add `AGENT_EVENT_TYPE_TASK_GRAPH_*`
  - Renumber all groups per the scheme in `agents.md`
  - Update `AgentEvent.run_id` → `AgentEvent.objective_id`, add `AgentEvent.task_id`

### 1.2 Add `task_graph.proto`

Define the task graph data model and the `OperatorService` RPC:

```protobuf
// api/agent/v1/task_graph.proto

enum TaskStatus {
  TASK_STATUS_UNSPECIFIED = 0;
  TASK_STATUS_PENDING     = 1;
  TASK_STATUS_BLOCKED     = 2;
  TASK_STATUS_READY       = 3;
  TASK_STATUS_ASSIGNED    = 4;
  TASK_STATUS_RUNNING     = 5;
  TASK_STATUS_COMPLETED   = 6;
  TASK_STATUS_FAILED      = 7;
  TASK_STATUS_CANCELLED   = 8;
}

message Task {
  string            id           = 1;
  string            objective_id = 2;
  string            description  = 3;
  TaskStatus        status       = 4;
  repeated string   depends_on   = 5;  // task IDs that must complete first
  string            result       = 6;  // populated on completion
  google.protobuf.Timestamp created_at   = 7;
  google.protobuf.Timestamp completed_at = 8;
}

message TaskGraph {
  string         objective_id = 1;
  repeated Task  tasks        = 2;
}

service OperatorService {
  rpc GetTaskGraph(GetTaskGraphRequest) returns (TaskGraph);
}
```

### 1.3 Run `dctl api build`

Regenerate Go, Rust, and TypeScript bindings. Verify no compile errors before proceeding.

---

## Phase 2 — Orchestrator Service

The Orchestrator is a standard Draft service using Chassis. It exposes the external RPC API and manages objective lifecycle state.

### 2.1 Storage schema

Create the PostgreSQL schema under `services/agent/orchestrator/migrations/`:

```sql
-- objectives: one row per objective, updated on every event
CREATE TABLE objectives (
  id                  TEXT PRIMARY KEY,
  status              TEXT NOT NULL,
  description         TEXT NOT NULL,
  task_graph_id       TEXT,
  last_event_sequence BIGINT NOT NULL DEFAULT 0,
  created_at          TIMESTAMPTZ NOT NULL,
  updated_at          TIMESTAMPTZ NOT NULL,
  completed_at        TIMESTAMPTZ,
  budget_tokens       BIGINT,
  tokens_used         BIGINT NOT NULL DEFAULT 0
);

-- task_graphs: DAG structure for each objective
CREATE TABLE task_graphs (
  id           TEXT PRIMARY KEY,
  objective_id TEXT NOT NULL REFERENCES objectives(id),
  created_at   TIMESTAMPTZ NOT NULL
);

-- tasks: one row per task node
CREATE TABLE tasks (
  id           TEXT PRIMARY KEY,
  objective_id TEXT NOT NULL REFERENCES objectives(id),
  graph_id     TEXT NOT NULL REFERENCES task_graphs(id),
  description  TEXT NOT NULL,
  status       TEXT NOT NULL,
  depends_on   TEXT[] NOT NULL DEFAULT '{}',
  result       TEXT,
  created_at   TIMESTAMPTZ NOT NULL,
  updated_at   TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ
);

-- steps: one row per LLM iteration within a task
CREATE TABLE steps (
  id           TEXT PRIMARY KEY,
  task_id      TEXT NOT NULL REFERENCES tasks(id),
  objective_id TEXT NOT NULL,
  index        INT NOT NULL,
  status       TEXT NOT NULL,
  tokens_used  BIGINT NOT NULL DEFAULT 0,
  output       TEXT,
  started_at   TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ
);

-- working_memory: agent scratchpad per task
CREATE TABLE working_memory (
  task_id     TEXT PRIMARY KEY REFERENCES tasks(id),
  messages    JSONB NOT NULL DEFAULT '[]',
  scratchpad  TEXT NOT NULL DEFAULT '',
  updated_at  TIMESTAMPTZ NOT NULL
);

-- task_queue: ready tasks pending dispatch, claimed with SELECT FOR UPDATE SKIP LOCKED
CREATE TABLE task_queue (
  task_id      TEXT PRIMARY KEY REFERENCES tasks(id),
  objective_id TEXT NOT NULL,
  queued_at    TIMESTAMPTZ NOT NULL
);
```

### 2.2 `OrchestratorService` RPC implementation

Implement in `services/agent/orchestrator/`:

| RPC | Behavior |
|---|---|
| `CreateObjective` | Validate input, persist objective row, emit `OBJECTIVE_CREATED` + `OBJECTIVE_STARTED` |
| `GetObjective` | Read objective snapshot from `objectives` table |
| `ListObjectives` | Paginated query of `objectives` table |
| `CancelObjective` | Validate state transition, emit `OBJECTIVE_CANCELLED` |
| `ApproveTask` | Validate objective is `paused`, emit `APPROVAL_RECEIVED` |

### 2.3 Catalyst subscriptions

The Orchestrator subscribes to the following events to keep the objective snapshot current:

| Event | Handler |
|---|---|
| `TASK_GRAPH_CREATED` | Record `graph_id` in objective row |
| `TASK_COMPLETED` | Increment completed task count in snapshot |
| `TASK_FAILED` | Record failure in snapshot |
| `OBJECTIVE_COMPLETED` | Transition objective to `completed`, set `completed_at` |
| `OBJECTIVE_FAILED` | Transition objective to `failed` |
| `APPROVAL_REQUESTED` | Transition objective to `paused` |

### 2.4 State machine enforcement

All status transitions must be validated before any event is emitted. Illegal transitions return a gRPC `FailedPrecondition` error. Use a transition table:

```go
var allowedTransitions = map[ObjectiveStatus][]ObjectiveStatus{
    StatusCreated: {StatusPending},
    StatusPending: {StatusRunning},
    StatusRunning: {StatusPaused, StatusCompleted, StatusFailed, StatusCancelled},
    StatusPaused:  {StatusRunning, StatusCancelled},
}
```

### 2.5 Budget tracking

On every `STEP_COMPLETED` event the Orchestrator increments `tokens_used` in the objective snapshot. When `tokens_used` crosses the configured threshold it emits `BUDGET_WARNING`; when it exceeds the limit it emits `BUDGET_EXCEEDED` and transitions the objective to `failed`.

---

## Phase 3 — Operator Service

The Operator is a new Draft service. It has no external RPC — it communicates exclusively through Catalyst events.

### 3.1 Objective subscription and planning

Subscribe to `OBJECTIVE_STARTED` events. On receipt:

1. Call the planner (initially a direct LLM call using the objective description as the prompt)
2. Parse the planner's response into a `TaskGraph` — a list of tasks with `depends_on` fields
3. Persist the `TaskGraph` and all `Task` rows to PostgreSQL
4. Emit `TASK_GRAPH_CREATED` followed by one `TASK_CREATED` per task
5. Insert all tasks with in-degree zero into `task_queue`
6. Emit `TASK_ASSIGNED` for each queued task

The planner prompt should instruct the LLM to return a structured JSON task list. Use structured output or tool use to enforce the schema from `task_graph.proto`.

### 3.2 DAG dispatch loop

The dispatch loop is event-driven, not a polling loop:

```go
// On TASK_COMPLETED
func (o *Operator) onTaskCompleted(event AgentEvent) {
    task := o.loadTask(event.TaskId)
    o.markCompleted(task, event.Payload.Result)

    for _, dependent := range o.dependents(task.Id) {
        o.decrementInDegree(dependent)
        if o.inDegree(dependent) == 0 {
            o.enqueue(dependent)
            o.emit(TASK_ASSIGNED, dependent)
        }
    }

    if o.allTasksDone(event.ObjectiveId) {
        o.emit(OBJECTIVE_COMPLETED, event.ObjectiveId)
    }
}
```

Use `SELECT ... FOR UPDATE SKIP LOCKED` on `task_queue` so multiple Operator instances can run without double-dispatching a task.

### 3.3 Failure policy

On `TASK_FAILED`, apply the policy configured on the `TaskGraph`:

| Policy | Behavior |
|---|---|
| `retry` | Re-enqueue the task up to the configured retry limit, emit `TASK_RETRYING` |
| `skip` | Mark the task skipped; treat dependents as if it completed with an empty result |
| `fail` | Emit `OBJECTIVE_FAILED` and stop processing the graph |

### 3.4 Human-in-the-loop

When a task requires approval before execution (flagged in the `Task` definition), the Operator emits `APPROVAL_REQUESTED` instead of `TASK_ASSIGNED` and marks the task `blocked`. On receiving `APPROVAL_RECEIVED` it enqueues the task and emits `TASK_ASSIGNED`.

### 3.5 Crash recovery

On startup, the Operator queries the `task_queue` table for any tasks that are in state `ready` but have no active assignment. These are tasks that were enqueued before a crash and never dispatched or claimed. The Operator re-emits `TASK_ASSIGNED` for each one.

---

## Phase 4 — Executor Updates

The Executor already exists but must be updated to align with the new event naming and the task/step distinction.

### 4.1 Subscription change

Change the Catalyst subscription from `STEP_STARTED` (Orchestrator-driven, old model) to `TASK_ASSIGNED` (Operator-driven, new model). The consumer group name stays `"agent-executors"`.

### 4.2 Task execution loop

A single task assignment may require multiple LLM steps before producing a final result. The Executor manages this loop internally:

```
TASK_ASSIGNED received
    │
    Load working memory for task
    │
    loop:
        Emit STEP_STARTED
        Construct prompt from working memory + task description
        Call LLM
        For each tool call in response:
            Emit TOOL_CALLED
            Execute tool
            Emit TOOL_RETURNED
        Emit STEP_COMPLETED
        Update working memory
        If LLM signals done → break
    │
    Emit TASK_COMPLETED with final result
```

### 4.3 Event field updates

All events emitted by the Executor must include both `objective_id` and `task_id` in the `AgentEvent` envelope. Update event construction accordingly.

---

## Phase 5 — Integration and Testing

### 5.1 Local integration test

Write an end-to-end test that:

1. Calls `CreateObjective` with a simple two-task objective (e.g., "summarize document A and document B")
2. Asserts `TASK_GRAPH_CREATED` is emitted with two tasks and no dependencies
3. Asserts both `TASK_ASSIGNED` events are emitted (parallel dispatch)
4. Stubs Executor responses for both tasks
5. Asserts `OBJECTIVE_COMPLETED` is emitted
6. Calls `GetObjective` and asserts status is `completed`

### 5.2 Sequential dependency test

Write a test for a two-task graph where task B depends on task A:

1. Assert only task A receives `TASK_ASSIGNED` initially
2. Complete task A
3. Assert task B receives `TASK_ASSIGNED` with task A's result available in working memory
4. Complete task B
5. Assert `OBJECTIVE_COMPLETED`

### 5.3 Crash recovery test

1. Start Operator, receive `OBJECTIVE_STARTED`, emit `TASK_GRAPH_CREATED`
2. Kill Operator before emitting `TASK_ASSIGNED`
3. Restart Operator
4. Assert Operator re-emits `TASK_ASSIGNED` for all ready tasks
5. Complete objective normally

---

## Milestone Summary

| Phase | Deliverable | Unblocks |
|---|---|---|
| 1 | Updated protos, `task_graph.proto` | Phases 2, 3, 4 in parallel |
| 2 | Orchestrator service with storage and RPC | Integration testing |
| 3 | Operator service with DAG execution | Integration testing |
| 4 | Updated Executor | Integration testing |
| 5 | End-to-end tests passing | Ship |
