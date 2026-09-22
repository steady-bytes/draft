// Command lineman is the task orchestrator described in
// docs/website/content/docs/architecture/lineman-implementation-plan.md:
// tracks objectives, tasks, and the agents (human, scripted, or AI) working
// them, with Scheduler/Loops time-based primitives. All state lives in
// Blueprint (see store.go); every state change produces a Catalyst event
// (catalyst.go) and a Beacon wide event (rpc.go's StartSpan/End calls).
package main

import (
	"context"
	"embed"
	"net/http"
	"time"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"
	linemanv1 "github.com/steady-bytes/draft/api/tooling/lineman/v1"
	"github.com/steady-bytes/draft/pkg/chassis"
)

//go:embed web-client/target/dx/lineman-pwa/release/web/public
var files embed.FS

const defaultTickerInterval = 5 * time.Second

func main() {
	logger := chassis.NewOTelLogger()
	chassis.NewMetricsReporter().Start()

	cfg := chassis.GetConfig()
	tickerInterval := cfg.GetDuration("lineman.ticker_interval")
	if tickerInterval <= 0 {
		tickerInterval = defaultTickerInterval
	}

	httpClient := http.DefaultClient
	kvClient := kvv1Connect.NewKeyValueServiceClient(httpClient, cfg.Entrypoint())

	// Tied to chassis.Closer(), not context.Background() directly -- this
	// context has to outlive every Catalyst event this publisher will ever
	// send, across the whole life of the process. Same reasoning as
	// services/tooling/bench/main.go's eventsCtx.
	eventsCtx, cancelEvents := context.WithCancel(context.Background())
	go func() {
		<-chassis.Closer()
		cancelEvents()
	}()
	events := NewCatalystPublisher(eventsCtx, httpClient, cfg, logger)

	ctrl := newController(logger, kvClient, events, tickerInterval)
	rpcHandler := NewHandler(logger, ctrl)

	foundryAddr := cfg.GetString("foundry.address")
	if foundryAddr == "" {
		foundryAddr = "http://localhost:9301"
	}
	foundryClient := newFoundryClient(foundryAddr)
	manifest := buildLinemanManifest()

	c := chassis.New(logger).
		Register(chassis.RegistrationOptions{
			Namespace: "tooling",
		}).
		// Registers every entity type with Blueprint's type registry so
		// each renders decoded, not as opaque bytes, in Blueprint's own
		// Key/Value browser -- see docs/architecture/kv-type-registry-implementation-plan.md.
		WithRegisteredType(&linemanv1.Objective{}).
		WithRegisteredType(&linemanv1.Task{}).
		WithRegisteredType(&linemanv1.Agent{}).
		WithRegisteredType(&linemanv1.ScheduledTask{}).
		WithRegisteredType(&linemanv1.Loop{}).
		WithRPCHandler(rpcHandler).
		// Scheduler/Loops firing -- an internal ticker, not a Catalyst
		// Consume subscription. See the implementation plan's Decisions
		// (Catalyst's Consume fan-out bug).
		WithRunner(func() {
			ctrl.runScheduler(eventsCtx)
		}).
		// Publishing to Foundry's catalog is the "ad hoc effect with an
		// inverse" shape chassis.Effect exists for -- publish on startup,
		// retract on graceful shutdown, so a no-longer-running Lineman
		// doesn't linger in the catalog as if it were still available.
		// Lineman does not implement StepExecutor; this manifest exists
		// purely for discovery/distribution (idea.md).
		Effect("foundry-catalog-entry", foundryCatalogEffectSetup(logger, foundryClient, manifest)).
		WithRoute(&ntv1.Route{
			Name: "lineman-rpc",
			Match: &ntv1.RouteMatch{
				Prefix: "/tooling.lineman.v1.LinemanService/",
			},
		}).
		WithRoute(&ntv1.Route{
			Name: "lineman-ui",
			Match: &ntv1.RouteMatch{
				Host:   "lineman.draft.localhost",
				Prefix: "/",
			},
			EnableHttp2: true,
		}).
		WithClientApplication(files, "web-client/target/dx/lineman-pwa/release/web/public")

	defer c.Start()
}
