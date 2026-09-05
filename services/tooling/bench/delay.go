// This file implements bench://delay@v1, a built-in executor that does nothing
// but wait for a configured duration before succeeding. It exists for pacing a
// workflow's own steps out over real time (eg. spacing repeated requests through
// Fuse's proxy 5s apart to observe behavior over time) — grpc-call and http-call
// have no delay of their own, and the one plugin that does (garage://catalyst-
// produce@v1's own `delay` field) is specific to publishing an event, not a
// general-purpose primitive worth repurposing for unrelated steps.
package main

import (
	"context"
	"fmt"
	"time"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"

	"google.golang.org/protobuf/types/known/structpb"
)

// delayExecutor is the Executor behind bench://delay@v1.
type delayExecutor struct{}

// NewDelayExecutor builds the bench://delay@v1 executor.
func NewDelayExecutor() Executor {
	return &delayExecutor{}
}

func (e *delayExecutor) Execute(ctx context.Context, step *workflowv1.Step, with *structpb.Struct) (*ExecutionResult, error) {
	durationStr := with.GetFields()["duration"].GetStringValue()
	if durationStr == "" {
		return nil, fmt.Errorf("delay: with.duration is required (a Go duration string, e.g. \"5s\")")
	}
	d, err := time.ParseDuration(durationStr)
	if err != nil {
		return nil, fmt.Errorf("delay: with.duration: %w", err)
	}

	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}

	result, err := structpb.NewStruct(map[string]interface{}{"waited": durationStr})
	if err != nil {
		return nil, fmt.Errorf("delay: %w", err)
	}
	return &ExecutionResult{Result: result, Passed: true}, nil
}
