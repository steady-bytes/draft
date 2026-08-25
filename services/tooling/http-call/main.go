// Command http-call is a Garage plugin: a small, separate Draft service
// that implements StepExecutor.Execute (see rpc.go) and publishes itself
// to Garage's PluginCatalogService on startup via the chassis.Effect
// pattern — retracting the catalog entry again on graceful shutdown.
// Mirrors services/tooling/catalyst-consume/main.go exactly; see
// services/tooling/slack-notify/main.go for the fuller rationale behind
// each piece (the "two separate registrations" comment there applies here
// unchanged).
//
// What this plugin actually does (rpc.go): makes a plain HTTP request from
// config (method/url/headers/body), returns status/headers/body as the
// step's result, and optionally evaluates config.expect assertions against
// the response.
package main

import (
	"net/http"

	"github.com/steady-bytes/draft/pkg/chassis"
)

func main() {
	logger := chassis.NewOTelLogger()
	chassis.NewMetricsReporter().Start()

	manifest, err := buildManifest()
	if err != nil {
		logger.WithError(err).Fatal("failed to build plugin manifest")
	}

	handler := NewHandler(logger, http.DefaultClient)

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
