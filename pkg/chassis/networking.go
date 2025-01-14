package chassis

import (
	"context"
	"fmt"
	"net/http"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	ntv1Connect "github.com/steady-bytes/draft/api/core/control_plane/networking/v1/v1connect"
	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	FuseAddressBlueprintKey = "fuse_address"
)

func (c *Runtime) withRoute(route *ntv1.Route) error {
	var (
		ctx = context.Background()
	)

	route = c.setAndValidateRoute(route)
	logger := c.logger.WithField("route_name", route.Name)

	val, err := anypb.New(&kvv1.Value{})
	if err != nil {
		logger.WithError(err).Error("failed to create anypb value struct")
		return err
	}

	// get fuse address from blueprint
	response, err := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, c.config.Entrypoint()).
		Get(ctx, connect.NewRequest(&kvv1.GetRequest{
			Key:   FuseAddressBlueprintKey,
			Value: val,
		}))
	if err != nil {
		logger.WithError(err).Error("failed to get fuse address")
		return err
	}

	// unmarshal value
	value := &kvv1.Value{}
	if err := anypb.UnmarshalTo(response.Msg.GetValue(), value, proto.UnmarshalOptions{}); err != nil {
		logger.WithError(err).Error("failed to unmarshal fuse address")
		return err
	}

	// add route to fuse
	_, err = ntv1Connect.NewNetworkingServiceClient(http.DefaultClient, value.Data).
		AddRoute(ctx, connect.NewRequest(&ntv1.AddRouteRequest{
			Route: route,
		}))
	if err != nil {
		logger.WithError(err).Error("failed to add route")
		return err
	}

	logger.Info("successfully added route")

	return nil
}

// setAndValidateRoute will set defaults on the Route for anything not specified by the user and
// validates the route is valid
// TODO: we should do validation through protobuf annotations instead
func (c *Runtime) setAndValidateRoute(route *ntv1.Route) *ntv1.Route {
	if route.Name == "" {
		route.Name = fmt.Sprintf("%s-%s", c.config.Domain(), c.config.Name())
	}
	if route.Match == nil {
		c.logger.Panic("route requested but no match provided")
	}
	if route.Match.Host == "" {
		route.Match.Host = c.config.GetString("service.network.external.host")
	}
	if route.Match.Host == "" {
		c.logger.Panic("route requested but no host provided in the match")
	}
	if route.Endpoint == nil {
		route.Endpoint = &ntv1.Endpoint{
			Host: c.getConfigInternalHost(),
			Port: c.getConfigInternalPort(),
		}
	}
	if route.Endpoint.Host == "" {
		route.Endpoint.Host = c.getConfigInternalHost()
	}
	if route.Endpoint.Port == 0 {
		route.Endpoint.Port = c.getConfigInternalPort()
	}
	return route
}

func (c *Runtime) getConfigInternalHost() string {
	host := c.config.GetString("service.network.internal.host")
	if host == "" {
		c.logger.Panic("route requested but no internal host name provided")
	}
	return host
}

func (c *Runtime) getConfigInternalPort() uint32 {
	port := c.config.GetUint32("service.network.internal.port")
	if port == 0 {
		c.logger.Panic("route requested but no internal port provided")
	}
	return port
}
