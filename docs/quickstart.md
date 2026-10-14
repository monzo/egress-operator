# Quickstart: try egress-operator locally with Kind

This guide runs egress-operator on a local [Kind](https://kind.sigs.k8s.io/) cluster and shows
which egress traffic is allowed and which is blocked. Everything is automated by
[`hack/kind/e2e.sh`](../hack/kind/e2e.sh) (the *Egress Policy Validation Lab*). Each step it runs is
explained below so you can repeat it on a real cluster.

## Prerequisites

* Docker
* [`kind`](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
* `kubectl`
* Go (the version in `go.mod`) and `make`
* Internet access (the lab talks to `example.com`)

## One command

```bash
make e2e-kind                 # create cluster, deploy, run all checks, delete cluster
KEEP_CLUSTER=1 make e2e-kind  # same, but keep the cluster to explore it
make e2e-kind-clean           # delete the cluster
```

Expected output ends with:

```
PASS dns-hijacked
PASS allowed-via-gateway
PASS wrong-label-denied
PASS unlabelled-denied
PASS direct-egress-denied
PASS control-direct-allowed
```

You can re-run a single check against a kept cluster with `hack/kind/e2e.sh check <name>`.

## What happens, step by step

### 1. A cluster that enforces NetworkPolicies

[`hack/kind/kind-config.yaml`](../hack/kind/kind-config.yaml) disables Kind's default CNI, and the
script installs Calico. Without a CNI that enforces NetworkPolicy, nothing here would be blocked.

### 2. Deploy the operator and its CRD

The script builds the operator image from this checkout, loads it into Kind, and applies
`config/default` through the [`hack/kind/overlay`](../hack/kind/overlay/kustomization.yaml) overlay,
which only changes the image name. On a real cluster you would run:

```bash
make docker-build docker-push IMG=yourrepo/egress-operator:v0.1
make install deploy IMG=yourrepo/egress-operator:v0.1
```

### 3. Swap CoreDNS for the build that includes the `egressoperator` plugin

The `coredns` Deployment in `kube-system` gets the image built from [`coredns-plugin/`](../coredns-plugin),
and the Corefile gets one extra line:

```Caddy
.:53 {
    egressoperator egress-operator-system cluster.local
    ...
}
```

### 4. Create an ExternalService

```yaml
apiVersion: egress.monzo.com/v1
kind: ExternalService
metadata:
  name: example
spec:
  dnsName: example.com
  hijackDns: true
  ports:
  - port: 443
    protocol: TCP
  minReplicas: 1
  maxReplicas: 1
```

The operator creates an Envoy Deployment, a Service and a NetworkPolicy, all named `example`, in
`egress-operator-system`:

```bash
kubectl -n egress-operator-system get deploy,svc,networkpolicy example
```

Gateway pods resolve `example.com` with the node's resolver. The lab adds
[`gateway-dns.yaml`](../hack/kind/manifests/gateway-dns.yaml) so they're allowed to reach it.

### 5. Lock down a namespace and label the allowed client

[`hack/kind/manifests/lab.yaml`](../hack/kind/manifests/lab.yaml) creates:

* a `lab` namespace whose pods may only send traffic inside the cluster (`default-deny-external-egress`)
* `allowed`: labelled `egress.monzo.com/allowed-example: "true"`
* `wrong-label`: labelled `egress.monzo.com/allowed-gateway: example` (the label the README used to give, see [#21](https://github.com/monzo/egress-operator/issues/21))
* `denied`: no label

### 6. Check the result

| Check | What it proves |
|---|---|
| `dns-hijacked` | `example.com` resolves to the gateway Service ClusterIP, not the real address |
| `allowed-via-gateway` | the labelled pod gets HTTP 200 from `https://example.com` through the gateway (TLS is end-to-end; Envoy only proxies TCP) |
| `wrong-label-denied` | the gateway NetworkPolicy rejects pods with the wrong label |
| `unlabelled-denied` | and pods with no label |
| `direct-egress-denied` | connecting straight to example.com's real IP is blocked by the namespace's default-deny policy, so the gateway is the only way out |
| `control-direct-allowed` | an unrestricted pod in `default` *can* reach the real IP, so the denials above are caused by the policies, not by missing internet access |

To poke at it yourself:

```bash
kubectl -n lab exec allowed -- curl -sS -o /dev/null -w '%{http_code} %{remote_ip}\n' https://example.com
kubectl -n lab exec denied  -- curl -sS --max-time 5 https://example.com   # times out / fails
kubectl -n egress-operator-system logs deploy/example                     # gateway access log
```

## Next steps

* [Blocking non-gateway traffic](../README.md#blocking-non-gateway-traffic) on a real cluster
* [Configuration](../README.md#configuration) of node selectors, topology spread and the Envoy image
* [FAQ](../README.md#faq)
