package main

import (
	"context"
	"sort"

	linemanv1 "github.com/steady-bytes/draft/api/tooling/lineman/v1"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// defaultObjectiveStates is used whenever CreateObjective's caller leaves
// states empty -- matches the Create Objective mockup's pre-filled default
// (see docs/architecture/lineman-implementation-plan.md's Implementation
// Plan, Phase 3).
var defaultObjectiveStates = []string{"Queued", "In Flight", "Done"}

func (c *controller) createObjective(ctx context.Context, name, description string, states []string) (*linemanv1.Objective, error) {
	if len(states) == 0 {
		states = append([]string(nil), defaultObjectiveStates...)
	}
	obj := &linemanv1.Objective{
		Id:          uuid.NewString(),
		Name:        name,
		Description: description,
		States:      states,
		CreatedAt:   timestamppb.Now(),
	}
	if err := kvSet(ctx, c.kv, obj.Id, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (c *controller) getObjective(ctx context.Context, id string) (*linemanv1.Objective, error) {
	return kvGet(ctx, c.kv, id, func() *linemanv1.Objective { return &linemanv1.Objective{} })
}

func (c *controller) listObjectives(ctx context.Context) ([]*linemanv1.Objective, error) {
	m, err := kvList(ctx, c.kv, func() *linemanv1.Objective { return &linemanv1.Objective{} })
	if err != nil {
		return nil, err
	}
	out := make([]*linemanv1.Objective, 0, len(m))
	for _, o := range m {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].GetCreatedAt().AsTime().Before(out[j].GetCreatedAt().AsTime())
	})
	return out, nil
}

// stateIsValid reports whether state is one of obj's defined states. A nil
// obj (standalone task/objective_id "") allows any non-empty state -- there's
// no objective to own a state list for it, matching the "a loop or scheduled
// task can target no objective at all" decision extended to plain tasks.
func stateIsValid(obj *linemanv1.Objective, state string) bool {
	if obj == nil {
		return state != ""
	}
	for _, s := range obj.GetStates() {
		if s == state {
			return true
		}
	}
	return false
}
