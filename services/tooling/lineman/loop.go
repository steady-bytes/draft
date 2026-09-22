// loop.go: the Loops primitive -- a recurring schedule that spawns a new
// Task instance every time it fires. Reuses runScheduler's ticker-driven
// firing mechanism (see scheduler.go); the only new logic here is computing
// each next fire time from a Recurrence and respecting end conditions.
package main

import (
	"context"
	"fmt"
	"time"

	linemanv1 "github.com/steady-bytes/draft/api/tooling/lineman/v1"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (c *controller) createLoop(ctx context.Context, objectiveID, description string, priority linemanv1.Priority, recurrence *linemanv1.Recurrence) (*linemanv1.Loop, error) {
	startsAt := recurrence.GetStartsAt().AsTime()
	if recurrence.GetStartsAt() == nil {
		startsAt = time.Now()
	}
	first, err := nextFireTime(startsAt, recurrence)
	if err != nil {
		return nil, err
	}
	l := &linemanv1.Loop{
		Id:          uuid.NewString(),
		ObjectiveId: objectiveID,
		Description: description,
		Priority:    priority,
		Recurrence:  recurrence,
		Status:      linemanv1.LoopStatus_LOOP_STATUS_ACTIVE,
		NextFireAt:  timestamppb.New(first),
	}
	if err := kvSet(ctx, c.kv, l.Id, l); err != nil {
		return nil, err
	}
	c.watchers.publishLoop(l)
	return l, nil
}

func (c *controller) listLoops(ctx context.Context, objectiveID string) ([]*linemanv1.Loop, error) {
	m, err := kvList(ctx, c.kv, func() *linemanv1.Loop { return &linemanv1.Loop{} })
	if err != nil {
		return nil, err
	}
	out := make([]*linemanv1.Loop, 0, len(m))
	for _, l := range m {
		if objectiveID != "" && l.GetObjectiveId() != objectiveID {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

func (c *controller) setLoopStatus(ctx context.Context, id string, status linemanv1.LoopStatus) (*linemanv1.Loop, error) {
	l, err := kvGet(ctx, c.kv, id, func() *linemanv1.Loop { return &linemanv1.Loop{} })
	if err != nil {
		return nil, err
	}
	l.Status = status
	if err := kvSet(ctx, c.kv, l.Id, l); err != nil {
		return nil, err
	}
	c.watchers.publishLoop(l)
	return l, nil
}

func (c *controller) deleteLoop(ctx context.Context, id string) error {
	if err := kvDelete(ctx, c.kv, id, &linemanv1.Loop{}); err != nil {
		return err
	}
	c.watchers.publish(&linemanv1.WatchResponse{Item: &linemanv1.WatchResponse_Loop{Loop: &linemanv1.Loop{Id: id}}, Removed: true})
	return nil
}

// fireDueLoops mirrors fireDueScheduledTasks -- called from the same
// runScheduler ticker (see scheduler.go), scanning Loop instead of
// ScheduledTask.
func (c *controller) fireDueLoops(ctx context.Context) {
	loops, err := c.listLoops(ctx, "")
	if err != nil {
		c.logger.WithError(err).Error("failed to list loops")
		return
	}
	now := time.Now()
	for _, l := range loops {
		if l.GetStatus() != linemanv1.LoopStatus_LOOP_STATUS_ACTIVE {
			continue
		}
		if l.GetNextFireAt().AsTime().After(now) {
			continue
		}
		c.fireLoop(ctx, l)
	}
}

func (c *controller) fireLoop(ctx context.Context, l *linemanv1.Loop) {
	// Loops only ever supply a name -- a spawned task's details start empty;
	// see Task.details' own doc comment.
	task, err := c.createTask(ctx, l.GetObjectiveId(), l.GetDescription(), "", l.GetPriority(), "")
	if err != nil {
		c.logger.WithError(err).WithField("loop_id", l.GetId()).Error("failed to fire loop")
		return
	}
	l.OccurrenceCount++

	if endConditionMet(l) {
		l.Status = linemanv1.LoopStatus_LOOP_STATUS_FINISHED
	} else {
		next, err := nextFireTime(l.GetNextFireAt().AsTime(), l.GetRecurrence())
		if err != nil {
			c.logger.WithError(err).WithField("loop_id", l.GetId()).Error("failed to compute next fire time; pausing loop")
			l.Status = linemanv1.LoopStatus_LOOP_STATUS_PAUSED
		} else {
			l.NextFireAt = timestamppb.New(next)
		}
	}

	if err := kvSet(ctx, c.kv, l.Id, l); err != nil {
		c.logger.WithError(err).WithField("loop_id", l.GetId()).Error("failed to persist fired loop")
		return
	}
	c.watchers.publishLoop(l)
	c.events.LoopFired(l, task.GetId())
}

func endConditionMet(l *linemanv1.Loop) bool {
	r := l.GetRecurrence()
	if r.GetEndsAfterOccurrences() > 0 && l.GetOccurrenceCount() >= int64(r.GetEndsAfterOccurrences()) {
		return true
	}
	if r.GetEndsAt() != nil && !r.GetEndsAt().AsTime().IsZero() && time.Now().After(r.GetEndsAt().AsTime()) {
		return true
	}
	return false
}

// nextFireTime computes the next fire time strictly after from, per
// recurrence.kind. "daily"/"weekly"/"every_n_days" are straightforward date
// math; "cron" is a deliberate non-goal of this pass (see the
// implementation plan's Non-Goals) -- picking and vendoring a parser is
// implementation-time work, not wired up yet.
func nextFireTime(from time.Time, r *linemanv1.Recurrence) (time.Time, error) {
	switch r.GetKind() {
	case "daily":
		return withTimeOfDay(from.AddDate(0, 0, 1), r.GetAt()), nil
	case "weekly":
		return withTimeOfDay(from.AddDate(0, 0, 7), r.GetAt()), nil
	case "every_n_days":
		n := int(r.GetIntervalDays())
		if n <= 0 {
			n = 1
		}
		return withTimeOfDay(from.AddDate(0, 0, n), r.GetAt()), nil
	case "cron":
		return time.Time{}, fmt.Errorf("cron recurrence not yet implemented (see implementation plan Non-Goals)")
	default:
		return time.Time{}, fmt.Errorf("unknown recurrence kind %q", r.GetKind())
	}
}

// withTimeOfDay applies recurrence.at ("HH:MM") to t's date if set and
// well-formed; otherwise returns t unchanged.
func withTimeOfDay(t time.Time, at string) time.Time {
	if at == "" {
		return t
	}
	parsed, err := time.Parse("15:04", at)
	if err != nil {
		return t
	}
	return time.Date(t.Year(), t.Month(), t.Day(), parsed.Hour(), parsed.Minute(), 0, 0, t.Location())
}
