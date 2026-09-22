package control_plane

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	tracev3 "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	extauthzv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	grpcwebv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_web/v3"
	router "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	upstreams "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"

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

// EnvoyBackend is the ProxyBackend that configures a separate Envoy process
// over xDS -- today's (pre-ProxyBackend) behavior, extracted behind the
// interface with no change in what it does. Exported because main.go needs
// to name the type directly to decide whether to also register the xDS/ADS
// RPC surface (see NewXDSRpc) -- that surface only makes sense when this
// backend is the one actually consuming it.
type EnvoyBackend struct {
	count           string
	xDSServer       server.Server
	logger          chassis.Logger
	cache           cache.SnapshotCache
	listenerAddress string
	listenerPort    uint32
}

const (
	// default listener values if key is not set in the `config.yaml` file when the service is run
	LISTENER_DEFAULT_NAME = "listener_0"

	DEFAULT_ROUTE_CONFIG_NAME = "route_config"

	// auth service discovery
	AuthServiceBlueprintKey = "auth_service_address"
	AUTH_CLUSTER_NAME       = "auth-service"
	AUTH_FILTER_NAME        = "envoy.filters.http.ext_authz"

	// OTLP tracing: empty TracingOTLPAddressConfigKey means tracing is
	// disabled entirely (same empty-string-means-off convention as
	// AuthServiceBlueprintKey/authEnabled below) -- Fuse has no hard
	// dependency on a collector existing.
	TracingOTLPAddressConfigKey = "fuse.tracing.otlp_address"
	TRACING_CLUSTER_NAME        = "otlp-tracing"
	TRACING_PROVIDER_NAME       = "envoy.tracers.opentelemetry"
)

// NewEnvoyBackend constructs the envoy ProxyBackend: an xDS snapshot cache
// and ADS server, seeded with an empty snapshot until the first Apply call.
func NewEnvoyBackend(logger chassis.Logger) *EnvoyBackend {
	var (
		ctx       = context.Background()
		snapCache = cache.NewSnapshotCache(false, cache.IDHash{}, logger)
		snapshot  = GenerateSnapshot()
		config    = chassis.GetConfig()
	)

	// ensure the snapshot is well-formed
	if err := snapshot.Consistent(); err != nil {
		logger.WithError(err).WithField("snapshot", snapshot).Panic("snapshot failed consistency check")
	}

	// set the snapshot to the cache
	if err := snapCache.SetSnapshot(ctx, "fuse-proxy-1", snapshot); err != nil {
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
	return &EnvoyBackend{
		xDSServer:       server.NewServer(ctx, snapCache, cb),
		logger:          logger,
		cache:           snapCache,
		listenerAddress: listenerAddress,
		listenerPort:    listenerPort,
	}
}

func (b *EnvoyBackend) Name() string { return ProxyBackendEnvoy }

// Capabilities reflects what's actually shipped today: TLS/MTLS are
// specced (Fuse — API Gateway's Phases 5-6, via the already-registered but
// unpopulated Secret Discovery Service) but unbuilt; WideEvents has a
// possible future path via Envoy's Access Log Service (see
// docs/website/content/docs/architecture/fuse-native-proxy.md#wideevent-parity-for-the-envoy-backend)
// but is likewise unbuilt. Flip these to true only once that work ships --
// this method's job is to tell the truth about what exists, not to signal
// intent.
func (b *EnvoyBackend) Capabilities() BackendCapabilities {
	return BackendCapabilities{TLS: false, MTLS: false, WideEvents: false, ACME: false}
}

// Apply rebuilds and pushes a full Envoy xDS snapshot for the given route
// table. This is controlPlane.apply()'s former body, unchanged in
// behavior -- only the route table now arrives as a parameter (already
// merged by the caller) instead of being read from Blueprint KV directly,
// since that read is controlPlane's job, not this backend's.
func (b *EnvoyBackend) Apply(routes []*ntv1.Route) error {
	var (
		ctx    = context.Background()
		client = kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, chassis.GetConfig().Entrypoint())
	)

	// Discover the auth service address. Empty string means auth is not deployed;
	// routes are treated as public and no ext_authz filter is added.
	authAddr := GetAuthServiceAddress(ctx, client)
	authEnabled := authAddr != ""

	var snapshot *cache.Snapshot
	var clusters []types.Resource
	var systemRoutes []types.Resource

	for _, newRoute := range routes {
		// Add individual service routes to the new snapshot
		clusterLoadAssignment := makeEndpoint(newRoute)
		clusters = append(clusters, makeCluster(newRoute, clusterLoadAssignment))
	}

	// Add the auth service cluster when auth is enabled so Envoy can reach it.
	if authEnabled {
		authCluster, err := makeAuthCluster(authAddr)
		if err != nil {
			b.logger.WithError(err).Warn("invalid auth_service_address — running without auth")
			authEnabled = false
		} else {
			clusters = append(clusters, authCluster)
		}
	}

	// OTLP tracing: same empty-means-disabled gate as auth above, read from
	// static config (not Blueprint) since a tracing collector is infra, not
	// a registered Draft process. When enabled, Envoy originates/propagates
	// a traceparent header on every request it proxies -- the first trace
	// context an external request gets, before it ever reaches a chassis
	// service.
	otlpAddr := chassis.GetConfig().GetString(TracingOTLPAddressConfigKey)
	var tracingConfig *hcm.HttpConnectionManager_Tracing
	if otlpAddr != "" {
		tracingCluster, err := makeTracingCluster(otlpAddr)
		if err != nil {
			b.logger.WithError(err).Warn("invalid fuse.tracing.otlp_address — running without tracing")
		} else {
			clusters = append(clusters, tracingCluster)
			tracingConfig = makeTracingConfig()
		}
	}

	systemRoutes = append(systemRoutes, makeRouterConfig(routes, authEnabled))

	newRouter := &router.Router{}

	routerConfig, err := anypb.New(newRouter)
	if err != nil {
		b.logger.Error(err.Error())
		return err
	}

	// Build the ordered HttpFilter chain. ext_authz must come before grpc_web, which must come
	// before the router.
	httpFilters := []*hcm.HttpFilter{}
	if authEnabled {
		extAuthzFilter, err := makeExtAuthzFilter(authAddr)
		if err != nil {
			b.logger.WithError(err).Error("failed to build ext_authz filter")
			return err
		}
		httpFilters = append(httpFilters, extAuthzFilter)
	}
	// grpc_web translates the grpc-web wire format browsers use into standard gRPC. It's a no-op
	// passthrough for non-grpc-web requests, so it's safe to enable unconditionally.
	grpcWebAny, err := anypb.New(&grpcwebv3.GrpcWeb{})
	if err != nil {
		b.logger.Error(err.Error())
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
		Tracing:    tracingConfig,
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
		// non-default -- without this, that request falls through to the catch-all "*"
		// virtual host instead of matching the dedicated one, since routes are registered with
		// just the bare host, not host:port.
		StripPortMode: &hcm.HttpConnectionManager_StripAnyHostPort{
			StripAnyHostPort: true,
		},
	}

	pbst, err := anypb.New(manager)
	if err != nil {
		b.logger.Error(err.Error())
		return err
	}

	// create the default listener envoy will use
	envoyListener := &listener.Listener{
		Name: LISTENER_DEFAULT_NAME,
		Address: &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Protocol: core.SocketAddress_TCP,
					Address:  b.listenerAddress,
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: b.listenerPort,
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

	snapshot, _ = cache.NewSnapshot(b.increment(),
		map[resource.Type][]types.Resource{
			resource.ClusterType:  clusters,
			resource.RouteType:    systemRoutes,
			resource.ListenerType: {envoyListener},
		},
	)

	// Apply the newly generated snapshot to the cache
	if err := b.cache.SetSnapshot(ctx, "fuse-proxy-1", snapshot); err != nil {
		b.logger.Errorf("snapshot error: %+v", err)
		return err
	}

	return nil
}

// Increase the version of the snapshot. At this point we are just generating a random UUID.
//
// TODO: Keep track of the version in `blueprint` to load historical routing configurations.
// Having an audit trail of routing configurations is important for debugging
func (b *EnvoyBackend) increment() string {
	b.count = uuid.New().String()
	return b.count
}

func makeCluster(r *ntv1.Route, loadAssignment *endpoint.ClusterLoadAssignment) *cluster.Cluster {
	// LOGICAL_DNS only ever resolves and uses the first host in a cluster's load assignment --
	// every other endpoint is silently ignored, which would make a load-balanced route (more than
	// one registered backend) actually just pin to whichever endpoint happened to sort first.
	// STRICT_DNS resolves every configured host independently and pools all of their resolved
	// addresses together, which is what round-robining across multiple backends actually needs.
	// Single-backend routes (still the overwhelming majority) keep LOGICAL_DNS, unchanged.
	discoveryType := cluster.Cluster_LOGICAL_DNS
	if len(r.GetEndpoints()) > 1 {
		discoveryType = cluster.Cluster_STRICT_DNS
	}

	c := &cluster.Cluster{
		Name:                 clusterName(r),
		ConnectTimeout:       durationpb.New(5 * time.Second),
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: discoveryType},
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

// makeTracingCluster builds an Envoy cluster pointing to the OTLP/gRPC
// collector (Beacon). Mirrors makeAuthCluster's plain-URL construction, plus
// the HTTP/2 upstream protocol options makeCluster sets for EnableHttp2
// routes -- OTLP export is gRPC, which requires an HTTP/2 upstream or the
// connection fails outright.
func makeTracingCluster(rawURL string) (*cluster.Cluster, error) {
	host, port, err := parseAuthServiceURL(rawURL)
	if err != nil {
		return nil, err
	}
	la := &endpoint.ClusterLoadAssignment{
		ClusterName: TRACING_CLUSTER_NAME,
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
	protocolOptionsAny, err := anypb.New(&upstreams.HttpProtocolOptions{
		UpstreamProtocolOptions: &upstreams.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &upstreams.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &upstreams.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
					Http2ProtocolOptions: &core.Http2ProtocolOptions{},
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return &cluster.Cluster{
		Name:                 TRACING_CLUSTER_NAME,
		ConnectTimeout:       durationpb.New(5 * time.Second),
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_LOGICAL_DNS},
		LbPolicy:             cluster.Cluster_ROUND_ROBIN,
		LoadAssignment:       la,
		DnsLookupFamily:      cluster.Cluster_V4_ONLY,
		TypedExtensionProtocolOptions: map[string]*anypb.Any{
			"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": protocolOptionsAny,
		},
	}, nil
}

// makeTracingConfig builds the HttpConnectionManager.Tracing block that
// makes Envoy originate/propagate a traceparent header on every request,
// exporting spans as OTLP to the cluster makeTracingCluster built.
func makeTracingConfig() *hcm.HttpConnectionManager_Tracing {
	otelConfigAny, err := anypb.New(&tracev3.OpenTelemetryConfig{
		GrpcService: &core.GrpcService{
			TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &core.GrpcService_EnvoyGrpc{ClusterName: TRACING_CLUSTER_NAME},
			},
		},
		ServiceName: "fuse",
	})
	if err != nil {
		return nil
	}
	return &hcm.HttpConnectionManager_Tracing{
		Provider: &tracev3.Tracing_Http{
			Name: TRACING_PROVIDER_NAME,
			ConfigType: &tracev3.Tracing_Http_TypedConfig{
				TypedConfig: otelConfigAny,
			},
		},
	}
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

// `makeRouterConfig` creates a route for the given cluster, and a virtual host for the process that is attempting to add the route.
//
// `routes` 			:one entry per logical route (already merged by mergeRoutes -- exactly one route/virtual host is built per name, regardless of how many endpoints back it).
// `authEnabled` 		:whether the auth service is registered; controls per-route ext_authz config generation.
func makeRouterConfig(routes []*ntv1.Route, authEnabled bool) *route.RouteConfiguration {
	var (
		virtualHosts       []*route.VirtualHost
		defaultVirtualHost = &route.VirtualHost{
			Name:    "default",
			Domains: []string{"*"},
			Routes:  []*route.Route{},
		}
	)

	for _, r := range routes {
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

// makeEndpoint builds one LbEndpoint per backend registered under this route (r.Endpoints, as
// populated by mergeRoutes -- multiple entries here is exactly the load-balanced case). Falls
// back to r.Endpoint alone if Endpoints is empty, which only happens for a Route that never went
// through mergeRoutes (defensive; every real caller in this package merges first).
func makeEndpoint(r *ntv1.Route) *endpoint.ClusterLoadAssignment {
	endpoints := r.GetEndpoints()
	if len(endpoints) == 0 && r.GetEndpoint() != nil {
		endpoints = []*ntv1.Endpoint{r.GetEndpoint()}
	}

	lbEndpoints := make([]*endpoint.LbEndpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		lbEndpoints = append(lbEndpoints, &endpoint.LbEndpoint{
			HostIdentifier: &endpoint.LbEndpoint_Endpoint{
				Endpoint: &endpoint.Endpoint{
					Address: &core.Address{
						Address: &core.Address_SocketAddress{
							SocketAddress: &core.SocketAddress{
								// defaulting to tcp, this can be changed but will also depend on the protocol
								// the upstream is using. In this case it's http.
								Protocol: core.SocketAddress_TCP,
								Address:  ep.GetHost(),
								PortSpecifier: &core.SocketAddress_PortValue{
									PortValue: ep.GetPort(),
								},
							},
						},
					},
				},
			},
		})
	}

	return &endpoint.ClusterLoadAssignment{
		ClusterName: clusterName(r),
		Endpoints: []*endpoint.LocalityLbEndpoints{{
			LbEndpoints: lbEndpoints,
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
