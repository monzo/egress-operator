# egress-operator
An operator to produce egress gateway pods and control access to them with network policies, and a coredns plugin to route egress traffic to these pods.

The idea is that instead of authorizing egress traffic with protocol inspection, 
you instead create a internal clusterIP for every external service you use, lock
it down to only a few pods via a network policy, and then set up your dns server 
to resolve the external service to that clusterIP.

Built with kubebuilder: https://book.kubebuilder.io/

The operator accepts ExternalService objects, which aren't namespaced, which define a dns name and ports for an external service.
In the `egress-operator-system` namespace, it creates:
- An envoy configmap for a TCP/UDP proxy to that service
- A deployment for some envoy pods with that config
- A service for that deployment
- A network policy only allowing pods in other namespaces with the label `egress.monzo.com/allowed-gateway: yourservice`

Some useful tips:

- `make install` - set up CRD in cluster
- `make run` - run locally against a remote cluster (create an ExternalService object to see stuff happen)
- `cd coredns-plugin && make docker` - produce a coredns image that contains the plugin

An example object:

```yaml
apiVersion: egress.monzo.com/v1
kind: ExternalService
metadata:
  name: google
spec:
  dnsName: google.com
  # optional, defaults to false, instructs dns server to rewrite queries for dnsName
  hijackDns: true
  ports:
  - port: 443
    # optional, defaults to TCP
    protocol: TCP
  # optional, defaults to 3
  replicas: 5
```

Example CoreDNS config:

```Caddy
.:53 {
    egressoperator egress-operator-system cluster.local
    kubernetes cluster.local
    forward . /etc/resolv.conf
}
```
