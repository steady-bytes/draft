package main

import (
	"context"
	"fmt"
	"sort"

	linemanv1 "github.com/steady-bytes/draft/api/tooling/lineman/v1"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// orderGap is the sparse-integer spacing new/reordered tasks are assigned
// within (see Decisions in the implementation plan). Resequencing only
// happens once two neighbors' orders are within orderResequenceThreshold of
// each other.
const (
	orderGap                 = 1000
	orderResequenceThreshold = 1
)

func (c *controller) resolveObjective(ctx context.Context, objectiveID string) (*linemanv1.Objective, error) {
	if objectiveID == "" {
		return nil, nil
	}
	return c.getObjective(ctx, objectiveID)
}

func (c *controller) createTask(ctx context.Context, objectiveID, name, details string, priority linemanv1.Priority, state string) (*linemanv1.Task, error) {
	obj, err := c.resolveObjective(ctx, objectiveID)
	if err != nil {
		return nil, fmt.Errorf("objective %q not found: %w", objectiveID, err)
	}
	if state == "" {
		if obj != nil && len(obj.GetStates()) > 0 {
			state = obj.GetStates()[0]
		} else {
			state = "Queued"
		}
	}
	if !stateIsValid(obj, state) {
		return nil, fmt.Errorf("state %q is not a valid state for objective %q", state, objectiveID)
	}

	maxOrder, err := c.maxOrderInState(ctx, objectiveID, state)
	if err != nil {
		return nil, err
	}

	now := timestamppb.Now()
	task := &linemanv1.Task{
		Id:          uuid.NewString(),
		ObjectiveId: objectiveID,
		Name:        name,
		Details:     details,
		State:       state,
		Priority:    priority,
		Order:       maxOrder + orderGap,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := kvSet(ctx, c.kv, task.Id, task); err != nil {
		return nil, err
	}
	c.watchers.publishTask(task, false)
	return task, nil
}

func (c *controller) getTask(ctx context.Context, id string) (*linemanv1.Task, error) {
	return kvGet(ctx, c.kv, id, func() *linemanv1.Task { return &linemanv1.Task{} })
}

func (c *controller) listTasks(ctx context.Context, objectiveID, state, agentID string) ([]*linemanv1.Task, error) {
	m, err := kvList(ctx, c.kv, func() *linemanv1.Task { return &linemanv1.Task{} })
	if err != nil {
		return nil, err
	}
	out := make([]*linemanv1.Task, 0, len(m))
	for _, t := range m {
		if objectiveID != "" && t.GetObjectiveId() != objectiveID {
			continue
		}
		if state != "" && t.GetState() != state {
			continue
		}
		if agentID != "" && t.GetAgentId() != agentID {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GetState() != out[j].GetState() {
			return out[i].GetState() < out[j].GetState()
		}
		return out[i].GetOrder() < out[j].GetOrder()
	})
	return out, nil
}

func (c *controller) maxOrderInState(ctx context.Context, objectiveID, state string) (int64, error) {
	tasks, err := c.listTasks(ctx, objectiveID, state, "")
	if err != nil {
		return 0, err
	}
	var max int64
	for _, t := range tasks {
		if t.GetOrder() > max {
			max = t.GetOrder()
		}
	}
	return max, nil
}

// updateTaskState validates new_state against the task's objective (if any),
// persists it, and returns the task alongside the state it transitioned
// from, so callers (rpc.go) can attach both to a WideEvent/Catalyst event.
// Transitions are unrestricted (any of the objective's states, in either
// direction) -- see Decisions.
func (c *controller) updateTaskState(ctx context.Context, taskID, newState string) (task *linemanv1.Task, fromState string, err error) {
	task, err = c.getTask(ctx, taskID)
	if err != nil {
		return nil, "", err
	}
	obj, err := c.resolveObjective(ctx, task.GetObjectiveId())
	if err != nil {
		return nil, "", err
	}
	if !stateIsValid(obj, newState) {
		return nil, "", fmt.Errorf("state %q is not a valid state for objective %q", newState, task.GetObjectiveId())
	}
	fromState = task.GetState()
	task.State = newState
	task.UpdatedAt = timestamppb.Now()
	// A task leaving Needs Input (any transition away from it) clears the
	// stale question -- ProvideGuidance already does this explicitly, but a
	// plain UpdateTaskState call (e.g. from the board) shouldn't leave a
	// dangling NeedsInput behind either.
	if newState != fromState {
		task.NeedsInput = nil
	}
	if err := kvSet(ctx, c.kv, task.Id, task); err != nil {
		return nil, "", err
	}
	c.watchers.publishTask(task, false)
	return task, fromState, nil
}

// reorderTask assigns task_id a new sparse order between before_task_id's
// and after_task_id's current orders (either may be empty for "start"/"end"
// of the list). Resequences the whole (objective_id, state) list once first,
// if there's no room left between neighbors -- see Decisions.
func (c *controller) reorderTask(ctx context.Context, taskID, beforeTaskID, afterTaskID string) (*linemanv1.Task, error) {
	task, err := c.getTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	newOrder, needsResequence, err := c.computeReorder(ctx, task, beforeTaskID, afterTaskID)
	if err != nil {
		return nil, err
	}
	if needsResequence {
		if err := c.resequence(ctx, task.GetObjectiveId(), task.GetState()); err != nil {
			return nil, err
		}
		newOrder, _, err = c.computeReorder(ctx, task, beforeTaskID, afterTaskID)
		if err != nil {
			return nil, err
		}
	}

	task.Order = newOrder
	task.UpdatedAt = timestamppb.Now()
	if err := kvSet(ctx, c.kv, task.Id, task); err != nil {
		return nil, err
	}
	c.watchers.publishTask(task, false)
	return task, nil
}

func (c *controller) computeReorder(ctx context.Context, task *linemanv1.Task, beforeTaskID, afterTaskID string) (order int64, needsResequence bool, err error) {
	var beforeOrder, afterOrder int64
	haveBefore, haveAfter := beforeTaskID != "", afterTaskID != ""

	if haveBefore {
		before, err := c.getTask(ctx, beforeTaskID)
		if err != nil {
			return 0, false, err
		}
		beforeOrder = before.GetOrder()
	}
	if haveAfter {
		after, err := c.getTask(ctx, afterTaskID)
		if err != nil {
			return 0, false, err
		}
		afterOrder = after.GetOrder()
	}

	switch {
	case haveBefore && haveAfter:
		if afterOrder-beforeOrder <= orderResequenceThreshold {
			return 0, true, nil
		}
		return beforeOrder + (afterOrder-beforeOrder)/2, false, nil
	case haveBefore && !haveAfter:
		return beforeOrder + orderGap, false, nil
	case !haveBefore && haveAfter:
		if afterOrder <= orderResequenceThreshold {
			return 0, true, nil
		}
		return afterOrder / 2, false, nil
	default:
		// Neither given: only sibling in the list.
		return orderGap, false, nil
	}
}

// resequence rewrites every task's order in (objectiveID, state) as clean
// multiples of orderGap, in their current relative order.
func (c *controller) resequence(ctx context.Context, objectiveID, state string) error {
	tasks, err := c.listTasks(ctx, objectiveID, state, "")
	if err != nil {
		return err
	}
	for i, t := range tasks {
		t.Order = int64(i+1) * orderGap
		if err := kvSet(ctx, c.kv, t.Id, t); err != nil {
			return err
		}
	}
	return nil
}

func (c *controller) assignAgent(ctx context.Context, taskID, agentID string) (*linemanv1.Task, error) {
	task, err := c.getTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	task.AgentId = agentID
	task.UpdatedAt = timestamppb.Now()
	if err := kvSet(ctx, c.kv, task.Id, task); err != nil {
		return nil, err
	}
	c.watchers.publishTask(task, false)
	return task, nil
}
