package controllers

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	egressv1 "github.com/monzo/egress-operator/api/v1"
)

func Test_envoyConfig(t *testing.T) {
	udp := corev1.ProtocolUDP
	tcp := corev1.ProtocolTCP
	type args struct {
		es *egressv1.ExternalService
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "udp and tcp",
			want: `admin:
  accessLogPath: /dev/stdout
  address:
    socketAddress:
      address: 0.0.0.0
      portValue: 11000
node:
  cluster: foo
staticResources:
  clusters:
  - connectTimeout: 1s
    dnsLookupFamily: V4_ONLY
    loadAssignment:
      clusterName: foo_UDP_100
      endpoints:
      - lbEndpoints:
        - endpoint:
            address:
              socketAddress:
                address: google.com
                portValue: 100
                protocol: UDP
    name: foo_UDP_100
    type: LOGICAL_DNS
  - connectTimeout: 1s
    dnsLookupFamily: V4_ONLY
    loadAssignment:
      clusterName: foo_TCP_101
      endpoints:
      - lbEndpoints:
        - endpoint:
            address:
              socketAddress:
                address: google.com
                portValue: 101
    name: foo_TCP_101
    type: LOGICAL_DNS
  listeners:
  - address:
      socketAddress:
        address: 0.0.0.0
        portValue: 100
        protocol: UDP
    filterChains:
    - filters:
      - name: envoy.filters.udp_listener.udp_proxy
        typedConfig:
          '@type': type.googleapis.com/envoy.extensions.filters.udp.udp_proxy.v3.UdpProxyConfig
          cluster: foo_UDP_100
          statPrefix: udp_proxy
    name: foo_UDP_100
  - address:
      socketAddress:
        address: 0.0.0.0
        portValue: 101
    filterChains:
    - filters:
      - name: envoy.tcp_proxy
        typedConfig:
          '@type': type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
          accessLog:
          - name: envoy.file_access_log
            typedConfig:
              '@type': type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog
              format: |
                [%START_TIME%] %BYTES_RECEIVED% %BYTES_SENT% %DURATION% "%DOWNSTREAM_REMOTE_ADDRESS%" "%UPSTREAM_HOST%"
              path: /dev/stdout
          cluster: foo_TCP_101
          statPrefix: tcp_proxy
    name: foo_TCP_101
`,
			args: args{
				&egressv1.ExternalService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "foo",
					},
					Spec: egressv1.ExternalServiceSpec{
						DnsName: "google.com",
						Ports: []egressv1.ExternalServicePort{
							{
								Port:     100,
								Protocol: &udp,
							},
							{
								Port:     101,
								Protocol: &tcp,
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := envoyConfig(tt.args.es); got != tt.want {
				t.Errorf("envoyConfig() = %v, want %v", got, tt.want)
				t.Error(cmp.Diff(got, tt.want))
			}
		})
	}
}
