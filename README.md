# egress-operator
An operator to produce egress gateway pods and control access to them with network policies, and a coredns plugin to route egress traffic to these pods.

The idea is that instead of authorizing egress traffic with protocol inspection, 
you instead create a internal clusterIP for every external service you use, lock
it down to only a few pods via a network policy, and then set up your dns server 
to resolve the external service to that clusterIP.

Built with kubebuilder: https://book.kubebuilder.io/

The operator accepts ExternalService objects, which aren't namespaced, which define a dns name and ports for an external service.
In the `egress-operator-system` namespace, it creates:
- An envoy configmap for a TCP/UDP proxy to that service (UDP not working until the next envoy release that enables it)
- A deployment for some envoy pods with that config
- A horizontal pod autoscaler to keep the deployment correctly sized
- A service for that deployment
- A network policy only allowing pods in other namespaces with the label `egress.monzo.com/allowed-gateway: yourservice`

## Pre-requisites

1. You need to have a private container repository for hosting the egress-operator image, such as an AWS Elastic Container Repository (ECR) or a GCP Container Registry (GCR), which needs to be accessible from your cluster. This will be referred to as `yourrepo` in the instructions below.
2. Your local system must have a recent version of `golang` for building the code, which you can install by following instructions [here](https://golang.org/doc/install).
3. Your local system must have Kubebuilder for code generation, which you can install by following instructions [here](https://book.kubebuilder.io/quick-start.html).
4. Your local system must have Kustomize for building the Kubernetes manifests, which you can install by following instructions [here](https://kubernetes-sigs.github.io/kustomize/installation/).
5. Your cluster must be running CoreDNS instead of kube-dns, which may not be the case if you are using a managed Kubernetes service. [This article](https://medium.com/google-cloud/using-coredns-on-gke-3973598ab561) provides some help for GCP Kubernetes Engine, and guidance for AWS Elastic Kubernetes Service can be found [here](https://docs.aws.amazon.com/eks/latest/userguide/coredns.html). 

## Installing

### Testing locally against a remote cluster

```bash
make run
``` 
This creates an ExternalService object to see the controller-manager creating managed resources in the remote cluster.

### Setting up CoreDNS plugin

The CoreDNS plugin rewrites responses for external service hostnames managed by egress-operator.

Build a CoreDNS image which contains the plugin:
```bash
cd coredns-plugin
make docker-build docker-push IMG=yourrepo/egress-operator-coredns:latest
```

You'll need to swap out the image of your coredns kubedns Deployment for `yourrepo/egress-operator-coredns:latest`:
```bash
kubectl edit deploy coredns -n kube-system   # Your Deployment name may vary
```

And edit the coredns Corefile in ConfigMap to put in `egressoperator egress-operator-system cluster.local`:
```bash
kubectl edit configmap coredns-config -n kube-system   # Your ConfigMap name may vary
```

Example CoreDNS config:

```Caddy
.:53 {
    egressoperator egress-operator-system cluster.local
    kubernetes cluster.local
    forward . /etc/resolv.conf
}
```

### Set up the controller manager and its `CustomResourceDefinition` in the cluster

```
make controller-gen
make deploy IMG=yourrepo/egress-operator:v0.1
```

## Usage

Once the controller and dns server are running, create ExternalService objects which denote what dns name you want
to capture traffic for. Dns queries for this name will be rewritten to point to gateway pods.

By default, your client pods need a label `egress.monzo.com/allowed-gateway: nameofgateway` to be able to reach
the destination, but you can always write an additional NetworkPolicy selecting gateway pods and allowing all traffic,
for testing purposes.

An example ExternalService:

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
  minReplicas: 5
  # optional, defaults to 12
  maxReplicas: 10
  # optional, defaults to 50
  targetCPUUtilizationPercentage: 30
  # optional, if not provided then defaults to 100m, 50Mi, 2, 1Gi
  resources:
    requests:
      cpu: 1
      memory: 100Mi
    limits:
      cpu: 2
      memory: 200Mi
```

### Blocking non-gateway traffic

This operator won't block any traffic for you, it simply sets up some permitted routes for traffic through the egress
gateways. You'll need a default-deny policy to block traffic that doesn't go through gateways. To do that, you probably
need a policy like this in every namespace that you want to control:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-external-egress
  namespace: your-application-namespace
spec:
  podSelector: {}
  policyTypes:
  - Egress
  egress:
  - to:
    - ipBlock:
        # ensure your internal IP range is allowed here
        # traffic to external IPs will not be allowed from this namespace.
        # therefore, pods will have to use egress gateways
        cidr: 10.0.0.0/8 
```

If you already have a default deny egress policy, the above won't be needed. You'll instead want to explicitly allow 
egress from your pods to all gateway pods. The ingress policies on gateway pods will ensure that only correct traffic is
allowed.
