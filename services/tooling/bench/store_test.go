package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestWorkflowStore_UpsertGetList(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	name := "store-test-upsert-workflow"
	w := &workflowv1.Workflow{
		Name:        name,
		Description: "first version",
		Trigger: &workflowv1.Trigger{
			Webhook: &workflowv1.WebhookTrigger{
				Slug:            "store-test-slug",
				SignatureHeader: "X-Bench-Signature",
				SecretRef:       "blueprint://secrets/bench/store-test",
			},
		},
		Steps: []*workflowv1.Step{
			{Name: "step-a", Uses: "bench://grpc-call@v1"},
		},
	}

	if err := store.UpsertWorkflow(ctx, w); err != nil {
		t.Fatalf("UpsertWorkflow (insert) returned unexpected error: %v", err)
	}

	got, err := store.GetWorkflow(ctx, name)
	if err != nil {
		t.Fatalf("GetWorkflow returned unexpected error: %v", err)
	}
	if got.GetDescription() != "first version" {
		t.Errorf("Description = %q, want %q", got.GetDescription(), "first version")
	}
	if got.GetTrigger().GetWebhook().GetSlug() != "store-test-slug" {
		t.Errorf("webhook slug = %q, want %q", got.GetTrigger().GetWebhook().GetSlug(), "store-test-slug")
	}

	// A workflow file that changed on disk since the last restart should update
	// its stored definition on the next upsert, not fail on the (name) primary
	// key — this is the "upsert, not insert" requirement from the brief.
	w.Description = "second version"
	w.Steps = append(w.Steps, &workflowv1.Step{Name: "step-b", Uses: "bench://grpc-call@v1", DependsOn: []string{"step-a"}})
	if err := store.UpsertWorkflow(ctx, w); err != nil {
		t.Fatalf("UpsertWorkflow (update) returned unexpected error: %v", err)
	}

	got, err = store.GetWorkflow(ctx, name)
	if err != nil {
		t.Fatalf("GetWorkflow after update returned unexpected error: %v", err)
	}
	if got.GetDescription() != "second version" {
		t.Errorf("Description after update = %q, want %q", got.GetDescription(), "second version")
	}
	if len(got.GetSteps()) != 2 {
		t.Errorf("len(Steps) after update = %d, want 2", len(got.GetSteps()))
	}

	workflows, err := store.ListWorkflows(ctx)
	if err != nil {
		t.Fatalf("ListWorkflows returned unexpected error: %v", err)
	}
	found := false
	for _, w := range workflows {
		if w.GetName() == name {
			found = true
		}
	}
	if !found {
		t.Errorf("ListWorkflows did not include %q", name)
	}
}

func TestWorkflowStore_GetWorkflow_NotFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.GetWorkflow(context.Background(), "store-test-does-not-exist")
	if !errors.Is(err, ErrWorkflowNotFound) {
		t.Fatalf("GetWorkflow error = %v, want ErrWorkflowNotFound", err)
	}
}

func TestRunStore_UpsertRunAndStepResults_GetRunRoundTrips(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	run := &workflowv1.Run{
		RunId:        "store-test-run-roundtrip",
		WorkflowName: "store-test-workflow",
		Status:       workflowv1.RunStatus_RUN_STATUS_RUNNING,
		StartedAt:    timestamppb.Now(),
	}
	if err := store.UpsertRun(ctx, run); err != nil {
		t.Fatalf("UpsertRun returned unexpected error: %v", err)
	}

	step := &workflowv1.StepResult{
		StepName:   "create-thing",
		Status:     workflowv1.StepStatus_STEP_STATUS_PASSED,
		Result:     mustStruct(t, map[string]interface{}{"id": "abc-123"}),
		StartedAt:  timestamppb.Now(),
		FinishedAt: timestamppb.Now(),
	}
	if err := store.UpsertStepResult(ctx, run.GetRunId(), step); err != nil {
		t.Fatalf("UpsertStepResult returned unexpected error: %v", err)
	}

	run.Status = workflowv1.RunStatus_RUN_STATUS_PASSED
	run.FinishedAt = timestamppb.Now()
	if err := store.UpsertRun(ctx, run); err != nil {
		t.Fatalf("UpsertRun (final) returned unexpected error: %v", err)
	}

	got, err := store.GetRun(ctx, run.GetRunId())
	if err != nil {
		t.Fatalf("GetRun returned unexpected error: %v", err)
	}
	if got.GetStatus() != workflowv1.RunStatus_RUN_STATUS_PASSED {
		t.Errorf("Status = %s, want PASSED", got.GetStatus())
	}
	if len(got.GetSteps()) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(got.GetSteps()))
	}
	if id := got.GetSteps()[0].GetResult().GetFields()["id"].GetStringValue(); id != "abc-123" {
		t.Errorf("step result id = %q, want %q", id, "abc-123")
	}
}

// TestRunStore_GetRun_StepWithNoResultRoundTrips is a regression test for a bug
// found live while proving Phase 4/5 end to end: json.RawMessage(nil) marshals to
// the literal 4-byte JSON value `null` (its documented behavior), and bun's jsonb
// appender writes that verbatim into the column rather than SQL NULL — confirmed
// against a live Postgres instance (`result IS NULL` reported false; `result::text`
// was the literal string "null"). GetRun then failed for *every* run containing a
// failed or skipped step (i.e. any step with no Result) with "unexpected token
// null", since protojson.Unmarshal correctly rejects "null" as not a valid
// google.protobuf.Struct. Fixed in model.go's stepResultRow.toProto.
func TestRunStore_GetRun_StepWithNoResultRoundTrips(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	run := &workflowv1.Run{
		RunId:        "store-test-run-nil-result",
		WorkflowName: "store-test-nil-result-workflow",
		Status:       workflowv1.RunStatus_RUN_STATUS_FAILED,
		StartedAt:    timestamppb.Now(),
		FinishedAt:   timestamppb.Now(),
	}
	if err := store.UpsertRun(ctx, run); err != nil {
		t.Fatalf("UpsertRun returned unexpected error: %v", err)
	}

	failedStep := &workflowv1.StepResult{
		StepName:   "failed-step",
		Status:     workflowv1.StepStatus_STEP_STATUS_FAILED,
		Error:      "something went wrong",
		StartedAt:  timestamppb.Now(),
		FinishedAt: timestamppb.Now(),
	}
	skippedStep := &workflowv1.StepResult{
		StepName:   "skipped-step",
		Status:     workflowv1.StepStatus_STEP_STATUS_SKIPPED,
		Error:      "skipped",
		StartedAt:  timestamppb.Now(),
		FinishedAt: timestamppb.Now(),
	}
	if err := store.UpsertStepResult(ctx, run.GetRunId(), failedStep); err != nil {
		t.Fatalf("UpsertStepResult(failedStep) returned unexpected error: %v", err)
	}
	if err := store.UpsertStepResult(ctx, run.GetRunId(), skippedStep); err != nil {
		t.Fatalf("UpsertStepResult(skippedStep) returned unexpected error: %v", err)
	}

	got, err := store.GetRun(ctx, run.GetRunId())
	if err != nil {
		t.Fatalf("GetRun returned unexpected error for a run with no-Result steps: %v", err)
	}
	if len(got.GetSteps()) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(got.GetSteps()))
	}
	for _, sr := range got.GetSteps() {
		if sr.GetResult() != nil {
			t.Errorf("step %q Result = %v, want nil", sr.GetStepName(), sr.GetResult())
		}
	}
}

func TestRunStore_GetRun_NotFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.GetRun(context.Background(), "store-test-run-does-not-exist")
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("GetRun error = %v, want ErrRunNotFound", err)
	}
}

func TestRunStore_ListRuns_FiltersAndPaginates(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	workflowName := "store-test-list-runs-workflow"
	otherWorkflowName := "store-test-list-runs-other-workflow"

	base := time.Now().Add(-time.Hour)
	for i := 0; i < 5; i++ {
		run := &workflowv1.Run{
			RunId:        fmt.Sprintf("store-test-list-runs-%d", i),
			WorkflowName: workflowName,
			Status:       workflowv1.RunStatus_RUN_STATUS_PASSED,
			StartedAt:    timestamppb.New(base.Add(time.Duration(i) * time.Second)),
		}
		if err := store.UpsertRun(ctx, run); err != nil {
			t.Fatalf("UpsertRun(%d) returned unexpected error: %v", i, err)
		}
	}
	// A run for a different workflow, which the workflow_name filter below should
	// exclude.
	if err := store.UpsertRun(ctx, &workflowv1.Run{
		RunId:        "store-test-list-runs-other",
		WorkflowName: otherWorkflowName,
		Status:       workflowv1.RunStatus_RUN_STATUS_PASSED,
		StartedAt:    timestamppb.New(base),
	}); err != nil {
		t.Fatalf("UpsertRun(other) returned unexpected error: %v", err)
	}

	// First page: pageSize 2, filtered to workflowName, ordered most-recent-first.
	page1, next1, err := store.ListRuns(ctx, workflowName, 2, "")
	if err != nil {
		t.Fatalf("ListRuns (page 1) returned unexpected error: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("len(page1) = %d, want 2", len(page1))
	}
	if page1[0].GetRunId() != "store-test-list-runs-4" || page1[1].GetRunId() != "store-test-list-runs-3" {
		t.Errorf("page1 run IDs = [%s, %s], want [store-test-list-runs-4, store-test-list-runs-3]", page1[0].GetRunId(), page1[1].GetRunId())
	}
	if next1 == "" {
		t.Fatalf("expected a non-empty next_page_token after page 1")
	}

	page2, next2, err := store.ListRuns(ctx, workflowName, 2, next1)
	if err != nil {
		t.Fatalf("ListRuns (page 2) returned unexpected error: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("len(page2) = %d, want 2", len(page2))
	}
	if page2[0].GetRunId() != "store-test-list-runs-2" || page2[1].GetRunId() != "store-test-list-runs-1" {
		t.Errorf("page2 run IDs = [%s, %s], want [store-test-list-runs-2, store-test-list-runs-1]", page2[0].GetRunId(), page2[1].GetRunId())
	}

	page3, next3, err := store.ListRuns(ctx, workflowName, 2, next2)
	if err != nil {
		t.Fatalf("ListRuns (page 3) returned unexpected error: %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("len(page3) = %d, want 1", len(page3))
	}
	if page3[0].GetRunId() != "store-test-list-runs-0" {
		t.Errorf("page3 run ID = %s, want store-test-list-runs-0", page3[0].GetRunId())
	}
	if next3 != "" {
		t.Errorf("expected an empty next_page_token after the last page, got %q", next3)
	}

	for _, run := range append(append(page1, page2...), page3...) {
		if run.GetWorkflowName() != workflowName {
			t.Errorf("ListRuns with workflow_name filter returned a run for %q", run.GetWorkflowName())
		}
	}
}

func TestRunStore_ListRuns_InvalidPageToken(t *testing.T) {
	store := newTestStore(t)
	_, _, err := store.ListRuns(context.Background(), "", 10, "not-a-valid-token")
	if !errors.Is(err, ErrInvalidRunPageToken) {
		t.Fatalf("ListRuns error = %v, want ErrInvalidRunPageToken", err)
	}
}
