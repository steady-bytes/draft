package control_plane

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	router "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	upstreams "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"

	"connectrpc.com/connect"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/envoyproxy/go-control-plane/pkg/test/v3"
	"github.com/google/uuid"
	"github.com/steady-bytes/draft/pkg/chassis"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
)

type (
	ControlPlane interface {
		cache.SnapshotCache

		LoadCache()
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
	// default listener values if key is not set in the `config.yaml` file when the service is run
	LISTENER_DEFAULT_NAME    = "listener_0"
	LISTENER_DEFAULT_ADDRESS = "0.0.0.0"
	LISTENER_DEFAULT_PORT    = 80
	// config keys
	LISTENER_ADDRESS_CONFIG_KEY = "fuse.listener.address"
	LISTENER_PORT_CONFIG_KEY    = "fuse.listener.port"

	DEFAULT_ROUTE_CONFIG_NAME = "route_config"
)

var (
	ErrFailedRouteMarshal = errors.New("failed to marshal route")
	ErrUnableToSaveRoute  = errors.New("unable to save route in the key/value store")
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
	listenerAddress := config.GetString(LISTENER_ADDRESS_CONFIG_KEY)
	if listenerAddress == "" {
		listenerAddress = LISTENER_DEFAULT_ADDRESS
	}
	listenerPort := config.GetUint32(LISTENER_PORT_CONFIG_KEY)
	if listenerPort == 0 {
		listenerPort = LISTENER_DEFAULT_PORT
	}
	return &controlPlane{
		xDSServer:       server.NewServer(ctx, cache, cb),
		logger:          logger,
		cache:           cache,
		listenerAddress: listenerAddress,
		listenerPort:    listenerPort,
	}
}

func (cp *controlPlane) LoadCache() {
	var (
		ctx    = context.Background()
		client = kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, chassis.GetConfig().Entrypoint())
	)

	err := cp.apply(ctx, client)
	if err != nil {
		cp.logger.WithError(err).Error("failed to load cache")
	}
}

func (cp *controlPlane) UpdateCacheWithNewRoute(route *ntv1.Route) error {
	var (
		ctx    = context.Background()
		client = kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, chassis.GetConfig().Entrypoint())
	)

	// upsert route in the blueprint key/value store
	val, err := anypb.New(route)
	if err != nil {
		cp.logger.Error(err.Error())
		return ErrUnableToSaveRoute
	}

	setReq := connect.NewRequest(&kvv1.SetRequest{
		Key:   route.Name,
		Value: val,
	})

	_, err = client.Set(ctx, setReq)
	if err != nil {
		cp.logger.Error(err.Error())
		return ErrUnableToSaveRoute
	}

	return cp.apply(ctx, client)
}

func (cp *controlPlane) apply(ctx context.Context, client kvv1Connect.KeyValueServiceClient) error {

	routeModel, err := anypb.New(&ntv1.Route{})
	if err != nil {
		cp.logger.Error(err.Error())
		return ErrFailedRouteMarshal
	}

	listRoutesReq := connect.NewRequest(&kvv1.ListRequest{
		Value: routeModel,
	})

	routes, err := client.List(ctx, listRoutesReq)
	if err != nil {
		cp.logger.Error(err.Error())
		return ErrUnableToSaveRoute
	}

	var snapshot *cache.Snapshot
	var clusters []types.Resource
	var systemRoutes []types.Resource

	for _, rr := range routes.Msg.GetValues() {
		newRoute := &ntv1.Route{}

		err := rr.UnmarshalTo(newRoute)
		if err != nil {
			cp.logger.Error(err.Error())
			return ErrFailedRouteMarshal
		}

		// Add individual service routes to the new snapshot
		clusterLoadAssignment := makeEndpoint(newRoute)
		clusters = append(clusters, makeCluster(newRoute, clusterLoadAssignment))
	}

	systemRoutes = append(systemRoutes, makeRouterConfig(routes.Msg.GetValues()))

	newRouter := &router.Router{}

	routerConfig, err := anypb.New(newRouter)
	if err != nil {
		cp.logger.Error(err.Error())
		return err
	}

	// HTTP filter configuration
	manager := &hcm.HttpConnectionManager{
		CodecType:  hcm.HttpConnectionManager_AUTO,
		StatPrefix: "http",
		RouteSpecifier: &hcm.HttpConnectionManager_Rds{
			Rds: &hcm.Rds{
				ConfigSource:    makeConfigSource(),
				RouteConfigName: routeConfigName(),
			},
		},
		HttpFilters: []*hcm.HttpFilter{{
			Name:       "fuse-http-router",
			ConfigType: &hcm.HttpFilter_TypedConfig{TypedConfig: routerConfig},
		}},
		UpgradeConfigs: []*hcm.HttpConnectionManager_UpgradeConfig{
			{
				UpgradeType: "websocket",
			},
		},
	}

	pbst, err := anypb.New(manager)
	if err != nil {
		cp.logger.Error(err.Error())
		return err
	}

	// create the default listener envoy will use
	listener := &listener.Listener{
		Name: LISTENER_DEFAULT_NAME,
		Address: &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Protocol: core.SocketAddress_TCP,
					Address:  cp.listenerAddress,
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: cp.listenerPort,
					},
				},
			},
		},
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name: "http-connection-manager",
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: pbst,
				},
			}},
		}},
	}

	snapshot, _ = cache.NewSnapshot(cp.increment(),
		map[resource.Type][]types.Resource{
			resource.ClusterType:  clusters,
			resource.RouteType:    systemRoutes,
			resource.ListenerType: {listener},
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
func (cp *controlPlane) increment() string {
	cp.count = uuid.New().String()
	return cp.count
}

func makeCluster(r *ntv1.Route, loadAssignment *endpoint.ClusterLoadAssignment) *cluster.Cluster {
	c := &cluster.Cluster{
		Name:                 clusterName(r),
		ConnectTimeout:       durationpb.New(5 * time.Second),
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_LOGICAL_DNS},
		LbPolicy:             cluster.Cluster_ROUND_ROBIN,
		LoadAssignment:       loadAssignment,
		DnsLookupFamily:      cluster.Cluster_V4_ONLY,
	}

	// enabling HTTP2 supports gRPC but can cause servers without HTTP2 support to fail the connection with a protocol error
	if r.EnableHttp2 {
		a, _ := anypb.New(&upstreams.HttpProtocolOptions{
			UpstreamProtocolOptions: &upstreams.HttpProtocolOptions_ExplicitHttpConfig_{
				ExplicitHttpConfig: &upstreams.HttpProtocolOptions_ExplicitHttpConfig{
					ProtocolConfig: &upstreams.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
						Http2ProtocolOptions: &core.Http2ProtocolOptions{},
					},
				},
			},
		})
		c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
			"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": a,
		}
	}

	return c
}

// `makeRoute` creates a route for the given cluster, and a virtual host for the process that is attempting to add the route.
//
// `nt_route` 			:route configuration that is being added to the snapshot.
func makeRouterConfig(routes map[string]*anypb.Any) *route.RouteConfiguration {
	var virtualHosts []*route.VirtualHost

	for _, rt := range routes {
		r := &ntv1.Route{}
		err := rt.UnmarshalTo(r)
		if err != nil {
			return nil
		}

		http := fmt.Sprintf("%s:80", r.Match.Host)
		https := fmt.Sprintf("%s:443", r.Match.Host)

		virtualHosts = append(virtualHosts, &route.VirtualHost{
			Name:    r.Name,
			Domains: []string{r.Match.Host, http, https},
			Routes: []*route.Route{{
				Match: &route.RouteMatch{
					PathSpecifier: &route.RouteMatch_Prefix{
						Prefix: r.Match.Prefix,
					},
				},
				Action: &route.Route_Route{
					Route: &route.RouteAction{
						ClusterSpecifier: &route.RouteAction_Cluster{
							Cluster: clusterName(r),
						},
					},
				},
				TypedPerFilterConfig: map[string]*anypb.Any{},
			}}})
	}

	return &route.RouteConfiguration{
		Name:         routeConfigName(),
		VirtualHosts: virtualHosts,
	}
}

func makeEndpoint(r *ntv1.Route) *endpoint.ClusterLoadAssignment {
	return &endpoint.ClusterLoadAssignment{
		ClusterName: clusterName(r),
		Endpoints: []*endpoint.LocalityLbEndpoints{{
			LbEndpoints: []*endpoint.LbEndpoint{{
				HostIdentifier: &endpoint.LbEndpoint_Endpoint{
					Endpoint: &endpoint.Endpoint{
						Address: &core.Address{
							Address: &core.Address_SocketAddress{
								SocketAddress: &core.SocketAddress{
									// defaulting to tcp, this can be changed but will also depend on the protocol
									// the upstream is using. In this case it's http.
									Protocol: core.SocketAddress_TCP,
									Address:  r.Endpoint.Host,
									PortSpecifier: &core.SocketAddress_PortValue{
										PortValue: r.Endpoint.Port,
									},
								},
							},
						},
					},
				},
			}},
		}},
	}
}

func makeConfigSource() *core.ConfigSource {
	source := &core.ConfigSource{}
	source.ResourceApiVersion = resource.DefaultAPIVersion
	source.ConfigSourceSpecifier = &core.ConfigSource_ApiConfigSource{
		ApiConfigSource: &core.ApiConfigSource{
			TransportApiVersion:       resource.DefaultAPIVersion,
			ApiType:                   core.ApiConfigSource_GRPC,
			SetNodeOnFirstMessageOnly: true,
			GrpcServices: []*core.GrpcService{{
				TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &core.GrpcService_EnvoyGrpc{ClusterName: "xds_cluster"},
				},
			}},
		},
	}
	return source
}

// `GenerateSnapshot` creates a snapshot with a cluster. This is only used to start the control plane.
func GenerateSnapshot() *cache.Snapshot {
	snap, _ := cache.NewSnapshot("1",
		map[resource.Type][]types.Resource{
			// resource.ClusterType: {makeCluster(DEFAULT_CLUSTER_NAME, &endpoint.ClusterLoadAssignment{})},
		},
	)
	return snap
}

func routeConfigName() string {
	return DEFAULT_ROUTE_CONFIG_NAME
}

func clusterName(r *ntv1.Route) string {
	return r.Name
}
