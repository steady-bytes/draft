// Command catalyst-produce is a Garage plugin: a small, separate Draft
// service that implements StepExecutor.Execute (see rpc.go) and publishes
// itself to Garage's PluginCatalogService on startup via the chassis.Effect
// pattern — retracting the catalog entry again on graceful shutdown. Mirrors
// services/tooling/catalyst-consume/main.go exactly; see
// services/tooling/slack-notify/main.go for the fuller rationale behind
// each piece (the "two separate registrations" comment there applies here
// unchanged).
//
// What this plugin actually does (rpc.go): builds a CloudEvent from
// config.event_type/source/subject/data and publishes it to Catalyst,
// handing back the published event's id/type/source as the step's result.
package main

import (
	"context"

	"github.com/steady-bytes/draft/pkg/chassis"
)

func main() {
	logger := chassis.NewOTelLogger()
	chassis.NewMetricsReporter().Start()

	manifest, err := buildManifest()
	if err != nil {
		logger.WithError(err).Fatal("failed to build plugin manifest")
	}

	catalystAddr := chassis.GetConfig().GetString("catalyst.address")
	if catalystAddr == "" {
		catalystAddr = defaultCatalystAddress
	}

	// ctx is tied to chassis.Closer() so the long-lived Produce stream
	// opened in NewHandler is torn down on graceful shutdown, not left
	// dangling — same lifecycle services/tooling/bench/catalyst.go's
	// NewCatalystPublisher documents for the identical construction.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-chassis.Closer()
		cancel()
	}()

	handler := NewHandler(ctx, logger, catalystAddr)

	// Deliberately no WithRoute here, for the same reason
	// slack-notify/main.go gives: Bench finds this plugin via Blueprint's
	// ordinary service discovery and calls Execute directly, not through
	// Fuse ingress.
	c := chassis.New(logger).
		WithRPCHandler(handler).
		Register(chassis.RegistrationOptions{
			Namespace: "plugins",
		})

	garageClient := newGarageClient(chassis.GetConfig().GetString("garage.address"))
	c.Effect("garage-catalog-entry", garageCatalogEffectSetup(logger, garageClient, manifest))

	defer c.Start()
}
