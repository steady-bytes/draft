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
// pointed at addr — see services/tooling/slack-notify/catalog.go's identical
// function for the fuller rationale (plain http.DefaultClient is fine for
// unary calls; no h2c dance needed here the way rpc.go's Catalyst Consume
// stream needs).
func newFoundryClient(addr string) plugincatalogv1connect.PluginCatalogServiceClient {
	return plugincatalogv1connect.NewPluginCatalogServiceClient(http.DefaultClient, addr)
}

// publishToFoundry calls Foundry's Publish RPC with manifest.
func publishToFoundry(ctx context.Context, client plugincatalogv1connect.PluginCatalogServiceClient, manifest *plugincatalogv1.PublishRequest) error {
	if _, err := client.Publish(ctx, connect.NewRequest(manifest)); err != nil {
		return fmt.Errorf("failed to publish %s@%s to foundry: %w", manifest.GetName(), manifest.GetVersion(), err)
	}
	return nil
}

// retractFromFoundry calls Foundry's Retract RPC for (name, version) — the
// dispose half of the "foundry-catalog-entry" chassis.Effect, run on
// graceful shutdown.
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
// c.Effect("foundry-catalog-entry", ...) in main.go — publish on startup,
// retract on graceful shutdown. See services/tooling/slack-notify/
// catalog.go's identical function for the fuller rationale.
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
