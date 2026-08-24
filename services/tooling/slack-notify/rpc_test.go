package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	stepexecutorv1 "github.com/steady-bytes/draft/api/tooling/step_executor/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
)

// noopLogger is a minimal chassis.Logger for tests: it satisfies the
// interface without writing anything anywhere. Mirrors
// services/tooling/garage/rpc_test.go's noopLogger.
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

func TestExecute_Success(t *testing.T) {
	var gotPayload slackWebhookPayload
	var gotContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok": true, "ts": "1234567890.123456"}`))
	}))
	defer server.Close()

	h := NewHandler(noopLogger{}, http.DefaultClient, func() string { return server.URL })

	resp, err := h.Execute(context.Background(), connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "notify",
		Config: stepConfig(t, map[string]interface{}{
			"channel": "#general",
			"message": "the build passed",
		}),
	}))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", gotContentType, "application/json")
	}
	if gotPayload.Channel != "#general" {
		t.Errorf("payload.Channel = %q, want %q", gotPayload.Channel, "#general")
	}
	if gotPayload.Text != "the build passed" {
		t.Errorf("payload.Text = %q, want %q", gotPayload.Text, "the build passed")
	}

	if !resp.Msg.GetSuccess() {
		t.Fatalf("Success = false, want true (error: %q)", resp.Msg.GetError())
	}
	messageTS := resp.Msg.GetResult().GetFields()["message_ts"].GetStringValue()
	if messageTS != "1234567890.123456" {
		t.Errorf("result.message_ts = %q, want %q", messageTS, "1234567890.123456")
	}
}

// TestExecute_Success_PlainTextResponse proves a legacy Slack incoming
// webhook's plain "ok" response (not JSON) doesn't break Execute — it's a
// successful post with nothing structured to surface as message_ts, not an
// error. See resultStruct's doc comment.
func TestExecute_Success_PlainTextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	h := NewHandler(noopLogger{}, http.DefaultClient, func() string { return server.URL })

	resp, err := h.Execute(context.Background(), connect.NewRequest(&stepexecutorv1.StepRequest{
		Config: stepConfig(t, map[string]interface{}{
			"channel": "#general",
			"message": "hello",
		}),
	}))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !resp.Msg.GetSuccess() {
		t.Fatalf("Success = false, want true (error: %q)", resp.Msg.GetError())
	}
	if resp.Msg.GetResult() == nil {
		t.Error("Result is nil, want a non-nil (possibly empty) Struct")
	}
}

func TestExecute_MissingChannel(t *testing.T) {
	h := NewHandler(noopLogger{}, http.DefaultClient, func() string { return "http://unused.invalid" })

	resp, err := h.Execute(context.Background(), connect.NewRequest(&stepexecutorv1.StepRequest{
		Config: stepConfig(t, map[string]interface{}{
			"message": "hello",
		}),
	}))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("Success = true, want false for missing channel")
	}
	if resp.Msg.GetError() == "" {
		t.Error("Error is empty, want a message describing the missing channel field")
	}
}

func TestExecute_MissingMessage(t *testing.T) {
	h := NewHandler(noopLogger{}, http.DefaultClient, func() string { return "http://unused.invalid" })

	resp, err := h.Execute(context.Background(), connect.NewRequest(&stepexecutorv1.StepRequest{
		Config: stepConfig(t, map[string]interface{}{
			"channel": "#general",
		}),
	}))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("Success = true, want false for missing message")
	}
}

func TestExecute_NilConfig(t *testing.T) {
	h := NewHandler(noopLogger{}, http.DefaultClient, func() string { return "http://unused.invalid" })

	_, err := h.Execute(context.Background(), connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "notify",
	}))
	if err == nil {
		t.Fatal("expected an error for a nil Config, got nil")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected a *connect.Error, got %T: %v", err, err)
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("Code = %v, want %v", connectErr.Code(), connect.CodeInvalidArgument)
	}
}

func TestExecute_SlackNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid_payload"))
	}))
	defer server.Close()

	h := NewHandler(noopLogger{}, http.DefaultClient, func() string { return server.URL })

	resp, err := h.Execute(context.Background(), connect.NewRequest(&stepexecutorv1.StepRequest{
		Config: stepConfig(t, map[string]interface{}{
			"channel": "#general",
			"message": "hello",
		}),
	}))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("Success = true, want false for a non-2xx slack response")
	}
	if resp.Msg.GetError() == "" {
		t.Error("Error is empty, want a message describing the slack failure")
	}
}

func TestExecute_MissingWebhookURL(t *testing.T) {
	h := NewHandler(noopLogger{}, http.DefaultClient, func() string { return "" })

	resp, err := h.Execute(context.Background(), connect.NewRequest(&stepexecutorv1.StepRequest{
		Config: stepConfig(t, map[string]interface{}{
			"channel": "#general",
			"message": "hello",
		}),
	}))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("Success = true, want false when slack.webhook_url is unconfigured")
	}
}
