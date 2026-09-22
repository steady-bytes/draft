// foundry.go: publishes Lineman's own plugin manifest to Foundry's catalog for
// discovery/distribution -- idea.md's "register lineman as a plugin to
// foundry so it can be added to other draft systems." Lineman does not
// implement StepExecutor and is never resolved as a foundry:// workflow
// step; the manifest exists purely so other Draft deployments can discover
// and add Lineman. Mirrors services/tooling/slack-notify/catalog.go exactly.
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

const linemanPluginVersion = "v1"

func newFoundryClient(addr string) plugincatalogv1connect.PluginCatalogServiceClient {
	return plugincatalogv1connect.NewPluginCatalogServiceClient(http.DefaultClient, addr)
}

func buildLinemanManifest() *plugincatalogv1.PublishRequest {
	return &plugincatalogv1.PublishRequest{
		Name:        "lineman",
		Version:     linemanPluginVersion,
		Description: "Task orchestrator: tracks objectives, tasks, and the agents (human, scripted, or AI) working them, with Scheduler/Loops time-based primitives.",
		Maintainer:  "steady-bytes",
		Source:      "https://github.com/steady-bytes/draft/tree/main/services/tooling/lineman",
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

// foundryCatalogEffectSetup builds the setup function passed to
// c.Effect("foundry-catalog-entry", ...) in main.go: publish on startup,
// retract the same (name, version) on graceful shutdown.
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
