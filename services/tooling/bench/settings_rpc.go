// This file implements Phase 11's SettingsService RPCs — see
// api/tooling/settings/v1/service.proto and
// docs/website/content/docs/architecture/bench-workflow-engine.md's
// "Connecting a plugin registry" — following the exact handler shape
// rpc.go's WorkflowService handler already establishes: a struct
// implementing both chassis.RPCRegistrar and the generated
// *ServiceHandler interface, backed by store.go's persistence layer and
// registry_write.go's shared add/update logic.
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	settingsv1 "github.com/steady-bytes/draft/api/tooling/settings/v1"
	settingsv1connect "github.com/steady-bytes/draft/api/tooling/settings/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
)

type (
	SettingsHandler interface {
		chassis.RPCRegistrar
		settingsv1connect.SettingsServiceHandler
	}
	settingsHandler struct {
		logger     chassis.Logger
		store      *pgResultStore
		httpClient connect.HTTPClient
	}
)

func NewSettingsHandler(logger chassis.Logger, store *pgResultStore, httpClient connect.HTTPClient) SettingsHandler {
	return &settingsHandler{logger: logger, store: store, httpClient: httpClient}
}

func (h *settingsHandler) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := settingsv1connect.NewSettingsServiceHandler(h)
	server.AddHandler(pattern, handler, true)
}

// ListPluginRegistries returns every configured registry, seeded from
// config or added/edited through the Settings page since.
func (h *settingsHandler) ListPluginRegistries(ctx context.Context, req *connect.Request[settingsv1.ListPluginRegistriesRequest]) (*connect.Response[settingsv1.ListPluginRegistriesResponse], error) {
	registries, err := h.store.ListPluginRegistries(ctx)
	if err != nil {
		h.logger.WithError(err).Error("failed to list plugin registries")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list plugin registries"))
	}
	return connect.NewResponse(&settingsv1.ListPluginRegistriesResponse{Registries: registries}), nil
}

// AddPluginRegistry persists a new registry — see registry_write.go's
// addPluginRegistry for the live-check + create-not-upsert semantics.
func (h *settingsHandler) AddPluginRegistry(ctx context.Context, req *connect.Request[settingsv1.AddPluginRegistryRequest]) (*connect.Response[settingsv1.AddPluginRegistryResponse], error) {
	name := strings.TrimSpace(req.Msg.GetName())
	address := strings.TrimSpace(req.Msg.GetAddress())

	r, err := addPluginRegistry(ctx, h.store, h.httpClient, name, address)
	if err != nil {
		switch {
		case errors.Is(err, ErrPluginRegistryAlreadyExists):
			return nil, connect.NewError(connect.CodeAlreadyExists, err)
		default:
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}
	return connect.NewResponse(&settingsv1.AddPluginRegistryResponse{Registry: r}), nil
}

// UpdatePluginRegistry replaces an existing registry's address — see
// registry_write.go's updatePluginRegistry.
func (h *settingsHandler) UpdatePluginRegistry(ctx context.Context, req *connect.Request[settingsv1.UpdatePluginRegistryRequest]) (*connect.Response[settingsv1.UpdatePluginRegistryResponse], error) {
	name := strings.TrimSpace(req.Msg.GetName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	address := strings.TrimSpace(req.Msg.GetAddress())

	r, err := updatePluginRegistry(ctx, h.store, h.httpClient, name, address)
	if err != nil {
		switch {
		case errors.Is(err, ErrPluginRegistryNotFound):
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("plugin registry %q not found", name))
		default:
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}
	return connect.NewResponse(&settingsv1.UpdatePluginRegistryResponse{Registry: r}), nil
}

// DeletePluginRegistry removes a registry — see store.go's
// DeletePluginRegistry.
func (h *settingsHandler) DeletePluginRegistry(ctx context.Context, req *connect.Request[settingsv1.DeletePluginRegistryRequest]) (*connect.Response[settingsv1.DeletePluginRegistryResponse], error) {
	name := strings.TrimSpace(req.Msg.GetName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}

	if err := h.store.DeletePluginRegistry(ctx, name); err != nil {
		if errors.Is(err, ErrPluginRegistryNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("plugin registry %q not found", name))
		}
		h.logger.WithError(err).WithField("registry", name).Error("failed to delete plugin registry")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete plugin registry"))
	}
	return connect.NewResponse(&settingsv1.DeletePluginRegistryResponse{}), nil
}
