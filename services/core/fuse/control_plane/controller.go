package control_plane

import (
	"context"
	"os"

	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	routeservice "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	runtimeservice "github.com/envoyproxy/go-control-plane/envoy/service/runtime/v3"
	secretservice "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"

	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/envoyproxy/go-control-plane/pkg/test/v3"

	"github.com/steady-bytes/draft/pkg/chassis"
)

type (
	ControlPlane interface {
		chassis.RPCRegistrar
		// xDS server
		server.Server
	}

	controlPlane struct {
		xDSServer server.Server
		logger    chassis.Logger
	}
)

func New(logger chassis.Logger) *controlPlane {
	var (
		cache    = cache.NewSnapshotCache(false, cache.IDHash{}, logger)
		snapshot = GenerateSnapshot()
		ctx      = context.Background()
	)

	// ensure the snapshot is well-formed
	if err := snapshot.Consistent(); err != nil {
		logger.Errorf("snapshot inconsistency: %+v\n%+v", snapshot, err)
		os.Exit(1)
	}

	// set the snapshot to the cache
	if err := cache.SetSnapshot(context.Background(), "example", snapshot); err != nil {
		logger.Errorf("snapshot error: %+v", err)
		os.Exit(1)
	}

	// TODO: find a more elegant way to handle debug enable.
	cb := &test.Callbacks{Debug: true}

	return &controlPlane{
		xDSServer: server.NewServer(ctx, cache, cb),
		logger:    logger,
	}
}

func (c *controlPlane) RegisterRPC(server chassis.Rpcer) {
	server.EnableReflection("control_plane")
	grpcServer := server.GetGrpcServer()

	discoverygrpc.RegisterAggregatedDiscoveryServiceServer(grpcServer, c.xDSServer)
	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, c.xDSServer)
	clusterservice.RegisterClusterDiscoveryServiceServer(grpcServer, c.xDSServer)
	routeservice.RegisterRouteDiscoveryServiceServer(grpcServer, c.xDSServer)
	listenerservice.RegisterListenerDiscoveryServiceServer(grpcServer, c.xDSServer)
	secretservice.RegisterSecretDiscoveryServiceServer(grpcServer, c.xDSServer)
	runtimeservice.RegisterRuntimeDiscoveryServiceServer(grpcServer, c.xDSServer)

	c.logger = server.Logger()
}
