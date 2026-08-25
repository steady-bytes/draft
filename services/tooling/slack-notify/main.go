// Command slack-notify is the reference Garage plugin described in
// docs/website/content/docs/architecture/garage-plugin-repository.md's
// "Implementation plan", Phase 3 ("The reference plugin"): a small, separate
// Draft service that implements StepExecutor.Execute (so Bench can call it
// directly once Bench's own Phase 6 garage:// resolution exists) and
// publishes itself to Garage's PluginCatalogService on startup via the
// chassis.Effect pattern shown in the doc's "Publishing and discovery"
// section — retracting the catalog entry again on graceful shutdown.
//
// Two separate registrations happen here, for two separate purposes (see
// "Publishing and discovery"): Register with Blueprint answers "where is a
// live instance of slack-notify right now" (ordinary service discovery);
// the "garage-catalog-entry" Effect answers "what versions of slack-notify
// exist, and what does this version's config look like" (catalog metadata,
// versioned independently of any running instance).
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

	handler := NewHandler(logger, http.DefaultClient, func() string {
		return chassis.GetConfig().GetString("slack.webhook_url")
	})

	// Deliberately no WithRoute here (unlike services/tooling/garage, which
	// exposes its RPCs through Fuse for external/browser access to its own
	// UI/API): per the design doc's "Publishing and discovery", Bench finds
	// this plugin via Blueprint's ordinary service discovery (its internal
	// advertise address) and calls Execute directly, not through Fuse
	// ingress. Adding a route here would make every plugin's startup
	// depend on Fuse's own fuse_service_address being in Blueprint, which
	// isn't otherwise a dependency this plugin has.
	c := chassis.New(logger).
		WithRPCHandler(handler).
		Register(chassis.RegistrationOptions{
			Namespace: "plugins",
		})

	// publishing to Garage's catalog is exactly the "ad hoc effect with an
	// inverse" shape chassis.Effect exists for — publish on startup, retract
	// on graceful shutdown, so a plugin that's no longer running doesn't
	// linger in the catalog as if it were. Garage's address is a static
	// config value (garage.address in config.yaml), not resolved dynamically
	// through Blueprint: Garage is a fixed, known dependency for this
	// plugin, the same way services resolve Blueprint's own address via
	// static service.entrypoint config rather than discovery.
	garageClient := newGarageClient(chassis.GetConfig().GetString("garage.address"))
	c.Effect("garage-catalog-entry", garageCatalogEffectSetup(logger, garageClient, manifest))

	defer c.Start()
}
