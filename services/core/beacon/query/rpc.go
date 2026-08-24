package query

import (
	"context"
	"errors"

	logsv1 "github.com/steady-bytes/draft/api/core/observability/logs/v1"
	logsv1connect "github.com/steady-bytes/draft/api/core/observability/logs/v1/v1connect"
	metricsv1connect "github.com/steady-bytes/draft/api/core/observability/metrics/v1/v1connect"
	tracesv1connect "github.com/steady-bytes/draft/api/core/observability/traces/v1/v1connect"

	"connectrpc.com/connect"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/services/core/beacon/store"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type (
	Rpc interface {
		chassis.RPCRegistrar
		logsv1connect.LogsServiceHandler
		tracesv1connect.TracesServiceHandler
		metricsv1connect.MetricsServiceHandler
	}

	rpc struct {
		controller        Controller
		tracesController  TracesController
		metricsController MetricsController
		logger            chassis.Logger
	}
)

func NewRPC(logger chassis.Logger, controller Controller, tracesController TracesController, metricsController MetricsController) Rpc {
	return &rpc{logger: logger, controller: controller, tracesController: tracesController, metricsController: metricsController}
}

func (h *rpc) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := logsv1connect.NewLogsServiceHandler(h)
	server.AddHandler(pattern, handler, true)

	tracesPattern, tracesHandler := tracesv1connect.NewTracesServiceHandler(h)
	server.AddHandler(tracesPattern, tracesHandler, true)

	metricsPattern, metricsHandler := metricsv1connect.NewMetricsServiceHandler(h)
	server.AddHandler(metricsPattern, metricsHandler, true)
}

func (h *rpc) QueryLogs(ctx context.Context, req *connect.Request[logsv1.QueryLogsRequest]) (*connect.Response[logsv1.QueryLogsResponse], error) {
	rows, err := h.controller.QueryLogs(ctx, req.Msg.GetFilter(), req.Msg.GetLimit(), req.Msg.GetAfter())
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
