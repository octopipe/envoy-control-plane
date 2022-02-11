package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
)

type NodeSnapshotDomain struct {
	srcPath    string
	targetHost string
	targetPort int
}

type NodeSnapshot struct {
	version      string
	clusterName  string
	nodeId       string
	routeName    string
	listenerName string
	listenerHost string
	listenerPort int
	domains      []NodeSnapshotDomain
}

func NewNodeSnapshot(
	version string,
	clusterName string,
	nodeId string,
	listenerHost string,
	listenerPort int,
	domains []string,
) NodeSnapshot {

	parseDomains := []NodeSnapshotDomain{}

	for _, d := range domains {
		initialSplit := strings.Split(d, "->")
		preffix := initialSplit[0]
		domain := strings.Split(initialSplit[1], ":")[0]
		port := strings.Split(initialSplit[1], ":")[1]

		if port == "" {
			port = "80"
		}

		parsedPort, _ := strconv.Atoi(port)

		parseDomains = append(parseDomains, NodeSnapshotDomain{
			srcPath:    preffix,
			targetHost: domain,
			targetPort: parsedPort,
		})
	}

	return NodeSnapshot{
		version:      version,
		clusterName:  clusterName,
		nodeId:       nodeId,
		routeName:    fmt.Sprintf(`%s-route`, nodeId),
		listenerName: fmt.Sprintf(`%s-listener`, nodeId),
		listenerHost: listenerHost,
		listenerPort: listenerPort,
		domains:      parseDomains,
	}
}

func (s *NodeSnapshot) GenerateSnapshot() cache.Snapshot {
	fmt.Println(s)

	snap, _ := cache.NewSnapshot(s.version,
		map[resource.Type][]types.Resource{
			resource.ClusterType:  s.makeClusters(),
			resource.ListenerType: {s.makeHTTPListener()},
		},
	)
	return snap
}

func (s *NodeSnapshot) makeClusters() []types.Resource {
	clusters := []types.Resource{}

	for _, d := range s.domains {
		clusterName := fmt.Sprintf("%s-%s-%s", s.clusterName, d.srcPath, d.targetHost)
		clusters = append(clusters, &cluster.Cluster{
			Name:                 clusterName,
			ConnectTimeout:       durationpb.New(5 * time.Second),
			ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_DiscoveryType(cluster.Cluster_LOGICAL_DNS)},
			LbPolicy:             cluster.Cluster_ROUND_ROBIN,
			LoadAssignment:       s.makeEndpoints(clusterName, d),
			DnsLookupFamily:      cluster.Cluster_V4_ONLY,
		})
	}

	return clusters
}

func (s *NodeSnapshot) makeEndpoints(clusterName string, domain NodeSnapshotDomain) *endpoint.ClusterLoadAssignment {
	return &endpoint.ClusterLoadAssignment{
		ClusterName: clusterName,
		Endpoints: []*endpoint.LocalityLbEndpoints{{
			LbEndpoints: []*endpoint.LbEndpoint{{
				HostIdentifier: &endpoint.LbEndpoint_Endpoint{
					Endpoint: &endpoint.Endpoint{
						Address: &core.Address{
							Address: &core.Address_SocketAddress{
								SocketAddress: &core.SocketAddress{
									Protocol: core.SocketAddress_TCP,
									Address:  domain.targetHost,
									PortSpecifier: &core.SocketAddress_PortValue{
										PortValue: uint32(domain.targetPort),
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

func (s *NodeSnapshot) makeRoute() *route.RouteConfiguration {
	routes := []*route.Route{}

	for _, d := range s.domains {
		routes = append(routes, &route.Route{
			Match: &route.RouteMatch{
				PathSpecifier: &route.RouteMatch_Prefix{
					Prefix: d.srcPath,
				},
			},
			Action: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_Cluster{
						Cluster: fmt.Sprintf("%s-%s-%s", s.clusterName, d.srcPath, d.targetHost),
					},
					HostRewriteSpecifier: &route.RouteAction_HostRewriteLiteral{
						HostRewriteLiteral: d.targetHost,
					},
				},
			},
		})
	}

	return &route.RouteConfiguration{
		Name: s.routeName,
		VirtualHosts: []*route.VirtualHost{{
			Name:    "local_service",
			Domains: []string{"*"},
			Routes:  routes,
		}},
	}
}

func (s *NodeSnapshot) makeHTTPListener() *listener.Listener {
	// HTTP filter configuration
	manager := &hcm.HttpConnectionManager{
		CodecType:  hcm.HttpConnectionManager_AUTO,
		StatPrefix: "http",
		RouteSpecifier: &hcm.HttpConnectionManager_RouteConfig{
			RouteConfig: s.makeRoute(),
		},
		HttpFilters: []*hcm.HttpFilter{{
			Name: wellknown.Router,
		}},
	}
	pbst, err := anypb.New(manager)
	if err != nil {
		panic(err)
	}

	return &listener.Listener{
		Name: s.listenerName,
		Address: &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Protocol: core.SocketAddress_TCP,
					Address:  s.listenerHost,
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: uint32(s.listenerPort),
					},
				},
			},
		},
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: pbst,
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
