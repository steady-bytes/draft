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
	"encoding/hex"
	"strings"
	"sync"
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
	spanKindInternal = 1 // Span_SPAN_KIND_INTERNAL — StartSpan's manual spans (no request/response boundary of their own).
	spanKindServer   = 2 // Span_SPAN_KIND_SERVER — this middleware only wraps inbound RPC handlers.
	spanKindClient   = 3 // Span_SPAN_KIND_CLIENT — NewTraceClientInterceptor's outbound calls.
	statusCodeOK     = 1 // Status_STATUS_CODE_OK
	statusCodeError  = 2 // Status_STATUS_CODE_ERROR
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

// ─── Context propagation ─────────────────────────────────────────────────────
//
// Closes the gap OTelLogger.WithContext's doc comment used to describe as a
// deliberate v1 no-op ("no context-propagated trace correlation... a caller
// that wants a log record correlated with a trace can already do so
// explicitly via WithField"): withSpanContext attaches the span this
// interceptor just generated to ctx, so WithContext(ctx) can read it back and
// call WithField itself — every existing `logger.WithContext(ctx)` call site
// in a handler wrapped by NewTraceInterceptor starts correlating
// automatically, no caller changes needed.

type spanContextKey struct{}

type spanContext struct {
	traceIDHex string
	spanIDHex  string
}

// withSpanContext returns a copy of ctx carrying traceID/spanID (raw OTel
// bytes, hex-encoded here to match the "hex strings" convention
// OTelLogger.encodeRecord already expects under the "trace_id"/"span_id"
// field keys).
func withSpanContext(ctx context.Context, traceID, spanID []byte) context.Context {
	return context.WithValue(ctx, spanContextKey{}, spanContext{
		traceIDHex: hex.EncodeToString(traceID),
		spanIDHex:  hex.EncodeToString(spanID),
	})
}

// spanFromContext returns the trace/span ids withSpanContext attached, if
// any — false if ctx was never wrapped (no NewTraceInterceptor in the call
// path, or a context that didn't descend from the wrapped handler's).
func spanFromContext(ctx context.Context) (traceIDHex, spanIDHex string, ok bool) {
	sc, ok := ctx.Value(spanContextKey{}).(spanContext)
	if !ok {
		return "", "", false
	}
	return sc.traceIDHex, sc.spanIDHex, true
}

// ─── Cross-process propagation (W3C Trace Context) ──────────────────────────
//
// traceParentHeader is the standard "traceparent" header
// (https://www.w3.org/TR/trace-context/#traceparent-header): a single header
// carrying {version}-{trace-id}-{parent-id}-{flags}, all hex, so any two
// chassis services — or a future non-Go one — can hand off a trace across a
// network call without a bespoke wire format. Only version "00" is produced
// or accepted; flags is always "01" (sampled) since this exporter has no
// sampling concept yet — every span is exported.

const traceParentHeaderName = "traceparent"

// formatTraceParent builds a traceparent header value for the given
// trace/span id hex strings (16/8 bytes respectively).
func formatTraceParent(traceIDHex, spanIDHex string) string {
	return "00-" + traceIDHex + "-" + spanIDHex + "-01"
}

// parseTraceParent extracts the trace-id and parent-id from an inbound
// traceparent header value. Returns ok=false for anything that doesn't match
// the expected shape (missing header, a future version, malformed hex) —
// callers treat that identically to "no header sent": start a fresh trace
// rather than fail the request over a propagation-format mismatch.
func parseTraceParent(value string) (traceID, spanID []byte, ok bool) {
	parts := strings.Split(value, "-")
	if len(parts) != 4 || parts[0] != "00" {
		return nil, nil, false
	}
	traceID, err := hex.DecodeString(parts[1])
	if err != nil || len(traceID) != 16 {
		return nil, nil, false
	}
	spanID, err = hex.DecodeString(parts[2])
	if err != nil || len(spanID) != 8 {
		return nil, nil, false
	}
	return traceID, spanID, true
}

// TraceParentHeader returns a W3C "traceparent" header value for the span
// carried by ctx — pass it as the "traceparent" header (or use
// NewTraceClientInterceptor to do this automatically for every connect-go
// client call) on an outbound request so the callee's own
// NewTraceInterceptor continues this trace instead of starting a new,
// disconnected one. ok is false if ctx carries no span (e.g. it never
// descended from a NewTraceInterceptor-wrapped handler or a StartSpan call).
func TraceParentHeader(ctx context.Context) (string, bool) {
	traceIDHex, spanIDHex, ok := spanFromContext(ctx)
	if !ok {
		return "", false
	}
	return formatTraceParent(traceIDHex, spanIDHex), true
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
// (bytes), span_id=2 (bytes), parent_span_id=4 (bytes), name=5 (string),
// kind=6 (varint enum), start_time_unix_nano=7 (fixed64),
// end_time_unix_nano=8 (fixed64), attributes=9 (repeated KeyValue),
// status=15 (Status message). parentSpanID is omitted from the wire (per
// appendBytesField's zero-value-omission) when empty, which is correct for a
// trace root — a span propagated in from another process (see
// parseTraceParent) or created by a nested StartSpan call always has one.
func encodeSpan(traceID, spanID, parentSpanID []byte, name string, kind uint64, startNano, endNano uint64, attrs map[string]string, statusCode uint64, statusMsg string) []byte {
	var buf []byte
	buf = appendBytesField(buf, 1, traceID)
	buf = appendBytesField(buf, 2, spanID)
	buf = appendBytesField(buf, 4, parentSpanID)
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
		traceID, parentID := traceAndParentFromHeader(req.Header())
		spanID := newSpanID()
		res, err := next(withSpanContext(ctx, traceID, spanID), req)
		i.reportSpan(traceID, spanID, parentID, req.Spec().Procedure, start, time.Now(), err)
		return res, err
	}
}

// WrapStreamingClient is left as a pure passthrough: this middleware
// instruments inbound RPC *handlers* (server-side), not outbound calls this
// process makes as a client — see NewTraceClientInterceptor for that side.
func (i *otelInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *otelInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if !i.exporter.Enabled() {
			return next(ctx, conn)
		}
		start := time.Now()
		traceID, parentID := traceAndParentFromHeader(conn.RequestHeader())
		spanID := newSpanID()
		err := next(withSpanContext(ctx, traceID, spanID), conn)
		i.reportSpan(traceID, spanID, parentID, conn.Spec().Procedure, start, time.Now(), err)
		return err
	}
}

// traceAndParentFromHeader continues the trace named by an inbound
// traceparent header (see parseTraceParent) if the caller sent one — e.g. a
// bench step calling out via NewTraceClientInterceptor — so this request's
// span becomes a child of the caller's span in one continuous trace, rather
// than the disconnected new trace every inbound request used to start
// unconditionally. Falls back to a fresh trace (no parent) when the header
// is absent or malformed.
func traceAndParentFromHeader(h interface{ Get(string) string }) (traceID, parentID []byte) {
	if tid, pid, ok := parseTraceParent(h.Get(traceParentHeaderName)); ok {
		return tid, pid
	}
	return newTraceID(), nil
}

// reportSpan builds and (async, fire-and-forget) exports one Span covering
// [start, end] for procedure, with OK/ERROR status derived from err.
func (i *otelInterceptor) reportSpan(traceID, spanID, parentID []byte, procedure string, start, end time.Time, err error) {
	statusCode := uint64(statusCodeOK)
	statusMsg := ""
	if err != nil {
		statusCode = statusCodeError
		statusMsg = err.Error()
	}

	span := encodeSpan(traceID, spanID, parentID, procedure, spanKindServer,
		uint64(start.UnixNano()), uint64(end.UnixNano()), nil, statusCode, statusMsg)
	body := encodeExportTraceRequest(i.serviceName, [][]byte{span})
	i.exporter.Export(tracesExportProcedure, body)
}

// ─── Manual spans (StartSpan) ────────────────────────────────────────────────
//
// NewTraceInterceptor only ever covers the synchronous body of one inbound RPC
// handler call — exactly right for a request/response endpoint, wrong for any
// handler that kicks off background work and returns before that work is
// done (bench's Scheduler.StartRun is the motivating case: the handler
// returns as soon as one DB row is written, while the workflow's steps keep
// running in a detached goroutine for the run's whole lifetime). StartSpan
// lets that background code report its own spans directly, without needing
// an inbound RPC to hang off of.
//
// See docs/website/content/docs/architecture/beacon-observability.md's
// "Manual spans — chassis.StartSpan" section for a full worked example
// (bench's per-workflow/per-step tracing).

// manualSpanExporter/manualSpanService are process-wide singletons:
// newOTelExporter starts a background send-loop goroutine and its own HTTP
// client every time it's called (see NewTraceInterceptor and NewOTelLogger,
// each of which is constructed exactly once per service at startup and
// keeps its exporter for the process's lifetime) — StartSpan must not repeat
// that per call, since a workflow can call it once per step, many times a
// second.
var (
	manualSpanOnce     sync.Once
	manualSpanExporter *OTelExporter
	manualSpanService  string
)

func manualExporter() (*OTelExporter, string) {
	manualSpanOnce.Do(func() {
		cfg := GetConfig()
		manualSpanExporter = newOTelExporter(cfg)
		manualSpanService = cfg.Name()
	})
	return manualSpanExporter, manualSpanService
}

// Span is an in-flight unit of work started by StartSpan. Call End exactly
// once, when the traced operation finishes, to report it to Beacon. A Span
// is not safe for concurrent use — start one per goroutine/step, not one
// shared across concurrently-running steps.
type Span struct {
	traceID, spanID, parentID []byte
	name                      string
	start                     time.Time
	attrs                     map[string]string
}

// StartSpan starts a new span named name and returns a context carrying it
// (so a nested StartSpan call, an outbound NewTraceClientInterceptor-wrapped
// request, or logger.WithContext(ctx) all pick it up) plus the Span to End
// once the traced work finishes.
//
// If ctx already carries a span — whether from an inbound RPC handler
// wrapped by NewTraceInterceptor, an inbound traceparent header that handler
// continued, or an outer StartSpan call — the new span is a child in that
// same trace, so a call chain builds one continuous end-to-end trace instead
// of disconnected fragments. If ctx carries no span, StartSpan begins a new
// trace.
func StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	var traceID, parentID []byte
	if sc, ok := ctx.Value(spanContextKey{}).(spanContext); ok {
		traceID, _ = hex.DecodeString(sc.traceIDHex)
		parentID, _ = hex.DecodeString(sc.spanIDHex)
	}
	if len(traceID) == 0 {
		traceID = newTraceID()
	}
	spanID := newSpanID()
	span := &Span{traceID: traceID, spanID: spanID, parentID: parentID, name: name, start: time.Now()}
	return withSpanContext(ctx, traceID, spanID), span
}

// SetAttribute attaches a string attribute reported alongside the span when
// End is called. Safe to call multiple times with different keys; a repeated
// key overwrites its previous value.
func (s *Span) SetAttribute(key, value string) {
	if s.attrs == nil {
		s.attrs = make(map[string]string)
	}
	s.attrs[key] = value
}

// End reports the span to Beacon, covering [start-of-StartSpan, now], with
// an OK status if err is nil or an ERROR status carrying err.Error()
// otherwise. A disabled exporter (telemetry.enabled: false, or unset) makes
// this a no-op, matching NewTraceInterceptor's behavior.
func (s *Span) End(err error) {
	exporter, serviceName := manualExporter()
	if !exporter.Enabled() {
		return
	}
	statusCode := uint64(statusCodeOK)
	statusMsg := ""
	if err != nil {
		statusCode = statusCodeError
		statusMsg = err.Error()
	}

	encoded := encodeSpan(s.traceID, s.spanID, s.parentID, s.name, spanKindInternal,
		uint64(s.start.UnixNano()), uint64(time.Now().UnixNano()), s.attrs, statusCode, statusMsg)
	body := encodeExportTraceRequest(serviceName, [][]byte{encoded})
	exporter.Export(tracesExportProcedure, body)
}

// ─── Client-side propagation ─────────────────────────────────────────────────

// clientTraceInterceptor implements connect.Interceptor, injecting the
// current ctx span (if any) as a traceparent header on every outbound
// connect-go client call, so the callee's NewTraceInterceptor continues this
// trace instead of starting a disconnected new one.
type clientTraceInterceptor struct{}

// NewTraceClientInterceptor constructs a connect.Interceptor for a
// connect-go *client* (connect.NewXxxClient(httpClient, baseURL,
// connect.WithInterceptors(chassis.NewTraceClientInterceptor()))) —
// the outbound counterpart to NewTraceInterceptor. For a plain net/http
// call instead of a generated connect-go client (e.g. bench's
// bench://grpc-call@v1 executor, which builds its own *http.Request), set
// the header directly with TraceParentHeader instead of reaching for this.
func NewTraceClientInterceptor() connect.Interceptor {
	return clientTraceInterceptor{}
}

func (clientTraceInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if tp, ok := TraceParentHeader(ctx); ok {
			req.Header().Set(traceParentHeaderName, tp)
		}
		return next(ctx, req)
	}
}

func (clientTraceInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		if tp, ok := TraceParentHeader(ctx); ok {
			conn.RequestHeader().Set(traceParentHeaderName, tp)
		}
		return conn
	}
}

// WrapStreamingHandler is left as a pure passthrough: this interceptor
// instruments outbound calls this process makes as a client, not inbound
// handlers — see NewTraceInterceptor for that side.
func (clientTraceInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
