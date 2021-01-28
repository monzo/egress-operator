IMG ?= ${ACC}.dkr.ecr.eu-west-1.amazonaws.com/monzo/egress-operator:coredns-with-plugin-$(shell git rev-parse --short head)

docker-build: build
	docker build . -t ${IMG}

docker-push:
	docker push ${IMG}

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o coredns ./cmd
