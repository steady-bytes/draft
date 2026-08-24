package query

import (
	"context"
	"strings"

	tracesv1 "github.com/steady-bytes/draft/api/core/observability/traces/v1"

	"connectrpc.com/connect"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/services/core/beacon/store"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type (
	// TracesController is Beacon's business logic for SearchTraces/GetTrace:
	// parse the BeaconQL filter once (the shared parser — see beaconql.go),
	// compile it against trace_roots's columns, and hand the bounded query off
	// to the store. Unlike Controller (logs), there is no live-tail half — see
	// TraceReceiver's doc comment in ingest/traces.go for why.
	TracesController interface {
		// SearchTraces returns trace_roots rows matching filter, most recent
		// first, bounded by limit and optionally paginated backward from before.
		SearchTraces(ctx context.Context, filter string, limit int32, before string) ([]store.TraceRootRow, error)
		// GetTrace returns every span for traceID, ordered for flame-graph
		// assembly (see store.Storer.GetTraceSpans).
		GetTrace(ctx context.Context, traceID string) ([]store.SpanRow, error)
	}

	tracesController struct {
		logger chassis.Logger
		store  store.Storer
	}
)

func NewTracesController(logger chassis.Logger, storer store.Storer) TracesController {
	return &tracesController{logger: logger, store: storer}
}

func (c *tracesController) SearchTraces(ctx context.Context, filter string, limit int32, before string) ([]store.TraceRootRow, error) {
	ast, err := ParseBeaconQL(filter)
	if err != nil {
		return nil, err
	}
	whereSQL, args, err := CompileTraceRoot(ast)
	if err != nil {
		return nil, err
	}
	return c.store.QueryTraceRoots(ctx, whereSQL, args, limit, before)
}

func (c *tracesController) GetTrace(ctx context.Context, traceID string) ([]store.SpanRow, error) {
	if strings.TrimSpace(traceID) == "" {
		return nil, &ParseError{Msg: "trace_id is required"}
	}
	return c.store.GetTraceSpans(ctx, traceID)
}

// ─── Connect-RPC handlers ───────────────────────────────────────────────────
//
// Implemented on the same *rpc type as QueryLogs/StreamLogs (see rpc.go) so
// one Rpc value satisfies both LogsServiceHandler and TracesServiceHandler —
// RegisterRPC (rpc.go) mounts both handler patterns on the same Connect-RPC
// server.

func (h *rpc) SearchTraces(ctx context.Context, req *connect.Request[tracesv1.SearchTracesRequest]) (*connect.Response[tracesv1.SearchTracesResponse], error) {
	rows, err := h.tracesController.SearchTraces(ctx, req.Msg.GetFilter(), req.Msg.GetLimit(), req.Msg.GetBefore())
	if err != nil {
		return nil, toConnectError(err)
	}
	traces := make([]*tracesv1.TraceRoot, len(rows))
	for i, r := range rows {
		traces[i] = traceRootToProto(r)
	}
	return connect.NewResponse(&tracesv1.SearchTracesResponse{Traces: traces}), nil
}

func (h *rpc) GetTrace(ctx context.Context, req *connect.Request[tracesv1.GetTraceRequest]) (*connect.Response[tracesv1.GetTraceResponse], error) {
	rows, err := h.tracesController.GetTrace(ctx, req.Msg.GetTraceId())
	if err != nil {
		return nil, toConnectError(err)
	}
	spans := make([]*tracesv1.Span, len(rows))
	for i, r := range rows {
		spans[i] = spanRowToProto(r)
	}
	return connect.NewResponse(&tracesv1.GetTraceResponse{Spans: spans}), nil
}

func traceRootToProto(r store.TraceRootRow) *tracesv1.TraceRoot {
	return &tracesv1.TraceRoot{
		TraceId:     r.TraceID,
		ServiceName: r.ServiceName,
		SpanName:    r.SpanName,
		StartTime:   timestamppb.New(r.StartTime),
		DurationNs:  r.DurationNs,
		StatusCode:  r.StatusCode,
	}
}

func spanRowToProto(r store.SpanRow) *tracesv1.Span {
	return &tracesv1.Span{
		TraceId:      r.TraceID,
		SpanId:       r.SpanID,
		ParentSpanId: r.ParentSpanID,
		ServiceName:  r.ServiceName,
		SpanName:     r.SpanName,
		Kind:         r.Kind,
		StartTime:    timestamppb.New(r.StartTime),
		DurationNs:   r.DurationNs,
		StatusCode:   r.StatusCode,
		Attributes:   r.Attributes,
	}
}
