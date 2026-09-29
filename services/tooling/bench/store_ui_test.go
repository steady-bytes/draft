package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// The tests below run against the shared test database, so each uses its own workflow names and
// starts its runs in a year no other test uses (2090 and later), which also lets RunCounts be
// asserted exactly.
func seedUIRun(t *testing.T, store *pgResultStore, id, workflow string, status workflowv1.RunStatus, started time.Time, took time.Duration, steps ...*workflowv1.StepResult) {
	t.Helper()
	run := &workflowv1.Run{RunId: id, WorkflowName: workflow, Status: status, StartedAt: timestamppb.New(started)}
	if took > 0 {
		run.FinishedAt = timestamppb.New(started.Add(took))
	}
	if err := store.UpsertRun(context.Background(), run); err != nil {
		t.Fatalf("UpsertRun %s: %v", id, err)
	}
	for _, s := range steps {
		if err := store.UpsertStepResult(context.Background(), id, s); err != nil {
			t.Fatalf("UpsertStepResult %s/%s: %v", id, s.GetStepName(), err)
		}
	}
}

func TestRunRecords_FilterAndPaginate(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	wf := "uiq-filter-" + t.Name()
	base := time.Date(2091, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		status := workflowv1.RunStatus_RUN_STATUS_PASSED
		if i == 3 {
			status = workflowv1.RunStatus_RUN_STATUS_FAILED
		}
		seedUIRun(t, store, fmt.Sprintf("%s-%d", wf, i), wf, status, base.Add(time.Duration(i)*time.Hour), time.Second)
	}
	seedUIRun(t, store, wf+"-live", wf, workflowv1.RunStatus_RUN_STATUS_RUNNING, base.Add(9*time.Hour), 0)

	all, next, err := store.RunRecords(ctx, runFilter{Workflow: wf}, 4, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 || next == "" || all[0].RunID != wf+"-live" || all[1].RunID != wf+"-4" {
		t.Fatalf("first page (newest first) = %+v next=%q", all, next)
	}
	if !all[0].InFlight() || all[0].Finished != (time.Time{}) {
		t.Errorf("the in-flight run should have no finish time: %+v", all[0])
	}
	rest, next, err := store.RunRecords(ctx, runFilter{Workflow: wf}, 4, next)
	if err != nil || next != "" || len(rest) != 2 || rest[1].RunID != wf+"-0" {
		t.Fatalf("second page = %+v next=%q err=%v", rest, next, err)
	}

	failed, _, _ := store.RunRecords(ctx, runFilter{Workflow: wf, Status: "failed"}, 10, "")
	if len(failed) != 1 || failed[0].RunID != wf+"-3" || !failed[0].Failed() {
		t.Errorf("failed filter = %+v", failed)
	}
	live, _, _ := store.RunRecords(ctx, runFilter{Workflow: wf, Status: "running"}, 10, "")
	if len(live) != 1 || live[0].RunID != wf+"-live" {
		t.Errorf("running filter = %+v", live)
	}
	recent, _, _ := store.RunRecords(ctx, runFilter{Workflow: wf, Since: base.Add(3 * time.Hour)}, 10, "")
	if len(recent) != 3 { // runs 3, 4 and the live one
		t.Errorf("since filter = %+v", recent)
	}
	if d, ok := recent[1].Duration(); !ok || d != time.Second {
		t.Errorf("duration = %v %v", d, ok)
	}
}

func TestLastRunRecords_TakesTheNewestNPerWorkflow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	a, b := "uil-a-"+t.Name(), "uil-b-"+t.Name()
	base := time.Date(2092, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		seedUIRun(t, store, fmt.Sprintf("%s-%d", a, i), a, workflowv1.RunStatus_RUN_STATUS_PASSED, base.Add(time.Duration(i)*time.Minute), time.Second)
	}
	seedUIRun(t, store, b+"-0", b, workflowv1.RunStatus_RUN_STATUS_FAILED, base, time.Second)

	got, err := store.LastRunRecords(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got[a]) != 3 || got[a][0].RunID != a+"-4" || got[a][2].RunID != a+"-2" {
		t.Errorf("workflow a: %+v", got[a])
	}
	if len(got[b]) != 1 || !got[b][0].Failed() {
		t.Errorf("workflow b: %+v", got[b])
	}
}

func TestRunCounts_ByOutcome(t *testing.T) {
	store := newTestStore(t)
	wf := "uic-" + t.Name()
	base := time.Date(2093, 1, 1, 0, 0, 0, 0, time.UTC)
	seedUIRun(t, store, wf+"-1", wf, workflowv1.RunStatus_RUN_STATUS_PASSED, base, time.Second)
	seedUIRun(t, store, wf+"-2", wf, workflowv1.RunStatus_RUN_STATUS_PASSED, base.Add(time.Minute), time.Second)
	seedUIRun(t, store, wf+"-3", wf, workflowv1.RunStatus_RUN_STATUS_FAILED, base.Add(2*time.Minute), time.Second)
	seedUIRun(t, store, wf+"-4", wf, workflowv1.RunStatus_RUN_STATUS_RUNNING, base.Add(3*time.Minute), 0)
	seedUIRun(t, store, wf+"-5", wf, workflowv1.RunStatus_RUN_STATUS_PENDING, base.Add(4*time.Minute), 0)

	got, err := store.RunCounts(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	if want := (runCounts{Total: 5, Passed: 2, Failed: 1, InFlight: 2}); got != want {
		t.Errorf("counts = %+v, want %+v", got, want)
	}
	// since is a lower bound on the start time.
	if got, _ := store.RunCounts(context.Background(), base.Add(2*time.Minute)); got.Total != 3 {
		t.Errorf("since: %+v", got)
	}
}

func TestFailedSteps_NamesTheFirstFailure(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	wf := "uif-" + t.Name()
	base := time.Date(2094, 1, 1, 0, 0, 0, 0, time.UTC)
	step := func(name string, status workflowv1.StepStatus, at time.Duration) *workflowv1.StepResult {
		return &workflowv1.StepResult{StepName: name, Status: status, StartedAt: timestamppb.New(base.Add(at)), FinishedAt: timestamppb.New(base.Add(at + time.Second))}
	}
	seedUIRun(t, store, wf+"-bad", wf, workflowv1.RunStatus_RUN_STATUS_FAILED, base, time.Minute,
		step("first", workflowv1.StepStatus_STEP_STATUS_PASSED, 0),
		step("second", workflowv1.StepStatus_STEP_STATUS_FAILED, time.Second),
		step("third", workflowv1.StepStatus_STEP_STATUS_FAILED, 2*time.Second),
	)
	seedUIRun(t, store, wf+"-good", wf, workflowv1.RunStatus_RUN_STATUS_PASSED, base, time.Minute,
		step("only", workflowv1.StepStatus_STEP_STATUS_PASSED, 0))

	got, err := store.FailedSteps(ctx, []string{wf + "-bad", wf + "-good", wf + "-missing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[wf+"-bad"] != "second" {
		t.Errorf("failed steps = %v", got)
	}
	if got, err := store.FailedSteps(ctx, nil); err != nil || len(got) != 0 {
		t.Errorf("no ids: %v %v", got, err)
	}
}
