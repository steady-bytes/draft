package main

import (
	"strings"
	"testing"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"

	"google.golang.org/protobuf/types/known/structpb"
)

func mustStruct(t *testing.T, m map[string]interface{}) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatalf("structpb.NewStruct: %v", err)
	}
	return s
}

func TestResolveTemplates_WholeFieldPreservesType(t *testing.T) {
	completed := map[string]*workflowv1.StepResult{
		"create-course": {
			StepName: "create-course",
			Status:   workflowv1.StepStatus_STEP_STATUS_PASSED,
			Result: mustStruct(t, map[string]interface{}{
				"id":     "course-123",
				"holes":  18.0,
				"public": true,
			}),
		},
	}

	with := mustStruct(t, map[string]interface{}{
		"request": map[string]interface{}{
			"id":     "{{ steps.create-course.result.id }}",
			"holes":  "{{ steps.create-course.result.holes }}",
			"public": "{{ steps.create-course.result.public }}",
		},
	})

	resolved, err := resolveTemplates(with, completed)
	if err != nil {
		t.Fatalf("resolveTemplates: unexpected error: %v", err)
	}

	req := resolved.GetFields()["request"].GetStructValue()
	if got, want := req.GetFields()["id"].GetStringValue(), "course-123"; got != want {
		t.Errorf("request.id = %q, want %q", got, want)
	}
	if got, want := req.GetFields()["holes"].GetNumberValue(), 18.0; got != want {
		t.Errorf("request.holes = %v, want %v (should stay a number, not become a string)", got, want)
	}
	if req.GetFields()["holes"].GetKind() == nil {
		t.Fatal("request.holes has no kind")
	}
	if _, isNumber := req.GetFields()["holes"].GetKind().(*structpb.Value_NumberValue); !isNumber {
		t.Errorf("request.holes kind = %T, want NumberValue", req.GetFields()["holes"].GetKind())
	}
	if got, want := req.GetFields()["public"].GetBoolValue(), true; got != want {
		t.Errorf("request.public = %v, want %v", got, want)
	}
}

func TestResolveTemplates_EmbeddedInLargerString(t *testing.T) {
	completed := map[string]*workflowv1.StepResult{
		"create-course": {
			StepName: "create-course",
			Status:   workflowv1.StepStatus_STEP_STATUS_PASSED,
			Result:   mustStruct(t, map[string]interface{}{"id": "course-123"}),
		},
	}

	with := mustStruct(t, map[string]interface{}{
		"message": "Seeded a test course: {{ steps.create-course.result.id }}",
	})

	resolved, err := resolveTemplates(with, completed)
	if err != nil {
		t.Fatalf("resolveTemplates: unexpected error: %v", err)
	}
	if got, want := resolved.GetFields()["message"].GetStringValue(), "Seeded a test course: course-123"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestResolveTemplates_DottedPath(t *testing.T) {
	completed := map[string]*workflowv1.StepResult{
		"create-name": {
			StepName: "create-name",
			Status:   workflowv1.StepStatus_STEP_STATUS_PASSED,
			Result: mustStruct(t, map[string]interface{}{
				"name": map[string]interface{}{
					"first_name": "Ada",
				},
			}),
		},
	}

	with := mustStruct(t, map[string]interface{}{
		"first": "{{ steps.create-name.result.name.first_name }}",
	})

	resolved, err := resolveTemplates(with, completed)
	if err != nil {
		t.Fatalf("resolveTemplates: unexpected error: %v", err)
	}
	if got, want := resolved.GetFields()["first"].GetStringValue(), "Ada"; got != want {
		t.Errorf("first = %q, want %q", got, want)
	}
}

func TestResolveTemplates_UncompletedStepIsAnError(t *testing.T) {
	with := mustStruct(t, map[string]interface{}{
		"id": "{{ steps.never-ran.result.id }}",
	})

	_, err := resolveTemplates(with, map[string]*workflowv1.StepResult{})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "never-ran") {
		t.Errorf("error = %q, want it to mention the referenced step", err.Error())
	}
}

func TestResolveTemplates_MissingFieldIsAnError(t *testing.T) {
	completed := map[string]*workflowv1.StepResult{
		"create-course": {
			StepName: "create-course",
			Status:   workflowv1.StepStatus_STEP_STATUS_PASSED,
			Result:   mustStruct(t, map[string]interface{}{"id": "course-123"}),
		},
	}
	with := mustStruct(t, map[string]interface{}{
		"name": "{{ steps.create-course.result.does-not-exist }}",
	})

	_, err := resolveTemplates(with, completed)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error = %q, want it to mention the missing field", err.Error())
	}
}

func TestResolveTemplates_StepRanButHasNoResult(t *testing.T) {
	completed := map[string]*workflowv1.StepResult{
		"create-course": {
			StepName: "create-course",
			Status:   workflowv1.StepStatus_STEP_STATUS_FAILED,
			Result:   nil,
		},
	}
	with := mustStruct(t, map[string]interface{}{
		"id": "{{ steps.create-course.result.id }}",
	})

	_, err := resolveTemplates(with, completed)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no recorded result") {
		t.Errorf("error = %q, want it to mention the missing result", err.Error())
	}
}

func TestResolveTemplates_NoTemplatesIsPassthrough(t *testing.T) {
	with := mustStruct(t, map[string]interface{}{
		"service": "golf-app.app.v1.CourseCreator",
		"method":  "CreateCourse",
	})

	resolved, err := resolveTemplates(with, nil)
	if err != nil {
		t.Fatalf("resolveTemplates: unexpected error: %v", err)
	}
	if got, want := resolved.GetFields()["service"].GetStringValue(), "golf-app.app.v1.CourseCreator"; got != want {
		t.Errorf("service = %q, want %q", got, want)
	}
}

func TestResolveTemplates_NilWithIsNil(t *testing.T) {
	resolved, err := resolveTemplates(nil, nil)
	if err != nil {
		t.Fatalf("resolveTemplates: unexpected error: %v", err)
	}
	if resolved != nil {
		t.Errorf("resolved = %v, want nil", resolved)
	}
}
