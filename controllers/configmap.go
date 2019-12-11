package controllers

import (
	"context"
	"fmt"
	"strconv"

	envoyv2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycorev2 "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	bootstrapv2 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v2"
	tcpproxyv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	udpproxyv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/udp/udp_proxy/v2alpha"
	"github.com/envoyproxy/go-control-plane/pkg/conversion"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/ptypes/duration"
	egressv1 "github.com/monzo/egress-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;create;patch

func (r *ExternalServiceReconciler) reconcileConfigMap(ctx context.Context, req ctrl.Request, es *egressv1.ExternalService) error {
	desired, err := configmap(es)
	if err != nil {
		return err
	}
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
	patched.Data = desired.Data

	return r.Client.Patch(ctx, patched, client.MergeFrom(c))
}

func protocolToEnvoy(p *corev1.Protocol) envoycorev2.SocketAddress_Protocol {
	if p == nil {
		return envoycorev2.SocketAddress_TCP
	}

	switch *p {
	case corev1.ProtocolUDP:
		return envoycorev2.SocketAddress_UDP
	default:
		return envoycorev2.SocketAddress_TCP
	}
}

func envoyConfig(es *egressv1.ExternalService) (string, error) {
	config := bootstrapv2.Bootstrap{
		Node: &envoycorev2.Node{
			Cluster: es.Name,
		},
		StaticResources: &bootstrapv2.Bootstrap_StaticResources{},
	}

	for _, port := range es.Spec.Ports {
		protocol := protocolToEnvoy(port.Protocol)
		name := fmt.Sprintf("%s_%s_%s", es.Name, envoycorev2.SocketAddress_Protocol_name[int32(protocol)], strconv.Itoa(int(port.Port)))
		cluster := &envoyv2.Cluster{
			Name: name,
			ClusterDiscoveryType: &envoyv2.Cluster_Type{
				Type: envoyv2.Cluster_LOGICAL_DNS,
			},
			ConnectTimeout: &duration.Duration{
				Seconds: 1,
			},
			LbPolicy:        envoyv2.Cluster_ROUND_ROBIN,
			DnsLookupFamily: envoyv2.Cluster_V4_ONLY,
			Hosts: []*envoycorev2.Address{
				{
					Address: &envoycorev2.Address_SocketAddress{
						SocketAddress: &envoycorev2.SocketAddress{
							Address:  es.Spec.DnsName,
							Protocol: protocol,
							PortSpecifier: &envoycorev2.SocketAddress_PortValue{
								PortValue: uint32(port.Port),
							},
						},
					},
				},
			},
		}

		var listener *envoyv2.Listener
		switch protocol {
		case envoycorev2.SocketAddress_TCP:
			filterConfig, err := conversion.MessageToStruct(&tcpproxyv2.TcpProxy{
				StatPrefix: "tcp_proxy",
				ClusterSpecifier: &tcpproxyv2.TcpProxy_Cluster{
					Cluster: name,
				},
			})
			if err != nil {
				return "", err
			}

			listener = &envoyv2.Listener{
				Name: name,
				Address: &envoycorev2.Address{
					Address: &envoycorev2.Address_SocketAddress{
						SocketAddress: &envoycorev2.SocketAddress{
							Protocol: protocol,
							Address:  "0.0.0.0",
							PortSpecifier: &envoycorev2.SocketAddress_PortValue{
								PortValue: uint32(port.Port),
							}}}},
				FilterChains: []*envoylistener.FilterChain{{
					Filters: []*envoylistener.Filter{{
						Name: "envoy.tcp_proxy",
						ConfigType: &envoylistener.Filter_Config{
							Config: filterConfig,
						}}}}},
			}
		case envoycorev2.SocketAddress_UDP:
			filterConfig, err := conversion.MessageToStruct(&udpproxyv2.UdpProxyConfig{
				StatPrefix: "udp_proxy",
				RouteSpecifier: &udpproxyv2.UdpProxyConfig_Cluster{
					Cluster: name,
				},
			})
			if err != nil {
				return "", err
			}

			listener = &envoyv2.Listener{
				Name: name,
				Address: &envoycorev2.Address{
					Address: &envoycorev2.Address_SocketAddress{
						SocketAddress: &envoycorev2.SocketAddress{
							Protocol: protocol,
							Address:  "0.0.0.0",
							PortSpecifier: &envoycorev2.SocketAddress_PortValue{
								PortValue: uint32(port.Port),
							}}}},
				FilterChains: []*envoylistener.FilterChain{{
					Filters: []*envoylistener.Filter{{
						Name: "envoy.filters.udp_listener.udp_proxy",
						ConfigType: &envoylistener.Filter_Config{
							Config: filterConfig,
						}}}}},
			}
		}

		config.StaticResources.Clusters = append(config.StaticResources.Clusters, cluster)
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

func configmap(es *egressv1.ExternalService) (*corev1.ConfigMap, error) {
	ec, err := envoyConfig(es)
	if err != nil {
		return nil, err
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      es.Name,
			Namespace: namespace,
			Labels:    labels(es),
		},
		Data: map[string]string{"envoy.yaml": ec},
	}, nil
}
