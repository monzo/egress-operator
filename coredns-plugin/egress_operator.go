package egressoperator

import (
	"context"
	"sync"

	"github.com/coredns/coredns/plugin/rewrite"
	"github.com/miekg/dns"
)

// EgressOperator is a plugin that automatically rewrites URLs like google.com to point to services managed by egressoperator
type EgressOperator struct {
	rewrite.Rewrite
	sync.RWMutex
}

// ServeDNS implements the plugin.Handler interface. This method gets called when egressoperator is used
// in a Server.
func (e *EgressOperator) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	e.RLock()
	defer e.RUnlock()
	return e.Rewrite.ServeDNS(ctx, w, r)
}

// Name implements the Handler interface.
func (e *EgressOperator) Name() string { return "egressoperator" }

func (e *EgressOperator) setRules(rules []rewrite.Rule) {
	e.Lock()
	e.Rules = rules
	e.Unlock()
}
