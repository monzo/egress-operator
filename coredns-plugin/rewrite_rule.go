package egressoperator

import (
	"context"

	"github.com/coredns/coredns/plugin/rewrite"
	"github.com/coredns/coredns/request"
)

type exactNameRule struct {
	NextAction string
	From       string
	To         string
	rewrite.ResponseRule
}

var _ rewrite.Rule = &exactNameRule{}

// Rewrite rewrites the current request based upon exact match of the name
// in the question section of the request.
func (rule *exactNameRule) Rewrite(ctx context.Context, state request.Request) rewrite.Result {
	if rule.From == state.Name() {
		state.Req.Question[0].Name = rule.To
		return rewrite.RewriteDone
	}
	return rewrite.RewriteIgnored
}

// Mode returns the processing nextAction
func (rule *exactNameRule) Mode() string { return rule.NextAction }

// GetResponseRule return a rule to rewrite the response with. Currently not implemented.
func (rule *exactNameRule) GetResponseRule() rewrite.ResponseRule { return rule.ResponseRule }
