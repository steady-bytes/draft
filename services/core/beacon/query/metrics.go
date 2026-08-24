package query

import (
	"context"
	"fmt"
	"time"

	metricsv1 "github.com/steady-bytes/draft/api/core/observability/metrics/v1"
	metricsv1connect "github.com/steady-bytes/draft/api/core/observability/metrics/v1/v1connect"

	"connectrpc.com/connect"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/services/core/beacon/store"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	// defaultLookback is how far back QueryMetrics looks when the client
	// supplies no start timestamp — generous enough to plot a useful chart,
	// bounded enough that a default request doesn't scan unbounded history.
	defaultLookback = time.Hour

	// defaultSteps is the sample-grid size QueryMetrics targets when the
	// client supplies no step — mirrors store.go's defaultLimit/maxLimit
	// convention for logs/traces. promql.go's maxPromQLSteps is the hard
	// backstop against a client-supplied step producing too fine a grid.
	defaultSteps = 60
)

type (
	// MetricsController is Beacon's business logic for QueryMetrics: parse
	// the PromQL-subset expression once (the parser — see promql.go),
	// resolve the [start, end]/step evaluation window, and hand off to
	// Evaluate to fetch/compute the resulting time series.
	MetricsController interface {
		QueryMetrics(ctx context.Context, queryStr, startStr, endStr, stepStr string) ([]MetricSeries, error)
	}

	metricsController struct {
		logger chassis.Logger
		store  store.Storer
	}
)

func NewMetricsController(logger chassis.Logger, storer store.Storer) MetricsController {
	return &metricsController{logger: logger, store: storer}
}

func (c *metricsController) QueryMetrics(ctx context.Context, queryStr, startStr, endStr, stepStr string) ([]MetricSeries, error) {
	expr, err := ParsePromQL(queryStr)
	if err != nil {
		return nil, err
	}

	end := time.Now().UTC()
	if endStr != "" {
		t, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			return nil, &ParseError{Msg: fmt.Sprintf("invalid end timestamp %q: must be RFC3339", endStr)}
		}
		end = t.UTC()
	}

	start := end.Add(-defaultLookback)
	if startStr != "" {
		t, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			return nil, &ParseError{Msg: fmt.Sprintf("invalid start timestamp %q: must be RFC3339", startStr)}
		}
		start = t.UTC()
	}

	step := end.Sub(start) / defaultSteps
	if step < time.Second {
		step = time.Second
	}
	if stepStr != "" {
		d, err := time.ParseDuration(stepStr)
		if err != nil {
			return nil, &ParseError{Msg: fmt.Sprintf("invalid step %q: must be a Go-style duration, e.g. \"15s\"", stepStr)}
		}
		if d <= 0 {
			return nil, &ParseError{Msg: "step must be positive"}
		}
		step = d
	}

	return Evaluate(ctx, c.store, expr, start, end, step)
}

// ─── Connect-RPC handler ────────────────────────────────────────────────────
//
// Implemented on the same *rpc type as QueryLogs/StreamLogs/SearchTraces/
// GetTrace (see rpc.go/traces.go) so one Rpc value satisfies
// LogsServiceHandler, TracesServiceHandler, and MetricsServiceHandler —
// RegisterRPC (rpc.go) mounts all three handler patterns on the same
// Connect-RPC server.

func (h *rpc) QueryMetrics(ctx context.Context, req *connect.Request[metricsv1.QueryMetricsRequest]) (*connect.Response[metricsv1.QueryMetricsResponse], error) {
	series, err := h.metricsController.QueryMetrics(ctx, req.Msg.GetQuery(), req.Msg.GetStart(), req.Msg.GetEnd(), req.Msg.GetStep())
	if err != nil {
		return nil, toConnectError(err)
	}
	out := make([]*metricsv1.TimeSeries, len(series))
	for i, s := range series {
		out[i] = seriesToProto(s)
	}
	return connect.NewResponse(&metricsv1.QueryMetricsResponse{Series: out}), nil
}

func seriesToProto(s MetricSeries) *metricsv1.TimeSeries {
	samples := make([]*metricsv1.Sample, len(s.Samples))
	for i, smp := range s.Samples {
		samples[i] = &metricsv1.Sample{
			Timestamp: timestamppb.New(smp.Timestamp),
			Value:     smp.Value,
		}
	}
	return &metricsv1.TimeSeries{
		MetricName: s.MetricName,
		Labels:     s.Labels,
		Samples:    samples,
	}
}

// compile-time assertion that *rpc satisfies MetricsServiceHandler alongside
// LogsServiceHandler/TracesServiceHandler — see rpc.go's Rpc interface.
var _ metricsv1connect.MetricsServiceHandler = (*rpc)(nil)
