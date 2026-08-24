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

// newGarageClient builds a Connect client for Garage's PluginCatalogService
// pointed at addr — see services/tooling/slack-notify/catalog.go's identical
// function for the fuller rationale (plain http.DefaultClient is fine for
// unary calls; no h2c dance needed here the way rpc.go's Catalyst Consume
// stream needs).
func newGarageClient(addr string) plugincatalogv1connect.PluginCatalogServiceClient {
	return plugincatalogv1connect.NewPluginCatalogServiceClient(http.DefaultClient, addr)
}

// publishToGarage calls Garage's Publish RPC with manifest.
func publishToGarage(ctx context.Context, client plugincatalogv1connect.PluginCatalogServiceClient, manifest *plugincatalogv1.PublishRequest) error {
	if _, err := client.Publish(ctx, connect.NewRequest(manifest)); err != nil {
		return fmt.Errorf("failed to publish %s@%s to garage: %w", manifest.GetName(), manifest.GetVersion(), err)
	}
	return nil
}

// retractFromGarage calls Garage's Retract RPC for (name, version) — the
// dispose half of the "garage-catalog-entry" chassis.Effect, run on
// graceful shutdown.
func retractFromGarage(ctx context.Context, client plugincatalogv1connect.PluginCatalogServiceClient, name, version string) error {
	if _, err := client.Retract(ctx, connect.NewRequest(&plugincatalogv1.RetractRequest{
		Name:    name,
		Version: version,
	})); err != nil {
		return fmt.Errorf("failed to retract %s@%s from garage: %w", name, version, err)
	}
	return nil
}

// garageCatalogEffectSetup builds the setup function passed to
// c.Effect("garage-catalog-entry", ...) in main.go — publish on startup,
// retract on graceful shutdown. See services/tooling/slack-notify/
// catalog.go's identical function for the fuller rationale.
func garageCatalogEffectSetup(logger chassis.Logger, client plugincatalogv1connect.PluginCatalogServiceClient, manifest *plugincatalogv1.PublishRequest) func() (func(context.Context) error, error) {
	return func() (func(context.Context) error, error) {
		if err := publishToGarage(context.Background(), client, manifest); err != nil {
			return nil, err
		}
		logger.WithField("name", manifest.GetName()).
			WithField("version", manifest.GetVersion()).
			Info("published plugin to garage catalog")

		return func(ctx context.Context) error {
			if err := retractFromGarage(ctx, client, manifest.GetName(), manifest.GetVersion()); err != nil {
				return err
			}
			logger.WithField("name", manifest.GetName()).
				WithField("version", manifest.GetVersion()).
				Info("retracted plugin from garage catalog")
			return nil
		}, nil
	}
}
