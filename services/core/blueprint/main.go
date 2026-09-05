package main

import (
	"context"
	"embed"
	"time"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
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
		}).
		// Exposes Blueprint's own UI (and, since it shares the same mux/port,
		// its RPC handlers) through Fuse on a dedicated subdomain, rather than
		// only being reachable directly on its bind port. Unlike every other
		// service's WithRoute call, this one can't run synchronously here in
		// the builder chain: WithRoute registers by querying Blueprint's own
		// KV store for Fuse's address, and Start() hasn't begun serving that
		// KV store yet at this point in main() -- Blueprint can't reach
		// itself before it's listening. Started as a WithRunner goroutine
		// instead (also unlike every other use of WithRunner in this repo,
		// which are all genuinely long-running loops, not a bounded retry) so
		// it runs after Start() fires off the mux listener, retrying with
		// backoff until that race resolves in our favor.
		WithRunner(func() {
			registerBlueprintUIRoute(c, logger)
		})

	defer c.Start()
}

// registerBlueprintUIRoute registers Blueprint's UI route with Fuse. This races against two
// things becoming ready that WithRoute's normal (pre-Start, synchronous, panic-on-failure)
// usage never has to: Blueprint's own mux accepting connections (withRoute registers by
// querying Blueprint's own KV store for Fuse's address, and Blueprint can't reach itself
// before it's listening), and Fuse itself starting up and self-registering its address into
// that KV store. Neither is bounded by anything this process controls, so this retries the
// whole registration (KV lookup + AddRoute, via TryWithRoute, which returns the error instead
// of panicking like WithRoute) with backoff, rather than only pre-checking Blueprint's own
// reachability and treating a single subsequent failure as fatal -- a Fuse that's simply
// slower to start than Blueprint's own mux init is the expected common case here, not a bug.
func registerBlueprintUIRoute(c *chassis.Runtime, logger chassis.Logger) {
	const (
		maxAttempts = 20
		retryDelay  = 500 * time.Millisecond
	)

	route := &ntv1.Route{
		Name: "core-blueprint-ui",
		Match: &ntv1.RouteMatch{
			Host:   "blueprint.draft.localhost",
			Prefix: "/",
		},
		// chassis's own server already speaks h2c (see pkg/chassis/builder.go's
		// Start, which always wraps the mux in an h2c.NewHandler) -- this just
		// tells Envoy's cluster to use HTTP/2 too. Required for Fuse's grpc_web
		// filter to work: it bridges a browser's grpc-web request into plain
		// gRPC, which is HTTP/2-only, so an HTTP/1.1-only upstream cluster
		// breaks it (found live: the web client's RPC calls returned
		// "malformed response" until this was set).
		EnableHttp2: true,
	}

	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err = c.TryWithRoute(route); err == nil {
			return
		}
		if attempt < maxAttempts {
			time.Sleep(retryDelay)
		}
	}
	logger.WithError(err).Panic("blueprint-ui route never registered with fuse after retrying")
}
