// Command catalyst-consume is a Garage plugin: a small, separate Draft
// service that implements StepExecutor.Execute (see rpc.go) and publishes
// itself to Garage's PluginCatalogService on startup via the chassis.Effect
// pattern — retracting the catalog entry again on graceful shutdown. Mirrors
// services/tooling/slack-notify's main.go exactly; see that file for the
// fuller rationale behind each piece (the "two separate registrations"
// comment there applies here unchanged).
//
// What this plugin actually does (rpc.go): waits for the next Catalyst
// CloudEvent matching a configured type, extracts named fields from its JSON
// payload, and hands them back as the step's result for later steps to
// reference via {{ steps.<name>.result }}.
package main

import (
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"
)

func main() {
	logger := zerolog.New()

	manifest, err := buildManifest()
	if err != nil {
		logger.WithError(err).Fatal("failed to build plugin manifest")
	}

	handler := NewHandler(logger, func() string {
		addr := chassis.GetConfig().GetString("catalyst.address")
		if addr == "" {
			addr = defaultCatalystAddress
		}
		return addr
	})

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
