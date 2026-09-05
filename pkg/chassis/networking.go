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
	resp, err := ntv1Connect.NewNetworkingServiceClient(http.DefaultClient, value.Data).
		AddRoute(ctx, connect.NewRequest(&ntv1.AddRouteRequest{
			Route: route,
		}))
	if err != nil {
		logger.WithError(err).Error("failed to add route")
		return err
	}

	// A rejection (eg. a conflict with an existing route) is a normal, successful RPC response
	// with Code != OK, not a transport-level err above -- checking only err let a rejected
	// AddRoute silently log "successfully added route" and continue, with the route actually
	// never persisted. Found via a real conflict (services/tooling/garage's PluginCatalogService
	// route silently losing to a stale, orphaned "tooling-bench" KV entry claiming the same
	// prefix) that went undetected until this exact route stopped being reachable through Fuse.
	if code := resp.Msg.GetCode(); code != ntv1.AddRouteResponseCode_OK {
		err := fmt.Errorf("fuse rejected route %q: %s (code %s)", route.Name, resp.Msg.GetMessage(), code)
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

// getConfigInternalHost returns the host Fuse's route endpoint should use to reach this
// process. Prefers service.network.route.host over service.network.internal.host when set:
// internal.host is also advertised to Blueprint's ServiceDiscoveryService (see Register's
// AdvertiseAddress, builder.go) and used by every host-native peer-to-peer call this process
// makes or receives outside of Fuse -- including chassis's own OTel exporter resolving Beacon.
// In a topology where Envoy runs in Docker but everything else runs natively on the host (the
// common local-dev setup), those two audiences need different values: Envoy needs
// "host.docker.internal" to reach a host-native process, but a host-native peer dialing that
// same address gets "no such host" (host.docker.internal only resolves inside Docker). route.host
// lets a service say "reach me at X for Fuse's routing purposes specifically" without changing
// what it advertises to every other peer. Falls back to internal.host so services that don't
// need the distinction (Envoy and every native process/service running in the same topology)
// don't need to set anything new.
func (c *Runtime) getConfigInternalHost() string {
	if host := c.config.GetString("service.network.route.host"); host != "" {
		return host
	}
	host := c.config.GetString("service.network.internal.host")
	if host == "" {
		c.logger.Panic("route requested but no internal host name provided")
	}
	return host
}

// getConfigInternalPort mirrors getConfigInternalHost's route.port/internal.port fallback.
func (c *Runtime) getConfigInternalPort() uint32 {
	if port := c.config.GetUint32("service.network.route.port"); port != 0 {
		return port
	}
	port := c.config.GetUint32("service.network.internal.port")
	if port == 0 {
		c.logger.Panic("route requested but no internal port provided")
	}
	return port
}
