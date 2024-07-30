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

	route = c.routeDefaults(route)

	val, err := anypb.New(&kvv1.Value{})
	if err != nil {
		c.logger.WithError(err).Error("failed to create anypb value struct")
		return err
	}

	// get fuse address from blueprint
	response, err := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, c.config.Entrypoint()).
		Get(ctx, connect.NewRequest(&kvv1.GetRequest{
			Key: FuseAddressBlueprintKey,
			Value: val,
		}))
	if err != nil {
		c.logger.WithError(err).Error("failed to get fuse address")
		return err
	}

	// unmarshal value
	value := &kvv1.Value{}
	if err := anypb.UnmarshalTo(response.Msg.GetValue(), value, proto.UnmarshalOptions{}); err != nil {
		c.logger.WithError(err).Error("failed to unmarshal fuse address")
		return err
	}

	// add route to fuse
	res, err := ntv1Connect.NewNetworkingServiceClient(http.DefaultClient, value.Data).
		AddRoute(ctx, connect.NewRequest(&ntv1.AddRouteRequest{
			Route: route,
		}))
	if err != nil {
		c.logger.WithError(err).Error("failed to add route")
		return err
	}

	c.logger.WithField("response", res).Info("successfully added route")

	return nil
}

// routeDefaults will set defaults on the Route for anything not specified by the user
func (c *Runtime) routeDefaults(route *ntv1.Route) (*ntv1.Route) {
	if route.Name == "" {
		route.Name = fmt.Sprintf("%s-%s", c.config.Domain(), c.config.Name())
	}
	if route.Endpoint == nil {
		route.Endpoint = &ntv1.Endpoint{
			Host: c.config.GetString("service.address"),
			Port: c.config.GetUint32("service.port"),
		}
	}
	if route.Endpoint.Host == "" {
		route.Endpoint.Host = c.config.GetString("service.address")
	}
	if route.Endpoint.Port == 0 {
		route.Endpoint.Port = c.config.GetUint32("service.port")
	}
	return route
}
