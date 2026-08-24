package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWorkflowsDir_SeedsEveryFile(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	dir := t.TempDir()
	writeWorkflowFixture(t, dir, "a.yaml", "workflows-test-dir-a", "workflows-test-dir-a-slug")
	writeWorkflowFixture(t, dir, "b.yml", "workflows-test-dir-b", "")
	// A non-YAML file in the same directory should be ignored, not treated as a
	// workflow.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("not a workflow"), 0o644); err != nil {
		t.Fatalf("failed to write README.md fixture: %v", err)
	}

	seeded, err := loadWorkflowsDir(ctx, dir, store, noopLogger{})
	if err != nil {
		t.Fatalf("loadWorkflowsDir returned unexpected error: %v", err)
	}
	if seeded != 2 {
		t.Fatalf("seeded = %d, want 2", seeded)
	}

	for _, name := range []string{"workflows-test-dir-a", "workflows-test-dir-b"} {
		if _, err := store.GetWorkflow(ctx, name); err != nil {
			t.Errorf("GetWorkflow(%q) after loadWorkflowsDir returned unexpected error: %v", name, err)
		}
	}
}

// TestLoadWorkflowsDir_SeedOnce is Phase 10's own defining behavior (see
// workflows.go's file comment and the design doc's "Authoring workflows
// without a file"): a file only seeds a workflow the first time its name is
// seen. Once it exists in the DB — including a change made through the
// UI/RPCs, simulated here via updateWorkflow directly — a later load of the
// same (even differently-worded) file must not revert it.
func TestLoadWorkflowsDir_SeedOnce(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeWorkflowFixture(t, dir, "seed-once.yaml", "workflows-test-seed-once", "")
	if seeded, err := loadWorkflowsDir(ctx, dir, store, noopLogger{}); err != nil || seeded != 1 {
		t.Fatalf("first loadWorkflowsDir: seeded=%d err=%v, want seeded=1 err=nil", seeded, err)
	}

	// Simulate a UI/RPC edit — the description changes, only in the DB.
	if _, err := updateWorkflow(ctx, store, "workflows-test-seed-once", `apiVersion: bench/v1
kind: Workflow
metadata:
  name: workflows-test-seed-once
  description: edited through the ui, not the file
steps:
  - name: step-a
    uses: bench://grpc-call@v1
    with:
      service: x
      method: y
`); err != nil {
		t.Fatalf("updateWorkflow returned unexpected error: %v", err)
	}

	// Restart: the file on disk is unchanged (still no description), but the
	// DB row now has one. A second loadWorkflowsDir must skip it entirely.
	seeded, err := loadWorkflowsDir(ctx, dir, store, noopLogger{})
	if err != nil {
		t.Fatalf("second loadWorkflowsDir returned unexpected error: %v", err)
	}
	if seeded != 0 {
		t.Errorf("seeded = %d on second load, want 0 (workflow already exists, should be skipped)", seeded)
	}

	w, err := store.GetWorkflow(ctx, "workflows-test-seed-once")
	if err != nil {
		t.Fatalf("GetWorkflow returned unexpected error: %v", err)
	}
	if w.GetDescription() != "edited through the ui, not the file" {
		t.Errorf("description = %q, want the UI edit to have survived the second load, not been reverted by the file", w.GetDescription())
	}
}

func TestLoadWorkflowsDir_MissingDirIsNotFatal(t *testing.T) {
	store := newTestStore(t)
	seeded, err := loadWorkflowsDir(context.Background(), "/nonexistent/workflows/dir/for/bench/tests", store, noopLogger{})
	if err != nil {
		t.Fatalf("loadWorkflowsDir returned unexpected error for a missing dir: %v", err)
	}
	if seeded != 0 {
		t.Errorf("seeded = %d, want 0", seeded)
	}
}

func TestLoadWorkflowsDir_InvalidWorkflowFailsStartup(t *testing.T) {
	store := newTestStore(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "invalid.yaml"), []byte("apiVersion: bench/v1\nkind: Workflow\nmetadata:\n  name: \"\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write invalid.yaml fixture: %v", err)
	}

	if _, err := loadWorkflowsDir(context.Background(), dir, store, noopLogger{}); err == nil {
		t.Fatal("expected an error for an invalid workflow file, got nil")
	}
}

func writeWorkflowFixture(t *testing.T, dir, filename, name, slug string) {
	t.Helper()
	trigger := ""
	if slug != "" {
		trigger = "trigger:\n  webhook:\n    slug: " + slug + "\n"
	}
	content := "apiVersion: bench/v1\n" +
		"kind: Workflow\n" +
		"metadata:\n" +
		"  name: " + name + "\n" +
		trigger +
		"steps:\n" +
		"  - name: step-a\n" +
		"    uses: bench://grpc-call@v1\n" +
		"    with:\n" +
		"      service: x\n" +
		"      method: y\n"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s fixture: %v", filename, err)
	}
}
