package main

import (
	_ "github.com/monzo/egress-operator/coredns-plugin"

	"github.com/coredns/coredns/core/dnsserver"
	_ "github.com/coredns/coredns/core/plugin"
	"github.com/coredns/coredns/coremain"
)

func init() {
	// insert after rewrite
	for i, d := range dnsserver.Directives {
		if d == "rewrite" {
			dnsserver.Directives = append(dnsserver.Directives[:i+1], append([]string{"egressoperator"}, dnsserver.Directives[i+1:]...)...)
			break
		}
	}
}

func main() {
	coremain.Run()
}
