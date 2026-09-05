// This file implements Phase 3's DAG scheduler: given a loaded, valid
// *workflowv1.Workflow, run its steps respecting the dependency graph built by
// loader.go's effectiveDependsOn (reused directly, not recomputed), starting a step
// once every dependency it names has completed, and running independent
// (simultaneously-unblocked) steps concurrently via real goroutines.
//
// Failure propagation: a step whose on_failure policy is FAILURE_POLICY_FAIL
// (the default) or whose retries (FAILURE_POLICY_RETRY) are exhausted is a "hard"
// failure — it fails the run, and every step that (transitively) depends on it is
// marked STEP_STATUS_SKIPPED rather than executed, the same way a failed job blocks
// its dependents in Argo/GitHub Actions/Drone. A step whose policy is
// FAILURE_POLICY_CONTINUE is a "soft" failure — matching the doc's
// notify-slack-on-seed-data example, it doesn't fail the run, and — since nothing
// downstream should be forced to treat it as blocking — its dependents are still
// attempted (if they template off its result, that's a normal template-resolution
// failure for them, not a scheduler decision).
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Scheduler runs workflows: resolving each step's executor, template-substituting
// its `with:` block from already-completed steps, executing it (with retries per
// its RetryPolicy), and persisting incremental progress via a ResultStore.
type Scheduler struct {
	store          ResultStore
	executors      map[string]Executor
	events         EventPublisher
	garageExecutor Executor
	logger         chassis.Logger
}

// NewScheduler builds the production Scheduler, wired to the one bench://
// executor Phase 3 ships (bench://grpc-call@v1) and, as of Phase 6/11, real
// garage:// plugin resolution against every currently-configured registry
// (garage_plugin.go) — resolver doubles as both the ServiceResolver
// grpc-call steps use and the PluginResolver garage:// steps use (see
// discovery.go's Resolver interface); registries is queried fresh on every
// garage:// step, not cached here, so a registry added/edited/removed
// through the Settings page takes effect without a restart (Phase 10c
// applied the same fix shape to webhook routing). Every other bench://
// executor (e.g. the not-yet-built bench://http-call@v1) still fails clearly
// at run time via executorFor below, rather than being silently unsupported.
// events publishes a CloudEvent on Catalyst for every run/step status transition
// (catalyst.go, Phase 7) — pass a *catalystPublisher for the real service, or
// noopEventPublisher{} anywhere that's not wanted/available.
func NewScheduler(store ResultStore, resolver Resolver, httpClient connect.HTTPClient, registries registryLister, events EventPublisher, logger chassis.Logger) *Scheduler {
	sched := newSchedulerWithExecutors(store, map[string]Executor{
		"bench://grpc-call@v1": NewGrpcCallExecutor(resolver),
		"bench://delay@v1":     NewDelayExecutor(),
	})
	sched.events = events
	sched.garageExecutor = NewGaragePluginExecutor(httpClient, registries, resolver)
	sched.logger = logger
	return sched
}

// newSchedulerWithExecutors builds a Scheduler against an arbitrary executor
// registry — used directly by tests to exercise DAG/concurrency/retry/templating
// behavior against fake executors, without a real network call in the loop. Event
// publishing defaults to a no-op so existing tests don't need a fake Catalyst
// connection to exercise the scheduler itself.
func newSchedulerWithExecutors(store ResultStore, executors map[string]Executor) *Scheduler {
	return &Scheduler{store: store, executors: executors, events: noopEventPublisher{}, logger: zerolog.New()}
}

// beginRun creates a new Run's identity and initial PENDING row and persists it
// synchronously, returning immediately once that single write completes — this is
// the piece of work that must finish before any caller (the -run CLI flag, the
// webhook handler, TriggerRun) can be told a run exists at all. Run and StartRun
// both start here; they differ only in what happens next: Run calls execute
// in-line and blocks on it, StartRun hands execute to a goroutine and returns
// immediately. See the doc's "webhook trigger and status API" section: "the
// trigger must return a run ID synchronously," not force the caller to wait on
// the whole workflow finishing.
func (s *Scheduler) beginRun(ctx context.Context, w *workflowv1.Workflow) (*workflowv1.Run, error) {
	run := &workflowv1.Run{
		RunId:        uuid.NewString(),
		WorkflowName: w.GetName(),
		Status:       workflowv1.RunStatus_RUN_STATUS_PENDING,
		StartedAt:    timestamppb.Now(),
	}
	if err := s.store.UpsertRun(ctx, run); err != nil {
		return nil, err
	}
	s.logger.WithField("workflow_name", w.GetName()).WithField("run_id", run.GetRunId()).Info("workflow triggered")
	return run, nil
}

// Run executes every step of w synchronously, respecting its dependency graph, and
// returns only once every step has finished (also persisted incrementally along the
// way via s.store). This is Phase 3's original entry point — the -run CLI flag and
// this package's own tests still use it directly, since blocking on the full
// workflow is exactly what a one-shot CLI invocation or a test assertion wants.
// TriggerRun and the webhook handler use StartRun instead — see its doc comment.
func (s *Scheduler) Run(ctx context.Context, w *workflowv1.Workflow) (*workflowv1.Run, error) {
	run, err := s.beginRun(ctx, w)
	if err != nil {
		return nil, err
	}
	return s.execute(ctx, run, w)
}

// StartRun begins a run of w and returns as soon as beginRun's single synchronous
// write completes — the run ID is available to the caller immediately, while step
// execution continues in a background goroutine. That goroutine runs against
// context.Background(), not ctx: ctx is typically an inbound HTTP request's
// context, which is canceled the instant the handler that called StartRun returns,
// and the run must keep going well past that point.
//
// A failure inside the detached goroutine (a step failing, or even a final
// persistence write failing) has no caller left to report to by the time it
// happens; execute already persists the run's terminal status itself as its last
// act, so the only thing StartRun's goroutine drops on the floor is a failure of
// that very last write — the same "best-effort, not surfaced further" persistence
// convention already established throughout this file (see runStep's comments).
func (s *Scheduler) StartRun(ctx context.Context, w *workflowv1.Workflow) (*workflowv1.Run, error) {
	run, err := s.beginRun(ctx, w)
	if err != nil {
		return nil, err
	}
	go func() {
		_, _ = s.execute(context.Background(), run, w)
	}()
	return run, nil
}

// execute runs every step of w to completion against the already-created run
// (moving it PENDING -> RUNNING -> a terminal status), respecting the dependency
// graph — the actual DAG walk, shared by both Run's synchronous and StartRun's
// asynchronous callers.
func (s *Scheduler) execute(ctx context.Context, run *workflowv1.Run, w *workflowv1.Workflow) (*workflowv1.Run, error) {
	// A run-level span rooted here, not in the TriggerRun/webhook RPC handler:
	// StartRun returns to that handler (closing its NewTraceInterceptor span)
	// the instant beginRun's one DB write finishes, well before any step has
	// run — see this scheduler's file comment and beginRun's doc comment. ctx
	// itself may or may not already carry a span (StartRun passes
	// context.Background(), so it won't; Run passes its caller's ctx, which
	// will if that caller is itself traced) — either way runSpan becomes the
	// root every step span below hangs off of.
	ctx, runSpan := chassis.StartSpan(ctx, "workflow:"+w.GetName())
	runSpan.SetAttribute("workflow_name", w.GetName())
	runSpan.SetAttribute("run_id", run.GetRunId())

	run.Status = workflowv1.RunStatus_RUN_STATUS_RUNNING
	if err := s.store.UpsertRun(ctx, run); err != nil {
		runSpan.End(err)
		return nil, err
	}
	s.events.RunStarted(run)

	steps := w.GetSteps()
	graph := effectiveDependsOn(steps)

	stepByName := make(map[string]*workflowv1.Step, len(steps))
	for _, st := range steps {
		stepByName[st.GetName()] = st
	}

	// done[name] closes once that step has a terminal StepResult recorded in
	// `results` — the synchronization primitive dependents block on before
	// deciding whether to run or be skipped.
	done := make(map[string]chan struct{}, len(steps))
	for _, st := range steps {
		done[st.GetName()] = make(chan struct{})
	}

	var (
		mu      sync.Mutex
		results = make(map[string]*workflowv1.StepResult, len(steps))
	)

	var wg sync.WaitGroup
	for _, st := range steps {
		wg.Add(1)
		go func(step *workflowv1.Step) {
			defer wg.Done()
			defer close(done[step.GetName()])

			blocked, blockedBy := waitForDependencies(graph[step.GetName()], done, &mu, results, stepByName)

			var sr *workflowv1.StepResult
			if blocked {
				sr = &workflowv1.StepResult{
					StepName:   step.GetName(),
					Status:     workflowv1.StepStatus_STEP_STATUS_SKIPPED,
					Error:      fmt.Sprintf("skipped: upstream dependency %q failed", blockedBy),
					StartedAt:  timestamppb.Now(),
					FinishedAt: timestamppb.Now(),
				}
				if err := s.store.UpsertStepResult(ctx, run.GetRunId(), sr); err != nil {
					// Persistence failures don't stop execution — Phase 3's
					// deliverable is correct in-memory pass/fail semantics with
					// best-effort incremental persistence, matching the "logged,
					// not fatal" precedent createSchema already sets for a
					// similar case in main.go.
					_ = err
				}
			} else {
				sr = s.runStep(ctx, run.GetRunId(), step, &mu, results)
			}
			s.logger.
				WithField("workflow_name", run.GetWorkflowName()).
				WithField("run_id", run.GetRunId()).
				WithField("step_name", sr.GetStepName()).
				WithField("status", sr.GetStatus().String()).
				Info("step completed")
			s.events.StepCompleted(run.GetRunId(), run.GetWorkflowName(), sr)

			mu.Lock()
			results[step.GetName()] = sr
			mu.Unlock()
		}(st)
	}
	wg.Wait()

	run.Steps = make([]*workflowv1.StepResult, 0, len(steps))
	runFailed := false
	for _, st := range steps {
		mu.Lock()
		sr := results[st.GetName()]
		mu.Unlock()
		run.Steps = append(run.Steps, sr)

		if sr.GetStatus() == workflowv1.StepStatus_STEP_STATUS_FAILED && st.GetOnFailure() != workflowv1.FailurePolicy_FAILURE_POLICY_CONTINUE {
			runFailed = true
		}
	}

	run.FinishedAt = timestamppb.Now()
	if runFailed {
		run.Status = workflowv1.RunStatus_RUN_STATUS_FAILED
	} else {
		run.Status = workflowv1.RunStatus_RUN_STATUS_PASSED
	}
	if err := s.store.UpsertRun(ctx, run); err != nil {
		runSpan.End(err)
		return run, err
	}

	if runFailed {
		s.logger.WithField("workflow_name", run.GetWorkflowName()).WithField("run_id", run.GetRunId()).Error("workflow failed")
		runSpan.End(fmt.Errorf("workflow %q failed", run.GetWorkflowName()))
	} else {
		s.logger.WithField("workflow_name", run.GetWorkflowName()).WithField("run_id", run.GetRunId()).Info("workflow succeeded")
		runSpan.End(nil)
	}
	s.events.RunFinished(run)

	return run, nil
}

// waitForDependencies blocks until every dependency in deps has a terminal result,
// then reports whether the calling step should be skipped because one of them was a
// hard failure (see the file comment for hard vs. soft failure).
func waitForDependencies(
	deps []string,
	done map[string]chan struct{},
	mu *sync.Mutex,
	results map[string]*workflowv1.StepResult,
	stepByName map[string]*workflowv1.Step,
) (blocked bool, blockedBy string) {
	for _, dep := range deps {
		<-done[dep]

		mu.Lock()
		depResult := results[dep]
		mu.Unlock()

		if depResult == nil {
			continue
		}
		if depResult.GetStatus() == workflowv1.StepStatus_STEP_STATUS_SKIPPED {
			return true, dep
		}
		if depResult.GetStatus() == workflowv1.StepStatus_STEP_STATUS_FAILED {
			depStep := stepByName[dep]
			if depStep.GetOnFailure() != workflowv1.FailurePolicy_FAILURE_POLICY_CONTINUE {
				return true, dep
			}
		}
	}
	return false, ""
}

// runStep executes one unblocked step to a terminal result: resolving its executor,
// template-resolving its `with:` from already-completed steps, and retrying per its
// RetryPolicy if on_failure is FAILURE_POLICY_RETRY. Persists a RUNNING row before
// starting and the terminal row once finished, per the brief's incremental-
// persistence requirement.
func (s *Scheduler) runStep(ctx context.Context, runID string, step *workflowv1.Step, mu *sync.Mutex, results map[string]*workflowv1.StepResult) *workflowv1.StepResult {
	// Child of execute's run-level span — see chassis.StartSpan's doc comment
	// for why a nested call like this automatically links to the parent
	// carried on ctx. This ctx (not the parameter above it) is what gets
	// passed to exec.Execute below, so bench://grpc-call@v1 and garage://
	// steps can read chassis.TraceParentHeader(ctx) and hand the trace off to
	// whatever they call — see grpc_call.go and garage_plugin.go.
	ctx, stepSpan := chassis.StartSpan(ctx, "step:"+step.GetName())
	stepSpan.SetAttribute("step_name", step.GetName())
	stepSpan.SetAttribute("uses", step.GetUses())

	startedAt := timestamppb.Now()
	if err := s.store.UpsertStepResult(ctx, runID, &workflowv1.StepResult{
		StepName:  step.GetName(),
		Status:    workflowv1.StepStatus_STEP_STATUS_RUNNING,
		StartedAt: startedAt,
	}); err != nil {
		_ = err // best-effort, see the matching comment in Run above
	}

	maxAttempts := 1
	var backoff time.Duration
	if step.GetOnFailure() == workflowv1.FailurePolicy_FAILURE_POLICY_RETRY {
		if attempts := step.GetRetry().GetAttempts(); attempts > 0 {
			maxAttempts = int(attempts)
		}
		if d, err := time.ParseDuration(step.GetRetry().GetBackoff()); err == nil {
			backoff = d
		}
	}

	var (
		request    *structpb.Struct
		result     *structpb.Struct
		detail     *structpb.Struct
		passed     bool
		failReason string
	)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		mu.Lock()
		snapshot := make(map[string]*workflowv1.StepResult, len(results))
		for k, v := range results {
			snapshot[k] = v
		}
		mu.Unlock()

		resolvedWith, err := resolveTemplates(step.GetWith(), snapshot)
		if err != nil {
			passed = false
			failReason = err.Error()
			// A templating error is a configuration problem, not a transient
			// one — retrying it would just fail identically every time. No
			// request to record: templating never produced one to send.
			request = nil
			break
		}
		// Recorded regardless of what happens next: this is what was actually
		// sent, and is exactly what's needed to troubleshoot a step that then
		// fails for any other reason (a bad response, a failed assertion).
		request = resolvedWith

		exec, err := s.executorFor(step.GetUses())
		if err != nil {
			passed = false
			failReason = err.Error()
			break
		}

		execResult, err := exec.Execute(ctx, step, resolvedWith)
		if err != nil {
			passed = false
			failReason = err.Error()
			result = nil
			detail = nil
		} else {
			result = execResult.Result
			detail = execResult.Detail
			passed = execResult.Passed
			failReason = execResult.FailureReason
		}

		if passed {
			break
		}
		if attempt < maxAttempts && backoff > 0 {
			time.Sleep(backoff)
		}
	}

	status := workflowv1.StepStatus_STEP_STATUS_PASSED
	errMsg := ""
	if !passed {
		status = workflowv1.StepStatus_STEP_STATUS_FAILED
		errMsg = failReason
	}

	sr := &workflowv1.StepResult{
		StepName:   step.GetName(),
		Status:     status,
		Request:    request,
		Result:     result,
		Detail:     detail,
		Error:      errMsg,
		StartedAt:  startedAt,
		FinishedAt: timestamppb.Now(),
	}
	if err := s.store.UpsertStepResult(ctx, runID, sr); err != nil {
		_ = err // best-effort, see the matching comment in Run above
	}

	var spanErr error
	if !passed {
		spanErr = errors.New(failReason)
	}
	stepSpan.End(spanErr)

	return sr
}

// executorFor resolves a step's `uses:` reference to the Executor that should run
// it. garage:// resolves through garageExecutor (garage_plugin.go, Phase 6); any
// bench:// reference other than the one executor this phase ships fails clearly
// rather than hanging or no-oping, so a workflow author gets an actionable error
// instead of silent misbehavior.
func (s *Scheduler) executorFor(uses string) (Executor, error) {
	scheme, _, ok := strings.Cut(uses, "://")
	if !ok {
		return nil, fmt.Errorf("step uses %q: missing scheme", uses)
	}

	switch scheme {
	case "garage":
		if s.garageExecutor == nil {
			return nil, fmt.Errorf("step uses %q: this scheduler was not constructed with a garage:// executor", uses)
		}
		return s.garageExecutor, nil
	case "bench":
		exec, ok := s.executors[uses]
		if !ok {
			return nil, fmt.Errorf("step uses %q: unsupported bench:// executor (only bench://grpc-call@v1 is implemented in this phase)", uses)
		}
		return exec, nil
	default:
		return nil, fmt.Errorf("step uses %q: unsupported scheme %q", uses, scheme)
	}
}
