package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// fakeProducer is a minimal acConnect.ProducerHandler that records every
// CloudEvent sent to it on received, then blocks (mirroring real
// Catalyst's own Produce behavior of holding the stream open until the
// client disconnects) until the request context is done.
type fakeProducer struct {
	acConnect.UnimplementedProducerHandler
	received chan *acv1.CloudEvent
}

func (f *fakeProducer) Produce(ctx context.Context, stream *connect.BidiStream[acv1.ProduceRequest, acv1.ProduceResponse]) error {
	for {
		req, err := stream.Receive()
		if err != nil {
			return nil
		}
		select {
		case f.received <- req.GetMessage():
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// newFakeProducerServer starts an httptest.Server that behaves like
// Catalyst's Producer service, h2c'd for the same reason
// catalyst-consume's newFakeConsumerServer is.
func newFakeProducerServer(t *testing.T) (*httptest.Server, chan *acv1.CloudEvent) {
	t.Helper()
	received := make(chan *acv1.CloudEvent, 8)
	mux := http.NewServeMux()
	pattern, handler := acConnect.NewProducerHandler(&fakeProducer{received: received})
	mux.Handle(pattern, handler)
	srv := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))
	t.Cleanup(srv.Close)
	return srv, received
}

func newTestHandler(t *testing.T, catalystAddr string) Handler {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return NewHandler(ctx, noopLogger{}, catalystAddr)
}

func TestExecute_Success(t *testing.T) {
	srv, received := newFakeProducerServer(t)
	h := newTestHandler(t, srv.URL)

	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "publish-order",
		Config: stepConfig(t, map[string]interface{}{
			"event_type": "myapp.v1.OrderPlaced",
			"subject":    "order-123",
			"data":       map[string]interface{}{"order_id": "order-123", "total": 42.5},
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
	if result["type"] != "myapp.v1.OrderPlaced" {
		t.Errorf("result.type = %v, want %q", result["type"], "myapp.v1.OrderPlaced")
	}
	if result["event_id"] == "" || result["event_id"] == nil {
		t.Error("result.event_id is empty")
	}
	if result["source"] != defaultSource {
		t.Errorf("result.source = %v, want %q", result["source"], defaultSource)
	}

	select {
	case event := <-received:
		if event.GetType() != "myapp.v1.OrderPlaced" {
			t.Errorf("received event Type = %q, want %q", event.GetType(), "myapp.v1.OrderPlaced")
		}
		if event.GetAttributes()["subject"].GetCeString() != "order-123" {
			t.Errorf("received event subject = %q, want %q", event.GetAttributes()["subject"].GetCeString(), "order-123")
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(event.GetTextData()), &payload); err != nil {
			t.Fatalf("received event payload was not valid JSON: %v", err)
		}
		if payload["order_id"] != "order-123" {
			t.Errorf("received event payload.order_id = %v, want %q", payload["order_id"], "order-123")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fake producer never received the published event")
	}
}

func TestExecute_CustomSource(t *testing.T) {
	srv, received := newFakeProducerServer(t)
	h := newTestHandler(t, srv.URL)

	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "publish",
		Config: stepConfig(t, map[string]interface{}{
			"event_type": "myapp.v1.Thing",
			"source":     "/tests/custom-source",
		}),
	})

	resp, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !resp.Msg.GetSuccess() {
		t.Fatalf("Success = false, want true (error: %s)", resp.Msg.GetError())
	}
	if got := resp.Msg.GetResult().AsMap()["source"]; got != "/tests/custom-source" {
		t.Errorf("result.source = %v, want %q", got, "/tests/custom-source")
	}

	select {
	case event := <-received:
		if event.GetSource() != "/tests/custom-source" {
			t.Errorf("received event Source = %q, want %q", event.GetSource(), "/tests/custom-source")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fake producer never received the published event")
	}
}

func TestExecute_MissingEventType(t *testing.T) {
	srv, _ := newFakeProducerServer(t)
	h := newTestHandler(t, srv.URL)

	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "publish",
		Config:   stepConfig(t, map[string]interface{}{}),
	})

	resp, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("Success = true, want false for a missing event_type")
	}
	if resp.Msg.GetError() == "" {
		t.Error("Error is empty, want a message about the missing event_type")
	}
}

func TestExecute_NilConfigIsRPCError(t *testing.T) {
	srv, _ := newFakeProducerServer(t)
	h := newTestHandler(t, srv.URL)

	req := connect.NewRequest(&stepexecutorv1.StepRequest{StepName: "publish"})

	_, err := h.Execute(context.Background(), req)
	if err == nil {
		t.Fatal("Execute returned nil error for a nil config, want a CodeInvalidArgument error")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("error code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
}

func TestExecute_UnreachableCatalystFailsTheStep(t *testing.T) {
	h := newTestHandler(t, "http://127.0.0.1:1") // nothing listens here

	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "publish",
		Config: stepConfig(t, map[string]interface{}{
			"event_type": "myapp.v1.Thing",
		}),
	})

	resp, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("Success = true, want false when Catalyst is unreachable")
	}
	if resp.Msg.GetError() == "" {
		t.Error("Error is empty, want a message about the failed publish")
	}
}

func TestExecute_DelayWaitsBeforePublishing(t *testing.T) {
	srv, received := newFakeProducerServer(t)
	h := newTestHandler(t, srv.URL)

	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "publish-delayed",
		Config: stepConfig(t, map[string]interface{}{
			"event_type": "myapp.v1.Delayed",
			"delay":      "150ms",
		}),
	})

	start := time.Now()
	resp, err := h.Execute(context.Background(), req)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !resp.Msg.GetSuccess() {
		t.Fatalf("Success = false, want true (error: %s)", resp.Msg.GetError())
	}
	if elapsed < 150*time.Millisecond {
		t.Errorf("Execute returned after %s, want it to have waited out the 150ms delay", elapsed)
	}

	select {
	case <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("fake producer never received the delayed event")
	}
}

func TestExecute_DelayRespectsContextCancellation(t *testing.T) {
	srv, _ := newFakeProducerServer(t)
	h := newTestHandler(t, srv.URL)

	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "publish-delayed",
		Config: stepConfig(t, map[string]interface{}{
			"event_type": "myapp.v1.Delayed",
			"delay":      "10s",
		}),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	resp, err := h.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("Success = true, want false when the context is canceled during config.delay")
	}
}

func TestExecute_InvalidDelay(t *testing.T) {
	srv, _ := newFakeProducerServer(t)
	h := newTestHandler(t, srv.URL)

	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "publish-delayed",
		Config: stepConfig(t, map[string]interface{}{
			"event_type": "myapp.v1.Delayed",
			"delay":      "not-a-duration",
		}),
	})

	resp, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("Success = true, want false for an invalid config.delay")
	}
}
