package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/protobuf/types/known/structpb"
)

// fakeResolver is a ServiceResolver that resolves every service name to the same
// fixed address, standing in for Blueprint in tests that don't need real service
// discovery — see discovery_test.go for tests of blueprintResolver's own address
// matching/caching logic.
type fakeResolver struct {
	addr string
	err  error
}

func (r *fakeResolver) Resolve(ctx context.Context, service string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	return r.addr, nil
}

// newConnectJSONServer starts an httptest.Server that behaves like a minimal
// Connect-JSON unary RPC endpoint: it decodes the JSON body, calls handler, and
// writes handler's return value back as the JSON response (or the given status
// code on error). Good enough to prove the grpc-call executor's HTTP mechanics
// without depending on a real Draft service in this test file.
func newConnectJSONServer(t *testing.T, handler func(path string, body map[string]interface{}) (status int, resp map[string]interface{})) *httptest.Server {
	t.Helper()
	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("server: failed to decode request body: %v", err)
			}
		}
		status, resp := handler(r.URL.Path, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if resp != nil {
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("server: failed to encode response: %v", err)
			}
		}
	})
	// The grpc-call executor's HTTP client speaks h2c (HTTP/2 over plaintext) —
	// the same protocol every real Draft service serves internally (see
	// services/core/auth/main.go's serveCheckEndpoint) — so the test double has
	// to speak it too, not plain HTTP/1.1, for the two ends to actually agree on
	// a protocol.
	srv := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))
	t.Cleanup(srv.Close)
	return srv
}

// addrOf strips the http:// scheme from an httptest server's URL, matching the
// "host:port" shape blueprintResolver.Resolve (and the real Process.IpAddress
// field it reads from) actually returns.
func addrOf(srv *httptest.Server) string {
	return strings.TrimPrefix(srv.URL, "http://")
}

func stepWithExpect(t *testing.T, expect map[string]interface{}) *workflowv1.Step {
	t.Helper()
	var expectStruct *structpb.Struct
	if expect != nil {
		s, err := structpb.NewStruct(expect)
		if err != nil {
			t.Fatalf("structpb.NewStruct: %v", err)
		}
		expectStruct = s
	}
	return &workflowv1.Step{Name: "test-step", Expect: expectStruct}
}

func withOf(t *testing.T, service, method string, request map[string]interface{}) *structpb.Struct {
	t.Helper()
	m := map[string]interface{}{"service": service, "method": method}
	if request != nil {
		m["request"] = request
	}
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatalf("structpb.NewStruct: %v", err)
	}
	return s
}

func TestGrpcCallExecutor_PassingCall(t *testing.T) {
	srv := newConnectJSONServer(t, func(path string, body map[string]interface{}) (int, map[string]interface{}) {
		if want := "/golf-app.app.v1.CourseCreator/CreateCourse"; path != want {
			t.Errorf("request path = %q, want %q", path, want)
		}
		if got, want := body["name"], "Pebble Beach"; got != want {
			t.Errorf("request.name = %v, want %v", got, want)
		}
		return http.StatusOK, map[string]interface{}{"id": "course-123"}
	})

	exec := NewGrpcCallExecutor(&fakeResolver{addr: addrOf(srv)})
	step := stepWithExpect(t, map[string]interface{}{
		"status": "OK",
		"body": map[string]interface{}{
			"id": map[string]interface{}{"exists": true},
		},
	})
	with := withOf(t, "golf-app.app.v1.CourseCreator", "CreateCourse", map[string]interface{}{"name": "Pebble Beach"})

	result, err := exec.Execute(context.Background(), step, with)
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("Passed = false, want true; reason: %s", result.FailureReason)
	}
	if got, want := result.Result.GetFields()["id"].GetStringValue(), "course-123"; got != want {
		t.Errorf("result.id = %q, want %q", got, want)
	}
}

func TestGrpcCallExecutor_EqualsAssertionFails(t *testing.T) {
	srv := newConnectJSONServer(t, func(path string, body map[string]interface{}) (int, map[string]interface{}) {
		return http.StatusOK, map[string]interface{}{"name": "Not Pebble Beach"}
	})

	exec := NewGrpcCallExecutor(&fakeResolver{addr: addrOf(srv)})
	step := stepWithExpect(t, map[string]interface{}{
		"status": "OK",
		"body": map[string]interface{}{
			"name": map[string]interface{}{"equals": "Pebble Beach"},
		},
	})
	with := withOf(t, "golf-app.app.v1.CourseCreator", "GetCourse", map[string]interface{}{"id": "course-123"})

	result, err := exec.Execute(context.Background(), step, with)
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if result.Passed {
		t.Fatal("Passed = true, want false")
	}
	if !strings.Contains(result.FailureReason, "expect.body.name") {
		t.Errorf("FailureReason = %q, want it to mention expect.body.name", result.FailureReason)
	}
	// The raw response is still recorded even though the assertion failed, so a
	// dependent step's templating (or a human debugging the run) can see what
	// actually came back.
	if got, want := result.Result.GetFields()["name"].GetStringValue(), "Not Pebble Beach"; got != want {
		t.Errorf("result.name = %q, want %q", got, want)
	}
}

func TestGrpcCallExecutor_NestedBodyAssertion(t *testing.T) {
	srv := newConnectJSONServer(t, func(path string, body map[string]interface{}) (int, map[string]interface{}) {
		return http.StatusOK, map[string]interface{}{
			"name": map[string]interface{}{"first_name": "Ada", "last_name": "Lovelace"},
		}
	})

	exec := NewGrpcCallExecutor(&fakeResolver{addr: addrOf(srv)})
	step := stepWithExpect(t, map[string]interface{}{
		"status": "OK",
		"body": map[string]interface{}{
			"name": map[string]interface{}{
				"first_name": map[string]interface{}{"equals": "Ada"},
				"last_name":  map[string]interface{}{"matches": "^Love.*"},
			},
		},
	})
	with := withOf(t, "examples.crud.v1.CrudService", "Read", map[string]interface{}{"id": "abc"})

	result, err := exec.Execute(context.Background(), step, with)
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("Passed = false, want true; reason: %s", result.FailureReason)
	}
}

func TestGrpcCallExecutor_NonOKStatusFailsAssertion(t *testing.T) {
	srv := newConnectJSONServer(t, func(path string, body map[string]interface{}) (int, map[string]interface{}) {
		return http.StatusNotFound, map[string]interface{}{"code": "not_found", "message": "course not found"}
	})

	exec := NewGrpcCallExecutor(&fakeResolver{addr: addrOf(srv)})
	step := stepWithExpect(t, map[string]interface{}{"status": "OK"})
	with := withOf(t, "golf-app.app.v1.CourseCreator", "GetCourse", nil)

	result, err := exec.Execute(context.Background(), step, with)
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if result.Passed {
		t.Fatal("Passed = true, want false")
	}
	if !strings.Contains(result.FailureReason, "404") {
		t.Errorf("FailureReason = %q, want it to mention the HTTP status", result.FailureReason)
	}
}

func TestGrpcCallExecutor_ExistsFalseAssertion(t *testing.T) {
	srv := newConnectJSONServer(t, func(path string, body map[string]interface{}) (int, map[string]interface{}) {
		return http.StatusOK, map[string]interface{}{"id": "course-123"}
	})

	exec := NewGrpcCallExecutor(&fakeResolver{addr: addrOf(srv)})
	step := stepWithExpect(t, map[string]interface{}{
		"body": map[string]interface{}{
			"error": map[string]interface{}{"exists": false},
		},
	})
	with := withOf(t, "golf-app.app.v1.CourseCreator", "CreateCourse", nil)

	result, err := exec.Execute(context.Background(), step, with)
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("Passed = false, want true; reason: %s", result.FailureReason)
	}
}

func TestGrpcCallExecutor_MissingServiceOrMethodIsAnError(t *testing.T) {
	exec := NewGrpcCallExecutor(&fakeResolver{addr: "unused:0"})
	step := stepWithExpect(t, nil)
	with, err := structpb.NewStruct(map[string]interface{}{"method": "CreateCourse"})
	if err != nil {
		t.Fatalf("structpb.NewStruct: %v", err)
	}

	_, err = exec.Execute(context.Background(), step, with)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestGrpcCallExecutor_ResolveFailureIsAnError(t *testing.T) {
	exec := NewGrpcCallExecutor(&fakeResolver{err: fmt.Errorf("service not found")})
	step := stepWithExpect(t, nil)
	with := withOf(t, "golf-app.app.v1.CourseCreator", "CreateCourse", nil)

	_, err := exec.Execute(context.Background(), step, with)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "service not found") {
		t.Errorf("error = %q, want it to wrap the resolver error", err.Error())
	}
}
