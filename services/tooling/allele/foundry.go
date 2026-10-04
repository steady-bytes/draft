// foundry.go: publishes Allele's own plugin manifest to Foundry's catalog for discovery/
// distribution -- mirrors services/tooling/relay/service/foundry.go exactly (itself mirroring
// lineman's own foundry.go). Allele is a standalone application with its own UI (once Phase 12
// lands) and RPC surface, never resolved as a foundry://... workflow step -- it does not implement
// StepExecutor, the same reasoning Relay/Lineman already settled; this manifest exists purely so
// other Draft deployments can discover and add it.
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

const alleleManifestVersion = "v1"

func NewFoundryClient(addr string) plugincatalogv1connect.PluginCatalogServiceClient {
	return plugincatalogv1connect.NewPluginCatalogServiceClient(http.DefaultClient, addr)
}

func BuildManifest() *plugincatalogv1.PublishRequest {
	return &plugincatalogv1.PublishRequest{
		Name:        "allele",
		Version:     alleleManifestVersion,
		Description: "AST-aware git server: symbol-level diff/merge, worktree overlap matrix, path-permission enforcement, provenance.",
		Maintainer:  "steady-bytes",
		Source:      "https://github.com/steady-bytes/draft/tree/main/services/tooling/allele",
	}
}

func publishToFoundry(ctx context.Context, client plugincatalogv1connect.PluginCatalogServiceClient, manifest *plugincatalogv1.PublishRequest) error {
	if _, err := client.Publish(ctx, connect.NewRequest(manifest)); err != nil {
		return fmt.Errorf("failed to publish %s@%s to foundry: %w", manifest.GetName(), manifest.GetVersion(), err)
	}
	return nil
}

func retractFromFoundry(ctx context.Context, client plugincatalogv1connect.PluginCatalogServiceClient, name, version string) error {
	if _, err := client.Retract(ctx, connect.NewRequest(&plugincatalogv1.RetractRequest{
		Name:    name,
		Version: version,
	})); err != nil {
		return fmt.Errorf("failed to retract %s@%s from foundry: %w", name, version, err)
	}
	return nil
}

// FoundryCatalogEffectSetup builds the setup function passed to
// chassis.New(...).Effect("foundry-catalog-entry", ...) in main.go: publish on startup, retract the
// same (name, version) on graceful shutdown -- the "ad hoc effect with an inverse" shape
// chassis.Effect exists for.
func FoundryCatalogEffectSetup(logger chassis.Logger, client plugincatalogv1connect.PluginCatalogServiceClient, manifest *plugincatalogv1.PublishRequest) func() (func(context.Context) error, error) {
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
