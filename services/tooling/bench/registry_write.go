// This file implements Phase 11's registry management: addPluginRegistry/
// updatePluginRegistry, the shared business logic behind both rpc.go's
// AddPluginRegistry/UpdatePluginRegistry RPCs and ui.go's Settings-page form
// handlers -- mirrors workflow_write.go's split (createWorkflow/
// updateWorkflow) exactly, so the two entry points can't drift on what
// "add"/"update" mean.
package main

import (
	"context"
	"errors"
	"fmt"

	plugincatalogv1 "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1"
	plugincatalogv1connect "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1/v1connect"
	settingsv1 "github.com/steady-bytes/draft/api/tooling/settings/v1"

	"connectrpc.com/connect"
)

// ErrPluginRegistryAlreadyExists is addPluginRegistry's failure when name is
// already taken -- Add is a real create, not store.go's own unconditional
// UpsertPluginRegistry, the same "create vs. update, deliberately not one
// upsert" reasoning workflow_write.go documents for workflows.
var ErrPluginRegistryAlreadyExists = errors.New("plugin registry already exists")

// pluginRegistryStore is the persistence surface addPluginRegistry/
// updatePluginRegistry need -- satisfied by *pgResultStore, narrowed so
// tests can substitute a fake without a real Postgres instance.
type pluginRegistryStore interface {
	GetPluginRegistry(ctx context.Context, name string) (*settingsv1.PluginRegistry, error)
	UpsertPluginRegistry(ctx context.Context, r *settingsv1.PluginRegistry) error
}

// checkRegistryReachable calls List (page_size 1) against addr to confirm
// something garage-compatible actually answers there before Bench trusts
// it -- the same "reject bad input at the boundary" discipline
// loader.go's ParseWorkflow already applies to workflow YAML, applied here
// so a broken address doesn't silently start failing every search and every
// garage:// step that happens to resolve against it.
func checkRegistryReachable(ctx context.Context, httpClient connect.HTTPClient, address string) error {
	client := plugincatalogv1connect.NewPluginCatalogServiceClient(httpClient, address, connect.WithGRPC())
	if _, err := client.List(ctx, connect.NewRequest(&plugincatalogv1.ListPluginsRequest{PageSize: 1})); err != nil {
		return fmt.Errorf("could not reach a garage-compatible plugin registry at %q: %w", address, err)
	}
	return nil
}

// addPluginRegistry live-checks address and persists (name, address) as a
// new registry. Fails ErrPluginRegistryAlreadyExists if name is already
// taken.
func addPluginRegistry(ctx context.Context, store pluginRegistryStore, httpClient connect.HTTPClient, name, address string) (*settingsv1.PluginRegistry, error) {
	if name == "" {
		return nil, errors.New("name is required")
	}
	if address == "" {
		return nil, errors.New("address is required")
	}

	if _, err := store.GetPluginRegistry(ctx, name); err == nil {
		return nil, fmt.Errorf("%w: %q", ErrPluginRegistryAlreadyExists, name)
	} else if !errors.Is(err, ErrPluginRegistryNotFound) {
		return nil, err
	}

	if err := checkRegistryReachable(ctx, httpClient, address); err != nil {
		return nil, err
	}

	r := &settingsv1.PluginRegistry{Name: name, Address: address}
	if err := store.UpsertPluginRegistry(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}

// updatePluginRegistry live-checks address and replaces name's stored
// address. Fails ErrPluginRegistryNotFound if name doesn't already exist.
func updatePluginRegistry(ctx context.Context, store pluginRegistryStore, httpClient connect.HTTPClient, name, address string) (*settingsv1.PluginRegistry, error) {
	if address == "" {
		return nil, errors.New("address is required")
	}

	if _, err := store.GetPluginRegistry(ctx, name); err != nil {
		return nil, err // ErrPluginRegistryNotFound propagates as-is
	}

	if err := checkRegistryReachable(ctx, httpClient, address); err != nil {
		return nil, err
	}

	r := &settingsv1.PluginRegistry{Name: name, Address: address}
	if err := store.UpsertPluginRegistry(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}
