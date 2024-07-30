package control_plane

import (
	"context"

	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/envoyproxy/go-control-plane/pkg/test/v3"
	"github.com/google/uuid"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"

	"github.com/steady-bytes/draft/pkg/chassis"
)

type (
	ControlPlane interface {
		cache.SnapshotCache

		UpdateCacheWithNewRoute(route *ntv1.Route) error
		Increment() string
	}

	controlPlane struct {
		count           string
		xDSServer       server.Server
		logger          chassis.Logger
		cache           cache.SnapshotCache
		listenerAddress string
		listenerPort    uint32
	}
)

const (
	defaultListenerAddress   = "0.0.0.0"
	defaultListenerPort      = 10000
	listenerAddressConfigKey = "fuse.listener.address"
	listenerPortConfigKey    = "fuse.listener.port"
)

func NewControlPlane(logger chassis.Logger) *controlPlane {
	var (
		ctx      = context.Background()
		cache    = cache.NewSnapshotCache(false, cache.IDHash{}, logger)
		snapshot = GenerateSnapshot()
		config   = chassis.GetConfig()
	)

	// ensure the snapshot is well-formed
	if err := snapshot.Consistent(); err != nil {
		logger.WithError(err).WithField("snapshot", snapshot).Panic("snapshot failed consistency check")
	}

	// set the snapshot to the cache
	if err := cache.SetSnapshot(ctx, "fuse-proxy-1", snapshot); err != nil {
		logger.WithError(err).WithField("snapshot", snapshot).Panic("failed to set snapshot")
	}

	// TODO: find a more elegant way to handle debug enable.
	cb := &test.Callbacks{Debug: true}

	// set listener attributes from config (or defaults)
	listenerAddress := config.GetString(listenerAddressConfigKey)
	if listenerAddress == "" {
		listenerAddress = defaultListenerAddress
	}
	listenerPort := config.GetUint32(listenerPortConfigKey)
	if listenerPort == 0 {
		listenerPort = defaultListenerPort
	}
	return &controlPlane{
		xDSServer:       server.NewServer(ctx, cache, cb),
		logger:          logger,
		cache:           cache,
		listenerAddress: listenerAddress,
		listenerPort:    listenerPort,
	}
}

func (cp *controlPlane) UpdateCacheWithNewRoute(route *ntv1.Route) error {
	var (
		ctx = context.Background()
	)

	clusterLoadAssignment := makeEndpoint(route)

	// make a new snapshot with the new route
	snapshot, _ := cache.NewSnapshot(cp.Increment(),
		map[resource.Type][]types.Resource{
			resource.ClusterType:  {makeCluster(route, clusterLoadAssignment)},
			resource.RouteType:    {makeRoute(route)},
			resource.ListenerType: {cp.makeHTTPListener(DEFAULT_LISTENER_NAME, route)},
		},
	)

	// Apply the newly generated snapshot to the cache
	if err := cp.cache.SetSnapshot(ctx, "fuse-proxy-1", snapshot); err != nil {
		cp.logger.Errorf("snapshot error: %+v", err)
		return err
	}

	return nil
}

// Increase the version of the snapshot. At this point we are just generating a random UUID.
//
// TODO: Keep track of the version in `blueprint` to load historical routing configurations.
// Having an audit trail of routing configurations is important for debugging
func (cp *controlPlane) Increment() string {
	cp.count = uuid.New().String()
	return cp.count
}
