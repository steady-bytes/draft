package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func findProblem(a analysis, contains string) (problem, bool) {
	for _, p := range a.Problems {
		if strings.Contains(p.Message, contains) {
			return p, true
		}
	}
	return problem{}, false
}

func TestAnalyzeAcceptsTheStarterAndDrawsItsPipeline(t *testing.T) {
	a := analyzeWorkflow(context.Background(), defaultWorkflowTemplate, nil)
	if len(a.Problems) != 0 {
		t.Fatalf("problems: %+v", a.Problems)
	}
	if len(a.Pipe) != 2 || !a.Pipe[0].Trigger || a.Pipe[0].Label != "Manual" || a.Pipe[1].Label != "01" || a.Pipe[1].Name != "step-a" {
		t.Errorf("pipeline: %+v", a.Pipe)
	}
	if a.Lines != 14 { // thirteen lines, plus the empty one after the trailing newline (as the text area counts)
		t.Errorf("lines = %d", a.Lines)
	}
}

func TestAnalyzeDrawsAWebhookTrigger(t *testing.T) {
	src := "metadata:\n  name: x\ntrigger:\n  webhook:\n    slug: crud-e2e\nsteps:\n  - name: a\n    uses: bench://sleep@v1\n"
	a := analyzeWorkflow(context.Background(), src, nil)
	if a.Pipe[0].Label != "Webhook" || a.Pipe[0].Name != "/webhooks/crud-e2e" {
		t.Errorf("trigger node: %+v", a.Pipe[0])
	}
}

func TestAnalyzePositionsProblems(t *testing.T) {
	src := "apiVersion: bench/v1\nmetadata:\n  name: x\nsteps:\n  - name: a\n    uses: bench://grpc-call@v1\n  - name: a\n    uses: ftp://x\n    depends_on: [ghost]\n    on_failure: explode\n"
	a := analyzeWorkflow(context.Background(), src, nil)
	for _, want := range []struct {
		text string
		line int
	}{
		{`Another step is already called "a"`, 7},
		{`uses "ftp://x" must start with bench:// or foundry://`, 8},
		{`depends_on names "ghost"`, 9},
		{`on_failure "explode" is not one of`, 10},
	} {
		p, ok := findProblem(a, want.text)
		if !ok {
			t.Errorf("no problem %q in %+v", want.text, a.Problems)
			continue
		}
		if p.Line != want.line || p.Col == 0 || p.Warn {
			t.Errorf("%q at %d:%d warn=%v, want line %d", want.text, p.Line, p.Col, p.Warn, want.line)
		}
	}
	// The second step is the broken one; the first is fine.
	if a.Pipe[1].Err || !a.Pipe[2].Err {
		t.Errorf("only the second step's node should be marked: %+v", a.Pipe)
	}
	if got := a.ErrorLines(); len(got) != 4 || got[0] != 7 {
		t.Errorf("error lines %v", got)
	}
}

func TestAnalyzeSyntaxErrorsCarryTheirLine(t *testing.T) {
	a := analyzeWorkflow(context.Background(), "metadata:\n  name: x\n steps: [\n", nil)
	if len(a.Problems) == 0 || a.Problems[0].Line == 0 {
		t.Fatalf("expected a positioned syntax problem: %+v", a.Problems)
	}
	if strings.Contains(a.Problems[0].Message, "yaml:") || strings.Contains(a.Problems[0].Message, "line ") {
		t.Errorf("message should not repeat the position: %q", a.Problems[0].Message)
	}
	if a.Errors() != 1 {
		t.Errorf("errors: %d", a.Errors())
	}
}

func TestAnalyzeEmptyAndWrongShapes(t *testing.T) {
	if a := analyzeWorkflow(context.Background(), "", nil); a.Errors() != 1 {
		t.Errorf("empty document: %+v", a.Problems)
	}
	if a := analyzeWorkflow(context.Background(), "- a\n- b\n", nil); a.Errors() != 1 {
		t.Errorf("a list is not a workflow: %+v", a.Problems)
	}
	a := analyzeWorkflow(context.Background(), "metadata:\n  name: x\n", nil)
	if p, ok := findProblem(a, "At least one step is required"); !ok || p.Line != 1 {
		t.Errorf("no steps: %+v", a.Problems)
	}
	a = analyzeWorkflow(context.Background(), "steps:\n  - name: a\n    uses: bench://x@v1\n", nil)
	if _, ok := findProblem(a, "metadata.name is required"); !ok {
		t.Errorf("no metadata: %+v", a.Problems)
	}
}

func TestAnalyzeFindsDependencyCycles(t *testing.T) {
	src := "metadata:\n  name: x\nsteps:\n  - name: a\n    uses: bench://x@v1\n    depends_on: [b]\n  - name: b\n    uses: bench://x@v1\n    depends_on: [a]\n"
	a := analyzeWorkflow(context.Background(), src, nil)
	if _, ok := findProblem(a, "depends_on cycle"); !ok {
		t.Errorf("cycle not found: %+v", a.Problems)
	}
}

func TestAnalyzeWarnsAboutUnknownKeysWithoutBlockingASave(t *testing.T) {
	src := "metadata:\n  name: x\n  descripton: typo\nsteps:\n  - name: a\n    uses: bench://x@v1\n    dependson: []\n"
	a := analyzeWorkflow(context.Background(), src, nil)
	if a.Errors() != 0 {
		t.Fatalf("typos in optional keys must not block a save: %+v", a.Problems)
	}
	if p, ok := findProblem(a, "Did you mean description?"); !ok || !p.Warn || p.Line != 3 {
		t.Errorf("metadata typo: %+v", a.Problems)
	}
	if p, ok := findProblem(a, "Did you mean depends_on?"); !ok || !p.Warn || p.Line != 7 {
		t.Errorf("step typo: %+v", a.Problems)
	}
	if got := a.ErrorLines(); len(got) != 0 {
		t.Errorf("warnings do not turn the gutter red: %v", got)
	}
}

func TestAnalyzeChecksWithAgainstThePluginSchema(t *testing.T) {
	src := "metadata:\n  name: x\nsteps:\n  - name: q\n    uses: foundry://grpc-call@v1\n    with:\n      service: s\n      methd: Read\n"
	lookup := func(_ context.Context, name, version string) (*pluginSchema, error) {
		if name != "grpc-call" || version != "v1" {
			return nil, nil
		}
		return &pluginSchema{
			Required:   []string{"address", "service", "method"},
			Properties: map[string]bool{"address": true, "service": true, "method": true, "request": true},
		}, nil
	}
	a := analyzeWorkflow(context.Background(), src, lookup)

	if p, ok := findProblem(a, "methd is not a field of grpc-call@v1. Did you mean method?"); !ok || p.Line != 8 || p.Col != 7 {
		t.Errorf("unknown key: %+v", a.Problems)
	}
	if p, ok := findProblem(a, "method is required by grpc-call@v1"); !ok || p.Line != 6 {
		t.Errorf("missing required: %+v", a.Problems)
	}
	if _, ok := findProblem(a, "service is required"); ok {
		t.Errorf("service was given: %+v", a.Problems)
	}
	if len(a.Checked) != 1 || a.Checked[0] != "grpc-call@v1" {
		t.Errorf("checked: %v", a.Checked)
	}
}

func TestAnalyzeCopesWithUnpublishedAndUnreachablePlugins(t *testing.T) {
	src := "metadata:\n  name: x\nsteps:\n  - name: q\n    uses: foundry://ghost@v1\n"

	missing := func(context.Context, string, string) (*pluginSchema, error) { return nil, nil }
	a := analyzeWorkflow(context.Background(), src, missing)
	if p, ok := findProblem(a, "ghost@v1 is not published to any configured registry"); !ok || !p.Warn || p.Line != 5 {
		t.Errorf("unpublished: %+v", a.Problems)
	}
	if a.Errors() != 0 {
		t.Errorf("an unpublished plugin is a warning, not a blocker: %+v", a.Problems)
	}

	down := func(context.Context, string, string) (*pluginSchema, error) {
		return nil, errors.New("connection refused")
	}
	a = analyzeWorkflow(context.Background(), src, down)
	if !a.SchemaUnavailable || len(a.Problems) != 0 {
		t.Errorf("unreachable registry: %+v", a)
	}
	if f := newEditorFeedback(a, false); !strings.Contains(f.Subtitle(), "unavailable") {
		t.Errorf("feedback should say the schemas were not checked: %q", f.Subtitle())
	}
}

func TestEditorFeedbackStatus(t *testing.T) {
	valid := newEditorFeedback(analyzeWorkflow(context.Background(), defaultWorkflowTemplate, nil), false)
	if valid.Status.Text != "Valid" || valid.Status.Class != "" {
		t.Errorf("valid: %+v", valid.Status)
	}
	bad := newEditorFeedback(analyzeWorkflow(context.Background(), "metadata:\n  name: x\nsteps:\n  - name: a\n", nil), true)
	if bad.Status.Class != "d-status--err" || !bad.OOB || bad.Errors == 0 || bad.ErrorLines == "" {
		t.Errorf("invalid: %+v", bad)
	}
	warn := newEditorFeedback(analyzeWorkflow(context.Background(), "metadata:\n  name: x\n  descripton: t\nsteps:\n  - name: a\n    uses: bench://x@v1\n", nil), false)
	if warn.Status.Class != "d-status--warn" || warn.Status.Text != "1 warning" {
		t.Errorf("warning: %+v", warn.Status)
	}
}

func TestWorkflowMetaToleratesBrokenYAML(t *testing.T) {
	if n, d := workflowMeta("metadata:\n  name: crud\n  description: hi\n"); n != "crud" || d != "hi" {
		t.Errorf("got %q %q", n, d)
	}
	if n, d := workflowMeta("metadata: [\n"); n != "" || d != "" {
		t.Errorf("broken yaml: %q %q", n, d)
	}
}

func TestEditDistance(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{{"", "", 0}, {"abc", "abc", 0}, {"usse", "uses", 2}, {"methd", "method", 1}, {"kitten", "sitting", 3}} {
		if got := editDistance(c.a, c.b); got != c.want {
			t.Errorf("editDistance(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	if closest("zzzzzz", []string{"name", "uses"}) != "" {
		t.Error("nothing is close to zzzzzz")
	}
}
