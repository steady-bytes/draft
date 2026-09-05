package chassis

// cloud_event_trace.go generalizes the W3C traceparent propagation
// otel_trace.go already does for RPC calls (TraceParentHeader for a
// connect-go/plain-http outbound call, NewTraceClientInterceptor for a
// connect-go client) to CloudEvents produced/consumed over Catalyst — the
// message-queue analog of the same problem. Without this, a CloudEvent
// produced by code running inside a traced span carries no record of that
// span at all, so a consumer's own span (or WideEvent) always starts a new,
// disconnected trace instead of continuing the one that caused the event —
// same gap TraceParentHeader already closed for direct RPC/HTTP calls, just
// unaddressed for the async/event-driven path until now.
//
// Convention: a CloudEvent extension attribute named "traceparent", same
// key and value format as the HTTP header (see otel_trace.go's
// formatTraceParent/parseTraceParent) — a producer that's inside a traced
// span sets it via CloudEventTraceParentAttribute; a consumer reconstructs
// the parent via StartSpanFromTraceParent. Adoption is per-producer/
// per-consumer, same as TraceParentHeader always was — nothing enforces
// this repo-wide.

import (
	"context"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
)

// CloudEventTraceParentAttributeKey is the CloudEvent extension attribute
// key a producer sets (via CloudEventTraceParentAttribute) and a consumer
// reads (via StartSpanFromTraceParent) to propagate trace context through
// Catalyst.
const CloudEventTraceParentAttributeKey = "traceparent"

// CloudEventTraceParentAttribute returns a CloudEvent extension attribute
// value carrying ctx's current span as a W3C traceparent (see
// TraceParentHeader) — the message-queue analog of TraceParentHeader for an
// outbound RPC call. Attach it to a CloudEvent's Attributes map under
// CloudEventTraceParentAttributeKey before producing:
//
//	event := &acv1.CloudEvent{ /* ... */ Attributes: map[string]*acv1.CloudEvent_CloudEventAttributeValue{} }
//	if attr, ok := chassis.CloudEventTraceParentAttribute(ctx); ok {
//		event.Attributes[chassis.CloudEventTraceParentAttributeKey] = attr
//	}
//
// Returns ok=false if ctx carries no span — the caller should simply not
// set the attribute in that case, not treat it as an error.
func CloudEventTraceParentAttribute(ctx context.Context) (*acv1.CloudEvent_CloudEventAttributeValue, bool) {
	traceIDHex, spanIDHex, _, ok := spanFromContext(ctx)
	if !ok {
		return nil, false
	}
	return &acv1.CloudEvent_CloudEventAttributeValue{
		Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeString{
			CeString: formatTraceParent(traceIDHex, spanIDHex),
		},
	}, true
}

// StartSpanFromTraceParent is StartSpan for a consumer whose parent context
// comes from a received message rather than an inbound HTTP request —
// exactly the case an inbound traceparent header covers for
// NewTraceInterceptor, generalized to any traceparent string a caller
// already has in hand (e.g. a consumed CloudEvent's
// CloudEventTraceParentAttributeKey attribute). Falls back to StartSpan's
// own behavior (continue whatever ctx already carries, or start a fresh
// trace) if traceparent is empty or malformed — a producer that predates
// this convention, or didn't run inside a traced span, is not an error
// case.
func StartSpanFromTraceParent(ctx context.Context, name string, traceparent string) (context.Context, *Span) {
	if traceID, parentID, ok := parseTraceParent(traceparent); ok {
		ctx = withSpanContext(ctx, traceID, parentID, newSpanLogBuffer())
	}
	return StartSpan(ctx, name)
}
