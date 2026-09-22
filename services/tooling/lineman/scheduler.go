// scheduler.go: the Scheduler primitive -- fire a single task once, at a
// specific future time. Firing is driven by an internal ticker (see
// runScheduler, started from main.go via WithRunner), not by consuming
// Lineman's own Catalyst events -- see the implementation plan's Decisions
// on Catalyst's Consume fan-out bug.
package main

import (
	"context"
	"time"

	linemanv1 "github.com/steady-bytes/draft/api/tooling/lineman/v1"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (c *controller) createScheduledTask(ctx context.Context, objectiveID, description string, priority linemanv1.Priority, fireAt *timestamppb.Timestamp) (*linemanv1.ScheduledTask, error) {
	s := &linemanv1.ScheduledTask{
		Id:          uuid.NewString(),
		ObjectiveId: objectiveID,
		Description: description,
		Priority:    priority,
		FireAt:      fireAt,
		Status:      linemanv1.ScheduledTaskStatus_SCHEDULED_TASK_STATUS_PENDING,
	}
	if err := kvSet(ctx, c.kv, s.Id, s); err != nil {
		return nil, err
	}
	c.watchers.publishScheduledTask(s)
	return s, nil
}

func (c *controller) listScheduledTasks(ctx context.Context, objectiveID string) ([]*linemanv1.ScheduledTask, error) {
	m, err := kvList(ctx, c.kv, func() *linemanv1.ScheduledTask { return &linemanv1.ScheduledTask{} })
	if err != nil {
		return nil, err
	}
	out := make([]*linemanv1.ScheduledTask, 0, len(m))
	for _, s := range m {
		if objectiveID != "" && s.GetObjectiveId() != objectiveID {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

func (c *controller) cancelScheduledTask(ctx context.Context, id string) error {
	s, err := kvGet(ctx, c.kv, id, func() *linemanv1.ScheduledTask { return &linemanv1.ScheduledTask{} })
	if err != nil {
		return err
	}
	if s.GetStatus() != linemanv1.ScheduledTaskStatus_SCHEDULED_TASK_STATUS_PENDING {
		return nil // already fired or cancelled -- a no-op, not an error (matches Foundry's Retract precedent)
	}
	s.Status = linemanv1.ScheduledTaskStatus_SCHEDULED_TASK_STATUS_CANCELLED
	if err := kvSet(ctx, c.kv, s.Id, s); err != nil {
		return err
	}
	c.watchers.publishScheduledTask(s)
	return nil
}

// runScheduler polls Blueprint for due ScheduledTask and Loop entries every
// tickerInterval and fires them, until ctx is done. Started as a
// WithRunner goroutine from main.go. One ticker for both primitives -- they
// share the same firing mechanism (see loop.go's fireDueLoops), no reason
// to run two separate polling loops.
func (c *controller) runScheduler(ctx context.Context) {
	ticker := time.NewTicker(c.tickerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.fireDueScheduledTasks(ctx)
			c.fireDueLoops(ctx)
		}
	}
}

func (c *controller) fireDueScheduledTasks(ctx context.Context) {
	pending, err := c.listScheduledTasks(ctx, "")
	if err != nil {
		c.logger.WithError(err).Error("failed to list scheduled tasks")
		return
	}
	now := time.Now()
	for _, s := range pending {
		if s.GetStatus() != linemanv1.ScheduledTaskStatus_SCHEDULED_TASK_STATUS_PENDING {
			continue
		}
		if s.GetFireAt().AsTime().After(now) {
			continue
		}
		c.fireScheduledTask(ctx, s)
	}
}

func (c *controller) fireScheduledTask(ctx context.Context, s *linemanv1.ScheduledTask) {
	// ScheduledTasks only ever supply a name -- a spawned task's details
	// start empty; see Task.details' own doc comment.
	task, err := c.createTask(ctx, s.GetObjectiveId(), s.GetDescription(), "", s.GetPriority(), "")
	if err != nil {
		c.logger.WithError(err).WithField("scheduled_task_id", s.GetId()).Error("failed to fire scheduled task")
		return
	}
	s.Status = linemanv1.ScheduledTaskStatus_SCHEDULED_TASK_STATUS_FIRED
	s.CreatedTaskId = task.GetId()
	if err := kvSet(ctx, c.kv, s.Id, s); err != nil {
		c.logger.WithError(err).WithField("scheduled_task_id", s.GetId()).Error("failed to persist fired scheduled task")
		return
	}
	c.watchers.publishScheduledTask(s)
	c.events.ScheduledTaskFired(s, task.GetId())
}
