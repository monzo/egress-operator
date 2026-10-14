# Image URL to use all building/pushing image targets
IMG ?= ${ACC}.dkr.ecr.eu-west-1.amazonaws.com/monzo/egress-operator:manager-$(shell git rev-parse --short HEAD)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Local directory for tool binaries (controller-gen, setup-envtest, envtest assets)
LOCALBIN ?= $(shell pwd)/bin

# Tool versions. SETUP_ENVTEST_VERSION tracks the controller-runtime minor version in go.mod.
SETUP_ENVTEST_VERSION ?= release-0.19
# Kubernetes version of the etcd/kube-apiserver binaries used by envtest
ENVTEST_K8S_VERSION ?= 1.31.x

ENVTEST ?= $(LOCALBIN)/setup-envtest

# Minimum statement coverage (percent) of the controllers package enforced by coverage-check
COVERAGE_MIN ?= 70

all: manager

# Run tests. envtest binaries (etcd, kube-apiserver) are downloaded into ./bin automatically;
# set KUBEBUILDER_ASSETS yourself to use binaries from elsewhere.
test: generate fmt vet manifests envtest
	KUBEBUILDER_ASSETS="$${KUBEBUILDER_ASSETS:-$$($(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)}" \
		go test ./... -coverprofile cover.out

# Fail if controllers coverage in cover.out is below COVERAGE_MIN. Run after `make test`.
coverage-check:
	@grep -E '^(mode:|github.com/monzo/egress-operator/controllers/)' cover.out > cover.controllers.out
	@pct=$$(go tool cover -func cover.controllers.out | awk '/^total:/ {sub("%","",$$3); print $$3}'); \
	echo "controllers coverage: $${pct}% (minimum $(COVERAGE_MIN)%)"; \
	awk -v p="$$pct" -v m="$(COVERAGE_MIN)" 'BEGIN { exit (p+0 < m+0) }' || { echo "coverage below minimum"; exit 1; }

# Run the Robot Framework acceptance suites in test/ (one per fixed issue).
# Pass ROBOT_ARGS, e.g. ROBOT_ARGS="--include issue-50" or ROBOT_ARGS="--exclude slow".
ROBOT_ARGS ?=
robot:
	uv run --with-requirements test/requirements.txt robot --outputdir test/results $(ROBOT_ARGS) test/

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests
	kustomize build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	cd ./config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths="./..."

# Build the docker image
docker-build: test
	docker buildx build . -t ${IMG} --platform=linux/amd64

# Push the docker image
docker-push:
	docker push ${IMG}

# Download setup-envtest into ./bin if necessary
envtest: $(ENVTEST)
$(ENVTEST):
	mkdir -p $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION)

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -ex ;\
	mkdir -p bin ;\
	GOBIN=$$(pwd)/bin go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.17.0 ;\
	}
CONTROLLER_GEN=./bin/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif
