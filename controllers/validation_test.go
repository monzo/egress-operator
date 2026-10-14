package controllers

import (
	"context"

	v1 "github.com/monzo/egress-operator/api/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// See https://github.com/monzo/egress-operator/issues/23: dnsName is a literal hostname,
// so regexes and wildcards must be rejected by the API server instead of silently accepted.
var _ = Describe("ExternalService dnsName validation", func() {
	externalService := func(name, dnsName string) *v1.ExternalService {
		return &v1.ExternalService{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: v1.ExternalServiceSpec{
				DnsName: dnsName,
				Ports:   []v1.ExternalServicePort{{Port: 443}},
			},
		}
	}

	for _, dnsName := range []string{
		`^global\.azure-devices-provisioning\.net$`, // regex
		"*.example.com",       // wildcard
		"https://example.com", // url
		"example .com",        // whitespace
	} {
		dnsName := dnsName
		It("rejects "+dnsName, func() {
			err := k8sClient.Create(context.Background(), externalService("invalid-dns-name", dnsName))
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "expected Invalid error, got %v", err)
		})
	}

	for name, dnsName := range map[string]string{
		"valid-simple": "example.com",
		"valid-dashes": "global.azure-devices-provisioning.net",
		"valid-fqdn":   "example.com.",
	} {
		name, dnsName := name, dnsName
		It("accepts "+dnsName, func() {
			es := externalService(name, dnsName)
			Expect(k8sClient.Create(context.Background(), es)).To(Succeed())
			Expect(k8sClient.Delete(context.Background(), es)).To(Succeed())
		})
	}
})
