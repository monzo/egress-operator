package controllers

import (
    "github.com/google/go-cmp/cmp"
    "testing"

    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    egressv1 "github.com/monzo/egress-operator/api/v1"
)

func Test_envoyConfig(t *testing.T) {
    udp := corev1.ProtocolUDP
    tcp := corev1.ProtocolTCP
    var maxConnections uint32
    maxConnections = 4096
    type args struct {
        es       *egressv1.ExternalService
        maxConns *uint32
    }
    tests := []struct {
        name string
        args args
        want string
    }{
        {
            name: "udp and tcp",
            want: `admin:
  accessLog:
  - name: envoy.stdout_access_log
    typedConfig:
      '@type': type.googleapis.com/envoy.extensions.access_loggers.stream.v3.StdoutAccessLog
      logFormat:
        contentType: application/json; charset=UTF-8
        omitEmptyValues: true
        textFormatSource:
          inlineString: '[%START_TIME%] %BYTES_RECEIVED% %BYTES_SENT% %DURATION% "%DOWNSTREAM_REMOTE_ADDRESS%"
            "%UPSTREAM_HOST%" "%UPSTREAM_CLUSTER%"'
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
    upstreamConnectionOptions:
      tcpKeepalive:
        keepaliveInterval: 5
        keepaliveProbes: 3
        keepaliveTime: 30
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
    upstreamConnectionOptions:
      tcpKeepalive:
        keepaliveInterval: 5
        keepaliveProbes: 3
        keepaliveTime: 30
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
          accessLog:
          - name: envoy.stdout_access_log
            typedConfig:
              '@type': type.googleapis.com/envoy.extensions.access_loggers.stream.v3.StdoutAccessLog
              logFormat:
                contentType: application/json; charset=UTF-8
                omitEmptyValues: true
                textFormatSource:
                  inlineString: '[%START_TIME%] %BYTES_RECEIVED% %BYTES_SENT% %DURATION%
                    "%DOWNSTREAM_REMOTE_ADDRESS%" "%UPSTREAM_HOST%" "%UPSTREAM_CLUSTER%"'
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
          - name: envoy.stdout_access_log
            typedConfig:
              '@type': type.googleapis.com/envoy.extensions.access_loggers.stream.v3.StdoutAccessLog
              logFormat:
                contentType: application/json; charset=UTF-8
                omitEmptyValues: true
                textFormatSource:
                  inlineString: '[%START_TIME%] %BYTES_RECEIVED% %BYTES_SENT% %DURATION%
                    "%DOWNSTREAM_REMOTE_ADDRESS%" "%UPSTREAM_HOST%" "%UPSTREAM_CLUSTER%"'
          cluster: foo_TCP_101
          statPrefix: tcp_proxy
    name: foo_TCP_101
`,
            args: args{
                es: &egressv1.ExternalService{
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
                maxConns: nil,
            },
        },
        {
            name: "udp and tcp with max connections",
            want: `admin:
  accessLog:
  - name: envoy.stdout_access_log
    typedConfig:
      '@type': type.googleapis.com/envoy.extensions.access_loggers.stream.v3.StdoutAccessLog
      logFormat:
        contentType: application/json; charset=UTF-8
        jsonFormat:
          authority: '%REQ(:AUTHORITY)%'
          bytes_received: '%BYTES_RECEIVED%'
          bytes_sent: '%BYTES_SENT%'
          connection_termination_details: '%CONNECTION_TERMINATION_DETAILS%'
          downstream_local_address: '%DOWNSTREAM_LOCAL_ADDRESS%'
          downstream_remote_address: '%DOWNSTREAM_REMOTE_ADDRESS%'
          duration: '%DURATION%'
          method: '%REQ(:METHOD)%'
          path: '%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%'
          protocol: '%PROTOCOL%'
          requested_server_name: '%REQUESTED_SERVER_NAME%'
          response_code: '%RESPONSE_CODE%'
          response_code_details: '%RESPONSE_CODE_DETAILS%'
          response_flags: '%RESPONSE_FLAGS%'
          start_time: '%START_TIME%'
          upstream_cluster: '%UPSTREAM_CLUSTER%'
          upstream_host: '%UPSTREAM_HOST%'
          upstream_local_address: '%UPSTREAM_LOCAL_ADDRESS%'
          upstream_service_time: '%RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)%'
          upstream_transport_failure_reason: '%UPSTREAM_TRANSPORT_FAILURE_REASON%'
          user_agent: '%REQ(USER-AGENT)%'
        omitEmptyValues: true
  address:
    socketAddress:
      address: 0.0.0.0
      portValue: 11000
node:
  cluster: foo
staticResources:
  clusters:
  - circuitBreakers:
      thresholds:
      - maxConnections: 4096
    connectTimeout: 1s
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
    upstreamConnectionOptions:
      tcpKeepalive:
        keepaliveInterval: 5
        keepaliveProbes: 3
        keepaliveTime: 30
  - circuitBreakers:
      thresholds:
      - maxConnections: 4096
    connectTimeout: 1s
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
    upstreamConnectionOptions:
      tcpKeepalive:
        keepaliveInterval: 5
        keepaliveProbes: 3
        keepaliveTime: 30
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
          accessLog:
          - name: envoy.stdout_access_log
            typedConfig:
              '@type': type.googleapis.com/envoy.extensions.access_loggers.stream.v3.StdoutAccessLog
              logFormat:
                contentType: application/json; charset=UTF-8
                jsonFormat:
                  authority: '%REQ(:AUTHORITY)%'
                  bytes_received: '%BYTES_RECEIVED%'
                  bytes_sent: '%BYTES_SENT%'
                  connection_termination_details: '%CONNECTION_TERMINATION_DETAILS%'
                  downstream_local_address: '%DOWNSTREAM_LOCAL_ADDRESS%'
                  downstream_remote_address: '%DOWNSTREAM_REMOTE_ADDRESS%'
                  duration: '%DURATION%'
                  method: '%REQ(:METHOD)%'
                  path: '%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%'
                  protocol: '%PROTOCOL%'
                  requested_server_name: '%REQUESTED_SERVER_NAME%'
                  response_code: '%RESPONSE_CODE%'
                  response_code_details: '%RESPONSE_CODE_DETAILS%'
                  response_flags: '%RESPONSE_FLAGS%'
                  start_time: '%START_TIME%'
                  upstream_cluster: '%UPSTREAM_CLUSTER%'
                  upstream_host: '%UPSTREAM_HOST%'
                  upstream_local_address: '%UPSTREAM_LOCAL_ADDRESS%'
                  upstream_service_time: '%RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)%'
                  upstream_transport_failure_reason: '%UPSTREAM_TRANSPORT_FAILURE_REASON%'
                  user_agent: '%REQ(USER-AGENT)%'
                omitEmptyValues: true
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
          - name: envoy.stdout_access_log
            typedConfig:
              '@type': type.googleapis.com/envoy.extensions.access_loggers.stream.v3.StdoutAccessLog
              logFormat:
                contentType: application/json; charset=UTF-8
                jsonFormat:
                  authority: '%REQ(:AUTHORITY)%'
                  bytes_received: '%BYTES_RECEIVED%'
                  bytes_sent: '%BYTES_SENT%'
                  connection_termination_details: '%CONNECTION_TERMINATION_DETAILS%'
                  downstream_local_address: '%DOWNSTREAM_LOCAL_ADDRESS%'
                  downstream_remote_address: '%DOWNSTREAM_REMOTE_ADDRESS%'
                  duration: '%DURATION%'
                  method: '%REQ(:METHOD)%'
                  path: '%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%'
                  protocol: '%PROTOCOL%'
                  requested_server_name: '%REQUESTED_SERVER_NAME%'
                  response_code: '%RESPONSE_CODE%'
                  response_code_details: '%RESPONSE_CODE_DETAILS%'
                  response_flags: '%RESPONSE_FLAGS%'
                  start_time: '%START_TIME%'
                  upstream_cluster: '%UPSTREAM_CLUSTER%'
                  upstream_host: '%UPSTREAM_HOST%'
                  upstream_local_address: '%UPSTREAM_LOCAL_ADDRESS%'
                  upstream_service_time: '%RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)%'
                  upstream_transport_failure_reason: '%UPSTREAM_TRANSPORT_FAILURE_REASON%'
                  user_agent: '%REQ(USER-AGENT)%'
                omitEmptyValues: true
          cluster: foo_TCP_101
          statPrefix: tcp_proxy
    name: foo_TCP_101
`,
            args: args{
                es: &egressv1.ExternalService{
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
                        JsonAdminAccessLogs:   true,
                        JsonClusterAccessLogs: true,
                    },
                },
                maxConns: &maxConnections,
            },
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            tt.args.es.Spec.EnvoyClusterMaxConnections = tt.args.maxConns
            got, err := envoyConfig(tt.args.es)
            if err != nil {
                t.Error(err)
            }
            if got != tt.want {
                t.Errorf("envoyConfig() = %v, want %v", got, tt.want)
                t.Error(cmp.Diff(got, tt.want))
            }
        })
    }
}
