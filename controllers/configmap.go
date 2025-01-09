package controllers

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"hash/fnv"
	"strconv"

	accesslogfilterv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	bootstrap "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	streamv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/stream/v3"
	aggregatev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/aggregate/v3"
	tcpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	udpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/udp/udp_proxy/v3"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	egressv1 "github.com/monzo/egress-operator/api/v1"
)

// +kubebuilder:rbac:namespace=egress-operator-system,groups=core,resources=configmaps,verbs=get;list;watch;create;patch

var (
	jsonLogFields = &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"authority":                         {Kind: &structpb.Value_StringValue{StringValue: "%REQ(:AUTHORITY)%"}},
			"bytes_received":                    {Kind: &structpb.Value_StringValue{StringValue: "%BYTES_RECEIVED%"}},
			"bytes_sent":                        {Kind: &structpb.Value_StringValue{StringValue: "%BYTES_SENT%"}},
			"connection_termination_details":    {Kind: &structpb.Value_StringValue{StringValue: "%CONNECTION_TERMINATION_DETAILS%"}},
			"downstream_local_address":          {Kind: &structpb.Value_StringValue{StringValue: "%DOWNSTREAM_LOCAL_ADDRESS%"}},
			"downstream_remote_address":         {Kind: &structpb.Value_StringValue{StringValue: "%DOWNSTREAM_REMOTE_ADDRESS%"}},
			"duration":                          {Kind: &structpb.Value_StringValue{StringValue: "%DURATION%"}},
			"method":                            {Kind: &structpb.Value_StringValue{StringValue: "%REQ(:METHOD)%"}},
			"path":                              {Kind: &structpb.Value_StringValue{StringValue: "%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%"}},
			"protocol":                          {Kind: &structpb.Value_StringValue{StringValue: "%PROTOCOL%"}},
			"requested_server_name":             {Kind: &structpb.Value_StringValue{StringValue: "%REQUESTED_SERVER_NAME%"}},
			"response_code":                     {Kind: &structpb.Value_StringValue{StringValue: "%RESPONSE_CODE%"}},
			"response_code_details":             {Kind: &structpb.Value_StringValue{StringValue: "%RESPONSE_CODE_DETAILS%"}},
			"response_flags":                    {Kind: &structpb.Value_StringValue{StringValue: "%RESPONSE_FLAGS%"}},
			"start_time":                        {Kind: &structpb.Value_StringValue{StringValue: "%START_TIME%"}},
			"upstream_cluster":                  {Kind: &structpb.Value_StringValue{StringValue: "%UPSTREAM_CLUSTER%"}},
			"upstream_host":                     {Kind: &structpb.Value_StringValue{StringValue: "%UPSTREAM_HOST%"}},
			"upstream_local_address":            {Kind: &structpb.Value_StringValue{StringValue: "%UPSTREAM_LOCAL_ADDRESS%"}},
			"upstream_service_time":             {Kind: &structpb.Value_StringValue{StringValue: "%RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)%"}},
			"upstream_transport_failure_reason": {Kind: &structpb.Value_StringValue{StringValue: "%UPSTREAM_TRANSPORT_FAILURE_REASON%"}},
			"user_agent":                        {Kind: &structpb.Value_StringValue{StringValue: "%REQ(USER-AGENT)%"}},
		},
	}
)

const (
	ClusterTextLogFormat = "[%START_TIME%] %BYTES_RECEIVED% %BYTES_SENT% %DURATION% \"%DOWNSTREAM_REMOTE_ADDRESS%\" \"%UPSTREAM_HOST%\" \"%UPSTREAM_CLUSTER%\""
	AdminTextLogFormat   = "[%START_TIME%] \"%REQ(:METHOD)% %REQ(X-ENVOY-ORIGINAL-PATH?:PATH)% %PROTOCOL%\" %RESPONSE_CODE% %RESPONSE_FLAGS% %BYTES_RECEIVED% %BYTES_SENT% %DURATION% %RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)% \"%REQ(X-FORWARDED-FOR)%\" \"%REQ(USER-AGENT)%\" \"%REQ(X-REQUEST-ID)%\" \"%REQ(:AUTHORITY)%\" \"%UPSTREAM_HOST%\"\n"
)

func (r *ExternalServiceReconciler) reconcileConfigMap(ctx context.Context, req ctrl.Request, es *egressv1.ExternalService, desired *corev1.ConfigMap) error {
	if err := ctrl.SetControllerReference(es, desired, r.Scheme); err != nil {
		return err
	}
	c := &corev1.ConfigMap{}
	if err := r.Get(ctx, req.NamespacedName, c); err != nil {
		if apierrs.IsNotFound(err) {
			return r.Client.Create(ctx, desired)
		}
		return err
	}

	patched := c.DeepCopy()
	mergeMap(desired.Labels, patched.Labels)
	mergeMap(desired.Annotations, patched.Annotations)
	patched.Data = desired.Data

	return ignoreNotFound(r.patchIfNecessary(ctx, patched, client.MergeFrom(c)))
}

func protocolToEnvoy(p *corev1.Protocol) envoycorev3.SocketAddress_Protocol {
	if p == nil {
		return envoycorev3.SocketAddress_TCP
	}

	switch *p {
	case corev1.ProtocolUDP:
		return envoycorev3.SocketAddress_UDP
	default:
		return envoycorev3.SocketAddress_TCP
	}
}

func adminPort(es *egressv1.ExternalService) int32 {
	disallowed := map[int32]struct{}{}

	for _, p := range es.Spec.Ports {
		if p.Protocol == nil || *p.Protocol == corev1.ProtocolTCP {
			disallowed[p.Port] = struct{}{}
		}
	}

	for i := int32(11000); i < 32768; i++ {
		if _, ok := disallowed[i]; !ok {
			return i
		}
	}

	panic("couldn't find a port for admin listener")
}

func getStdoutTextAccessLog(format string) ([]*accesslogfilterv3.AccessLog, error) {
	ac, err := anypb.New(&streamv3.StdoutAccessLog{
		AccessLogFormat: &streamv3.StdoutAccessLog_LogFormat{
			LogFormat: &envoycorev3.SubstitutionFormatString{
				Format: &envoycorev3.SubstitutionFormatString_TextFormatSource{
					TextFormatSource: &envoycorev3.DataSource{
						Specifier: &envoycorev3.DataSource_InlineString{
							InlineString: format,
						},
					},
				},
				OmitEmptyValues: true,
				ContentType:     "application/json; charset=UTF-8",
			},
		},
	})
	if err != nil {
		return nil, err
	}
	accessLog := []*accesslogfilterv3.AccessLog{{
		Name:       "envoy.stdout_access_log",
		ConfigType: &accesslogfilterv3.AccessLog_TypedConfig{TypedConfig: ac},
	}}

	return accessLog, nil
}

func getJsonAccessLog() ([]*accesslogfilterv3.AccessLog, error) {
	ac, err := anypb.New(&streamv3.StdoutAccessLog{
		AccessLogFormat: &streamv3.StdoutAccessLog_LogFormat{
			LogFormat: &envoycorev3.SubstitutionFormatString{
				Format: &envoycorev3.SubstitutionFormatString_JsonFormat{
					JsonFormat: jsonLogFields,
				},
				OmitEmptyValues: true,
				ContentType:     "application/json; charset=UTF-8",
			},
		},
	})
	if err != nil {
		return nil, err
	}
	accessLog := []*accesslogfilterv3.AccessLog{{
		Name:       "envoy.stdout_access_log",
		ConfigType: &accesslogfilterv3.AccessLog_TypedConfig{TypedConfig: ac},
	}}

	return accessLog, nil
}

func getClusterAccessLog(es *egressv1.ExternalService) ([]*accesslogfilterv3.AccessLog, error) {
	if es.Spec.JsonClusterAccessLogs {
		return getJsonAccessLog()
	}
	return getStdoutTextAccessLog(ClusterTextLogFormat)
}

func getAdminAccessLog(es *egressv1.ExternalService) ([]*accesslogfilterv3.AccessLog, error) {
	if es.Spec.JsonAdminAccessLogs {
		return getJsonAccessLog()
	}
	return getStdoutTextAccessLog(AdminTextLogFormat)
}

func envoyConfig(es *egressv1.ExternalService) (string, error) {
	adminAccessLog, err := getAdminAccessLog(es)
	if err != nil {
		return "", err
	}

	clusterAccessLog, err := getClusterAccessLog(es)
	if err != nil {
		return "", err
	}

	config := bootstrap.Bootstrap{
		Node: &envoycorev3.Node{
			Cluster: es.Name,
		},
		Admin: &bootstrap.Admin{
			Address: &envoycorev3.Address{Address: &envoycorev3.Address_SocketAddress{
				SocketAddress: &envoycorev3.SocketAddress{
					Address:  "0.0.0.0",
					Protocol: envoycorev3.SocketAddress_TCP,
					PortSpecifier: &envoycorev3.SocketAddress_PortValue{
						PortValue: uint32(adminPort(es)),
					},
				},
			}},
			AccessLog: adminAccessLog,
		},
		StaticResources: &bootstrap.Bootstrap_StaticResources{},
	}

	for _, port := range es.Spec.Ports {
		var dnsRefreshRate *duration.Duration
		if es.Spec.EnvoyDnsRefreshRateS != 0 {
			dnsRefreshRate = &durationpb.Duration{Seconds: es.Spec.EnvoyDnsRefreshRateS}
		}
		var clusters []*envoyv3.Cluster
		protocol := protocolToEnvoy(port.Protocol)
		name := fmt.Sprintf("%s_%s_%s", es.Name, envoycorev3.SocketAddress_Protocol_name[int32(protocol)], strconv.Itoa(int(port.Port)))
		clusterNameForListener := name
		clusters = append(clusters, &envoyv3.Cluster{
			Name: name,
			ClusterDiscoveryType: &envoyv3.Cluster_Type{
				Type: envoyv3.Cluster_LOGICAL_DNS,
			},
			ConnectTimeout: &duration.Duration{
				Seconds: 1,
			},
			LbPolicy:        envoyv3.Cluster_ROUND_ROBIN,
			DnsLookupFamily: envoyv3.Cluster_V4_ONLY,
			UpstreamConnectionOptions: &envoyv3.UpstreamConnectionOptions{
				TcpKeepalive: &envoycorev3.TcpKeepalive{
					KeepaliveProbes:   &wrapperspb.UInt32Value{Value: 3},
					KeepaliveTime:     &wrapperspb.UInt32Value{Value: 30},
					KeepaliveInterval: &wrapperspb.UInt32Value{Value: 5},
				},
			},

			DnsRefreshRate: dnsRefreshRate,
			RespectDnsTtl:  es.Spec.EnvoyRespectDnsTTL,
			LoadAssignment: &envoyendpoint.ClusterLoadAssignment{
				ClusterName: name,
				Endpoints: []*envoyendpoint.LocalityLbEndpoints{
					{
						LbEndpoints: []*envoyendpoint.LbEndpoint{
							{
								HostIdentifier: &envoyendpoint.LbEndpoint_Endpoint{
									Endpoint: &envoyendpoint.Endpoint{
										Address: &envoycorev3.Address{
											Address: &envoycorev3.Address_SocketAddress{
												SocketAddress: &envoycorev3.SocketAddress{
													Address:  es.Spec.DnsName,
													Protocol: protocol,
													PortSpecifier: &envoycorev3.SocketAddress_PortValue{
														PortValue: uint32(port.Port),
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		})

		// If we want to override the normal DNS lookup and set the IP address
		// overwrite the Hosts field with one for each IP
		if len(es.Spec.IpOverride) > 0 {
			overrideCluster := generateOverrideCluster(name, es.Spec, port, protocol)
			clusters = append([]*envoyv3.Cluster{overrideCluster}, clusters...)
			aggregateCluster, err := generateAggregateCluster(fmt.Sprintf("%v-aggregate", name), overrideCluster.Name, name)
			if err != nil {
				return "", err
			}
			// Prepend to list
			clusters = append([]*envoyv3.Cluster{aggregateCluster}, clusters...)
			clusterNameForListener = aggregateCluster.Name
		}

		if es.Spec.EnvoyClusterMaxConnections != nil {
			cbs := &envoyv3.CircuitBreakers{
				Thresholds: []*envoyv3.CircuitBreakers_Thresholds{
					{
						MaxConnections: &wrappers.UInt32Value{Value: *es.Spec.EnvoyClusterMaxConnections},
					},
				},
				PerHostThresholds: nil,
			}
			for _, cluster := range clusters {
				cluster.CircuitBreakers = cbs
			}
		}

		var listener *envoylistener.Listener
		switch protocol {
		case envoycorev3.SocketAddress_TCP:
			filterConfig, err := anypb.New(&tcpproxyv3.TcpProxy{
				AccessLog:  clusterAccessLog,
				StatPrefix: "tcp_proxy",
				ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{
					Cluster: clusterNameForListener,
				},
			})
			if err != nil {
				return "", err
			}

			listener = &envoylistener.Listener{
				Name: name,
				Address: &envoycorev3.Address{
					Address: &envoycorev3.Address_SocketAddress{
						SocketAddress: &envoycorev3.SocketAddress{
							Protocol: protocol,
							Address:  "0.0.0.0",
							PortSpecifier: &envoycorev3.SocketAddress_PortValue{
								PortValue: uint32(port.Port),
							}}}},
				FilterChains: []*envoylistener.FilterChain{{
					Filters: []*envoylistener.Filter{{
						Name: "envoy.tcp_proxy",
						ConfigType: &envoylistener.Filter_TypedConfig{
							TypedConfig: filterConfig,
						}}}}},
			}
		case envoycorev3.SocketAddress_UDP:
			filterConfig, err := anypb.New(&udpproxyv3.UdpProxyConfig{
				AccessLog:  clusterAccessLog,
				StatPrefix: "udp_proxy",
				RouteSpecifier: &udpproxyv3.UdpProxyConfig_Cluster{
					Cluster: name,
				},
			})
			if err != nil {
				return "", err
			}

			listener = &envoylistener.Listener{
				Name: name,
				Address: &envoycorev3.Address{
					Address: &envoycorev3.Address_SocketAddress{
						SocketAddress: &envoycorev3.SocketAddress{
							Protocol: protocol,
							Address:  "0.0.0.0",
							PortSpecifier: &envoycorev3.SocketAddress_PortValue{
								PortValue: uint32(port.Port),
							}}}},
				FilterChains: []*envoylistener.FilterChain{{
					Filters: []*envoylistener.Filter{{
						Name: "envoy.filters.udp_listener.udp_proxy",
						ConfigType: &envoylistener.Filter_TypedConfig{
							TypedConfig: filterConfig,
						}}}}},
			}
		}

		config.StaticResources.Clusters = append(config.StaticResources.Clusters, clusters...)
		config.StaticResources.Listeners = append(config.StaticResources.Listeners, listener)
	}

	m := &jsonpb.Marshaler{}

	json, err := m.MarshalToString(&config)
	if err != nil {
		return "", err
	}

	y, err := yaml.JSONToYAML([]byte(json))
	if err != nil {
		return "", err
	}

	return string(y), nil
}

func configmap(es *egressv1.ExternalService) (*corev1.ConfigMap, string, error) {
	ec, err := envoyConfig(es)
	if err != nil {
		return nil, "", err
	}
	h := fnv.New32a()
	h.Write([]byte(ec))
	sum := h.Sum32()

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        es.Name,
			Namespace:   namespace,
			Labels:      labels(es),
			Annotations: annotations(es),
		},
		Data: map[string]string{"envoy.yaml": ec},
	}, fmt.Sprintf("%x", sum), nil
}

func generateOverrideCluster(name string, spec egressv1.ExternalServiceSpec, port egressv1.ExternalServicePort, protocol envoycorev3.SocketAddress_Protocol) *envoyv3.Cluster {
	overrideClusterName := fmt.Sprintf("%v-override", name)
	var dnsRefreshRate *duration.Duration
	if spec.EnvoyDnsRefreshRateS != 0 {
		dnsRefreshRate = &durationpb.Duration{Seconds: spec.EnvoyDnsRefreshRateS}
	}
	var endpoints []*envoyendpoint.LocalityLbEndpoints

	for _, ip := range spec.IpOverride {
		endpoints = append(endpoints, &envoyendpoint.LocalityLbEndpoints{
			LbEndpoints: []*envoyendpoint.LbEndpoint{
				{
					HostIdentifier: &envoyendpoint.LbEndpoint_Endpoint{
						Endpoint: &envoyendpoint.Endpoint{
							HealthCheckConfig: &envoyendpoint.Endpoint_HealthCheckConfig{
								PortValue: uint32(port.Port),
							},
							Address: &envoycorev3.Address{
								Address: &envoycorev3.Address_SocketAddress{
									SocketAddress: &envoycorev3.SocketAddress{
										Address:  ip,
										Protocol: protocol,
										PortSpecifier: &envoycorev3.SocketAddress_PortValue{
											PortValue: uint32(port.Port),
										},
									},
								},
							},
						},
					},
				},
			},
		})
	}
	return &envoyv3.Cluster{
		Name: overrideClusterName,
		ClusterDiscoveryType: &envoyv3.Cluster_Type{
			Type: envoyv3.Cluster_STATIC,
		},
		ConnectTimeout: &duration.Duration{
			Seconds: 1,
		},
		CloseConnectionsOnHostHealthFailure: true,
		HealthChecks: []*envoycorev3.HealthCheck{
			{
				Timeout: &duration.Duration{
					Seconds: 1,
				},
				Interval: &duration.Duration{
					Seconds: 10,
				},
				ReuseConnection:    wrapperspb.Bool(false),
				UnhealthyThreshold: wrapperspb.UInt32(2),
				HealthyThreshold:   wrapperspb.UInt32(3),
				EventLogPath:       "/dev/stdout",
				HealthChecker:      &envoycorev3.HealthCheck_TcpHealthCheck_{},
			},
		},
		LbPolicy:        envoyv3.Cluster_ROUND_ROBIN,
		DnsLookupFamily: envoyv3.Cluster_V4_ONLY,
		LoadAssignment: &envoyendpoint.ClusterLoadAssignment{
			ClusterName: overrideClusterName,
			Endpoints:   endpoints,
		},

		DnsRefreshRate: dnsRefreshRate,
		RespectDnsTtl:  spec.EnvoyRespectDnsTTL,
	}
}

func generateAggregateCluster(name string, clusters ...string) (*envoyv3.Cluster, error) {
	aggregateClusterConfig, err := ptypes.MarshalAny(&aggregatev3.ClusterConfig{
		Clusters: clusters,
	})
	if err != nil {
		return nil, err
	}
	cluster := &envoyv3.Cluster{
		Name: name,
		ConnectTimeout: &duration.Duration{
			Seconds: 1,
		},
		LbPolicy: envoyv3.Cluster_CLUSTER_PROVIDED,
		ClusterDiscoveryType: &envoyv3.Cluster_ClusterType{
			ClusterType: &envoyv3.Cluster_CustomClusterType{
				Name:        "envoy.clusters.aggregate",
				TypedConfig: aggregateClusterConfig,
			},
		},
	}
	return cluster, nil
}
