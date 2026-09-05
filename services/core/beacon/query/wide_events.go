package query

import (
	"context"
	"strings"

	wideeventsv1 "github.com/steady-bytes/draft/api/core/observability/wide_events/v1"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/services/core/beacon/store"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type (
	// WideEventsController is Beacon's business logic for
	// SearchWideEvents/GetWideEvent/CreateWideEvent — parse the BeaconQL
	// filter once (the shared parser, see beaconql.go), compile it against
	// wide_events' columns, and hand the bounded query off to the store.
	// Mirrors TracesController's shape exactly.
	WideEventsController interface {
		// SearchWideEvents returns wide_events rows matching filter, most
		// recent first, bounded by limit and optionally paginated backward
		// from before.
		SearchWideEvents(ctx context.Context, filter string, limit int32, before string) ([]store.WideEventRow, error)
		// GetWideEvent returns one row by span_id, full logs/attributes
		// included.
		GetWideEvent(ctx context.Context, spanID string) (store.WideEventRow, error)
	}

	wideEventsController struct {
		logger chassis.Logger
		store  store.Storer
	}
)

func NewWideEventsController(logger chassis.Logger, storer store.Storer) WideEventsController {
	return &wideEventsController{logger: logger, store: storer}
}

func (c *wideEventsController) SearchWideEvents(ctx context.Context, filter string, limit int32, before string) ([]store.WideEventRow, error) {
	ast, err := ParseBeaconQL(filter)
	if err != nil {
		return nil, err
	}
	whereSQL, args, err := CompileWideEvent(ast)
	if err != nil {
		return nil, err
	}
	return c.store.QueryWideEvents(ctx, whereSQL, args, limit, before)
}

func (c *wideEventsController) GetWideEvent(ctx context.Context, spanID string) (store.WideEventRow, error) {
	if strings.TrimSpace(spanID) == "" {
		return store.WideEventRow{}, &ParseError{Msg: "span_id is required"}
	}
	return c.store.GetWideEvent(ctx, spanID)
}

// ─── Connect-RPC handlers ───────────────────────────────────────────────────
//
// Implemented on the same *rpc type as QueryLogs/SearchTraces/etc (see
// rpc.go) so one Rpc value satisfies WideEventsServiceHandler too —
// RegisterRPC (rpc.go) mounts this handler pattern on the same Connect-RPC
// server as everything else.

func (h *rpc) SearchWideEvents(ctx context.Context, req *connect.Request[wideeventsv1.SearchWideEventsRequest]) (*connect.Response[wideeventsv1.SearchWideEventsResponse], error) {
	rows, err := h.wideEventsController.SearchWideEvents(ctx, req.Msg.GetFilter(), req.Msg.GetLimit(), req.Msg.GetBefore())
	if err != nil {
		return nil, toConnectError(err)
	}
	events := make([]*wideeventsv1.WideEvent, len(rows))
	for i, r := range rows {
		events[i] = wideEventRowToProto(r)
	}
	return connect.NewResponse(&wideeventsv1.SearchWideEventsResponse{Events: events}), nil
}

func (h *rpc) GetWideEvent(ctx context.Context, req *connect.Request[wideeventsv1.GetWideEventRequest]) (*connect.Response[wideeventsv1.GetWideEventResponse], error) {
	row, err := h.wideEventsController.GetWideEvent(ctx, req.Msg.GetSpanId())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&wideeventsv1.GetWideEventResponse{Event: wideEventRowToProto(row)}), nil
}

// CreateWideEvent submits a WideEvent directly -- the path for callers with
// no chassis span to hang one off of (chiefly the Dioxus/WASM web clients).
// It does not write to ClickHouse directly: it builds a CloudEvent from the
// request and produces it via Catalyst (h.produceWideEvent, wired up
// alongside the rest of *rpc's construction — see rpc.go), the same as
// chassis's own automatic per-span production, so every other consumer
// built against this stream sees browser-submitted WideEvents too.
func (h *rpc) CreateWideEvent(ctx context.Context, req *connect.Request[wideeventsv1.CreateWideEventRequest]) (*connect.Response[wideeventsv1.CreateWideEventResponse], error) {
	event := req.Msg.GetEvent()
	if event == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errRequiredEvent)
	}
	if event.GetSpanId() == "" {
		// CloudEvents requires a unique id; a browser can't be trusted to
		// generate a collision-free one.
		event.SpanId = uuid.NewString()
	}
	if event.GetStartTime() == nil {
		event.StartTime = timestamppb.Now()
	}

	if err := h.produceWideEvent(event); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&wideeventsv1.CreateWideEventResponse{SpanId: event.GetSpanId()}), nil
}

func wideEventRowToProto(r store.WideEventRow) *wideeventsv1.WideEvent {
	logs := make([]*wideeventsv1.LogLine, len(r.Logs))
	for i, l := range r.Logs {
		logs[i] = &wideeventsv1.LogLine{
			Timestamp: timestamppb.New(l.Timestamp),
			Severity:  l.Severity,
			Body:      l.Body,
		}
	}
	return &wideeventsv1.WideEvent{
		TraceId:            r.TraceID,
		SpanId:             r.SpanID,
		ParentSpanId:       r.ParentSpanID,
		ServiceName:        r.ServiceName,
		SpanName:           r.SpanName,
		StartTime:          timestamppb.New(r.StartTime),
		DurationNs:         r.DurationNs,
		StatusCode:         r.StatusCode,
		Logs:               logs,
		Attributes:         r.Attributes,
		BusinessAttributes: r.BusinessAttributes,
		RuntimeAttributes:  r.RuntimeAttributes,
	}
}
