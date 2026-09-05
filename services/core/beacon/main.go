package main

import (
	"context"
	"embed"
	"net"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"github.com/steady-bytes/draft/services/core/beacon/ingest"
	"github.com/steady-bytes/draft/services/core/beacon/query"
	"github.com/steady-bytes/draft/services/core/beacon/store"
)

// defaultCatalystAddress matches every other Catalyst client in this repo's
// own fallback for the same config key (e.g. examples/consumer/main.go).
const defaultCatalystAddress = "http://localhost:2220"

//go:embed web-client/target/dx/beacon-pwa/release/web/public
var files embed.FS

func main() {
	logger := chassis.NewOTelLogger()
	chassis.NewMetricsReporter().Start()

	cfg := chassis.GetConfig()

	var chCfg store.ClickHouseConfig
	if err := cfg.UnmarshalKey("clickhouse", &chCfg); err != nil {
		logger.WithField("error", err.Error()).Error("failed to read clickhouse config")
	}

	var storer store.Storer
	if chCfg.Enabled {
		s, err := store.NewClickHouseStore(chCfg)
		if err != nil {
			logger.WithField("error", err.Error()).Error("failed to connect to clickhouse — falling back to noop store")
			storer = store.NewNoopStore()
		} else {
			storer = s
			logger.WithField("database", chCfg.Database).Info("connected to clickhouse")
		}
	} else {
		storer = store.NewNoopStore()
	}

	writer := ingest.NewWriter(storer, logger)
	controller := query.NewController(logger, storer)
	receiver := ingest.NewLogsReceiver(writer, controller, logger)

	spanWriter := ingest.NewSpanWriter(storer, logger)
	tracesController := query.NewTracesController(logger, storer)
	traceReceiver := ingest.NewTraceReceiver(spanWriter, logger)

	metricWriter := ingest.NewMetricWriter(storer, logger)
	metricsController := query.NewMetricsController(logger, storer)
	metricsReceiver := ingest.NewMetricsReceiver(metricWriter, logger)

	catalystAddr := cfg.GetString("catalyst.address")
	if catalystAddr == "" {
		catalystAddr = defaultCatalystAddress
	}
	wideEventWriter := ingest.NewWideEventWriter(storer, logger)
	wideEventsController := query.NewWideEventsController(logger, storer)

	// wideEventCtx is tied to chassis.Closer() so both the Produce stream
	// query.NewRPC opens below and the Consume loop started via WithRunner
	// are torn down on graceful shutdown — same construction (and same
	// before-chassis.New ordering) as
	// services/tooling/catalyst-produce/main.go's own ctx for the identical
	// reason: a Connect bidi stream doesn't dial until the first Send, so
	// opening it this early is cheap even if Catalyst isn't up yet.
	wideEventCtx, cancelWideEventCtx := context.WithCancel(context.Background())
	go func() {
		<-chassis.Closer()
		cancelWideEventCtx()
	}()

	// The OTLP/gRPC receiver is a separate listener on the standard OTLP port —
	// chassis owns service.network.bind_port (Beacon's own QueryLogs/StreamLogs
	// Connect-RPC surface), not this. Any off-the-shelf OTel SDK or Collector
	// exports here unmodified. The log, trace, and metrics OTLP services are
	// all registered on this same grpcServer instance, one listener for all
	// OTLP signals — not a separate port per signal.
	otlpAddr := cfg.GetString("otlp.bind_address")
	if otlpAddr == "" {
		otlpAddr = "0.0.0.0:4317"
	}
	go func() {
		lis, err := net.Listen("tcp", otlpAddr)
		if err != nil {
			logger.WithField("error", err.Error()).WithField("address", otlpAddr).Panic("failed to bind OTLP gRPC listener")
		}
		grpcServer := grpc.NewServer()
		collectorlogsv1.RegisterLogsServiceServer(grpcServer, receiver)
		collectortracev1.RegisterTraceServiceServer(grpcServer, traceReceiver)
		collectormetricsv1.RegisterMetricsServiceServer(grpcServer, metricsReceiver)
		logger.WithField("address", otlpAddr).Info("OTLP log/trace/metrics receiver listening")
		if err := grpcServer.Serve(lis); err != nil {
			logger.WithField("error", err.Error()).Error("OTLP gRPC server stopped")
		}
	}()

	rpc := query.NewRPC(wideEventCtx, logger, controller, tracesController, metricsController, wideEventsController, catalystAddr)

	defer chassis.New(logger).
		Register(chassis.RegistrationOptions{
			Namespace: "core",
		}).
		WithRPCHandler(rpc).
		// Beacon consumes WideEvents produced by chassis (automatic per-span
		// production) or by Beacon's own CreateWideEvent RPC (see
		// query/rpc.go) -- either way, every WideEvent flows through Catalyst
		// as a CloudEvent, and this is the only thing that writes to the
		// `wide_events` table. Started as a WithRunner goroutine, same
		// pattern as bench's scheduler and blueprint's own registration
		// retry -- runs for the life of the process, torn down via
		// chassis.Closer() on shutdown.
		WithRunner(func() {
			ingest.ConsumeWideEvents(wideEventCtx, logger, catalystAddr, wideEventWriter)
		}).
		WithRoute(&ntv1.Route{
			Match: &ntv1.RouteMatch{
				// LogsService, TracesService, and MetricsService all share this
				// literal package prefix, so one route covers all three of
				// Beacon's Connect-RPC services rather than registering each
				// individually.
				Prefix: "/core.observability.",
			},
		}).
		// A second, distinctly-named route: exposes Beacon's UI (served by
		// WithClientApplication below, on this same mux/port) through Fuse on
		// its own subdomain. Needs an explicit Name — chassis auto-derives an
		// unset Route.Name from "<domain>-<service>" the same way the route
		// above did, and a second unnamed call here would silently overwrite
		// it instead of adding a second route (both would resolve to the same
		// key: "core-beacon").
		WithRoute(&ntv1.Route{
			Name: "core-beacon-ui",
			Match: &ntv1.RouteMatch{
				Host:   "beacon.draft.localhost",
				Prefix: "/",
			},
			// See blueprint/main.go's identical field for why: Fuse's grpc_web filter
			// bridges browser grpc-web calls into plain (HTTP/2-only) gRPC, which
			// breaks against an HTTP/1.1-only upstream cluster.
			EnableHttp2: true,
		}).
		WithClientApplication(files, "web-client/target/dx/beacon-pwa/release/web/public").
		Start()
}
