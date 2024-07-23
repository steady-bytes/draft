package control_plane

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	routeservice "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	runtimeservice "github.com/envoyproxy/go-control-plane/envoy/service/runtime/v3"
	secretservice "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	ntConnect "github.com/steady-bytes/draft/api/core/control_plane/networking/v1/v1connect"
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
	pattern, handler := ntConnect.NewNetworkingServiceHandler(h)
	server.AddHandler(pattern, handler, true)
}

var (
	AddingRoute       = "Add route request received"
	ErrNotImplemented = errors.New("not implemented")
)

func (h *rpc) AddRoute(ctx context.Context, req *connect.Request[ntv1.AddRouteRequest]) (*connect.Response[ntv1.AddRouteResponse], error) {
	var (
		logger = h.logger.WithContext(ctx)
		msg    = req.Msg
	)

	logger.WithField("msg", msg).Debug(AddingRoute)

	// validate incoming request

	// Add route to blueprint

	// create a new snapshot in the cache

	return nil, ErrNotImplemented
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
