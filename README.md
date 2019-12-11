# egress-operator
An operator to produce egress gateway pods and control access to them with network policies.

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

The status of each ExternalService will be updated with the `clusterIP` of the underlying Service object,
so your dns plugin need only watch those objects, and resolve `dnsName` to `clusterIP`

Some useful tips:

- `make install` - set up CRD in cluster
- `make run` - run locally against a remote cluster (create an ExternalService object to see stuff happen)

An example object:

```yaml
apiVersion: egress.monzo.com/v1
kind: ExternalService
metadata:
  name: google
spec:
  dnsName: google.com
  ports:
  - port: 443
    # optional, defaults to TCP
    protocol: TCP
  # optional, defaults to 3
  replicas: 5
```
