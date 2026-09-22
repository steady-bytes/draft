package control_plane

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	ntConnect "github.com/steady-bytes/draft/api/core/control_plane/networking/v1/v1connect"
	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"

	"connectrpc.com/connect"
	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	routeservice "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	runtimeservice "github.com/envoyproxy/go-control-plane/envoy/service/runtime/v3"
	secretservice "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"github.com/steady-bytes/draft/pkg/chassis"
	"google.golang.org/protobuf/types/known/anypb"
)

/////////////////////
// Fuse rpc Interface
/////////////////////

type (
	Rpc interface {
		chassis.RPCRegistrar

		ntConnect.NetworkingServiceHandler
	}

	rpc struct {
		logger       chassis.Logger
		controlPlane *controlPlane
	}
)

// rpc interface to `fuse` `control_plane`
func NewRPC(logger chassis.Logger, cp *controlPlane) Rpc {
	return &rpc{
		logger:       logger,
		controlPlane: cp,
	}
}

// register the `fuse` control plance rpc interface
func (h *rpc) RegisterRPC(server chassis.Rpcer) {
	// TODO: for some reason the h.logger doesn't work even though I *think* it should be instantiated in the chassis by this point
	val, err := anypb.New(&kvv1.Value{
		Data: chassis.GetConfig().GetString("fuse.address"),
	})
	if err != nil {
		h.logger.WithError(err).Panic("failed create kvv1.Value struct")
		panic("failed create kvv1.Value struct")
	}

	// add the fuse address to blueprint
	ctx := context.Background()
	kvClient := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, chassis.GetConfig().Entrypoint())
	_, err = kvClient.Set(ctx, connect.NewRequest(&kvv1.SetRequest{
		Key:   chassis.FuseAddressBlueprintKey,
		Value: val,
	}))
	if err != nil {
		h.logger.WithError(err).Panic("failed to register fuse address with blueprint")
		panic("failed to register fuse address with blueprint")
	}
	h.logger.Info("registered fuse address with blueprint")

	pattern, handler := ntConnect.NewNetworkingServiceHandler(h, connect.WithInterceptors(chassis.NewTraceInterceptor()))
	server.AddHandler(pattern, handler, true)
}

var (
	AddingRoute = "Add route request received"
	// Errors
	ErrInvalidRequest           = errors.New("invalid request")
	ErrInvalidRoute             = errors.New("invalid route")
	ErrInvalidRoutePrefix       = errors.New("invalid route prefix")
	ErrInvalidRouteName         = errors.New("invalid route name")
	ErrInvalidRouteHost         = errors.New("invalid route host: must be a hostname or a single leading wildcard label (eg. *.draft.localhost)")
	ErrUnableToUpdateProxyCache = errors.New("unable to update proxy cache")

	// hostPattern accepts a plain hostname (api.draft.localhost) or a single leading wildcard
	// label (*.draft.localhost). Envoy's virtual host domain matcher already understands the
	// wildcard form directly — this only guards against malformed input (eg. more than one "*").
	hostPattern = regexp.MustCompile(`^(\*\.)?[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*$`)
)

func (h *rpc) AddRoute(ctx context.Context, req *connect.Request[ntv1.AddRouteRequest]) (*connect.Response[ntv1.AddRouteResponse], error) {
	var (
		logger = h.logger.WithContext(ctx)
		msg    = req.Msg
		err    error
	)

	logger.WithField("msg", msg).Info(AddingRoute)

	// validate incoming request
	// TODO: Add validation to the proto message
	if msg == nil {
		return nil, ErrInvalidRequest
	}

	if msg.GetRoute() == nil {
		return nil, ErrInvalidRoute
	}

	if msg.GetRoute().Match.Prefix == "" {
		return nil, ErrInvalidRoutePrefix
	}

	if msg.GetRoute().Name == "" {
		return nil, ErrInvalidRouteName
	}

	if host := msg.GetRoute().GetMatch().GetHost(); host != "" && !hostPattern.MatchString(host) {
		return nil, ErrInvalidRouteHost
	}

	conflicts, err := h.controlPlane.FindConflicts(ctx, msg.GetRoute(), "")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if len(conflicts) > 0 {
		return connect.NewResponse(&ntv1.AddRouteResponse{
			Code:              ntv1.AddRouteResponseCode_INVALID_REQUEST,
			Message:           fmt.Sprintf("conflicts with existing route(s): %s", strings.Join(conflicts, ", ")),
			ConflictingRoutes: conflicts,
		}), nil
	}

	if capMsg := h.controlPlane.checkCapabilities(msg.GetRoute()); capMsg != "" {
		return connect.NewResponse(&ntv1.AddRouteResponse{
			Code:    ntv1.AddRouteResponseCode_INVALID_REQUEST,
			Message: capMsg,
		}), nil
	}

	if err != h.controlPlane.UpdateCacheWithNewRoute(msg.GetRoute()) {
		return nil, ErrUnableToUpdateProxyCache
	}

	return &connect.Response[ntv1.AddRouteResponse]{
		Msg: &ntv1.AddRouteResponse{
			Code: ntv1.AddRouteResponseCode_OK,
		},
	}, nil
}

// DeleteRoute implements Rpc.
func (h *rpc) DeleteRoute(ctx context.Context, req *connect.Request[ntv1.DeleteRouteRequest]) (*connect.Response[ntv1.DeleteRouteResponse], error) {
	name := req.Msg.GetName()
	if name == "" {
		return nil, ErrInvalidRouteName
	}
	if err := h.controlPlane.DeleteRoute(ctx, name); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&ntv1.DeleteRouteResponse{
		Code: ntv1.DeleteRouteCode_DELETE_ROUTE_OK,
	}), nil
}

// ValidateRoute implements Rpc. It runs the same conflict check as AddRoute without persisting
// anything, so callers (eg. the blueprint UI) can check before submitting.
func (h *rpc) ValidateRoute(ctx context.Context, req *connect.Request[ntv1.ValidateRouteRequest]) (*connect.Response[ntv1.ValidateRouteResponse], error) {
	conflicts, err := h.controlPlane.FindConflicts(ctx, req.Msg.GetRoute(), req.Msg.GetExistingName())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp := &ntv1.ValidateRouteResponse{
		Valid:             len(conflicts) == 0,
		ConflictingRoutes: conflicts,
	}
	switch {
	case len(conflicts) > 0:
		resp.Message = fmt.Sprintf("conflicts with existing route(s): %s", strings.Join(conflicts, ", "))
	default:
		if capMsg := h.controlPlane.checkCapabilities(req.Msg.GetRoute()); capMsg != "" {
			resp.Valid = false
			resp.Message = capMsg
		}
	}
	return connect.NewResponse(resp), nil
}

// ListRoutes implements Rpc.
func (h *rpc) ListRoutes(ctx context.Context, req *connect.Request[ntv1.ListRoutesRequest]) (*connect.Response[ntv1.ListRoutesResponse], error) {
	routes, err := h.controlPlane.ListRoutes(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&ntv1.ListRoutesResponse{Routes: routes}), nil
}

///////////////////////
// xDS Server Interface
///////////////////////

type (
	XDSRpc interface {
		chassis.RPCRegistrar
	}

	xDSRpc struct {
		backend *EnvoyBackend
		logger  chassis.Logger
	}
)

// NewXDSRpc registers the xDS/ADS surface that a separate Envoy process
// connects back to. Only meaningful when the envoy ProxyBackend is active
// -- main.go only calls this when backend, as constructed, is an
// *EnvoyBackend; there is nothing for a native-backend cluster to consume
// here, so it isn't registered in that case.
func NewXDSRpc(logger chassis.Logger, backend *EnvoyBackend) XDSRpc {
	return &xDSRpc{
		logger:  logger,
		backend: backend,
	}
}

// The provided rpc interface from `go-control-plane` uses the native gRPC server. That is hoisted from
// the chassis to the application level.
func (c *xDSRpc) RegisterRPC(server chassis.Rpcer) {
	grpcServer := server.GetGrpcServer()
	discoverygrpc.RegisterAggregatedDiscoveryServiceServer(grpcServer, c.backend.xDSServer)
	server.AddHandler("/envoy.service.discovery.v3.AggregatedDiscoveryService/", grpcServer, false)
	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, c.backend.xDSServer)
	server.AddHandler("/envoy.service.endpoint.v3.EndpointDiscoveryService/", grpcServer, false)
	clusterservice.RegisterClusterDiscoveryServiceServer(grpcServer, c.backend.xDSServer)
	server.AddHandler("/envoy.service.cluster.v3.ClusterDiscoveryService/", grpcServer, false)
	routeservice.RegisterRouteDiscoveryServiceServer(grpcServer, c.backend.xDSServer)
	server.AddHandler("/envoy.service.route.v3.RouteDiscoveryService/", grpcServer, false)
	listenerservice.RegisterListenerDiscoveryServiceServer(grpcServer, c.backend.xDSServer)
	server.AddHandler("/envoy.service.listener.v3.ListenerDiscoveryService/", grpcServer, false)
	secretservice.RegisterSecretDiscoveryServiceServer(grpcServer, c.backend.xDSServer)
	server.AddHandler("/envoy.service.secret.v3.SecretDiscoveryService/", grpcServer, false)
	runtimeservice.RegisterRuntimeDiscoveryServiceServer(grpcServer, c.backend.xDSServer)
	server.AddHandler("/envoy.service.runtime.v3.RuntimeDiscoveryService/", grpcServer, false)
}
