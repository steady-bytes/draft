package main

import (
	"context"
	"embed"
	"time"

	kv "github.com/steady-bytes/draft/services/core/blueprint/key_value"
	sd "github.com/steady-bytes/draft/services/core/blueprint/service_discovery"

	"github.com/steady-bytes/draft/pkg/chassis"
)

//go:embed web-client/target/dx/blueprint-pwa/release/web/public
var files embed.FS

func main() {

	var (
		// chassis.NewOTelLogger reports Blueprint's own structured logs to
		// Beacon over OTLP (resolved via Blueprint's own service-discovery
		// registry — see pkg/chassis/otel_logger.go), rather than zerolog's
		// stdout-only output. Enabled via config.yaml's `telemetry.enabled`.
		logger             = chassis.NewOTelLogger()
		keyValueModel      = kv.NewModel()
		keyValueController = kv.NewController(keyValueModel)
		keyValueRPC        = kv.NewRPC(logger, keyValueController)
	)

	c := chassis.New(logger).
		WithRepository(keyValueModel).
		WithConsensus(chassis.Raft, keyValueController)

	// initialize service discovery components here since the controller requires the RaftController from the chassis
	var (
		serviceDiscoveryController = sd.NewController(keyValueController, c.RaftController)
		serviceDiscoveryRPC        = sd.NewRPC(logger, serviceDiscoveryController)
	)

	// chassis.NewMetricsReporter reports Blueprint's own Go runtime metrics
	// (goroutines, heap, GC) to Beacon over OTLP. Start is non-blocking — it
	// spawns its own background sampling goroutine.
	chassis.NewMetricsReporter().Start()

	c.WithRPCHandler(keyValueRPC).
		WithRPCHandler(serviceDiscoveryRPC).
		WithClientApplication(files, "web-client/target/dx/blueprint-pwa/release/web/public").
		WithRunner(func() {
			ticker := time.NewTicker(sd.ReapInterval)
			defer ticker.Stop()
			for range ticker.C {
				serviceDiscoveryController.Reap(context.Background(), logger)
			}
		})

	defer c.Start()
}
