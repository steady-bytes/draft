package main

import (
	"context"
	"fmt"
	"net/http"

	plugincatalogv1 "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1"
	plugincatalogv1connect "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
)

// newFoundryClient builds a Connect client for Foundry's PluginCatalogService
// pointed at addr (this service's own static foundry.address config value —
// see config.yaml). Mirrors the plain http.DefaultClient construction
// services/core/auth and services/core/heartbeat use for their own
// ad hoc Blueprint RPC calls: these are unary calls, so the h2c/AllowHTTP
// dance chassis's own Blueprint streaming client needs (builder.go's
// newBlueprintClient) isn't required here.
func newFoundryClient(addr string) plugincatalogv1connect.PluginCatalogServiceClient {
	return plugincatalogv1connect.NewPluginCatalogServiceClient(http.DefaultClient, addr)
}

// publishToFoundry calls Foundry's Publish RPC with manifest. Returns a
// wrapped error on failure so the caller (the chassis.Effect setup in
// main.go) can log a useful message before treating it as the fatal
// condition chassis.Effect always treats a failed setup as.
func publishToFoundry(ctx context.Context, client plugincatalogv1connect.PluginCatalogServiceClient, manifest *plugincatalogv1.PublishRequest) error {
	if _, err := client.Publish(ctx, connect.NewRequest(manifest)); err != nil {
		return fmt.Errorf("failed to publish %s@%s to foundry: %w", manifest.GetName(), manifest.GetVersion(), err)
	}
	return nil
}

// retractFromFoundry calls Foundry's Retract RPC for (name, version). This is
// the dispose half of the "foundry-catalog-entry" chassis.Effect — run during
// graceful shutdown so a no-longer-running plugin doesn't linger in the
// catalog as if it were still available. Retracting an already-retracted or
// never-published entry is a documented no-op on Foundry's side (see
// services/tooling/foundry/rpc.go's Retract), not an error, so this doesn't
// need to special-case that here.
func retractFromFoundry(ctx context.Context, client plugincatalogv1connect.PluginCatalogServiceClient, name, version string) error {
	if _, err := client.Retract(ctx, connect.NewRequest(&plugincatalogv1.RetractRequest{
		Name:    name,
		Version: version,
	})); err != nil {
		return fmt.Errorf("failed to retract %s@%s from foundry: %w", name, version, err)
	}
	return nil
}

// foundryCatalogEffectSetup builds the setup function passed to
// c.Effect("foundry-catalog-entry", ...) in main.go, per the design doc's
// "Publishing and discovery" section: publish manifest to Foundry's catalog
// on startup, and return a dispose that retracts the same (name, version) on
// graceful shutdown.
func foundryCatalogEffectSetup(logger chassis.Logger, client plugincatalogv1connect.PluginCatalogServiceClient, manifest *plugincatalogv1.PublishRequest) func() (func(context.Context) error, error) {
	return func() (func(context.Context) error, error) {
		if err := publishToFoundry(context.Background(), client, manifest); err != nil {
			return nil, err
		}
		logger.WithField("name", manifest.GetName()).
			WithField("version", manifest.GetVersion()).
			Info("published plugin to foundry catalog")

		return func(ctx context.Context) error {
			if err := retractFromFoundry(ctx, client, manifest.GetName(), manifest.GetVersion()); err != nil {
				return err
			}
			logger.WithField("name", manifest.GetName()).
				WithField("version", manifest.GetVersion()).
				Info("retracted plugin from foundry catalog")
			return nil
		}, nil
	}
}
