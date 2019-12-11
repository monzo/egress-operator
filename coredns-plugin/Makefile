docker: build
	docker build . -t egress-operator-coredns

build:
	GOOS=linux GOARCH=amd64 go build -o coredns ./cmd