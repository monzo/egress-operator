# Image URL to use all building/pushing image targets
IMG ?= ${ACC}.dkr.ecr.eu-west-1.amazonaws.com/monzo/egress-operator:manager-$(shell git rev-parse --short head)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Local directory for tool binaries. controller-gen is always installed here and a copy
# already on $PATH is ignored, so `make manifests` does not depend on a C compiler.
LOCALBIN ?= $(shell pwd)/bin
CONTROLLER_TOOLS_VERSION ?= v0.17.0
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen

all: manager

# Run tests
test: generate fmt vet manifests
	go test ./... -coverprofile cover.out

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
	CGO_ENABLED=0 $(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	CGO_ENABLED=0 $(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths="./..."

# Build the docker image
docker-build: test
	docker buildx build . -t ${IMG} --platform=linux/amd64

# Push the docker image
docker-push:
	docker push ${IMG}

# Download the pinned controller-gen into ./bin if necessary. A controller-gen already on $PATH is
# deliberately ignored: other versions generate different output, and old ones (installed via
# `go get` with cgo) are what broke issue #17. CGO_ENABLED=0 means no C compiler is needed.
controller-gen: $(CONTROLLER_GEN)
# The stamp file makes a CONTROLLER_TOOLS_VERSION change trigger a reinstall.
$(CONTROLLER_GEN): $(LOCALBIN)/.controller-gen-$(CONTROLLER_TOOLS_VERSION)
	CGO_ENABLED=0 GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)
$(LOCALBIN)/.controller-gen-$(CONTROLLER_TOOLS_VERSION):
	mkdir -p $(LOCALBIN)
	rm -f $(LOCALBIN)/.controller-gen-* && touch $@
