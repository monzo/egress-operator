package controllers

import (
	"testing"

	v1 "github.com/monzo/egress-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The README tells users which label their client pods need; this pins that contract
// (see https://github.com/monzo/egress-operator/issues/21).
func TestNetworkPolicyAllowsPodsWithAllowedLabel(t *testing.T) {
	es := &v1.ExternalService{ObjectMeta: metav1.ObjectMeta{Name: "google"}}

	np := networkPolicy(es)

	if len(np.Spec.Ingress) != 1 || len(np.Spec.Ingress[0].From) != 1 {
		t.Fatalf("expected exactly one ingress peer, got %+v", np.Spec.Ingress)
	}
	peer := np.Spec.Ingress[0].From[0]
	want := map[string]string{"egress.monzo.com/allowed-google": "true"}
	if got := peer.PodSelector.MatchLabels; len(got) != 1 || got["egress.monzo.com/allowed-google"] != "true" {
		t.Errorf("pod selector = %v, want %v", got, want)
	}
	if peer.NamespaceSelector == nil || len(peer.NamespaceSelector.MatchLabels) != 0 || len(peer.NamespaceSelector.MatchExpressions) != 0 {
		t.Errorf("namespace selector = %v, want empty selector (all namespaces)", peer.NamespaceSelector)
	}
}
