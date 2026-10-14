#!/usr/bin/env bash
# Kubernetes Egress Policy Validation Lab.
#
# Spins up a Kind cluster with Calico (so NetworkPolicies are enforced), deploys egress-operator
# and its CoreDNS plugin built from this checkout, and checks which egress traffic is allowed.
#
#   hack/kind/e2e.sh up            create cluster, build + load images, deploy everything
#   hack/kind/e2e.sh check <name>  run one check (see `checks` below), exit 0 on success
#   hack/kind/e2e.sh all           up + every check
#   hack/kind/e2e.sh down          delete the cluster
#
# Environment: CLUSTER_NAME (default egress-lab), CALICO_VERSION, KEEP_CLUSTER.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
LAB=$ROOT/hack/kind
CLUSTER_NAME=${CLUSTER_NAME:-egress-lab}
CALICO_VERSION=${CALICO_VERSION:-v3.32.2}
OPERATOR_IMAGE=egress-operator:lab
COREDNS_IMAGE=egress-operator-coredns:lab
NS=egress-operator-system
DEST=example.com
CURL_TIMEOUT=10

KUBECTL=(kubectl --context "kind-$CLUSTER_NAME")
checks=(dns-hijacked allowed-via-gateway wrong-label-denied unlabelled-denied direct-egress-denied control-direct-allowed)

log() { printf '\n==> %s\n' "$*" >&2; }

require() {
	for bin in "$@"; do
		command -v "$bin" >/dev/null || { echo "missing required tool: $bin" >&2; exit 1; }
	done
}

up() {
	require kind kubectl docker
	local arch
	arch=$(docker version -f '{{.Server.Arch}}')

	if ! kind get clusters | grep -qx "$CLUSTER_NAME"; then
		log "Creating Kind cluster $CLUSTER_NAME"
		kind create cluster --name "$CLUSTER_NAME" --config "$LAB/kind-config.yaml" --wait 0s
	fi

	log "Installing Calico $CALICO_VERSION"
	"${KUBECTL[@]}" apply --server-side -f "https://raw.githubusercontent.com/projectcalico/calico/$CALICO_VERSION/manifests/calico.yaml" >/dev/null
	"${KUBECTL[@]}" -n kube-system rollout status ds/calico-node --timeout=5m
	"${KUBECTL[@]}" wait --for=condition=Ready nodes --all --timeout=5m

	log "Building images for linux/$arch"
	docker buildx build --platform "linux/$arch" -t "$OPERATOR_IMAGE" --load "$ROOT"
	make -C "$ROOT/coredns-plugin" build GOARCH="$arch"
	docker buildx build --platform "linux/$arch" -t "$COREDNS_IMAGE" --load "$ROOT/coredns-plugin"
	kind load docker-image --name "$CLUSTER_NAME" "$OPERATOR_IMAGE" "$COREDNS_IMAGE"

	log "Deploying egress-operator"
	"${KUBECTL[@]}" apply -k "$LAB/overlay"
	"${KUBECTL[@]}" -n "$NS" rollout status deploy/egress-operator-controller-manager --timeout=5m

	log "Switching CoreDNS to the egress-operator build"
	local corefile
	corefile=$("${KUBECTL[@]}" -n kube-system get configmap coredns -o jsonpath='{.data.Corefile}')
	if ! grep -q egressoperator <<<"$corefile"; then
		corefile=$(sed "s/^\.:53 {/.:53 {\n    egressoperator $NS cluster.local/" <<<"$corefile")
		"${KUBECTL[@]}" -n kube-system create configmap coredns --from-literal=Corefile="$corefile" \
			--dry-run=client -o yaml | "${KUBECTL[@]}" apply -f -
	fi
	"${KUBECTL[@]}" -n kube-system set image deploy/coredns coredns="$COREDNS_IMAGE"
	"${KUBECTL[@]}" -n kube-system rollout restart deploy/coredns
	"${KUBECTL[@]}" -n kube-system rollout status deploy/coredns --timeout=5m

	log "Creating ExternalService and lab workloads"
	"${KUBECTL[@]}" apply -f "$LAB/manifests/gateway-dns.yaml" -f "$LAB/manifests/externalservice.yaml" -f "$LAB/manifests/lab.yaml"
	wait_for "gateway deployment" "${KUBECTL[@]}" -n "$NS" get deploy/example
	"${KUBECTL[@]}" -n "$NS" rollout status deploy/example --timeout=5m
	"${KUBECTL[@]}" -n lab wait --for=condition=Ready pod --all --timeout=5m
	wait_for "DNS hijack label" sh -c \
		"[ \"\$(${KUBECTL[*]} -n $NS get svc example -o jsonpath='{.metadata.labels.egress\.monzo\.com/hijack-dns}')\" = true ]"
	# Envoy reports ready before its first upstream DNS resolution succeeds; give it time to warm up.
	# Not fatal: if it never works, the checks report it.
	wait_for "gateway upstream" lab_curl allowed || true
}

wait_for() {
	local what=$1; shift
	for _ in $(seq 60); do
		"$@" >/dev/null 2>&1 && return 0
		sleep 5
	done
	echo "timed out waiting for $what" >&2
	return 1
}

# curl from a lab pod; prints "<http_code> <remote_ip>" and returns curl's exit code
lab_curl() {
	local pod=$1; shift
	"${KUBECTL[@]}" -n lab exec "$pod" -- curl -sS -o /dev/null --max-time "$CURL_TIMEOUT" \
		-w '%{http_code} %{remote_ip}' "$@" "https://$DEST/"
}

gateway_ip() { "${KUBECTL[@]}" -n "$NS" get svc example -o jsonpath='{.spec.clusterIP}'; }

# The destination's real public address, resolved outside the cluster (cluster DNS is hijacked)
real_ip() { python3 -c "import socket,sys; print(socket.gethostbyname(sys.argv[1]))" "$DEST"; }

expect_fail() {
	local out
	if out=$("$@" 2>&1); then
		echo "expected failure, but request succeeded: $out" >&2
		return 1
	fi
	echo "blocked as expected: $out"
}

check() {
	case $1 in
	dns-hijacked)
		# curl reports the address it connected to: it must be the gateway Service, not the real host
		local out
		out=$(lab_curl allowed)
		echo "$out (gateway ClusterIP $(gateway_ip))"
		[ "${out#* }" = "$(gateway_ip)" ]
		;;
	allowed-via-gateway)
		local out
		out=$(lab_curl allowed)
		echo "$out"
		[ "${out%% *}" = 200 ]
		;;
	wrong-label-denied) expect_fail lab_curl wrong-label ;;
	unlabelled-denied) expect_fail lab_curl denied ;;
	direct-egress-denied)
		# bypass the gateway by connecting to the real IP: default-deny-external-egress must block it
		expect_fail lab_curl allowed --resolve "$DEST:443:$(real_ip)"
		;;
	control-direct-allowed)
		# without the lab policies the same direct request works, so the denials above are meaningful
		local ip
		ip=$(real_ip)
		"${KUBECTL[@]}" -n default run "control-$RANDOM" --rm -i --restart=Never --quiet \
			--image=curlimages/curl:8.11.1 -- \
			curl -sS -o /dev/null --max-time "$CURL_TIMEOUT" -w '%{http_code}\n' --resolve "$DEST:443:$ip" "https://$DEST/" |
			tee /dev/stderr | grep -qx 200
		;;
	*)
		echo "unknown check: $1 (known: ${checks[*]})" >&2
		return 2
		;;
	esac
}

down() { kind delete cluster --name "$CLUSTER_NAME"; }

case ${1:-} in
up) up ;;
check) check "${2:?check name required}" ;;
down) down ;;
all)
	up
	failed=0
	for c in "${checks[@]}"; do
		if check "$c"; then echo "PASS $c"; else echo "FAIL $c"; failed=1; fi
	done
	[ -n "${KEEP_CLUSTER:-}" ] || down
	exit $failed
	;;
*)
	sed -n '2,13p' "$0"
	exit 2
	;;
esac
