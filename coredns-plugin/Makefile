IMG ?= ${ACC}.dkr.ecr.eu-west-1.amazonaws.com/monzo/egress-operator:coredns-with-plugin-$(shell git rev-parse --short HEAD)

docker-build: build
	docker build . -t ${IMG}

docker-push:
	docker push ${IMG}

GOARCH ?= amd64

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build -o coredns ./cmd
