package control_plane

import (
	"context"
	"os"

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
		count string

		xDSServer server.Server
		logger    chassis.Logger
		cache     cache.SnapshotCache
	}
)

func NewControlPlane(logger chassis.Logger) *controlPlane {
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
	if err := cache.SetSnapshot(ctx, "fuse-proxy-1", snapshot); err != nil {
		logger.Errorf("snapshot error: %+v", err)
		os.Exit(1)
	}

	// TODO: find a more elegant way to handle debug enable.
	cb := &test.Callbacks{Debug: true}

	return &controlPlane{
		xDSServer: server.NewServer(ctx, cache, cb),
		logger:    logger,
		cache:     cache,
	}
}

func (cp *controlPlane) UpdateCacheWithNewRoute(route *ntv1.Route) error {
	var (
		ctx = context.Background()
	)

	version := cp.Increment()

	snapshot, _ := cache.NewSnapshot(version,
		map[resource.Type][]types.Resource{
			resource.ClusterType:  {makeCluster(CLUSTER_NAME)},
			resource.RouteType:    {makeRoute(cp.urlDomain, cp.name, cp.clusterName, route)},
			resource.ListenerType: {makeHTTPListener(ListenerName, RouteName)},
		},
	)

	// Get snapshot from the cache
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
