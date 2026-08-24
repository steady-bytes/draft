package main

import (
	"context"
	"errors"
	"testing"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
)

// noopLogger is a minimal chassis.Logger for tests: it satisfies the interface
// without writing anything anywhere, matching services/tooling/garage/rpc_test.go's
// own noopLogger.
type noopLogger struct{}

func (noopLogger) Start(chassis.Config)                         {}
func (noopLogger) SetLevel(chassis.LogLevel)                    {}
func (noopLogger) GetLevel() chassis.LogLevel                   { return chassis.InfoLevel }
func (noopLogger) Wrap(err error) error                         { return err }
func (l noopLogger) WithError(error) chassis.Logger             { return l }
func (l noopLogger) WithContext(context.Context) chassis.Logger { return l }
func (l noopLogger) WithField(string, any) chassis.Logger       { return l }
func (l noopLogger) WithFields(chassis.Fields) chassis.Logger   { return l }
func (l noopLogger) WithCallDepth(int) chassis.Logger           { return l }
func (noopLogger) Trace(string)                                 {}
func (noopLogger) Debug(string)                                 {}
func (noopLogger) Debugf(string, ...any)                        {}
func (noopLogger) Info(string)                                  {}
func (noopLogger) Infof(string, ...any)                         {}
func (noopLogger) Warn(string)                                  {}
func (noopLogger) Warnf(string, ...any)                         {}
func (noopLogger) Error(string)                                 {}
func (noopLogger) Errorf(string, ...any)                        {}
func (noopLogger) WrappedError(error, string)                   {}
func (noopLogger) Fatal(string)                                 {}
func (noopLogger) Panic(string)                                 {}

var _ chassis.Logger = noopLogger{}

// fakeRunner is a scripted workflowRunner: it records every workflow StartRun was
// called with and returns a canned Run (or a canned error), without touching a
// real scheduler/executor/Blueprint — rpc.go's TriggerRun only needs to know that
// StartRun was called and to relay whatever it returned, not exercise the DAG
// executor itself (scheduler_test.go already covers that).
type fakeRunner struct {
	calls []*workflowv1.Workflow
	run   *workflowv1.Run
	err   error
}

func (r *fakeRunner) StartRun(ctx context.Context, w *workflowv1.Workflow) (*workflowv1.Run, error) {
	r.calls = append(r.calls, w)
	if r.err != nil {
		return nil, r.err
	}
	return r.run, nil
}

func connectCode(t *testing.T, err error) connect.Code {
	t.Helper()
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected a *connect.Error, got %T: %v", err, err)
	}
	return connectErr.Code()
}

func TestTriggerRun_Success(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	name := "rpc-test-trigger-run-workflow"
	w := &workflowv1.Workflow{
		Name:  name,
		Steps: []*workflowv1.Step{{Name: "step-a", Uses: "bench://grpc-call@v1"}},
	}
	if err := store.UpsertWorkflow(ctx, w); err != nil {
		t.Fatalf("UpsertWorkflow returned unexpected error: %v", err)
	}

	runner := &fakeRunner{run: &workflowv1.Run{
		RunId:        "rpc-test-trigger-run-id",
		WorkflowName: name,
		Status:       workflowv1.RunStatus_RUN_STATUS_PENDING,
	}}
	h := NewHandler(noopLogger{}, store, runner)

	resp, err := h.TriggerRun(ctx, connect.NewRequest(&workflowv1.TriggerRunRequest{WorkflowName: name}))
	if err != nil {
		t.Fatalf("TriggerRun returned unexpected error: %v", err)
	}
	if resp.Msg.GetRunId() != "rpc-test-trigger-run-id" {
		t.Errorf("RunId = %q, want %q", resp.Msg.GetRunId(), "rpc-test-trigger-run-id")
	}
	if resp.Msg.GetStatus() != workflowv1.RunStatus_RUN_STATUS_PENDING {
		t.Errorf("Status = %s, want PENDING", resp.Msg.GetStatus())
	}
	if len(runner.calls) != 1 || runner.calls[0].GetName() != name {
		t.Fatalf("expected StartRun to be called once with workflow %q, calls = %v", name, runner.calls)
	}
}

func TestTriggerRun_UnknownWorkflow(t *testing.T) {
	store := newTestStore(t)
	runner := &fakeRunner{}
	h := NewHandler(noopLogger{}, store, runner)

	_, err := h.TriggerRun(context.Background(), connect.NewRequest(&workflowv1.TriggerRunRequest{WorkflowName: "rpc-test-does-not-exist"}))
	if err == nil {
		t.Fatal("expected an error for an unknown workflow, got nil")
	}
	if code := connectCode(t, err); code != connect.CodeNotFound {
		t.Errorf("code = %s, want CodeNotFound", code)
	}
	if len(runner.calls) != 0 {
		t.Errorf("expected StartRun not to be called for an unknown workflow, got %d calls", len(runner.calls))
	}
}

func TestTriggerRun_MissingWorkflowName(t *testing.T) {
	store := newTestStore(t)
	h := NewHandler(noopLogger{}, store, &fakeRunner{})

	_, err := h.TriggerRun(context.Background(), connect.NewRequest(&workflowv1.TriggerRunRequest{}))
	if code := connectCode(t, err); code != connect.CodeInvalidArgument {
		t.Errorf("code = %s, want CodeInvalidArgument", code)
	}
}

func TestGetRun_Success(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	run := &workflowv1.Run{
		RunId:        "rpc-test-get-run-id",
		WorkflowName: "rpc-test-get-run-workflow",
		Status:       workflowv1.RunStatus_RUN_STATUS_PASSED,
	}
	if err := store.UpsertRun(ctx, run); err != nil {
		t.Fatalf("UpsertRun returned unexpected error: %v", err)
	}

	h := NewHandler(noopLogger{}, store, &fakeRunner{})
	resp, err := h.GetRun(ctx, connect.NewRequest(&workflowv1.GetRunRequest{RunId: run.GetRunId()}))
	if err != nil {
		t.Fatalf("GetRun returned unexpected error: %v", err)
	}
	if resp.Msg.GetRunId() != run.GetRunId() {
		t.Errorf("RunId = %q, want %q", resp.Msg.GetRunId(), run.GetRunId())
	}
	if resp.Msg.GetStatus() != workflowv1.RunStatus_RUN_STATUS_PASSED {
		t.Errorf("Status = %s, want PASSED", resp.Msg.GetStatus())
	}
}

func TestGetRun_NotFound(t *testing.T) {
	store := newTestStore(t)
	h := NewHandler(noopLogger{}, store, &fakeRunner{})

	_, err := h.GetRun(context.Background(), connect.NewRequest(&workflowv1.GetRunRequest{RunId: "rpc-test-get-run-does-not-exist"}))
	if code := connectCode(t, err); code != connect.CodeNotFound {
		t.Errorf("code = %s, want CodeNotFound", code)
	}
}

func TestListWorkflows_ReturnsLoadedWorkflows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	name := "rpc-test-list-workflows-workflow"
	if err := store.UpsertWorkflow(ctx, &workflowv1.Workflow{Name: name}); err != nil {
		t.Fatalf("UpsertWorkflow returned unexpected error: %v", err)
	}

	h := NewHandler(noopLogger{}, store, &fakeRunner{})
	resp, err := h.ListWorkflows(ctx, connect.NewRequest(&workflowv1.ListWorkflowsRequest{}))
	if err != nil {
		t.Fatalf("ListWorkflows returned unexpected error: %v", err)
	}

	found := false
	for _, w := range resp.Msg.GetWorkflows() {
		if w.GetName() == name {
			found = true
		}
	}
	if !found {
		t.Errorf("ListWorkflows response did not include %q", name)
	}
}

func TestListRuns_FiltersByWorkflowName(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	workflowName := "rpc-test-list-runs-workflow"
	if err := store.UpsertRun(ctx, &workflowv1.Run{RunId: "rpc-test-list-runs-run", WorkflowName: workflowName}); err != nil {
		t.Fatalf("UpsertRun returned unexpected error: %v", err)
	}

	h := NewHandler(noopLogger{}, store, &fakeRunner{})
	resp, err := h.ListRuns(ctx, connect.NewRequest(&workflowv1.ListRunsRequest{WorkflowName: workflowName}))
	if err != nil {
		t.Fatalf("ListRuns returned unexpected error: %v", err)
	}

	for _, run := range resp.Msg.GetRuns() {
		if run.GetWorkflowName() != workflowName {
			t.Errorf("ListRuns returned a run for workflow %q, want only %q", run.GetWorkflowName(), workflowName)
		}
	}
}

func testWorkflowYAML(name string) string {
	return "apiVersion: bench/v1\n" +
		"kind: Workflow\n" +
		"metadata:\n" +
		"  name: " + name + "\n" +
		"steps:\n" +
		"  - name: step-a\n" +
		"    uses: bench://grpc-call@v1\n" +
		"    with:\n" +
		"      service: x\n" +
		"      method: y\n"
}

func TestCreateWorkflow_Success(t *testing.T) {
	store := newTestStore(t)
	h := NewHandler(noopLogger{}, store, &fakeRunner{})

	resp, err := h.CreateWorkflow(context.Background(), connect.NewRequest(&workflowv1.CreateWorkflowRequest{
		Yaml: testWorkflowYAML("rpc-test-create-workflow"),
	}))
	if err != nil {
		t.Fatalf("CreateWorkflow returned unexpected error: %v", err)
	}
	if got := resp.Msg.GetWorkflow().GetName(); got != "rpc-test-create-workflow" {
		t.Errorf("workflow.name = %q, want %q", got, "rpc-test-create-workflow")
	}

	if _, err := store.GetWorkflow(context.Background(), "rpc-test-create-workflow"); err != nil {
		t.Errorf("GetWorkflow after CreateWorkflow returned unexpected error: %v", err)
	}
}

func TestCreateWorkflow_AlreadyExists(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := NewHandler(noopLogger{}, store, &fakeRunner{})

	yamlDoc := testWorkflowYAML("rpc-test-create-workflow-dup")
	if _, err := h.CreateWorkflow(ctx, connect.NewRequest(&workflowv1.CreateWorkflowRequest{Yaml: yamlDoc})); err != nil {
		t.Fatalf("first CreateWorkflow returned unexpected error: %v", err)
	}

	_, err := h.CreateWorkflow(ctx, connect.NewRequest(&workflowv1.CreateWorkflowRequest{Yaml: yamlDoc}))
	if err == nil {
		t.Fatal("second CreateWorkflow with the same name returned nil error, want AlreadyExists")
	}
	if code := connectCode(t, err); code != connect.CodeAlreadyExists {
		t.Errorf("code = %v, want %v", code, connect.CodeAlreadyExists)
	}
}

func TestCreateWorkflow_InvalidYAML(t *testing.T) {
	store := newTestStore(t)
	h := NewHandler(noopLogger{}, store, &fakeRunner{})

	_, err := h.CreateWorkflow(context.Background(), connect.NewRequest(&workflowv1.CreateWorkflowRequest{
		Yaml: "not: valid: workflow: yaml:",
	}))
	if err == nil {
		t.Fatal("CreateWorkflow with invalid yaml returned nil error, want InvalidArgument")
	}
	if code := connectCode(t, err); code != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", code, connect.CodeInvalidArgument)
	}
}

func TestUpdateWorkflow_Success(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := NewHandler(noopLogger{}, store, &fakeRunner{})

	name := "rpc-test-update-workflow"
	if _, err := h.CreateWorkflow(ctx, connect.NewRequest(&workflowv1.CreateWorkflowRequest{Yaml: testWorkflowYAML(name)})); err != nil {
		t.Fatalf("CreateWorkflow returned unexpected error: %v", err)
	}

	updated := "apiVersion: bench/v1\nkind: Workflow\nmetadata:\n  name: " + name + "\n  description: updated\nsteps:\n  - name: step-a\n    uses: bench://grpc-call@v1\n    with:\n      service: x\n      method: y\n"
	resp, err := h.UpdateWorkflow(ctx, connect.NewRequest(&workflowv1.UpdateWorkflowRequest{Name: name, Yaml: updated}))
	if err != nil {
		t.Fatalf("UpdateWorkflow returned unexpected error: %v", err)
	}
	if got := resp.Msg.GetWorkflow().GetDescription(); got != "updated" {
		t.Errorf("description = %q, want %q", got, "updated")
	}
}

func TestUpdateWorkflow_NotFound(t *testing.T) {
	store := newTestStore(t)
	h := NewHandler(noopLogger{}, store, &fakeRunner{})

	_, err := h.UpdateWorkflow(context.Background(), connect.NewRequest(&workflowv1.UpdateWorkflowRequest{
		Name: "rpc-test-update-workflow-missing",
		Yaml: testWorkflowYAML("rpc-test-update-workflow-missing"),
	}))
	if err == nil {
		t.Fatal("UpdateWorkflow for a nonexistent workflow returned nil error, want NotFound")
	}
	if code := connectCode(t, err); code != connect.CodeNotFound {
		t.Errorf("code = %v, want %v", code, connect.CodeNotFound)
	}
}

func TestUpdateWorkflow_NameMismatchRejected(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := NewHandler(noopLogger{}, store, &fakeRunner{})

	name := "rpc-test-update-workflow-rename"
	if _, err := h.CreateWorkflow(ctx, connect.NewRequest(&workflowv1.CreateWorkflowRequest{Yaml: testWorkflowYAML(name)})); err != nil {
		t.Fatalf("CreateWorkflow returned unexpected error: %v", err)
	}

	_, err := h.UpdateWorkflow(ctx, connect.NewRequest(&workflowv1.UpdateWorkflowRequest{
		Name: name,
		Yaml: testWorkflowYAML("rpc-test-update-workflow-renamed"),
	}))
	if err == nil {
		t.Fatal("UpdateWorkflow with a mismatched metadata.name returned nil error, want InvalidArgument")
	}
	if code := connectCode(t, err); code != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", code, connect.CodeInvalidArgument)
	}
}

func TestDeleteWorkflow_Success(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := NewHandler(noopLogger{}, store, &fakeRunner{})

	name := "rpc-test-delete-workflow"
	if _, err := h.CreateWorkflow(ctx, connect.NewRequest(&workflowv1.CreateWorkflowRequest{Yaml: testWorkflowYAML(name)})); err != nil {
		t.Fatalf("CreateWorkflow returned unexpected error: %v", err)
	}

	if _, err := h.DeleteWorkflow(ctx, connect.NewRequest(&workflowv1.DeleteWorkflowRequest{Name: name})); err != nil {
		t.Fatalf("DeleteWorkflow returned unexpected error: %v", err)
	}

	if _, err := store.GetWorkflow(ctx, name); !errors.Is(err, ErrWorkflowNotFound) {
		t.Errorf("GetWorkflow after DeleteWorkflow returned err=%v, want ErrWorkflowNotFound", err)
	}
}

func TestDeleteWorkflow_NotFound(t *testing.T) {
	store := newTestStore(t)
	h := NewHandler(noopLogger{}, store, &fakeRunner{})

	_, err := h.DeleteWorkflow(context.Background(), connect.NewRequest(&workflowv1.DeleteWorkflowRequest{
		Name: "rpc-test-delete-workflow-missing",
	}))
	if err == nil {
		t.Fatal("DeleteWorkflow for a nonexistent workflow returned nil error, want NotFound")
	}
	if code := connectCode(t, err); code != connect.CodeNotFound {
		t.Errorf("code = %v, want %v", code, connect.CodeNotFound)
	}
}
