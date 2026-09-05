package chassis

import (
	"context"
	"encoding/hex"
	"testing"
)

// TestCloudEventTraceParentAttribute_NoSpan confirms a plain context (no
// span attached) correctly reports ok=false rather than fabricating a
// traceparent — a producer running outside any traced span should simply
// not set the attribute, not send a garbage one.
func TestCloudEventTraceParentAttribute_NoSpan(t *testing.T) {
	if _, ok := CloudEventTraceParentAttribute(context.Background()); ok {
		t.Fatal("expected ok=false for a context with no span")
	}
}

// TestCloudEventTraceParentAttribute_RoundTrip is the actual contract this
// whole file exists for: a producer inside a traced span attaches a
// traceparent to a CloudEvent (CloudEventTraceParentAttribute), and a
// consumer on the other end of Catalyst reconstructs it
// (StartSpanFromTraceParent) into a span that's a *child of the producer's
// own span*, in the *same trace* — proving the propagation actually chains
// correctly end to end, not just that each half compiles independently.
func TestCloudEventTraceParentAttribute_RoundTrip(t *testing.T) {
	// Simulate a producer's own request-scoped span the same way
	// otelInterceptor.WrapUnary does: withSpanContext(ctx, traceID, spanID, buf).
	traceID := newTraceID()
	producerSpanID := newSpanID()
	producerCtx := withSpanContext(context.Background(), traceID, producerSpanID, newSpanLogBuffer())

	attr, ok := CloudEventTraceParentAttribute(producerCtx)
	if !ok {
		t.Fatal("expected ok=true for a context carrying a span")
	}
	traceparent := attr.GetCeString()
	if traceparent == "" {
		t.Fatal("expected a non-empty ce_string traceparent value")
	}

	// Consumer side: no inbound HTTP request, just the traceparent string
	// pulled out of the received CloudEvent's own attribute.
	consumerCtx, span := StartSpanFromTraceParent(context.Background(), "handle-event", traceparent)

	if got, want := hex.EncodeToString(span.traceID), hex.EncodeToString(traceID); got != want {
		t.Fatalf("consumer span trace_id = %s, want %s (same trace as producer)", got, want)
	}
	if got, want := hex.EncodeToString(span.parentID), hex.EncodeToString(producerSpanID); got != want {
		t.Fatalf("consumer span parent_span_id = %s, want %s (producer's own span_id)", got, want)
	}

	// The returned context also carries the new (consumer) span, not the
	// producer's — a nested StartSpan/logger.WithContext call from here on
	// should build on the consumer's span, same as any other StartSpan caller.
	if _, _, _, ok := spanFromContext(consumerCtx); !ok {
		t.Fatal("expected the returned context to carry the new consumer span")
	}
}

// TestStartSpanFromTraceParent_Malformed confirms a malformed/empty
// traceparent falls back to StartSpan's own behavior (a fresh trace) rather
// than erroring — a producer that predates this convention, or didn't run
// inside a traced span, is not an error case.
func TestStartSpanFromTraceParent_Malformed(t *testing.T) {
	_, span := StartSpanFromTraceParent(context.Background(), "handle-event", "not-a-valid-traceparent")
	if len(span.traceID) == 0 {
		t.Fatal("expected a fresh trace_id to still be assigned")
	}
	if len(span.parentID) != 0 {
		t.Fatal("expected no parent_id for a malformed traceparent")
	}
}
