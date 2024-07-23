package control_plane

import (
	"context"
	"os"

	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/envoyproxy/go-control-plane/pkg/test/v3"

	"github.com/steady-bytes/draft/pkg/chassis"
)

type (
	ControlPlane interface {
		cache.SnapshotCache
	}

	controlPlane struct {
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
