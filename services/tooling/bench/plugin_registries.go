// This file implements Phase 11's config seeding: bench.plugin_registries in
// config.yaml declares the plugin registries a fresh Bench should start
// with, loaded the same seed-once way workflows.go's loadWorkflowsDir seeds
// workflows from workflows_dir -- a registry already in the database (from
// a prior seed, or added/edited through the Settings page since) is left
// alone; config never re-syncs over a change made at runtime. See
// docs/website/content/docs/architecture/bench-workflow-engine.md's
// "Connecting a plugin registry" for the full rationale.
package main

import (
	"context"
	"fmt"

	settingsv1 "github.com/steady-bytes/draft/api/tooling/settings/v1"
	"github.com/steady-bytes/draft/pkg/chassis"
)

// defaultRegistryName is seeded from the legacy garage.address config value
// when bench.plugin_registries is unset entirely -- backward compatible with
// every config.yaml and every garage:// reference (including
// catalyst-consume-e2e.yaml, proven live in Phase 6) written before Phase 11
// existed, none of which need to change.
const defaultRegistryName = "default"

// pluginRegistryConfig is one entry of the bench.plugin_registries config
// list -- a plain struct for chassis.Config's UnmarshalKey, not the proto
// type (same "parse into a plain Go shape first" boundary loader.go's
// yamlWorkflowDoc already establishes for workflow YAML).
type pluginRegistryConfig struct {
	Name    string `mapstructure:"name"`
	Address string `mapstructure:"address"`
}

// loadPluginRegistries seeds bench.plugin_registries (or, if unset, one
// "default" registry from the legacy garage.address key) into the database,
// skipping any name that already exists there. Returns how many were newly
// seeded this run.
func loadPluginRegistries(ctx context.Context, cfg chassis.Config, store *pgResultStore, logger chassis.Logger) (int, error) {
	var configured []pluginRegistryConfig
	if err := cfg.UnmarshalKey("bench.plugin_registries", &configured); err != nil {
		return 0, fmt.Errorf("failed to parse bench.plugin_registries: %w", err)
	}

	if len(configured) == 0 {
		if addr := cfg.GetString("garage.address"); addr != "" {
			configured = []pluginRegistryConfig{{Name: defaultRegistryName, Address: addr}}
		} else {
			return 0, nil
		}
	}

	existing, err := store.ListPluginRegistries(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list existing plugin registries: %w", err)
	}
	alreadySeeded := make(map[string]bool, len(existing))
	for _, r := range existing {
		alreadySeeded[r.GetName()] = true
	}

	seeded := 0
	for _, entry := range configured {
		if entry.Name == "" || entry.Address == "" {
			return seeded, fmt.Errorf("bench.plugin_registries entry %+v: name and address are both required", entry)
		}
		if alreadySeeded[entry.Name] {
			logger.WithField("registry", entry.Name).
				Info("plugin registry already exists in the database; not re-seeding from config (edit it through the Settings page instead)")
			continue
		}
		if err := store.UpsertPluginRegistry(ctx, &settingsv1.PluginRegistry{Name: entry.Name, Address: entry.Address}); err != nil {
			return seeded, fmt.Errorf("failed to seed plugin registry %q: %w", entry.Name, err)
		}
		logger.WithField("registry", entry.Name).WithField("address", entry.Address).Info("seeded plugin registry from config")
		seeded++
	}
	return seeded, nil
}
