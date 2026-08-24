package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	stepexecutorv1 "github.com/steady-bytes/draft/api/tooling/step_executor/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/protobuf/types/known/structpb"
)

// noopLogger is a minimal chassis.Logger for tests: it satisfies the
// interface without writing anything anywhere. Mirrors
// services/tooling/slack-notify/rpc_test.go's noopLogger.
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

// fakeConsumer is a minimal acConnect.ConsumerHandler that sends exactly the
// events given to newFakeConsumerServer, one per Consume call, then blocks
// until the client disconnects or the request context is done — mirroring
// real Catalyst's own Consume behavior of holding the stream open
// indefinitely (services/core/catalyst/broker/rpc.go's `<-ctx.Done()`).
type fakeConsumer struct {
	acConnect.UnimplementedConsumerHandler
	events []*acv1.CloudEvent
}

func (f *fakeConsumer) Consume(ctx context.Context, _ *connect.Request[acv1.ConsumeRequest], stream *connect.ServerStream[acv1.ConsumeResponse]) error {
	for _, event := range f.events {
		if err := stream.Send(&acv1.ConsumeResponse{Message: event}); err != nil {
			return err
		}
	}
	<-ctx.Done()
	return ctx.Err()
}

// newFakeConsumerServer starts an httptest.Server that behaves like Catalyst's
// Consumer service, h2c'd the same way grpc_call_test.go's newConnectJSONServer
// is for the same underlying reason (Connect streaming needs HTTP/2).
func newFakeConsumerServer(t *testing.T, events ...*acv1.CloudEvent) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	pattern, handler := acConnect.NewConsumerHandler(&fakeConsumer{events: events})
	mux.Handle(pattern, handler)
	srv := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))
	t.Cleanup(srv.Close)
	return srv
}

func TestExecute_Success(t *testing.T) {
	srv := newFakeConsumerServer(t, &acv1.CloudEvent{
		Type:   "tooling.workflow.v1.RunFinished",
		Source: "/services/bench",
		Id:     "evt-1",
		Attributes: map[string]*acv1.CloudEvent_CloudEventAttributeValue{
			"subject": {Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeString{CeString: "run-123"}},
		},
		Data: &acv1.CloudEvent_TextData{TextData: `{"runId":"run-123","workflowName":"crud-e2e","status":"RUN_STATUS_PASSED"}`},
	})

	h := NewHandler(noopLogger{}, func() string { return srv.URL })

	resp, err := h.Execute(context.Background(), connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "wait-for-finish",
		Config: stepConfig(t, map[string]interface{}{
			"event_type": "tooling.workflow.v1.RunFinished",
			"timeout":    "2s",
			"fields": map[string]interface{}{
				"run_id":   "runId",
				"workflow": "workflowName",
			},
		}),
	}))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !resp.Msg.GetSuccess() {
		t.Fatalf("Success = false, Error = %q", resp.Msg.GetError())
	}

	result := resp.Msg.GetResult().GetFields()
	if got := result["run_id"].GetStringValue(); got != "run-123" {
		t.Errorf("result.run_id = %q, want %q", got, "run-123")
	}
	if got := result["workflow"].GetStringValue(); got != "crud-e2e" {
		t.Errorf("result.workflow = %q, want %q", got, "crud-e2e")
	}
	event := result["_event"].GetStructValue().GetFields()
	if got := event["subject"].GetStringValue(); got != "run-123" {
		t.Errorf("result._event.subject = %q, want %q", got, "run-123")
	}
	payload := result["payload"].GetStructValue().GetFields()
	if got := payload["status"].GetStringValue(); got != "RUN_STATUS_PASSED" {
		t.Errorf("result.payload.status = %q, want %q", got, "RUN_STATUS_PASSED")
	}
}

func TestExecute_SkipsNonMatchingTypesThenMatches(t *testing.T) {
	srv := newFakeConsumerServer(t,
		&acv1.CloudEvent{Type: "tooling.workflow.v1.RunStarted", Data: &acv1.CloudEvent_TextData{TextData: `{}`}},
		&acv1.CloudEvent{Type: "tooling.workflow.v1.StepCompleted", Data: &acv1.CloudEvent_TextData{TextData: `{}`}},
		&acv1.CloudEvent{Type: "tooling.workflow.v1.RunFinished", Data: &acv1.CloudEvent_TextData{TextData: `{"status":"RUN_STATUS_PASSED"}`}},
	)

	h := NewHandler(noopLogger{}, func() string { return srv.URL })

	resp, err := h.Execute(context.Background(), connect.NewRequest(&stepexecutorv1.StepRequest{
		Config: stepConfig(t, map[string]interface{}{
			"event_type": "tooling.workflow.v1.RunFinished",
			"timeout":    "2s",
		}),
	}))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !resp.Msg.GetSuccess() {
		t.Fatalf("Success = false, Error = %q", resp.Msg.GetError())
	}
	payload := resp.Msg.GetResult().GetFields()["payload"].GetStructValue().GetFields()
	if got := payload["status"].GetStringValue(); got != "RUN_STATUS_PASSED" {
		t.Errorf("payload.status = %q, want %q (should have skipped the two earlier non-matching events)", got, "RUN_STATUS_PASSED")
	}
}

func TestExecute_TimesOutWhenNoMatch(t *testing.T) {
	srv := newFakeConsumerServer(t, &acv1.CloudEvent{Type: "some.other.type", Data: &acv1.CloudEvent_TextData{TextData: `{}`}})

	h := NewHandler(noopLogger{}, func() string { return srv.URL })

	start := time.Now()
	resp, err := h.Execute(context.Background(), connect.NewRequest(&stepexecutorv1.StepRequest{
		Config: stepConfig(t, map[string]interface{}{
			"event_type": "tooling.workflow.v1.RunFinished",
			"timeout":    "200ms",
		}),
	}))
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("Success = true, want false (no matching event was ever sent)")
	}
	if !strings.Contains(resp.Msg.GetError(), "timed out") {
		t.Errorf("Error = %q, want it to mention a timeout", resp.Msg.GetError())
	}
	if elapsed < 200*time.Millisecond {
		t.Errorf("Execute returned after %s, want it to have actually waited out the configured timeout", elapsed)
	}
}

func TestExecute_MissingEventTypeConfig(t *testing.T) {
	h := NewHandler(noopLogger{}, func() string { return "http://unused" })

	resp, err := h.Execute(context.Background(), connect.NewRequest(&stepexecutorv1.StepRequest{
		Config: stepConfig(t, map[string]interface{}{}),
	}))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("Success = true, want false (event_type is required)")
	}
	if !strings.Contains(resp.Msg.GetError(), "event_type") {
		t.Errorf("Error = %q, want it to mention event_type", resp.Msg.GetError())
	}
}

func TestExecute_NilConfigIsRPCError(t *testing.T) {
	h := NewHandler(noopLogger{}, func() string { return "http://unused" })

	_, err := h.Execute(context.Background(), connect.NewRequest(&stepexecutorv1.StepRequest{}))
	if err == nil {
		t.Fatal("Execute returned nil error for a nil Config, want a connect error")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("error code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
}

func TestExtractPath(t *testing.T) {
	payload := map[string]interface{}{
		"runId": "run-1",
		"name": map[string]interface{}{
			"firstName": "Ada",
		},
	}

	tests := []struct {
		path   string
		want   interface{}
		wantOK bool
	}{
		{"runId", "run-1", true},
		{"name.firstName", "Ada", true},
		{"missing", nil, false},
		{"name.missing", nil, false},
		{"runId.tooDeep", nil, false},
	}
	for _, tt := range tests {
		got, ok := extractPath(payload, tt.path)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("extractPath(payload, %q) = (%v, %v), want (%v, %v)", tt.path, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestParseConfig_Defaults(t *testing.T) {
	eventType, timeout, fields, err := parseConfig(stepConfig(t, map[string]interface{}{
		"event_type": "tooling.workflow.v1.RunFinished",
	}))
	if err != nil {
		t.Fatalf("parseConfig returned unexpected error: %v", err)
	}
	if eventType != "tooling.workflow.v1.RunFinished" {
		t.Errorf("eventType = %q, want %q", eventType, "tooling.workflow.v1.RunFinished")
	}
	if timeout != defaultWaitTimeout {
		t.Errorf("timeout = %v, want default %v", timeout, defaultWaitTimeout)
	}
	if len(fields) != 0 {
		t.Errorf("fields = %v, want empty", fields)
	}
}

func TestParseConfig_InvalidTimeout(t *testing.T) {
	_, _, _, err := parseConfig(stepConfig(t, map[string]interface{}{
		"event_type": "x",
		"timeout":    "not-a-duration",
	}))
	if err == nil {
		t.Fatal("parseConfig returned nil error for an invalid timeout, want an error")
	}
}
