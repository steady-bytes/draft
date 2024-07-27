package control_plane

import (
	"fmt"
	"time"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	router "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	DEFAULT_CLUSTER_NAME     = "draft-fuse"
	DEFAULT_LISTENER_ADDRESS = "0.0.0.0"
	DEFAULT_LISTENER_PORT    = 10000
	DEFAULT_LISTENER_NAME    = "listener_0"

	// RouteName    = "local_route"
	// ListenerName = "listener_0"
	// ListenerPort = 10000
	// UpstreamHost = "www.envoyproxy.io"
	// UpstreamPort = 80
)

func makeCluster(clusterName string, loadAssignment *endpoint.ClusterLoadAssignment) *cluster.Cluster {
	return &cluster.Cluster{
		Name:                 clusterName,
		ConnectTimeout:       durationpb.New(5 * time.Second),
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_LOGICAL_DNS},
		LbPolicy:             cluster.Cluster_ROUND_ROBIN,
		LoadAssignment:       loadAssignment,
		DnsLookupFamily:      cluster.Cluster_V4_ONLY,
	}
}

// `urlDomain` 			:`url_domain` found in the `config.yaml` file of a process. (ie. steady-bytes.com)
// `virtualHostName`	:draft `chassis.Namespace` that is defined in `main.go` and configured in the `chassis.Builder`. (ie. fuse, or file_host)
// `upstreamHost` 		:the host ip (IPv4) that the cluster will be forwarding the traffic to
// `upstreamPort`		:the port that the cluster will be forwarding the traffic to
func makeEndpoint(urlDomain, virtualHostName, upstreamHost string, upstreamPort uint32) *endpoint.ClusterLoadAssignment {
	domain := makeDomain(urlDomain, virtualHostName)

	return &endpoint.ClusterLoadAssignment{
		ClusterName: domain,
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
									// I think this is the ip address of the process that is trying to add the route.
									Address: upstreamHost,
									PortSpecifier: &core.SocketAddress_PortValue{
										// I think this is the port of the process that is trying to add the route.
										PortValue: upstreamPort,
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

func makeDomain(urlDomain, virtualHostName string) string {
	return fmt.Sprintf("%s.%s", urlDomain, virtualHostName)
}

// `makeRoute` creates a route for the given cluster, and a virtual host for the process that is attempting to add the route.
//
// `urlDomain` 			:`url_domain` found in the `config.yaml` file of a process. (ie. steady-bytes.com)
// `nt_route` 			:route configuration that is being added to the snapshot.
func makeRoute(urlDomain string, nt_route *ntv1.Route) *route.RouteConfiguration {
	// The domain is the virtual host name and the route name combined.
	// (i.e file_host.steady-bytes.com)
	//
	// TODO: Add the support for root domains. (i.e "*")
	// so that is possible to have a route for all domains. It's currently not needed but it's a good feature to have.
	domain := makeDomain(urlDomain, nt_route.GetName())

	return &route.RouteConfiguration{
		Name: urlDomain,
		VirtualHosts: []*route.VirtualHost{{
			Name:    nt_route.GetName(),
			Domains: []string{domain},
			Routes: []*route.Route{{
				Match: &route.RouteMatch{
					PathSpecifier: &route.RouteMatch_Prefix{
						Prefix: nt_route.GetMatch().Prefix,
					},
				},
				Action: &route.Route_Route{
					Route: &route.RouteAction{
						ClusterSpecifier: &route.RouteAction_Cluster{
							Cluster: domain,
						},
					},
				},
			}},
		}},
	}
}

// `makeHTTPListener`
func makeHTTPListener(listenerName, urlDomain string) *listener.Listener {
	routerConfig, _ := anypb.New(&router.Router{})

	// HTTP filter configuration
	manager := &hcm.HttpConnectionManager{
		CodecType:  hcm.HttpConnectionManager_AUTO,
		StatPrefix: "http",
		RouteSpecifier: &hcm.HttpConnectionManager_Rds{
			Rds: &hcm.Rds{
				ConfigSource:    makeConfigSource(),
				RouteConfigName: urlDomain,
			},
		},
		HttpFilters: []*hcm.HttpFilter{{
			Name:       "http-router",
			ConfigType: &hcm.HttpFilter_TypedConfig{TypedConfig: routerConfig},
		}},
	}

	pbst, err := anypb.New(manager)
	if err != nil {
		panic(err)
	}

	return &listener.Listener{
		Name: listenerName,
		Address: &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Protocol: core.SocketAddress_TCP,
					Address:  DEFAULT_LISTENER_ADDRESS,
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: DEFAULT_LISTENER_PORT,
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
			// TransportSocket is used to configure the TLS settings for the listener.
			// TransportSocket: &core.TransportSocket{
			// 	Name:       "envoy.transport_sockets.tls",
			// 	ConfigType: &core.TransportSocket_TypedConfig{}},
			// },
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
			resource.ClusterType: {makeCluster(DEFAULT_CLUSTER_NAME, &endpoint.ClusterLoadAssignment{})},
		},
	)
	return snap
}
