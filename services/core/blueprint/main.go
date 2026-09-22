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
		// Blueprint doesn't register itself anywhere else, unlike every other service in this
		// repo -- it's the registry, not a client of it. Registering here (under the shared
		// "blueprint" name every raft node's config.yaml already sets) is what makes it show up
		// in its own Service Registry at all, one row per raft node (see
		// docs/architecture/service-registry-identity.md's deterministic-identity work for why
		// that grouping is safe across restarts). Same WithRunner-after-Start() reasoning as
		// registerBlueprintUIRoute below: Register's own Initialize call targets this process's
		// own entrypoint, which can't be reached before Start() is listening. Unlike
		// registerBlueprintUIRoute, no custom retry wrapper is needed here -- Register already
		// retries internally (5 attempts, 5s apart) before panicking, a longer budget than
		// registerBlueprintUIRoute's proven-sufficient one for the identical race.
		WithRunner(func() {
			c.Register(chassis.RegistrationOptions{Namespace: "core"})
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
		}).
		// Blueprint's first-ever dependency on Catalyst -- see "Type Mutation Events" in
		// docs/website/content/docs/architecture/core-services.md. One-way: Blueprint reads
		// from Catalyst here, Catalyst has no reciprocal dependency on Blueprint's KV.
		WithRunner(func() {
			kv.StartTypeMutationConsumer(logger, keyValueController)
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
		// 60s total. Originally 20*500ms=10s, which assumed Fuse would be one of the first
		// things up after Blueprint. That stopped holding once run-local-watch.sh started
		// Catalyst before Fuse (needed so the 4 extra raft nodes' own startup+join sequence
		// doesn't delay Fuse past this budget -- see that script's ordering comment) --
		// Catalyst's own startup (build + Postgres/ClickHouse-backed init) can alone take
		// several seconds, before Fuse even begins. Found live: this process panicking here
		// after only 10s -- before Fuse had even started -- killed Blueprint entirely, which
		// then cascaded into Catalyst's own Register() call failing ("failed to connect to
		// blueprint") since the Blueprint it was registering with no longer existed. A slower
		// machine, a cold build cache, or anything else ahead of Fuse in the startup sequence
		// taking longer than expected all hit this same failure mode. 60s gives real headroom
		// without being unbounded -- a Fuse that's still unreachable after a full minute is a
		// real problem worth panicking loudly for, not a case to retry forever.
		maxAttempts = 120
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
