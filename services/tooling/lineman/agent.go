package main

import (
	"context"
	"fmt"
	"sort"

	linemanv1 "github.com/steady-bytes/draft/api/tooling/lineman/v1"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// heartbeat upserts the Agent record for agent_id (creating it, with
// display_name/kind, the first time this id is seen) and, if task_id is
// set, updates that task's agent_id/current_action -- see the design
// brief's Task Detail page, whose "Current Action" line is exactly this
// field.
func (c *controller) heartbeat(ctx context.Context, agentID, displayName string, kind linemanv1.AgentKind, taskID, currentAction string) error {
	if agentID == "" {
		return fmt.Errorf("agent_id is required")
	}
	agent, err := kvGet(ctx, c.kv, agentID, func() *linemanv1.Agent { return &linemanv1.Agent{} })
	if err != nil {
		agent = &linemanv1.Agent{Id: agentID}
		if displayName != "" {
			agent.DisplayName = displayName
		} else {
			agent.DisplayName = agentID
		}
		agent.Kind = kind
	}
	agent.LastSeenAt = timestamppb.Now()
	if err := kvSet(ctx, c.kv, agent.Id, agent); err != nil {
		return err
	}
	c.watchers.publishAgent(agent)

	if taskID != "" {
		task, err := c.getTask(ctx, taskID)
		if err != nil {
			return fmt.Errorf("heartbeat for unknown task %q: %w", taskID, err)
		}
		task.AgentId = agentID
		task.CurrentAction = currentAction
		task.UpdatedAt = timestamppb.Now()
		if err := kvSet(ctx, c.kv, task.Id, task); err != nil {
			return err
		}
		c.watchers.publishTask(task, false)
	}
	return nil
}

func (c *controller) listAgents(ctx context.Context) ([]*linemanv1.Agent, error) {
	m, err := kvList(ctx, c.kv, func() *linemanv1.Agent { return &linemanv1.Agent{} })
	if err != nil {
		return nil, err
	}
	out := make([]*linemanv1.Agent, 0, len(m))
	for _, a := range m {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].GetDisplayName() < out[j].GetDisplayName()
	})
	return out, nil
}

// requestInput is agent-called: records the blocking question on the task
// and moves it to the caller-implied "Needs Input" state. Per the design
// brief's Decisions, Lineman doesn't hardcode a state name for this -- the
// caller (the agent, driven by whatever convention its objective's author
// set up) passes state itself via a subsequent UpdateTaskState-shaped call
// if it wants a state change; requestInput only ever writes the question.
// Kept separate from UpdateTaskState so a caller can't accidentally set
// needs_input without also being explicit that this is a blocking request,
// and vice versa.
func (c *controller) requestInput(ctx context.Context, taskID, question string, options []string) (*linemanv1.Task, error) {
	task, err := c.getTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	task.NeedsInput = &linemanv1.NeedsInput{
		Question:    question,
		Options:     options,
		RequestedAt: timestamppb.Now(),
	}
	task.UpdatedAt = timestamppb.Now()
	if err := kvSet(ctx, c.kv, task.Id, task); err != nil {
		return nil, err
	}
	c.watchers.publishTask(task, false)
	return task, nil
}

// provideGuidance is human-called: records the response, clears the
// blocking question, and (if next_state is set) transitions the task --
// typically back to whatever state it was in before Needs Input.
func (c *controller) provideGuidance(ctx context.Context, taskID, response, nextState string) (*linemanv1.Task, error) {
	task, err := c.getTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	task.NeedsInput = nil
	task.CurrentAction = fmt.Sprintf("Guidance received: %s", response)
	task.UpdatedAt = timestamppb.Now()

	if nextState != "" {
		obj, err := c.resolveObjective(ctx, task.GetObjectiveId())
		if err != nil {
			return nil, err
		}
		if !stateIsValid(obj, nextState) {
			return nil, fmt.Errorf("state %q is not a valid state for objective %q", nextState, task.GetObjectiveId())
		}
		task.State = nextState
	}

	if err := kvSet(ctx, c.kv, task.Id, task); err != nil {
		return nil, err
	}
	c.watchers.publishTask(task, false)
	return task, nil
}
