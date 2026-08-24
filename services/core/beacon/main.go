package main

import (
	"embed"
	"net"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"

	"github.com/steady-bytes/draft/services/core/beacon/ingest"
	"github.com/steady-bytes/draft/services/core/beacon/query"
	"github.com/steady-bytes/draft/services/core/beacon/store"
)

//go:embed web-client/target/dx/beacon-pwa/release/web/public
var files embed.FS

func main() {
	logger := zerolog.New()

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

	rpc := query.NewRPC(logger, controller, tracesController, metricsController)

	defer chassis.New(logger).
		Register(chassis.RegistrationOptions{
			Namespace: "core",
		}).
		WithRPCHandler(rpc).
		WithRoute(&ntv1.Route{
			Match: &ntv1.RouteMatch{
				// LogsService, TracesService, and MetricsService all share this
				// literal package prefix, so one route covers all three of
				// Beacon's Connect-RPC services rather than registering each
				// individually.
				Prefix: "/core.observability.",
			},
		}).
		WithClientApplication(files, "web-client/target/dx/beacon-pwa/release/web/public").
		Start()
}
