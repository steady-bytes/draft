package control_plane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

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
		Key: chassis.FuseAddressBlueprintKey,
		Value: val,
	}))
	if err != nil {
		h.logger.WithError(err).Panic("failed to register fuse address with blueprint")
		panic("failed to register fuse address with blueprint")
	}
	h.logger.Info("registered fuse address with blueprint")

	pattern, handler := ntConnect.NewNetworkingServiceHandler(h)
	server.AddHandler(pattern, handler, true)
}

var (
	AddingRoute = "Add route request received"
	// Errors
	ErrNotImplemented           = errors.New("not implemented")
	ErrInvalidRequest           = errors.New("invalid request")
	ErrInvalidRoute             = errors.New("invalid route")
	ErrInvalidRoutePrefix       = errors.New("invalid route prefix")
	ErrInvalidRouteName         = errors.New("invalid route name")
	ErrUnableToSaveRoute        = errors.New("unable to save route in the key/value store")
	ErrUnableToUpdateProxyCache = errors.New("unable to update proxy cache")
)

func (h *rpc) AddRoute(ctx context.Context, req *connect.Request[ntv1.AddRouteRequest]) (*connect.Response[ntv1.AddRouteResponse], error) {
	var (
		logger = h.logger.WithContext(ctx)
		msg    = req.Msg
		err    error
	)

	logger.WithField("msg", msg).Debug(AddingRoute)

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

	route := msg.GetRoute()
	routeJSON, err := json.Marshal(route)
	if err != nil {
		logger.Error(err.Error())
		return nil, ErrInvalidRoute
	}

	// upsert route in the blueprint key/value store
	val, err := anypb.New(&kvv1.Value{
		// I think I like saving the `Route` as a JSON string
		Data: string(routeJSON),
	})
	if err != nil {
		logger.Error(err.Error())
		return nil, ErrUnableToSaveRoute
	}

	setReq := connect.NewRequest(&kvv1.SetRequest{
		Key:   msg.GetRoute().Name,
		Value: val,
	})

	client := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, chassis.GetConfig().Entrypoint())
	_, err = client.Set(context.Background(), setReq)
	if err != nil {
		logger.Error(err.Error())
		return nil, ErrUnableToSaveRoute
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
func (h *rpc) DeleteRoute(context.Context, *connect.Request[ntv1.DeleteRouteRequest]) (*connect.Response[ntv1.DeleteRouteResponse], error) {
	return nil, ErrNotImplemented
}

// ListRoutes implements Rpc.
func (h *rpc) ListRoutes(context.Context, *connect.Request[ntv1.ListRoutesRequest]) (*connect.Response[ntv1.ListRoutesResponse], error) {
	return nil, ErrNotImplemented
}

///////////////////////
// xDS Server Interface
///////////////////////

type (
	XDSRpc interface {
		chassis.RPCRegistrar
	}

	xDSRpc struct {
		controlPlane *controlPlane
		logger       chassis.Logger
	}
)

// rpc interface to envoy xDS server
func NewXDSRpc(logger chassis.Logger, cp *controlPlane) XDSRpc {
	return &xDSRpc{
		logger:       logger,
		controlPlane: cp,
	}
}

// The provided rpc interface from `go-control-plane` uses the native gRPC server. That is hoisted from
// the chassis to the application level.
func (c *xDSRpc) RegisterRPC(server chassis.Rpcer) {
	grpcServer := server.GetGrpcServer()
	discoverygrpc.RegisterAggregatedDiscoveryServiceServer(grpcServer, c.controlPlane.xDSServer)
	server.AddHandler("/envoy.service.discovery.v3.AggregatedDiscoveryService/", grpcServer, false)
	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, c.controlPlane.xDSServer)
	server.AddHandler("/envoy.service.endpoint.v3.EndpointDiscoveryService/", grpcServer, false)
	clusterservice.RegisterClusterDiscoveryServiceServer(grpcServer, c.controlPlane.xDSServer)
	server.AddHandler("/envoy.service.cluster.v3.ClusterDiscoveryService/", grpcServer, false)
	routeservice.RegisterRouteDiscoveryServiceServer(grpcServer, c.controlPlane.xDSServer)
	server.AddHandler("/envoy.service.route.v3.RouteDiscoveryService/", grpcServer, false)
	listenerservice.RegisterListenerDiscoveryServiceServer(grpcServer, c.controlPlane.xDSServer)
	server.AddHandler("/envoy.service.listener.v3.ListenerDiscoveryService/", grpcServer, false)
	secretservice.RegisterSecretDiscoveryServiceServer(grpcServer, c.controlPlane.xDSServer)
	server.AddHandler("/envoy.service.secret.v3.SecretDiscoveryService/", grpcServer, false)
	runtimeservice.RegisterRuntimeDiscoveryServiceServer(grpcServer, c.controlPlane.xDSServer)
	server.AddHandler("/envoy.service.runtime.v3.RuntimeDiscoveryService/", grpcServer, false)
}
