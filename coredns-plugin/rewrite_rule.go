package egressoperator

import (
	"context"
	"regexp"

	"github.com/coredns/coredns/plugin/rewrite"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

type exactNameRule struct {
	NextAction string
	From       string
	To         string

	AnswerPattern     *regexp.Regexp
	AnswerReplacement string
}

var (
	_ rewrite.Rule         = &exactNameRule{}
	_ rewrite.ResponseRule = &exactNameRule{}
)

// Rewrite rewrites the current request based upon exact match of the name
// in the question section of the request.
func (rule *exactNameRule) Rewrite(ctx context.Context, state request.Request) (rewrite.ResponseRules, rewrite.Result) {
	if rule.From == state.Name() {
		state.Req.Question[0].Name = rule.To
		return rewrite.ResponseRules{rule}, rewrite.RewriteDone
	}
	return nil, rewrite.RewriteIgnored
}

// Mode returns the processing nextAction
func (rule *exactNameRule) Mode() string { return rule.NextAction }

// RewriteResponse rewrites the Name of response records matching AnswerPattern
// back to the original question name, so clients see an answer for what they
// asked rather than the internal egress-gateway service name.
func (rule *exactNameRule) RewriteResponse(res *dns.Msg, rr dns.RR) {
	h := rr.Header()
	if rule.AnswerPattern.MatchString(h.Name) {
		h.Name = rule.AnswerPattern.ReplaceAllString(h.Name, rule.AnswerReplacement)
	}
}
