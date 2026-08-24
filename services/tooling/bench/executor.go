// This file defines the Executor interface every `uses:` target implements, and the
// outcome shape a step's Execute call reports back to the scheduler. See grpc_call.go
// for the one executor Phase 3 actually ships (bench://grpc-call@v1); scheduler.go
// resolves a step's `uses:` string to an Executor (or a clear "not implemented"
// error for anything else, including every garage:// reference — Garage resolution
// is explicitly Phase 6, out of scope here).
package main

import (
	"context"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"

	"google.golang.org/protobuf/types/known/structpb"
)

// Executor runs one step, given its `with:` block already template-resolved (see
// templating.go) against steps that have already completed.
type Executor interface {
	// Execute performs the step's work and evaluates its `expect:` assertions
	// (step.GetExpect()) itself — Bench doesn't interpret assertion vocabulary,
	// per the doc's "Built-in executors vs. Garage plugins" section: each
	// executor defines what its own `expect:` block means.
	//
	// A non-nil error indicates an infrastructure-level failure (couldn't resolve
	// the target service, couldn't reach it, malformed response, misconfigured
	// `with:`) rather than a normal assertion failure — the scheduler treats both
	// as the step failing, but keeps the error a caller can act on.
	Execute(ctx context.Context, step *workflowv1.Step, with *structpb.Struct) (*ExecutionResult, error)
}

// ExecutionResult is what a step's Execute call reports back: the raw response to
// persist as the step's StepResult.Result (so later steps' templating can reference
// it, per the doc's data-flow example), whether the configured `expect:` assertions
// passed, and — when they didn't — why, in a form fit to store in
// StepResult.Error.
type ExecutionResult struct {
	Result        *structpb.Struct
	Passed        bool
	FailureReason string
	// Detail is optional, executor-specific diagnostic information -- e.g.
	// grpc_call.go reports the resolved address, the exact URL it called,
	// and the HTTP status it got back. Persisted alongside Result/the
	// resolved request on StepResult (see the request/detail fields on
	// api/tooling/workflow/v1's StepResult) so a failed step's run-detail
	// page can show more than just an error string.
	Detail *structpb.Struct
}
