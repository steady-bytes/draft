package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	stepexecutorv1 "github.com/steady-bytes/draft/api/tooling/step_executor/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
)

// noopLogger is a minimal chassis.Logger for tests — mirrors
// services/tooling/catalyst-consume/rpc_test.go's noopLogger.
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

func stepConfig(t *testing.T, fields map[string]interface{}) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(fields)
	if err != nil {
		t.Fatalf("failed to build config struct: %v", err)
	}
	return s
}

func newTestHandler() Handler {
	return NewHandler(noopLogger{}, http.DefaultClient)
}

func TestExecute_GETSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("server saw method %q, want GET", r.Method)
		}
		if got := r.Header.Get("X-Test"); got != "yes" {
			t.Errorf("server saw X-Test header %q, want %q", got, "yes")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"Ada","age":30}`))
	}))
	defer srv.Close()

	h := newTestHandler()
	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "get-thing",
		Config: stepConfig(t, map[string]interface{}{
			"method":  "GET",
			"url":     srv.URL,
			"headers": map[string]interface{}{"X-Test": "yes"},
			"expect": map[string]interface{}{
				"status": "OK",
				"body": map[string]interface{}{
					"name": map[string]interface{}{"equals": "Ada"},
					"age":  map[string]interface{}{"exists": true},
				},
			},
		}),
	})

	resp, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !resp.Msg.GetSuccess() {
		t.Fatalf("Success = false, want true (error: %s)", resp.Msg.GetError())
	}

	result := resp.Msg.GetResult().AsMap()
	if result["status"] != float64(200) {
		t.Errorf("result.status = %v, want 200", result["status"])
	}
	body, ok := result["body"].(map[string]interface{})
	if !ok || body["name"] != "Ada" {
		t.Errorf("result.body = %v, want a map with name=Ada", result["body"])
	}
}

func TestExecute_POSTWithJSONBody(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("server saw method %q, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("server saw Content-Type %q, want application/json", ct)
		}
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	h := newTestHandler()
	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "create-thing",
		Config: stepConfig(t, map[string]interface{}{
			"method":  "POST",
			"url":     srv.URL,
			"headers": map[string]interface{}{"Content-Type": "application/json"},
			"body":    map[string]interface{}{"name": "Ada"},
			"expect":  map[string]interface{}{"status": float64(201)},
		}),
	})

	resp, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !resp.Msg.GetSuccess() {
		t.Fatalf("Success = false, want true (error: %s)", resp.Msg.GetError())
	}
	if receivedBody["name"] != "Ada" {
		t.Errorf("server received body %v, want name=Ada", receivedBody)
	}
}

func TestExecute_ExpectStatusMismatchFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := newTestHandler()
	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "get-missing",
		Config: stepConfig(t, map[string]interface{}{
			"method": "GET",
			"url":    srv.URL,
			"expect": map[string]interface{}{"status": "OK"},
		}),
	})

	resp, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("Success = true, want false for a 404 when expect.status is OK")
	}
	if resp.Msg.GetError() == "" {
		t.Error("Error is empty, want a message about the status mismatch")
	}
}

func TestExecute_ExpectExactStatusCodeMatches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := newTestHandler()
	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "expect-404",
		Config: stepConfig(t, map[string]interface{}{
			"method": "GET",
			"url":    srv.URL,
			"expect": map[string]interface{}{"status": float64(404)},
		}),
	})

	resp, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !resp.Msg.GetSuccess() {
		t.Fatalf("Success = false, want true when expect.status: 404 matches a real 404 (error: %s)", resp.Msg.GetError())
	}
}

func TestExecute_NoExpectDefaultsToAny2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	h := newTestHandler()
	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "no-expect",
		Config: stepConfig(t, map[string]interface{}{
			"method": "GET",
			"url":    srv.URL,
		}),
	})

	resp, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("Success = true, want false for a 500 with no expect: configured (default success is any 2xx)")
	}
}

func TestExecute_NonJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("plain text response"))
	}))
	defer srv.Close()

	h := newTestHandler()
	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "get-text",
		Config: stepConfig(t, map[string]interface{}{
			"method": "GET",
			"url":    srv.URL,
		}),
	})

	resp, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !resp.Msg.GetSuccess() {
		t.Fatalf("Success = false, want true (error: %s)", resp.Msg.GetError())
	}
	if got := resp.Msg.GetResult().AsMap()["body"]; got != "plain text response" {
		t.Errorf("result.body = %v, want the raw string %q", got, "plain text response")
	}
}

func TestExecute_MissingMethodOrURL(t *testing.T) {
	h := newTestHandler()

	for _, tc := range []struct {
		name   string
		config map[string]interface{}
	}{
		{"missing method", map[string]interface{}{"url": "http://example.com"}},
		{"missing url", map[string]interface{}{"method": "GET"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := connect.NewRequest(&stepexecutorv1.StepRequest{
				StepName: "bad-config",
				Config:   stepConfig(t, tc.config),
			})
			resp, err := h.Execute(context.Background(), req)
			if err != nil {
				t.Fatalf("Execute returned unexpected error: %v", err)
			}
			if resp.Msg.GetSuccess() {
				t.Fatal("Success = true, want false")
			}
		})
	}
}

func TestExecute_NilConfigIsRPCError(t *testing.T) {
	h := newTestHandler()
	req := connect.NewRequest(&stepexecutorv1.StepRequest{StepName: "call"})

	_, err := h.Execute(context.Background(), req)
	if err == nil {
		t.Fatal("Execute returned nil error for a nil config, want a CodeInvalidArgument error")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("error code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
}

func TestExecute_UnreachableURLFails(t *testing.T) {
	h := newTestHandler()
	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "unreachable",
		Config: stepConfig(t, map[string]interface{}{
			"method":  "GET",
			"url":     "http://127.0.0.1:1",
			"timeout": "1s",
		}),
	})

	resp, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("Success = true, want false for an unreachable URL")
	}
}
