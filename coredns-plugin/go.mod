module github.com/monzo/egress-operator/coredns-plugin

go 1.13

require (
	github.com/caddyserver/caddy v1.0.4
	github.com/coredns/coredns v1.6.5
	github.com/miekg/dns v1.1.25
	k8s.io/api v0.0.0-20190620084959-7cf5895f2711
	k8s.io/apimachinery v0.0.0-20190612205821-1799e75a0719
	k8s.io/client-go v0.0.0-20190620085101-78d2af792bab
)

replace github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.0.0+incompatible
