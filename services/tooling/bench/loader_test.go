package main

import (
	"strings"
	"testing"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
)

// TestLoadWorkflowFile_Valid loads the full worked example from the doc's
// "Primitives" section (including its garage://slack-notify@v2 step — Phase 2 only
// loads and validates it, it doesn't execute it) and checks the conversion produced
// the expected wire-type shape, including the two fields that can't be
// yaml.Unmarshal'd directly: With/Expect (*structpb.Struct) and OnFailure
// (FailurePolicy enum).
func TestLoadWorkflowFile_Valid(t *testing.T) {
	w, err := LoadWorkflowFile("testdata/valid_course_creation.yaml")
	if err != nil {
		t.Fatalf("LoadWorkflowFile: unexpected error: %v", err)
	}

	if got, want := w.GetName(), "course-creation-e2e"; got != want {
		t.Errorf("workflow name = %q, want %q", got, want)
	}
	if got, want := w.GetDescription(), "Create a course via CourseCreator, then confirm it's queryable."; got != want {
		t.Errorf("workflow description = %q, want %q", got, want)
	}

	webhook := w.GetTrigger().GetWebhook()
	if webhook == nil {
		t.Fatal("trigger.webhook is nil")
	}
	if got, want := webhook.GetSlug(), "course-creation-e2e"; got != want {
		t.Errorf("webhook.slug = %q, want %q", got, want)
	}
	if got, want := webhook.GetSignatureHeader(), "X-Bench-Signature"; got != want {
		t.Errorf("webhook.signature_header = %q, want %q", got, want)
	}
	if got, want := webhook.GetSecretRef(), "blueprint://secrets/bench/course-creation-webhook"; got != want {
		t.Errorf("webhook.secret_ref = %q, want %q", got, want)
	}

	steps := w.GetSteps()
	if len(steps) != 3 {
		t.Fatalf("len(steps) = %d, want 3", len(steps))
	}

	createCourse := steps[0]
	if got, want := createCourse.GetName(), "create-course"; got != want {
		t.Errorf("steps[0].name = %q, want %q", got, want)
	}
	if got, want := createCourse.GetUses(), "bench://grpc-call@v1"; got != want {
		t.Errorf("steps[0].uses = %q, want %q", got, want)
	}
	// on_failure was omitted for this step, so it must default to FAIL, not
	// UNSPECIFIED (the enum's zero value).
	if got, want := createCourse.GetOnFailure(), workflowv1.FailurePolicy_FAILURE_POLICY_FAIL; got != want {
		t.Errorf("steps[0].on_failure = %v, want %v", got, want)
	}
	if createCourse.GetWith() == nil {
		t.Fatal("steps[0].with is nil")
	}
	if got, want := createCourse.GetWith().GetFields()["service"].GetStringValue(), "golf-app.app.v1.CourseCreator"; got != want {
		t.Errorf("steps[0].with.service = %q, want %q", got, want)
	}
	request := createCourse.GetWith().GetFields()["request"].GetStructValue()
	if request == nil {
		t.Fatal("steps[0].with.request is not a struct")
	}
	if got, want := request.GetFields()["name"].GetStringValue(), "Pebble Beach"; got != want {
		t.Errorf("steps[0].with.request.name = %q, want %q", got, want)
	}
	if got, want := request.GetFields()["holes"].GetNumberValue(), 18.0; got != want {
		t.Errorf("steps[0].with.request.holes = %v, want %v", got, want)
	}
	if createCourse.GetExpect() == nil {
		t.Fatal("steps[0].expect is nil")
	}

	queryCourse := steps[1]
	if got, want := queryCourse.GetDependsOn(), []string{"create-course"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("steps[1].depends_on = %v, want %v", got, want)
	}

	notify := steps[2]
	if got, want := notify.GetUses(), "garage://slack-notify@v2"; got != want {
		t.Errorf("steps[2].uses = %q, want %q", got, want)
	}
	if got, want := notify.GetOnFailure(), workflowv1.FailurePolicy_FAILURE_POLICY_CONTINUE; got != want {
		t.Errorf("steps[2].on_failure = %v, want %v", got, want)
	}
	// notify has no explicit depends_on in the YAML, so it must default to the
	// *previous* step, not be left empty.
	if got, want := len(notify.GetDependsOn()), 0; got != want {
		t.Errorf("steps[2].depends_on literal length = %d, want %d (defaulting happens at cycle-check time, not in the stored proto)", got, want)
	}
}

// TestParseWorkflow_Invalid covers the required-field, uses: scheme, dangling
// depends_on, and cycle-detection validation rules — both the literal-YAML cycle
// case and the "cycle only exists once defaults are applied" case the brief calls
// out specifically.
func TestParseWorkflow_Invalid(t *testing.T) {
	tests := []struct {
		name            string
		file            string
		yaml            string
		wantErrContains string
	}{
		{
			name:            "bad uses scheme",
			file:            "testdata/bad_scheme.yaml",
			wantErrContains: "must have scheme bench:// or garage://",
		},
		{
			name:            "dangling depends_on reference",
			file:            "testdata/dangling_dependency.yaml",
			wantErrContains: `depends_on undeclared step "nonexistent-step"`,
		},
		{
			name:            "explicit cycle",
			file:            "testdata/cycle.yaml",
			wantErrContains: "cycle detected",
		},
		{
			name:            "cycle only visible after applying the depends_on default",
			file:            "testdata/implicit_cycle.yaml",
			wantErrContains: "cycle detected",
		},
		{
			name: "missing workflow name",
			yaml: `
apiVersion: bench/v1
kind: Workflow
metadata:
  description: no name here
steps:
  - name: only-step
    uses: bench://grpc-call@v1
`,
			wantErrContains: "workflow name is required",
		},
		{
			name: "step missing name",
			yaml: `
apiVersion: bench/v1
kind: Workflow
metadata:
  name: missing-step-name
steps:
  - uses: bench://grpc-call@v1
`,
			wantErrContains: "name is required",
		},
		{
			name: "step missing uses",
			yaml: `
apiVersion: bench/v1
kind: Workflow
metadata:
  name: missing-uses
steps:
  - name: no-uses
`,
			wantErrContains: "uses is required",
		},
		{
			name: "uses with no scheme at all",
			yaml: `
apiVersion: bench/v1
kind: Workflow
metadata:
  name: no-scheme
steps:
  - name: no-scheme-step
    uses: grpc-call@v1
`,
			wantErrContains: "must have scheme bench:// or garage://",
		},
		{
			name: "duplicate step names",
			yaml: `
apiVersion: bench/v1
kind: Workflow
metadata:
  name: duplicate-step-names
steps:
  - name: same-name
    uses: bench://grpc-call@v1
  - name: same-name
    uses: bench://grpc-call@v1
`,
			wantErrContains: `duplicate step name "same-name"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.file != "" {
				_, err = LoadWorkflowFile(tt.file)
			} else {
				_, err = ParseWorkflow([]byte(tt.yaml))
			}
			if err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErrContains)
			}
		})
	}
}

// TestLoadWorkflowFile_MissingFile checks the loader reports a clear I/O error
// rather than a generic or nil one when the path doesn't exist.
func TestLoadWorkflowFile_MissingFile(t *testing.T) {
	_, err := LoadWorkflowFile("testdata/does-not-exist.yaml")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "does-not-exist.yaml") {
		t.Errorf("error = %q, want it to mention the missing path", err.Error())
	}
}
