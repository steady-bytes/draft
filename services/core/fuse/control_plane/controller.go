package control_plane

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	extauthzv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	grpcwebv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_web/v3"
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
		DeleteRoute(ctx context.Context, name string) error
		ListRoutes(ctx context.Context) ([]*ntv1.Route, error)
		FindConflicts(ctx context.Context, candidate *ntv1.Route) ([]string, error)
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

	// auth service discovery
	AuthServiceBlueprintKey = "auth_service_address"
	AUTH_CLUSTER_NAME       = "auth-service"
	AUTH_FILTER_NAME        = "envoy.filters.http.ext_authz"
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
		logger = cp.logger.WithField("route_name", route.Name)
		client = kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, chassis.GetConfig().Entrypoint())
	)

	logger.Info("updating cache with new route")

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
		logger.Error(err.Error())
		return ErrUnableToSaveRoute
	}

	return cp.apply(ctx, client)
}

// DeleteRoute removes a route from the blueprint key/value store and rebuilds the Envoy snapshot
// without it, mirroring what UpdateCacheWithNewRoute already does on add.
func (cp *controlPlane) DeleteRoute(ctx context.Context, name string) error {
	client := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, chassis.GetConfig().Entrypoint())

	routeModel, err := anypb.New(&ntv1.Route{})
	if err != nil {
		cp.logger.Error(err.Error())
		return ErrFailedRouteMarshal
	}

	_, err = client.Delete(ctx, connect.NewRequest(&kvv1.DeleteRequest{
		Key:   name,
		Value: routeModel,
	}))
	if err != nil {
		cp.logger.Error(err.Error())
		return err
	}

	return cp.apply(ctx, client)
}

// FindConflicts returns the names of any existing routes that share the same (host, match_type,
// prefix) tuple as candidate. Excludes candidate.Name itself so re-registering an unchanged route
// doesn't flag against itself.
func (cp *controlPlane) FindConflicts(ctx context.Context, candidate *ntv1.Route) ([]string, error) {
	existing, err := cp.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}

	key := routeKey(candidate.GetMatch())
	var conflicts []string
	for _, r := range existing {
		if r.GetName() == candidate.GetName() {
			continue
		}
		if r.GetMatch().GetHost() == candidate.GetMatch().GetHost() && routeKey(r.GetMatch()) == key {
			conflicts = append(conflicts, r.GetName())
		}
	}
	return conflicts, nil
}

// routeKey normalizes match_type (UNSPECIFIED behaves as PREFIX, matching the compiled Envoy
// behavior in makeRouterConfig) so two routes that would compile to the same Envoy route conflict
// even if one left match_type unset.
func routeKey(m *ntv1.RouteMatch) string {
	mt := m.GetMatchType()
	if mt == ntv1.MatchType_MATCH_TYPE_UNSPECIFIED {
		mt = ntv1.MatchType_MATCH_TYPE_PREFIX
	}
	return fmt.Sprintf("%d:%s", mt, m.GetPrefix())
}

func (cp *controlPlane) ListRoutes(ctx context.Context) ([]*ntv1.Route, error) {
	client := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, chassis.GetConfig().Entrypoint())

	routeModel, err := anypb.New(&ntv1.Route{})
	if err != nil {
		return nil, ErrFailedRouteMarshal
	}

	resp, err := client.List(ctx, connect.NewRequest(&kvv1.ListRequest{Value: routeModel}))
	if err != nil {
		return nil, ErrUnableToSaveRoute
	}

	routes := make([]*ntv1.Route, 0, len(resp.Msg.GetValues()))
	for _, v := range resp.Msg.GetValues() {
		r := &ntv1.Route{}
		if err := v.UnmarshalTo(r); err != nil {
			return nil, ErrFailedRouteMarshal
		}
		routes = append(routes, r)
	}
	return routes, nil
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

	// Discover the auth service address. Empty string means auth is not deployed;
	// routes are treated as public and no ext_authz filter is added.
	authAddr := cp.getAuthServiceAddress(ctx, client)
	authEnabled := authAddr != ""

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

	// Add the auth service cluster when auth is enabled so Envoy can reach it.
	if authEnabled {
		authCluster, err := makeAuthCluster(authAddr)
		if err != nil {
			cp.logger.WithError(err).Warn("invalid auth_service_address — running without auth")
			authEnabled = false
		} else {
			clusters = append(clusters, authCluster)
		}
	}

	systemRoutes = append(systemRoutes, makeRouterConfig(routes.Msg.GetValues(), authEnabled))

	newRouter := &router.Router{}

	routerConfig, err := anypb.New(newRouter)
	if err != nil {
		cp.logger.Error(err.Error())
		return err
	}

	// Build the ordered HttpFilter chain. ext_authz must come before grpc_web, which must come
	// before the router.
	httpFilters := []*hcm.HttpFilter{}
	if authEnabled {
		extAuthzFilter, err := makeExtAuthzFilter(authAddr)
		if err != nil {
			cp.logger.WithError(err).Error("failed to build ext_authz filter")
			return err
		}
		httpFilters = append(httpFilters, extAuthzFilter)
	}
	// grpc_web translates the grpc-web wire format browsers use into standard gRPC. It's a no-op
	// passthrough for non-grpc-web requests, so it's safe to enable unconditionally.
	grpcWebAny, err := anypb.New(&grpcwebv3.GrpcWeb{})
	if err != nil {
		cp.logger.Error(err.Error())
		return err
	}
	httpFilters = append(httpFilters, &hcm.HttpFilter{
		Name:       "envoy.filters.http.grpc_web",
		ConfigType: &hcm.HttpFilter_TypedConfig{TypedConfig: grpcWebAny},
	})
	httpFilters = append(httpFilters, &hcm.HttpFilter{
		Name:       "fuse-http-router",
		ConfigType: &hcm.HttpFilter_TypedConfig{TypedConfig: routerConfig},
	})

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
		HttpFilters: httpFilters,
		UpgradeConfigs: []*hcm.HttpConnectionManager_UpgradeConfig{
			{
				UpgradeType: "websocket",
			},
		},
		// disable with 0 value
		StreamIdleTimeout: &durationpb.Duration{},
		// Host-based routing (subdomain-per-service UI routes, RouteMatch.host generally)
		// matches VirtualHost.Domains against the request's Host/:authority header verbatim,
		// port included, unless told otherwise. A browser includes the port whenever it's
		// non-default (eg. Host: blueprint.draft.localhost:10000 hitting this listener's own
		// non-standard port) -- without this, that request falls through to the catch-all "*"
		// virtual host instead of matching the dedicated one, since routes are registered with
		// just the bare host (eg. "blueprint.draft.localhost"), not host:port. Confirmed live:
		// curl with an explicit Host header (no port) matched correctly and masked this: only
		// testing through an actual browser against the real listener port surfaced it.
		StripPortMode: &hcm.HttpConnectionManager_StripAnyHostPort{
			StripAnyHostPort: true,
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

// getAuthServiceAddress reads the auth service address from Blueprint KV.
// Returns an empty string if the auth service is not registered.
func (cp *controlPlane) getAuthServiceAddress(ctx context.Context, client kvv1Connect.KeyValueServiceClient) string {
	val, err := anypb.New(&kvv1.Value{})
	if err != nil {
		return ""
	}
	resp, err := client.Get(ctx, connect.NewRequest(&kvv1.GetRequest{
		Key:   AuthServiceBlueprintKey,
		Value: val,
	}))
	if err != nil {
		return ""
	}
	value := &kvv1.Value{}
	if err := resp.Msg.GetValue().UnmarshalTo(value); err != nil {
		return ""
	}
	return value.Data
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

// makeAuthCluster builds an Envoy cluster that points to the auth service.
func makeAuthCluster(rawURL string) (*cluster.Cluster, error) {
	host, port, err := parseAuthServiceURL(rawURL)
	if err != nil {
		return nil, err
	}
	la := &endpoint.ClusterLoadAssignment{
		ClusterName: AUTH_CLUSTER_NAME,
		Endpoints: []*endpoint.LocalityLbEndpoints{{
			LbEndpoints: []*endpoint.LbEndpoint{{
				HostIdentifier: &endpoint.LbEndpoint_Endpoint{
					Endpoint: &endpoint.Endpoint{
						Address: &core.Address{
							Address: &core.Address_SocketAddress{
								SocketAddress: &core.SocketAddress{
									Protocol:      core.SocketAddress_TCP,
									Address:       host,
									PortSpecifier: &core.SocketAddress_PortValue{PortValue: port},
								},
							},
						},
					},
				},
			}},
		}},
	}
	return &cluster.Cluster{
		Name:                 AUTH_CLUSTER_NAME,
		ConnectTimeout:       durationpb.New(5 * time.Second),
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_LOGICAL_DNS},
		LbPolicy:             cluster.Cluster_ROUND_ROBIN,
		LoadAssignment:       la,
		DnsLookupFamily:      cluster.Cluster_V4_ONLY,
	}, nil
}

// makeExtAuthzFilter builds the ext_authz HttpFilter that points to the auth service cluster.
func makeExtAuthzFilter(rawURL string) (*hcm.HttpFilter, error) {
	extAuthzConfig := &extauthzv3.ExtAuthz{
		Services: &extauthzv3.ExtAuthz_HttpService{
			HttpService: &extauthzv3.HttpService{
				ServerUri: &core.HttpUri{
					Uri: rawURL,
					HttpUpstreamType: &core.HttpUri_Cluster{
						Cluster: AUTH_CLUSTER_NAME,
					},
					Timeout: durationpb.New(250 * time.Millisecond),
				},
			},
		},
		TransportApiVersion: resource.DefaultAPIVersion,
		// deny the request if the auth service is unavailable
		FailureModeAllow: false,
	}
	extAuthzAny, err := anypb.New(extAuthzConfig)
	if err != nil {
		return nil, err
	}
	return &hcm.HttpFilter{
		Name:       AUTH_FILTER_NAME,
		ConfigType: &hcm.HttpFilter_TypedConfig{TypedConfig: extAuthzAny},
	}, nil
}

// makePerRouteAuthConfig returns the typed_per_filter_config for the ext_authz filter on
// a single route. When authEnabled is false (no auth service registered) nil is returned.
func makePerRouteAuthConfig(r *ntv1.Route, authEnabled bool) *anypb.Any {
	if !authEnabled {
		return nil
	}

	auth := r.GetAuth()

	// No auth field, disabled auth, or explicit bypass → disable the check for this route.
	if auth == nil || !auth.Enabled || auth.Policy == ntv1.AuthPolicy_AUTH_POLICY_BYPASS {
		perRoute := &extauthzv3.ExtAuthzPerRoute{
			Override: &extauthzv3.ExtAuthzPerRoute_Disabled{Disabled: true},
		}
		a, _ := anypb.New(perRoute)
		return a
	}

	// Build context extensions carrying the policy requirements so the auth service
	// can enforce them without needing its own route table.
	contextExtensions := map[string]string{}
	switch auth.Policy {
	case ntv1.AuthPolicy_AUTH_POLICY_GROUPS:
		if len(auth.RequiredGroups) > 0 {
			contextExtensions["required_groups"] = strings.Join(auth.RequiredGroups, ",")
		}
	case ntv1.AuthPolicy_AUTH_POLICY_SCOPES:
		if len(auth.RequiredScopes) > 0 {
			contextExtensions["required_scopes"] = strings.Join(auth.RequiredScopes, " ")
		}
	}

	perRoute := &extauthzv3.ExtAuthzPerRoute{
		Override: &extauthzv3.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &extauthzv3.CheckSettings{
				ContextExtensions: contextExtensions,
			},
		},
	}
	a, _ := anypb.New(perRoute)
	return a
}

// `makeRoute` creates a route for the given cluster, and a virtual host for the process that is attempting to add the route.
//
// `nt_route` 			:route configuration that is being added to the snapshot.
// `authEnabled` 		:whether the auth service is registered; controls per-route ext_authz config generation.
func makeRouterConfig(routes map[string]*anypb.Any, authEnabled bool) *route.RouteConfiguration {
	var (
		virtualHosts       []*route.VirtualHost
		defaultVirtualHost = &route.VirtualHost{
			Name:    "default",
			Domains: []string{"*"},
			Routes:  []*route.Route{},
		}
	)

	for _, rt := range routes {
		r := &ntv1.Route{}
		err := rt.UnmarshalTo(r)
		if err != nil {
			return nil
		}

		perRouteConfig := map[string]*anypb.Any{}
		if authCfg := makePerRouteAuthConfig(r, authEnabled); authCfg != nil {
			perRouteConfig[AUTH_FILTER_NAME] = authCfg
		}

		// match_type is unset (UNSPECIFIED) on every route registered before this field existed;
		// treat that the same as PREFIX so those routes keep compiling identically. See routeKey,
		// which applies the same normalization for conflict detection.
		routeMatch := &route.RouteMatch{}
		if r.Match.GetMatchType() == ntv1.MatchType_MATCH_TYPE_EXACT {
			routeMatch.PathSpecifier = &route.RouteMatch_Path{Path: r.Match.Prefix}
		} else {
			routeMatch.PathSpecifier = &route.RouteMatch_Prefix{Prefix: r.Match.Prefix}
		}

		envoyRoute := &route.Route{
			Match: routeMatch,
			Action: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_Cluster{
						Cluster: clusterName(r),
					},
					// disable with 0 value
					Timeout: &durationpb.Duration{},
				},
			},
			TypedPerFilterConfig: perRouteConfig,
		}

		// if no host is requested, add to default host
		if r.Match.Host == "" {
			defaultVirtualHost.Routes = append(defaultVirtualHost.Routes, envoyRoute)
		} else {
			virtualHosts = append(virtualHosts, &route.VirtualHost{
				Name:    r.Name,
				Domains: []string{r.Match.Host},
				Routes:  []*route.Route{envoyRoute},
			})
		}
	}

	// only include the default virtual host if it's being used
	if len(defaultVirtualHost.Routes) > 0 {
		virtualHosts = append(virtualHosts, defaultVirtualHost)
	}

	// TODO: This is a bit of a hack to force simple prefixes like "/" to be pushed to the last place in the routes slice.
	//		Doing this is important since you might have multiple services (routes) attached to a single host with
	// 		one hosting a web-client with a prefix of "/" and others hosting APIs with prefixes like "/examples.crud.v1.CrudService/".
	// 		This needs to be revisited with a proper pattern defined for enabling developers to define RouteMatch ordering.
	//
	// EXACT routes (compiled to route.RouteMatch_Path, not _Prefix) must sort before every PREFIX route regardless of
	// path length: Envoy's route.RouteMatch.GetPrefix() returns "" for a _Path-specified match, so comparing raw
	// GetPrefix() values (the previous version of this sort) silently treated every EXACT route as if it had the
	// shortest possible prefix -- losing to a catch-all "/" PREFIX route instead of winning as the more specific
	// match. Two EXACT routes never need ordering between each other: identical (host, EXACT, path) tuples are
	// already rejected as a conflict in AddRoute, so within one virtual host at most one can match a given path.
	for _, vh := range virtualHosts {
		slices.SortFunc(vh.Routes, func(a, b *route.Route) int {
			aExact := a.Match.GetPath() != ""
			bExact := b.Match.GetPath() != ""
			if aExact != bExact {
				if aExact {
					return -1
				}
				return 1
			}
			if aExact {
				return 0
			}
			return len(b.Match.GetPrefix()) - len(a.Match.GetPrefix())
		})
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

func parseAuthServiceURL(rawURL string) (host string, port uint32, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", 0, err
	}
	host = u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		portStr = "80"
	}
	p, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		return "", 0, err
	}
	return host, uint32(p), nil
}
