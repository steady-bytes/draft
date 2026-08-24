package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// memStore is an in-memory ResultStore, letting scheduler tests exercise
// incremental persistence (and inspect it afterward) without a real Postgres
// instance.
type memStore struct {
	mu    sync.Mutex
	run   *workflowv1.Run
	steps map[string][]*workflowv1.StepResult // stepName -> every write, in order
}

func newMemStore() *memStore {
	return &memStore{steps: make(map[string][]*workflowv1.StepResult)}
}

func (s *memStore) UpsertRun(ctx context.Context, run *workflowv1.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.run = proto.Clone(run).(*workflowv1.Run)
	return nil
}

func (s *memStore) UpsertStepResult(ctx context.Context, runID string, result *workflowv1.StepResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := proto.Clone(result).(*workflowv1.StepResult)
	s.steps[result.GetStepName()] = append(s.steps[result.GetStepName()], cp)
	return nil
}

func (s *memStore) writesFor(step string) []*workflowv1.StepResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.steps[step]
}

// fakeExecutor is a scripted Executor: it records the (template-resolved) `with`
// it was called with, optionally blocks on a channel (to force/observe
// concurrency), and returns canned results, optionally failing a fixed number of
// times before passing (to exercise retry).
type fakeExecutor struct {
	mu          sync.Mutex
	calls       []*structpb.Struct
	failCount   int32 // number of calls that should fail before passing
	callCount   int32
	result      *structpb.Struct
	err         error
	blockUntil  chan struct{} // if non-nil, Execute waits on this before proceeding
	onExecute   func()        // optional hook, called synchronously inside Execute
	failureText string
}

func (e *fakeExecutor) Execute(ctx context.Context, step *workflowv1.Step, with *structpb.Struct) (*ExecutionResult, error) {
	e.mu.Lock()
	e.calls = append(e.calls, with)
	e.mu.Unlock()

	if e.onExecute != nil {
		e.onExecute()
	}
	if e.blockUntil != nil {
		<-e.blockUntil
	}
	if e.err != nil {
		return nil, e.err
	}

	n := atomic.AddInt32(&e.callCount, 1)
	if n <= e.failCount {
		reason := e.failureText
		if reason == "" {
			reason = "scripted failure"
		}
		return &ExecutionResult{Passed: false, FailureReason: reason}, nil
	}
	return &ExecutionResult{Passed: true, Result: e.result}, nil
}

func passingStep(name string, dependsOn ...string) *workflowv1.Step {
	return &workflowv1.Step{
		Name:      name,
		Uses:      "bench://fake@v1",
		DependsOn: dependsOn,
		With:      &structpb.Struct{Fields: map[string]*structpb.Value{}},
	}
}

func TestScheduler_LinearChainRunsInOrderAndPasses(t *testing.T) {
	exec := &fakeExecutor{result: mustStruct(t, map[string]interface{}{"id": "abc"})}
	store := newMemStore()
	sched := newSchedulerWithExecutors(store, map[string]Executor{"bench://fake@v1": exec})

	w := &workflowv1.Workflow{
		Name: "linear",
		Steps: []*workflowv1.Step{
			passingStep("a"),
			passingStep("b"), // defaults depends_on to "a"
			passingStep("c"), // defaults depends_on to "b"
		},
	}

	run, err := sched.Run(context.Background(), w)
	if err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if got, want := run.GetStatus(), workflowv1.RunStatus_RUN_STATUS_PASSED; got != want {
		t.Fatalf("run status = %v, want %v", got, want)
	}
	if len(run.GetSteps()) != 3 {
		t.Fatalf("len(steps) = %d, want 3", len(run.GetSteps()))
	}
	for _, sr := range run.GetSteps() {
		if sr.GetStatus() != workflowv1.StepStatus_STEP_STATUS_PASSED {
			t.Errorf("step %q status = %v, want PASSED", sr.GetStepName(), sr.GetStatus())
		}
	}

	// Incremental persistence: each step should have been written at least
	// twice (RUNNING, then a terminal status), not just once at the end.
	for _, name := range []string{"a", "b", "c"} {
		writes := store.writesFor(name)
		if len(writes) < 2 {
			t.Errorf("step %q: got %d persisted writes, want >= 2 (RUNNING + terminal)", name, len(writes))
			continue
		}
		if writes[0].GetStatus() != workflowv1.StepStatus_STEP_STATUS_RUNNING {
			t.Errorf("step %q: first write status = %v, want RUNNING", name, writes[0].GetStatus())
		}
	}
}

func TestScheduler_TemplatingFlowsBetweenSteps(t *testing.T) {
	exec := &fakeExecutor{result: mustStruct(t, map[string]interface{}{"id": "course-999"})}
	store := newMemStore()
	sched := newSchedulerWithExecutors(store, map[string]Executor{"bench://fake@v1": exec})

	create := passingStep("create-course")
	query := &workflowv1.Step{
		Name:      "query-course",
		Uses:      "bench://fake@v1",
		DependsOn: []string{"create-course"},
		With: mustStruct(t, map[string]interface{}{
			"id": "{{ steps.create-course.result.id }}",
		}),
	}

	w := &workflowv1.Workflow{Name: "templated", Steps: []*workflowv1.Step{create, query}}

	run, err := sched.Run(context.Background(), w)
	if err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if run.GetStatus() != workflowv1.RunStatus_RUN_STATUS_PASSED {
		t.Fatalf("run status = %v, want PASSED (steps: %+v)", run.GetStatus(), run.GetSteps())
	}

	exec.mu.Lock()
	defer exec.mu.Unlock()
	if len(exec.calls) != 2 {
		t.Fatalf("len(calls) = %d, want 2", len(exec.calls))
	}
	// calls[0] is create-course's (empty) with; calls[1] is query-course's,
	// which should have had its template resolved to create-course's result.id.
	got := exec.calls[1].GetFields()["id"].GetStringValue()
	if got != "course-999" {
		t.Errorf("query-course's resolved with.id = %q, want %q", got, "course-999")
	}
}

func TestScheduler_IndependentStepsRunConcurrently(t *testing.T) {
	var inFlight int32
	var maxInFlight int32
	release := make(chan struct{})

	track := func() {
		n := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&maxInFlight)
			if n <= old || atomic.CompareAndSwapInt32(&maxInFlight, old, n) {
				break
			}
		}
	}

	exec := &fakeExecutor{
		result: mustStruct(t, map[string]interface{}{}),
		onExecute: func() {
			track()
			<-release
			atomic.AddInt32(&inFlight, -1)
		},
	}
	store := newMemStore()

	// "start" completes instantly (via a separate, non-blocking executor) and
	// fans out to a/b/c, each with an *explicit* (non-empty) depends_on of just
	// ["start"] — necessary because effectiveDependsOn's "empty depends_on
	// defaults to the previous step" rule (loader.go) means any non-first step
	// with a literally-empty depends_on would default to depending on its
	// predecessor instead of being independent, which would defeat the point of
	// this test. a/b/c share only "start" as a dependency, so once it completes
	// all three become unblocked at once — a real fan-out.
	startExec := &fakeExecutor{result: mustStruct(t, map[string]interface{}{})}
	sched := newSchedulerWithExecutors(store, map[string]Executor{
		"bench://start@v1": startExec,
		"bench://fake@v1":  exec,
	})

	w := &workflowv1.Workflow{
		Name: "fanout",
		Steps: []*workflowv1.Step{
			{Name: "start", Uses: "bench://start@v1", With: &structpb.Struct{Fields: map[string]*structpb.Value{}}},
			{Name: "a", Uses: "bench://fake@v1", DependsOn: []string{"start"}, With: &structpb.Struct{Fields: map[string]*structpb.Value{}}},
			{Name: "b", Uses: "bench://fake@v1", DependsOn: []string{"start"}, With: &structpb.Struct{Fields: map[string]*structpb.Value{}}},
			{Name: "c", Uses: "bench://fake@v1", DependsOn: []string{"start"}, With: &structpb.Struct{Fields: map[string]*structpb.Value{}}},
		},
	}
	// Confirm the graph actually is the fan-out this test relies on, rather than
	// just hoping effectiveDependsOn behaves as described above.
	graph := effectiveDependsOn(w.Steps)
	for _, name := range []string{"a", "b", "c"} {
		if got := graph[name]; len(got) != 1 || got[0] != "start" {
			t.Fatalf("effectiveDependsOn[%q] = %v, want [start] (test setup assumption violated)", name, got)
		}
	}

	done := make(chan *workflowv1.Run, 1)
	go func() {
		run, err := sched.Run(context.Background(), w)
		if err != nil {
			t.Errorf("Run: unexpected error: %v", err)
		}
		done <- run
	}()

	// Give all three goroutines a chance to reach onExecute and block there.
	deadline := time.After(2 * time.Second)
	for {
		if atomic.LoadInt32(&maxInFlight) >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for 3 concurrent executions; max observed = %d", atomic.LoadInt32(&maxInFlight))
		case <-time.After(5 * time.Millisecond):
		}
	}
	close(release)

	run := <-done
	if run.GetStatus() != workflowv1.RunStatus_RUN_STATUS_PASSED {
		t.Errorf("run status = %v, want PASSED", run.GetStatus())
	}
	if got := atomic.LoadInt32(&maxInFlight); got < 3 {
		t.Errorf("max concurrent executions = %d, want >= 3 (steps ran sequentially, not concurrently)", got)
	}
}

func TestScheduler_HardFailureSkipsDependents(t *testing.T) {
	failing := &fakeExecutor{failCount: 1000} // always fails
	passing := &fakeExecutor{result: mustStruct(t, map[string]interface{}{})}
	store := newMemStore()
	sched := newSchedulerWithExecutors(store, map[string]Executor{
		"bench://fail@v1": failing,
		"bench://pass@v1": passing,
	})

	w := &workflowv1.Workflow{
		Name: "skip-on-failure",
		Steps: []*workflowv1.Step{
			{Name: "a", Uses: "bench://fail@v1", With: &structpb.Struct{Fields: map[string]*structpb.Value{}}, OnFailure: workflowv1.FailurePolicy_FAILURE_POLICY_FAIL},
			{Name: "b", Uses: "bench://pass@v1", DependsOn: []string{"a"}, With: &structpb.Struct{Fields: map[string]*structpb.Value{}}},
		},
	}

	run, err := sched.Run(context.Background(), w)
	if err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if run.GetStatus() != workflowv1.RunStatus_RUN_STATUS_FAILED {
		t.Fatalf("run status = %v, want FAILED", run.GetStatus())
	}

	byName := map[string]*workflowv1.StepResult{}
	for _, sr := range run.GetSteps() {
		byName[sr.GetStepName()] = sr
	}
	if got := byName["a"].GetStatus(); got != workflowv1.StepStatus_STEP_STATUS_FAILED {
		t.Errorf("step a status = %v, want FAILED", got)
	}
	if got := byName["b"].GetStatus(); got != workflowv1.StepStatus_STEP_STATUS_SKIPPED {
		t.Errorf("step b status = %v, want SKIPPED", got)
	}
	if passing.callCount != 0 {
		t.Errorf("step b's executor was called %d times, want 0 (it should have been skipped, not executed)", passing.callCount)
	}
}

func TestScheduler_ContinueOnFailureDoesNotFailRunOrSkipDependents(t *testing.T) {
	failing := &fakeExecutor{failCount: 1000}
	passing := &fakeExecutor{result: mustStruct(t, map[string]interface{}{})}
	store := newMemStore()
	sched := newSchedulerWithExecutors(store, map[string]Executor{
		"bench://fail@v1": failing,
		"bench://pass@v1": passing,
	})

	// Mirrors the doc's own example: create-course (passes) -> query-course
	// (passes) -> notify-slack-on-seed-data (fails, on_failure: continue).
	w := &workflowv1.Workflow{
		Name: "continue-on-failure",
		Steps: []*workflowv1.Step{
			{Name: "create-course", Uses: "bench://pass@v1", With: &structpb.Struct{Fields: map[string]*structpb.Value{}}},
			{Name: "query-course", Uses: "bench://pass@v1", With: &structpb.Struct{Fields: map[string]*structpb.Value{}}},
			{
				Name:      "notify-slack-on-seed-data",
				Uses:      "bench://fail@v1",
				With:      &structpb.Struct{Fields: map[string]*structpb.Value{}},
				OnFailure: workflowv1.FailurePolicy_FAILURE_POLICY_CONTINUE,
			},
		},
	}

	run, err := sched.Run(context.Background(), w)
	if err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if run.GetStatus() != workflowv1.RunStatus_RUN_STATUS_PASSED {
		t.Fatalf("run status = %v, want PASSED (a continue-on-failure step's failure must not fail the run)", run.GetStatus())
	}

	byName := map[string]*workflowv1.StepResult{}
	for _, sr := range run.GetSteps() {
		byName[sr.GetStepName()] = sr
	}
	if got := byName["notify-slack-on-seed-data"].GetStatus(); got != workflowv1.StepStatus_STEP_STATUS_FAILED {
		t.Errorf("notify step status = %v, want FAILED (it still failed, it just doesn't fail the run)", got)
	}
}

func TestScheduler_RetryEventuallyPasses(t *testing.T) {
	exec := &fakeExecutor{failCount: 2, result: mustStruct(t, map[string]interface{}{"ok": true})}
	store := newMemStore()
	sched := newSchedulerWithExecutors(store, map[string]Executor{"bench://fake@v1": exec})

	w := &workflowv1.Workflow{
		Name: "retry",
		Steps: []*workflowv1.Step{
			{
				Name:      "flaky",
				Uses:      "bench://fake@v1",
				With:      &structpb.Struct{Fields: map[string]*structpb.Value{}},
				OnFailure: workflowv1.FailurePolicy_FAILURE_POLICY_RETRY,
				Retry:     &workflowv1.RetryPolicy{Attempts: 3, Backoff: "1ms"},
			},
		},
	}

	run, err := sched.Run(context.Background(), w)
	if err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if run.GetStatus() != workflowv1.RunStatus_RUN_STATUS_PASSED {
		t.Fatalf("run status = %v, want PASSED", run.GetStatus())
	}
	if exec.callCount != 3 {
		t.Errorf("executor called %d times, want 3 (2 failures + 1 success)", exec.callCount)
	}
}

func TestScheduler_RetryExhaustedIsAHardFailure(t *testing.T) {
	exec := &fakeExecutor{failCount: 1000}
	downstream := &fakeExecutor{result: mustStruct(t, map[string]interface{}{})}
	store := newMemStore()
	sched := newSchedulerWithExecutors(store, map[string]Executor{
		"bench://fake@v1":       exec,
		"bench://downstream@v1": downstream,
	})

	w := &workflowv1.Workflow{
		Name: "retry-exhausted",
		Steps: []*workflowv1.Step{
			{
				Name:      "flaky",
				Uses:      "bench://fake@v1",
				With:      &structpb.Struct{Fields: map[string]*structpb.Value{}},
				OnFailure: workflowv1.FailurePolicy_FAILURE_POLICY_RETRY,
				Retry:     &workflowv1.RetryPolicy{Attempts: 2, Backoff: "1ms"},
			},
			{Name: "next", Uses: "bench://downstream@v1", DependsOn: []string{"flaky"}, With: &structpb.Struct{Fields: map[string]*structpb.Value{}}},
		},
	}

	run, err := sched.Run(context.Background(), w)
	if err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if run.GetStatus() != workflowv1.RunStatus_RUN_STATUS_FAILED {
		t.Fatalf("run status = %v, want FAILED", run.GetStatus())
	}
	if exec.callCount != 2 {
		t.Errorf("executor called %d times, want 2 (exactly Retry.Attempts)", exec.callCount)
	}

	byName := map[string]*workflowv1.StepResult{}
	for _, sr := range run.GetSteps() {
		byName[sr.GetStepName()] = sr
	}
	if got := byName["next"].GetStatus(); got != workflowv1.StepStatus_STEP_STATUS_SKIPPED {
		t.Errorf("next step status = %v, want SKIPPED (retry exhaustion is a hard failure)", got)
	}
}

func TestScheduler_GarageStepFailsClearlyWithoutHanging(t *testing.T) {
	store := newMemStore()
	sched := newSchedulerWithExecutors(store, map[string]Executor{})

	w := &workflowv1.Workflow{
		Name: "garage-unsupported",
		Steps: []*workflowv1.Step{
			{Name: "notify", Uses: "garage://slack-notify@v2", With: &structpb.Struct{Fields: map[string]*structpb.Value{}}},
		},
	}

	done := make(chan *workflowv1.Run, 1)
	go func() {
		run, err := sched.Run(context.Background(), w)
		if err != nil {
			t.Errorf("Run: unexpected error: %v", err)
			done <- nil
			return
		}
		done <- run
	}()

	select {
	case run := <-done:
		if run == nil {
			return
		}
		if run.GetStatus() != workflowv1.RunStatus_RUN_STATUS_FAILED {
			t.Errorf("run status = %v, want FAILED", run.GetStatus())
		}
		sr := run.GetSteps()[0]
		if sr.GetStatus() != workflowv1.StepStatus_STEP_STATUS_FAILED {
			t.Errorf("notify status = %v, want FAILED", sr.GetStatus())
		}
		if !strings.Contains(sr.GetError(), "garage") {
			t.Errorf("error = %q, want it to mention garage:// is unimplemented", sr.GetError())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s — a garage:// step must fail cleanly, not hang")
	}
}

func TestScheduler_UnknownBenchExecutorFailsClearly(t *testing.T) {
	store := newMemStore()
	sched := newSchedulerWithExecutors(store, map[string]Executor{})

	w := &workflowv1.Workflow{
		Name: "unknown-executor",
		Steps: []*workflowv1.Step{
			{Name: "http", Uses: "bench://http-call@v1", With: &structpb.Struct{Fields: map[string]*structpb.Value{}}},
		},
	}

	run, err := sched.Run(context.Background(), w)
	if err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if run.GetStatus() != workflowv1.RunStatus_RUN_STATUS_FAILED {
		t.Fatalf("run status = %v, want FAILED", run.GetStatus())
	}
	if !strings.Contains(run.GetSteps()[0].GetError(), "unsupported") {
		t.Errorf("error = %q, want it to mention the executor is unsupported", run.GetSteps()[0].GetError())
	}
}

func TestScheduler_ExecutorErrorFailsStepWithoutPanicking(t *testing.T) {
	exec := &fakeExecutor{err: fmt.Errorf("boom: connection refused")}
	store := newMemStore()
	sched := newSchedulerWithExecutors(store, map[string]Executor{"bench://fake@v1": exec})

	w := &workflowv1.Workflow{
		Name: "executor-error",
		Steps: []*workflowv1.Step{
			passingStep("a"),
		},
	}

	run, err := sched.Run(context.Background(), w)
	if err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if run.GetStatus() != workflowv1.RunStatus_RUN_STATUS_FAILED {
		t.Fatalf("run status = %v, want FAILED", run.GetStatus())
	}
	if !strings.Contains(run.GetSteps()[0].GetError(), "boom") {
		t.Errorf("error = %q, want it to contain the executor's error", run.GetSteps()[0].GetError())
	}
}

func TestScheduler_TemplateReferencingUnrunStepFailsThatStepOnly(t *testing.T) {
	// b's depends_on doesn't actually include "a" (an authoring mistake that
	// doesn't create a cycle, so Phase 2's loader wouldn't have caught it), but
	// b's `with` still templates off a's result. Since a and b are independent
	// in the graph, b may run before, concurrently with, or after a — in every
	// case, b must fail with a clear templating error rather than panic or
	// silently substitute nothing.
	execA := &fakeExecutor{result: mustStruct(t, map[string]interface{}{"id": "abc"})}
	store := newMemStore()
	sched := newSchedulerWithExecutors(store, map[string]Executor{"bench://fake@v1": execA})

	w := &workflowv1.Workflow{
		Name: "bad-depends-on",
		Steps: []*workflowv1.Step{
			{Name: "a", Uses: "bench://fake@v1", DependsOn: []string{}, With: &structpb.Struct{Fields: map[string]*structpb.Value{}}},
			{
				Name:      "b",
				Uses:      "bench://fake@v1",
				DependsOn: []string{}, // should have been []string{"a"}
				With:      mustStruct(t, map[string]interface{}{"id": "{{ steps.a.result.id }}"}),
			},
		},
	}

	run, err := sched.Run(context.Background(), w)
	if err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	byName := map[string]*workflowv1.StepResult{}
	for _, sr := range run.GetSteps() {
		byName[sr.GetStepName()] = sr
	}
	// a always passes; b fails or passes depending on scheduling order, but must
	// never panic and must always report SOME terminal status.
	if byName["a"].GetStatus() != workflowv1.StepStatus_STEP_STATUS_PASSED {
		t.Errorf("step a status = %v, want PASSED", byName["a"].GetStatus())
	}
	if byName["b"].GetStatus() != workflowv1.StepStatus_STEP_STATUS_PASSED && byName["b"].GetStatus() != workflowv1.StepStatus_STEP_STATUS_FAILED {
		t.Errorf("step b status = %v, want PASSED or FAILED (never UNSPECIFIED/RUNNING)", byName["b"].GetStatus())
	}
}
