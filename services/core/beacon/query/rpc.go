package query

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"sync"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	logsv1 "github.com/steady-bytes/draft/api/core/observability/logs/v1"
	logsv1connect "github.com/steady-bytes/draft/api/core/observability/logs/v1/v1connect"
	metricsv1connect "github.com/steady-bytes/draft/api/core/observability/metrics/v1/v1connect"
	tracesv1connect "github.com/steady-bytes/draft/api/core/observability/traces/v1/v1connect"
	wideeventsv1 "github.com/steady-bytes/draft/api/core/observability/wide_events/v1"
	wideeventsv1connect "github.com/steady-bytes/draft/api/core/observability/wide_events/v1/v1connect"

	"connectrpc.com/connect"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/services/core/beacon/store"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var errRequiredEvent = errors.New("event is required")

type (
	Rpc interface {
		chassis.RPCRegistrar
		logsv1connect.LogsServiceHandler
		tracesv1connect.TracesServiceHandler
		metricsv1connect.MetricsServiceHandler
		wideeventsv1connect.WideEventsServiceHandler
	}

	rpc struct {
		controller           Controller
		tracesController     TracesController
		metricsController    MetricsController
		wideEventsController WideEventsController
		logger               chassis.Logger
		wideEventProducerMu  sync.Mutex
		wideEventProducer    *connect.BidiStreamForClient[acv1.ProduceRequest, acv1.ProduceResponse]
	}
)

// NewRPC constructs Beacon's Connect-RPC handler. Opens its own Produce
// stream against Catalyst immediately (same construction as
// services/tooling/catalyst-produce/rpc.go's NewHandler — a Connect bidi
// stream doesn't dial until the first Send, so opening it early is cheap
// even if Catalyst isn't up yet), reused for the process's lifetime —
// CreateWideEvent (wide_events.go) sends on it, so browser-submitted
// WideEvents flow through Catalyst exactly like chassis's own automatic
// per-span production, visible to every other consumer built against that
// stream, not just Beacon's own (see ingest/wide_events.go's
// ConsumeWideEvents).
func NewRPC(ctx context.Context, logger chassis.Logger, controller Controller, tracesController TracesController, metricsController MetricsController, wideEventsController WideEventsController, catalystAddr string) Rpc {
	producerClient := acConnect.NewProducerClient(h2cClient(), catalystAddr, connect.WithGRPC())
	return &rpc{
		logger:               logger,
		controller:           controller,
		tracesController:     tracesController,
		metricsController:    metricsController,
		wideEventsController: wideEventsController,
		wideEventProducer:    producerClient.Produce(ctx),
	}
}

func (h *rpc) RegisterRPC(server chassis.Rpcer) {
	interceptor := connect.WithInterceptors(chassis.NewTraceInterceptor())

	pattern, handler := logsv1connect.NewLogsServiceHandler(h, interceptor)
	server.AddHandler(pattern, handler, true)

	tracesPattern, tracesHandler := tracesv1connect.NewTracesServiceHandler(h, interceptor)
	server.AddHandler(tracesPattern, tracesHandler, true)

	metricsPattern, metricsHandler := metricsv1connect.NewMetricsServiceHandler(h, interceptor)
	server.AddHandler(metricsPattern, metricsHandler, true)

	wideEventsPattern, wideEventsHandler := wideeventsv1connect.NewWideEventsServiceHandler(h, interceptor)
	server.AddHandler(wideEventsPattern, wideEventsHandler, true)
}

// produceWideEvent builds a CloudEvent from event and sends it on the shared
// Produce stream — matches catalyst-produce/rpc.go's send (mutex-guarded: a
// Connect bidi stream's Send isn't safe for concurrent use, and concurrent
// browser tabs can call CreateWideEvent at the same time) and buildEvent's
// envelope convention exactly (TextData/protojson, not proto_data; `time` as
// a ce_timestamp extension attribute) — see
// docs/website/content/docs/architecture/wide-events.md's CloudEvent
// envelope table.
func (h *rpc) produceWideEvent(event *wideeventsv1.WideEvent) error {
	body, err := protojson.Marshal(event)
	if err != nil {
		return err
	}
	cloudEvent := &acv1.CloudEvent{
		Id:          event.GetSpanId(),
		Source:      "/services/" + event.GetServiceName(),
		SpecVersion: "1.0",
		Type:        wideEventType,
		Data:        &acv1.CloudEvent_TextData{TextData: string(body)},
		Attributes: map[string]*acv1.CloudEvent_CloudEventAttributeValue{
			"time": {Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeTimestamp{CeTimestamp: event.GetStartTime()}},
		},
	}

	h.wideEventProducerMu.Lock()
	defer h.wideEventProducerMu.Unlock()
	return h.wideEventProducer.Send(&acv1.ProduceRequest{Message: cloudEvent})
}

// wideEventType matches ingest/wide_events.go's own constant — duplicated
// rather than imported to avoid a query<->ingest package dependency for one
// string; keep the two in sync if this ever changes.
const wideEventType = "core.observability.wide_events.v1.WideEvent"

// h2cClient returns an HTTP client that speaks HTTP/2 cleartext (h2c), which
// is required for gRPC streaming against Catalyst's plain-TCP server — same
// duplicated-per-file helper every other Producer/Consumer client in this
// repo already has.
func h2cClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
	}
}

func (h *rpc) QueryLogs(ctx context.Context, req *connect.Request[logsv1.QueryLogsRequest]) (*connect.Response[logsv1.QueryLogsResponse], error) {
	rows, err := h.controller.QueryLogs(ctx, req.Msg.GetFilter(), req.Msg.GetLimit(), req.Msg.GetAfter(), req.Msg.GetBefore(), req.Msg.GetAscending())
	if err != nil {
		return nil, toConnectError(err)
	}
	records := make([]*logsv1.LogRecord, len(rows))
	for i, r := range rows {
		records[i] = rowToProto(r)
	}
	return connect.NewResponse(&logsv1.QueryLogsResponse{Records: records}), nil
}

func (h *rpc) StreamLogs(ctx context.Context, req *connect.Request[logsv1.StreamLogsRequest], stream *connect.ServerStream[logsv1.StreamLogsResponse]) error {
	// connect-go's ServerStream doesn't flush response headers to the client
	// until the first Send call. Without this, a filter matching zero
	// historical rows — with no live traffic arriving to trigger a later
	// Send either — leaves the client's initial stream_logs().await call
	// hanging indefinitely (observed firsthand: a click-to-filter selection
	// narrow enough to match nothing left the UI stuck on "connecting…"
	// forever, even though the RPC itself was healthy end to end). Sending
	// an empty heartbeat immediately forces that flush regardless of what
	// follows — including a malformed-filter error, which previously could
	// hang the same way instead of surfacing promptly. StreamLogsResponse's
	// `record` field is optional; the web client's stream loop already
	// no-ops on `record: None` (see stream.rs's `if let Some(record) = ...`),
	// so this is invisible in the UI.
	if err := stream.Send(&logsv1.StreamLogsResponse{}); err != nil {
		return toConnectError(err)
	}

	err := h.controller.StreamLogs(ctx, req.Msg.GetFilter(), req.Msg.GetLimit(), req.Msg.GetAfter(), func(row store.LogRow) error {
		return stream.Send(&logsv1.StreamLogsResponse{Record: rowToProto(row)})
	})
	if err != nil {
		return toConnectError(err)
	}
	return nil
}

// toConnectError maps a BeaconQL ParseError to CodeInvalidArgument with the
// parser's own message (never a raw ClickHouse error for a malformed filter —
// see Phase 2's "How to test" requirement), and everything else to
// CodeInternal.
func toConnectError(err error) error {
	var parseErr *ParseError
	if errors.As(err, &parseErr) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}

func rowToProto(r store.LogRow) *logsv1.LogRecord {
	return &logsv1.LogRecord{
		Timestamp:          timestamppb.New(r.Timestamp),
		TraceId:            r.TraceID,
		SpanId:             r.SpanID,
		SeverityText:       r.SeverityText,
		SeverityNumber:     uint32(r.SeverityNumber),
		ServiceName:        r.ServiceName,
		Body:               r.Body,
		Attributes:         r.Attributes,
		ResourceAttributes: r.ResourceAttributes,
	}
}
