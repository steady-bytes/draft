// Command garage is the plugin catalog service described in
// docs/website/content/docs/architecture/garage-plugin-repository.md.
//
// Phase 1 (Scaffolding) proved the service starts, opens its Postgres
// connection, and registers with Blueprint. Phase 2: PluginCatalogService's
// RPCs (Publish/Retract/Get/List/Search — see rpc.go) are wired up and
// backed by real Postgres persistence (store.go, and model.go's
// createSchema filling the schema-creation gap Phase 1 deliberately left
// open). Phase 4 (see ui.go) adds the server-side rendered Catalog and
// Plugin detail pages alongside those RPCs, on the same mux/port.
package main

import (
	"context"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"
	"github.com/steady-bytes/draft/pkg/repositories/postgres/bun"
)

func main() {
	var (
		logger = zerolog.New()
		db     = bun.New("")
		st     = newStore(db)
	)

	defer chassis.New(logger).
		WithRepository(db).
		WithRunner(func() {
			// Not required for RPCs to be registered, but proves the pluginRow
			// mapping in model.go is valid against a real Postgres instance and
			// creates the table Publish/Retract/Get/List/Search need. A missing
			// local Postgres shouldn't block the rest of Garage's startup, so
			// failures here are logged, not fatal — same as Bench's Phase 1.
			if err := createSchema(context.Background(), db); err != nil {
				logger.WithError(err).Error("failed to create garage schema")
				return
			}
			logger.Info("garage schema ready")
		}).
		WithRPCHandler(NewHandler(logger, st)).
		// The UI (Phase 4, ui.go) is registered as a second RPCRegistrar
		// rather than a second listener: both calls append handlers onto the
		// same underlying mux (see chassis.Runtime.withRpc), so RPC traffic
		// (/tooling.plugin_catalog.v1.PluginCatalogService/...) and page
		// traffic (/, /plugins/..., /static/...) share one port. See ui.go's
		// doc comment for the full rationale and the one known quirk of
		// reusing AddHandler this way.
		WithRPCHandler(NewUIHandler(logger, st)).
		WithRoute(&ntv1.Route{
			Match: &ntv1.RouteMatch{
				Prefix: "/tooling.plugin_catalog.v1.PluginCatalogService/",
			},
		}).
		Register(chassis.RegistrationOptions{
			Namespace: "tooling",
		}).
		Start()
}
