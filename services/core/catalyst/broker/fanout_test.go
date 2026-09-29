// This file is the end-to-end regression test for the Consume fan-out bug: it drives the broker
// through the exact same wire path a real client does (a real HTTP/2 server, the generated Connect
// clients, real streaming RPCs) rather than calling internal methods directly, so it would have
// failed against the pre-2026-09-27 code the same way a real second subscriber did in production —
// see atomicMap.go's Broadcast for the bug this proves fixed.
package broker

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// noopLogger is a minimal chassis.Logger for tests — mirrors the same shape
// services/tooling/bench/rpc_test.go and services/tooling/foundry/rpc_test.go already use.
type noopLogger struct{}

func (noopLogger) Start(chassis.Config)                   {}
func (noopLogger) SetLevel(chassis.LogLevel)              {}
func (noopLogger) GetLevel() chassis.LogLevel             { return chassis.InfoLevel }
func (noopLogger) Wrap(err error) error                   { return err }
func (l noopLogger) WithError(error) chassis.Logger       { return l }
func (l noopLogger) WithContext(context.Context) chassis.Logger { return l }
func (l noopLogger) WithField(string, any) chassis.Logger  { return l }
func (l noopLogger) WithFields(chassis.Fields) chassis.Logger { return l }
func (l noopLogger) WithCallDepth(int) chassis.Logger      { return l }
func (noopLogger) Trace(string)                            {}
func (noopLogger) Debug(string)                             {}
func (noopLogger) Debugf(string, ...any)                    {}
func (noopLogger) Info(string)                              {}
func (noopLogger) Infof(string, ...any)                     {}
func (noopLogger) Warn(string)                              {}
func (noopLogger) Warnf(string, ...any)                     {}
func (noopLogger) Error(string)                             {}
func (noopLogger) Errorf(string, ...any)                    {}
func (noopLogger) WrappedError(error, string)               {}
func (noopLogger) Fatal(string)                             {}
func (noopLogger) Panic(string)                             {}

// newTestServer starts the broker's real RPC handlers (the same acConnect.NewProducerHandler /
// NewConsumerHandler RegisterRPC mounts, wired here without the rest of chassis.Runtime since a
// plain http.ServeMux is all Connect needs) behind a real HTTP/2 (h2c, cleartext) server — Connect's
// bidi and server-streaming RPCs need HTTP/2, which httptest.NewServer alone does not speak.
func newTestServer(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	controller := NewController(noopLogger{}, NewNoopStore())
	h := NewRPC(noopLogger{}, controller)

	mux := http.NewServeMux()
	pattern, handler := acConnect.NewProducerHandler(h)
	mux.Handle(pattern, handler)
	pattern, handler = acConnect.NewConsumerHandler(h)
	mux.Handle(pattern, handler)

	srv := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))
	t.Cleanup(srv.Close)

	// h2c (cleartext HTTP/2): the same "AllowHTTP + DialTLS returns a plain TCP conn"
	// prior-knowledge trick used throughout this codebase (chassis's wideEventH2CClient,
	// pkg/chassis/builder.go's newBlueprintClient) to speak HTTP/2 streaming without TLS.
	client := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
	}
	return srv, client
}

// receiveWithTimeout blocks for stream.Receive() to return true, or fails the test after timeout —
// a real regression here is a permanent hang (the message never arrives), not a slow one, so a
// generous timeout only guards against that without making the test itself slow to pass.
func receiveWithTimeout(t *testing.T, name string, stream *connect.ServerStreamForClient[acv1.ConsumeResponse], timeout time.Duration) *acv1.CloudEvent {
	t.Helper()
	done := make(chan bool, 1)
	go func() { done <- stream.Receive() }()
	select {
	case ok := <-done:
		if !ok {
			t.Fatalf("%s: stream ended without a message: %v", name, stream.Err())
		}
		return stream.Msg().GetMessage()
	case <-time.After(timeout):
		t.Fatalf("%s: no message received within %s", name, timeout)
		return nil
	}
}

// openResult is what an asynchronously-opened Consume call eventually produces.
type openResult struct {
	stream *connect.ServerStreamForClient[acv1.ConsumeResponse]
	err    error
}

// openConsumerAsync starts a Consume call in its own goroutine and returns a channel for its
// result rather than the result itself. It must run async: rpc.Consume (see rpc.go) never calls
// stream.Send until an event actually matches, and Go's HTTP/2 server does not flush response
// headers before a handler's first Write — so consumerClient.Consume itself does not return until
// the first matching event is produced. Calling it synchronously here, before the matching
// Produce below, would deadlock the test on the very first open.
func openConsumerAsync(ctx context.Context, consumerClient acConnect.ConsumerClient, eventType, source string) <-chan openResult {
	out := make(chan openResult, 1)
	go func() {
		stream, err := consumerClient.Consume(ctx, connect.NewRequest(&acv1.ConsumeRequest{
			Message: &acv1.CloudEvent{Source: source, Type: eventType},
		}))
		out <- openResult{stream: stream, err: err}
	}()
	return out
}

// awaitConsumer waits for an openConsumerAsync result and returns its first received message,
// failing the test on error or timeout.
func awaitConsumer(t *testing.T, name string, ch <-chan openResult, timeout time.Duration) *acv1.CloudEvent {
	t.Helper()
	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatalf("Consume(%s): %v", name, res.err)
		}
		return receiveWithTimeout(t, name, res.stream, timeout)
	case <-time.After(timeout):
		t.Fatalf("%s: Consume never returned within %s", name, timeout)
		return nil
	}
}

// TestConsumeFansOutToEveryMatchingSubscriber is the black-box regression test for the bug
// atomicMap.Broadcast's doc comment describes: two independent Consume subscribers open for the
// SAME declared event type, one Produce call, both must receive it. Against the pre-2026-09-27
// broker this failed exactly as it did in production — the event reached only one of the two,
// chosen essentially at random — because every subscriber, of every declared type, shared one
// routing key and one unbuffered delivery channel.
func TestConsumeFansOutToEveryMatchingSubscriber(t *testing.T) {
	srv, client := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consumerClient := acConnect.NewConsumerClient(client, srv.URL, connect.WithGRPC())

	first := openConsumerAsync(ctx, consumerClient, "widget.created", "consumer-a")
	second := openConsumerAsync(ctx, consumerClient, "widget.created", "consumer-b")
	other := openConsumerAsync(ctx, consumerClient, "gadget.created", "consumer-c") // a different declared type

	// Registration is a handoff onto an unbuffered channel inside the RPC handler
	// (consumer.Consume → controller.consume): by the time the request body has reached the
	// server, controller.consume's background goroutine has dequeued it and called Subscribe —
	// all of which happens independently of, and before, the client ever sees response headers
	// (see openConsumerAsync). A brief pause covers the small remaining gap between the request
	// landing and Subscribe returning, exactly as a real client already has to tolerate: nothing
	// here promises a subscription is live the instant Consume() is called, in this
	// implementation or any pub/sub system's.
	time.Sleep(100 * time.Millisecond)

	producerClient := acConnect.NewProducerClient(client, srv.URL, connect.WithGRPC())
	produceStream := producerClient.Produce(ctx)
	defer produceStream.CloseRequest()
	if err := produceStream.Send(&acv1.ProduceRequest{
		Message: &acv1.CloudEvent{Id: "evt-1", Source: "producer", Type: "widget.created", SpecVersion: "1.0"},
	}); err != nil {
		t.Fatalf("Send widget.created: %v", err)
	}
	// A second, differently-typed event lets consumer-c's Consume call resolve too (it never
	// received anything for evt-1, so without this it would just hang until ctx is cancelled at
	// the end of the test) — and doubles as confirmation that type-based routing, not just
	// fan-out, survives the real wire path.
	if err := produceStream.Send(&acv1.ProduceRequest{
		Message: &acv1.CloudEvent{Id: "evt-2", Source: "producer", Type: "gadget.created", SpecVersion: "1.0"},
	}); err != nil {
		t.Fatalf("Send gadget.created: %v", err)
	}

	gotFirst := awaitConsumer(t, "consumer-a", first, 2*time.Second)
	gotSecond := awaitConsumer(t, "consumer-b", second, 2*time.Second)
	gotOther := awaitConsumer(t, "consumer-c", other, 2*time.Second)

	if gotFirst.GetId() != "evt-1" {
		t.Errorf("consumer-a got %+v, want evt-1", gotFirst)
	}
	if gotSecond.GetId() != "evt-1" {
		t.Errorf("consumer-b got %+v, want evt-1", gotSecond)
	}
	if gotOther.GetId() != "evt-2" {
		t.Errorf("consumer-c got %+v, want evt-2 (its own declared type, not consumer-a/b's evt-1)", gotOther)
	}
}
