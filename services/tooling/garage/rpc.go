package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	plugincatalogv1 "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1"
	plugincatalogv1connect "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
)

// This file wires PluginCatalogService (Publish/Retract/Get/List/Search) —
// see api/tooling/plugin_catalog/v1/service.proto — onto store.go's
// persistence layer, following the same handler shape as
// services/examples/crud/service/rpc.go: a struct implementing both
// chassis.RPCRegistrar (so main.go can pass it to WithRPCHandler) and the
// generated ServiceHandler interface.

type (
	Handler interface {
		chassis.RPCRegistrar
		plugincatalogv1connect.PluginCatalogServiceHandler
	}
	handler struct {
		logger chassis.Logger
		store  *store
	}
)

func NewHandler(logger chassis.Logger, store *store) Handler {
	return &handler{
		logger: logger,
		store:  store,
	}
}

func (h *handler) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := plugincatalogv1connect.NewPluginCatalogServiceHandler(h, connect.WithInterceptors(chassis.NewTraceInterceptor()))
	server.AddHandler(pattern, handler, true)
}

// Publish validates and inserts a new plugin version.
func (h *handler) Publish(ctx context.Context, req *connect.Request[plugincatalogv1.PublishRequest]) (*connect.Response[plugincatalogv1.PublishResponse], error) {
	msg := req.Msg

	if strings.TrimSpace(msg.GetName()) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if strings.TrimSpace(msg.GetVersion()) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("version is required"))
	}
	if err := validateSchemaShape(msg.GetConfigSchema(), "config_schema"); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := validateSchemaShape(msg.GetResultSchema(), "result_schema"); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	plugin := &plugincatalogv1.Plugin{
		Name:         msg.GetName(),
		Version:      msg.GetVersion(),
		Description:  msg.GetDescription(),
		Maintainer:   msg.GetMaintainer(),
		Source:       msg.GetSource(),
		ConfigSchema: msg.GetConfigSchema(),
		ResultSchema: msg.GetResultSchema(),
	}

	row, err := h.store.publish(ctx, plugin)
	if err != nil {
		if errors.Is(err, ErrAlreadyPublished) {
			return nil, connect.NewError(connect.CodeAlreadyExists, err)
		}
		h.logger.WithError(err).Error("failed to publish plugin")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to publish plugin"))
	}

	published, err := row.toProto()
	if err != nil {
		h.logger.WithError(err).Error("failed to convert published plugin row to proto")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to publish plugin"))
	}

	return connect.NewResponse(&plugincatalogv1.PublishResponse{Plugin: published}), nil
}

// Retract removes a published (name, version), called from a plugin's
// chassis.Effect inverse on graceful shutdown (see the design doc's
// "Publishing and discovery").
//
// Retracting an already-retracted or never-published (name, version) is
// treated as a no-op, not an error: a shutdown path shouldn't fail because
// the catalog entry it's trying to remove is already gone (e.g. Publish
// itself failed earlier, or shutdown retries after a partial failure). The
// caller's actual intent — "make sure this isn't listed" — is satisfied
// either way, and Bench/Blueprint's picture of "what's live right now" ends
// up correct regardless of which branch was taken to get there.
func (h *handler) Retract(ctx context.Context, req *connect.Request[plugincatalogv1.RetractRequest]) (*connect.Response[plugincatalogv1.RetractResponse], error) {
	msg := req.Msg

	if strings.TrimSpace(msg.GetName()) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if strings.TrimSpace(msg.GetVersion()) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("version is required"))
	}

	if _, err := h.store.retract(ctx, msg.GetName(), msg.GetVersion()); err != nil {
		h.logger.WithError(err).Error("failed to retract plugin")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to retract plugin"))
	}

	return connect.NewResponse(&plugincatalogv1.RetractResponse{}), nil
}

// Get looks up the exact (name, version) plugin.
func (h *handler) Get(ctx context.Context, req *connect.Request[plugincatalogv1.GetPluginRequest]) (*connect.Response[plugincatalogv1.Plugin], error) {
	msg := req.Msg

	if strings.TrimSpace(msg.GetName()) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if strings.TrimSpace(msg.GetVersion()) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("version is required"))
	}

	row, err := h.store.get(ctx, msg.GetName(), msg.GetVersion())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("plugin %s@%s not found", msg.GetName(), msg.GetVersion()))
		}
		h.logger.WithError(err).Error("failed to get plugin")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get plugin"))
	}

	p, err := row.toProto()
	if err != nil {
		h.logger.WithError(err).Error("failed to convert plugin row to proto")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get plugin"))
	}

	return connect.NewResponse(p), nil
}

// List returns published plugins, keyset-paginated on (published_at, id).
func (h *handler) List(ctx context.Context, req *connect.Request[plugincatalogv1.ListPluginsRequest]) (*connect.Response[plugincatalogv1.ListPluginsResponse], error) {
	msg := req.Msg

	rows, nextPageToken, err := h.store.list(ctx, msg.GetPageSize(), msg.GetPageToken())
	if err != nil {
		if errors.Is(err, ErrInvalidPageToken) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		h.logger.WithError(err).Error("failed to list plugins")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list plugins"))
	}

	plugins, err := rowsToProto(rows)
	if err != nil {
		h.logger.WithError(err).Error("failed to convert plugin rows to proto")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list plugins"))
	}

	return connect.NewResponse(&plugincatalogv1.ListPluginsResponse{
		Plugins:       plugins,
		NextPageToken: nextPageToken,
	}), nil
}

// Search matches query against a plugin's name and description.
func (h *handler) Search(ctx context.Context, req *connect.Request[plugincatalogv1.SearchPluginsRequest]) (*connect.Response[plugincatalogv1.SearchPluginsResponse], error) {
	msg := req.Msg

	rows, err := h.store.search(ctx, msg.GetQuery())
	if err != nil {
		h.logger.WithError(err).Error("failed to search plugins")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to search plugins"))
	}

	plugins, err := rowsToProto(rows)
	if err != nil {
		h.logger.WithError(err).Error("failed to convert plugin rows to proto")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to search plugins"))
	}

	return connect.NewResponse(&plugincatalogv1.SearchPluginsResponse{Plugins: plugins}), nil
}

func rowsToProto(rows []*pluginRow) ([]*plugincatalogv1.Plugin, error) {
	plugins := make([]*plugincatalogv1.Plugin, 0, len(rows))
	for _, row := range rows {
		p, err := row.toProto()
		if err != nil {
			return nil, err
		}
		plugins = append(plugins, p)
	}
	return plugins, nil
}

// validateSchemaShape is a deliberately trivial check on a config_schema/
// result_schema payload — not a JSON-Schema-of-JSON-Schema meta-validator
// (out of scope per the Phase 2 brief), just enough to reject the obvious
// garbage a full validator would also catch: a "type" keyword that isn't a
// string or list of strings, or a "required" keyword that isn't a list.
// Both are JSON Schema keywords with a fixed shape; getting either wrong is
// a real, easy authoring mistake (e.g. `type: object` typo'd as a bare
// unquoted word that YAML parses as something other than a string upstream)
// worth catching at publish time rather than the first time Bench tries to
// validate a step's `with:` block against it. A nil Struct (config_schema/
// result_schema are optional on a manifest) is valid and skipped entirely.
func validateSchemaShape(s *structpb.Struct, field string) error {
	if s == nil {
		return nil
	}

	fields := s.GetFields()

	if typeVal, ok := fields["type"]; ok {
		switch typeVal.GetKind().(type) {
		case *structpb.Value_StringValue, *structpb.Value_ListValue:
			// valid: JSON Schema's "type" is a string or an array of strings.
		default:
			return fmt.Errorf("%s.type must be a string or array of strings", field)
		}
	}

	if requiredVal, ok := fields["required"]; ok {
		if _, ok := requiredVal.GetKind().(*structpb.Value_ListValue); !ok {
			return fmt.Errorf("%s.required must be an array of field names", field)
		}
	}

	return nil
}
