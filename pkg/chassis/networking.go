package chassis

import (
	"context"
	"errors"
	"net/http"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	ntv1Connect "github.com/steady-bytes/draft/api/core/control_plane/networking/v1/v1connect"

	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"

	"connectrpc.com/connect"
)

const (
	// default local address for the `fuse` gRPC server
	LOCAL_FUSE_ADDRESS = "http://127.0.0.1:18000"
)

var (
	ErrFailedToAddRoute       = errors.New("failed to add route")
	ErrCouldNotGetFuseAddress = errors.New("could not get fuse address")
)

func (c *Runtime) withRoute(route *ntv1.Route) error {
	var (
		ctx = context.Background()
	)

	// get `fuse` address from `blueprint`
	// TODO: Before using, validate what key fuse is storing it's address under in the registry
	_, err := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, c.config.Entrypoint()).
		Get(ctx, connect.NewRequest(&kvv1.GetRequest{}))
	if err != nil {
		c.logger.WithError(err).Error("failed to get fuse address")
		return ErrCouldNotGetFuseAddress
	}

	res, err := ntv1Connect.NewNetworkingServiceClient(http.DefaultClient, LOCAL_FUSE_ADDRESS).
		AddRoute(ctx, connect.NewRequest(&ntv1.AddRouteRequest{
			Route: route,
		}))
	if err != nil {
		c.logger.WithError(err).Error("failed to add route")
		return ErrFailedToAddRoute
	}

	c.logger.WithField("response", res).Info("successfully added route")

	return nil
}
