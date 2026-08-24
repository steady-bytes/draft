package chassis

// otel_trace.go is Phase 10 of Beacon's observability plan (see
// docs/website/content/docs/architecture/beacon-observability.md, "Phase 10 —
// chassis integration"): RPC-handler middleware that opens one span per
// request and ships it to Beacon over OTLP.
//
// Beacon's own RPC surface is Connect-RPC (see rpc.go's Rpcer.AddHandler and
// services/core/beacon/query/rpc.go's RegisterRPC, which calls the generated
// logsv1connect.NewLogsServiceHandler(h) etc. — the same pattern
// services/core/catalyst/broker/rpc.go's RegisterRPC follows for
// broker.NewBrokerServiceHandler(h)). Every generated NewXXXServiceHandler
// function accepts variadic connect.HandlerOption, and
// connect.WithInterceptors(...) is one — so a connect.Interceptor (per the
// Phase 10 brief's own framing of the choice) is the hook that slots into an
// existing NewXXXServiceHandler(h, ...) call with the least ceremony, no
// http.Handler-wrapping required:
//
//	pattern, handler := logsv1connect.NewLogsServiceHandler(h,
//		connect.WithInterceptors(chassis.NewTraceInterceptor()))
//	server.AddHandler(pattern, handler, true)
//
// This file is never wired into any existing service's RegisterRPC by this
// phase — see the Phase 10 report's scope boundary. See otel_logger.go's top
// doc comment for why the OTLP transport and protobuf wire encoding here are
// hand-rolled (no go.opentelemetry.io/proto/otlp, no google.golang.org/grpc
// ClientConn) rather than using generated client types.

import (
	"context"
	"crypto/rand"
	"time"

	"connectrpc.com/connect"
)

// ─── Trace/Span (trace/v1, collector/trace/v1) ──────────────────────────────
//
// Field numbers verified against trace.pb.go / trace_service.pb.go in
// $GOMODCACHE/go.opentelemetry.io/proto/otlp@v1.7.0/trace/v1/ and
// collector/trace/v1/ — see otel_logger.go's top doc comment for the
// verification methodology shared by every OTLP message this exporter
// builds.

// OTel span kind/status-code enum values, verified against trace.pb.go's
// Span_SPAN_KIND_* / Status_STATUS_CODE_* consts.
const (
	spanKindServer  = 2 // Span_SPAN_KIND_SERVER — this middleware only wraps inbound RPC handlers.
	statusCodeOK    = 1 // Status_STATUS_CODE_OK
	statusCodeError = 2 // Status_STATUS_CODE_ERROR
)

// newTraceID/newSpanID generate random 16/8-byte IDs per the OTel spec
// (trace_id and span_id are raw binary on the wire — see trace.pb.go's
// `TraceId []byte protobuf:"bytes,1,...` / `SpanId []byte
// protobuf:"bytes,2,...`; hex-encoding is only a display/storage convention,
// e.g. services/core/beacon/store's LogRow/SpanRow and
// ingest/logs.go|traces.go's hex.EncodeToString calls). crypto/rand.Read
// never returns a short read or non-nil error for these fixed, small buffer
// sizes on any platform Go supports, so the error is intentionally
// discarded rather than threaded through every caller of an otherwise
// infallible ID generator.
func newTraceID() []byte {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return b
}

func newSpanID() []byte {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return b
}

// encodeStatus encodes a Status message. Field numbers (trace.pb.go):
// message=2 (string), code=3 (varint enum) — field 1 (deprecated_code) no
// longer exists in the v1.7.0 generated struct and is never written.
func encodeStatus(code uint64, message string) []byte {
	var buf []byte
	buf = appendStringField(buf, 2, message)
	buf = appendVarintField(buf, 3, code)
	return buf
}

// encodeSpan encodes one Span. Field numbers (trace.pb.go): trace_id=1
// (bytes), span_id=2 (bytes), name=5 (string), kind=6 (varint enum),
// start_time_unix_nano=7 (fixed64), end_time_unix_nano=8 (fixed64),
// attributes=9 (repeated KeyValue), status=15 (Status message).
// parent_span_id=4 is always omitted — this middleware only ever reports
// one span per request with no parent (see this file's top doc comment: no
// nested/child span propagation in this phase).
func encodeSpan(traceID, spanID []byte, name string, kind uint64, startNano, endNano uint64, attrs map[string]string, statusCode uint64, statusMsg string) []byte {
	var buf []byte
	buf = appendBytesField(buf, 1, traceID)
	buf = appendBytesField(buf, 2, spanID)
	buf = appendStringField(buf, 5, name)
	buf = appendVarintField(buf, 6, kind)
	buf = appendFixed64Field(buf, 7, startNano)
	buf = appendFixed64Field(buf, 8, endNano)
	buf = append(buf, encodeAttributesRepeated(9, attrs)...)
	buf = appendMessageField(buf, 15, encodeStatus(statusCode, statusMsg))
	return buf
}

// encodeExportTraceRequest wraps one or more encoded Spans into a full
// ExportTraceServiceRequest: Span -> ScopeSpans.spans=2 ->
// ResourceSpans{resource=1, scope_spans=2} ->
// ExportTraceServiceRequest.resource_spans=1 (trace.pb.go /
// trace_service.pb.go).
func encodeExportTraceRequest(serviceName string, spans [][]byte) []byte {
	var scopeSpans []byte
	for _, s := range spans {
		scopeSpans = appendMessageField(scopeSpans, 2, s)
	}

	var resourceSpans []byte
	resourceSpans = appendMessageField(resourceSpans, 1, encodeResource(serviceName))
	resourceSpans = appendMessageField(resourceSpans, 2, scopeSpans)

	var req []byte
	req = appendMessageField(req, 1, resourceSpans)
	return req
}

// ─── Interceptor ─────────────────────────────────────────────────────────────

// otelInterceptor implements connect.Interceptor, opening one root span per
// unary or server-streaming request it wraps and exporting it to Beacon over
// OTLP on completion.
type otelInterceptor struct {
	exporter    *OTelExporter
	serviceName string
}

// NewTraceInterceptor constructs a connect.Interceptor a service passes to
// a generated v1connect.NewXXXServiceHandler(h, connect.WithInterceptors(...))
// call — see this file's top doc comment for the exact call shape. Reads the
// same "telemetry" config section as OTelLogger (see
// OTelExporterConfig) via chassis.GetConfig(); when telemetry.enabled is
// false, every wrapped call is a zero-overhead passthrough to next.
func NewTraceInterceptor() connect.Interceptor {
	cfg := GetConfig()
	return &otelInterceptor{
		exporter:    newOTelExporter(cfg),
		serviceName: cfg.Name(),
	}
}

func (i *otelInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if !i.exporter.Enabled() {
			return next(ctx, req)
		}
		start := time.Now()
		traceID, spanID := newTraceID(), newSpanID()
		res, err := next(ctx, req)
		i.reportSpan(traceID, spanID, req.Spec().Procedure, start, time.Now(), err)
		return res, err
	}
}

// WrapStreamingClient is left as a pure passthrough: this middleware
// instruments inbound RPC *handlers* (server-side), not outbound calls this
// process makes as a client — see this file's top doc comment.
func (i *otelInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *otelInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if !i.exporter.Enabled() {
			return next(ctx, conn)
		}
		start := time.Now()
		traceID, spanID := newTraceID(), newSpanID()
		err := next(ctx, conn)
		i.reportSpan(traceID, spanID, conn.Spec().Procedure, start, time.Now(), err)
		return err
	}
}

// reportSpan builds and (async, fire-and-forget) exports one Span covering
// [start, end] for procedure, with OK/ERROR status derived from err.
func (i *otelInterceptor) reportSpan(traceID, spanID []byte, procedure string, start, end time.Time, err error) {
	statusCode := uint64(statusCodeOK)
	statusMsg := ""
	if err != nil {
		statusCode = statusCodeError
		statusMsg = err.Error()
	}

	span := encodeSpan(traceID, spanID, procedure, spanKindServer,
		uint64(start.UnixNano()), uint64(end.UnixNano()), nil, statusCode, statusMsg)
	body := encodeExportTraceRequest(i.serviceName, [][]byte{span})
	i.exporter.Export(tracesExportProcedure, body)
}
