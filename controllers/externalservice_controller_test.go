package controllers

import (
	"context"
	"time"

	"github.com/golang/protobuf/proto"
	v1 "github.com/monzo/egress-operator/api/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/types"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("ExternalService Controller", func() {
	Context("Default external service", func() {
		key := types.NamespacedName{
			Name:      "google",
			Namespace: "egress-operator-system",
		}

		It("Should create successfully", func() {
			created := &v1.ExternalService{
				ObjectMeta: metav1.ObjectMeta{
					Name: key.Name,
				},
				Spec: v1.ExternalServiceSpec{
					DnsName: "google.com",
					Ports: []v1.ExternalServicePort{
						{Port: 443},
					},
				},
			}

			Expect(k8sClient.Create(context.Background(), created)).Should(Succeed())

			assertState(key, created)
		})

		It("Should update successfully", func() {
			updated := &v1.ExternalService{
				ObjectMeta: metav1.ObjectMeta{
					Name: key.Name,
				},
			}

			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: key.Name}, updated)).Should(Succeed())

			updated.Spec.Ports = []v1.ExternalServicePort{
				{Port: 443},
				{Port: 256},
			}

			updated.Spec.MinReplicas = proto.Int(4)

			Expect(k8sClient.Update(context.Background(), updated)).Should(Succeed())

			assertState(key, updated)
		})

		It("Should revert deletion successfully", func() {
			current := &v1.ExternalService{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: key.Name}, current)).Should(Succeed())

			for _, obj := range [...]metav1.Object{&appsv1.Deployment{}, &networkingv1.NetworkPolicy{}, &corev1.Service{},
				&corev1.ConfigMap{}, &autoscalingv1.HorizontalPodAutoscaler{}} {

				obj.SetName(key.Name)
				obj.SetNamespace(key.Namespace)

				Expect(k8sClient.Delete(context.Background(), obj.(runtime.Object))).Should(Succeed())

				assertState(key, current)
			}
		})

		It("Should revert dodgy updates successfully", func() {
			current := &v1.ExternalService{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: key.Name}, current)).Should(Succeed())

			for _, obj := range [...]metav1.Object{&appsv1.Deployment{}, &networkingv1.NetworkPolicy{}, &corev1.Service{},
				&corev1.ConfigMap{}, &autoscalingv1.HorizontalPodAutoscaler{}} {

				Expect(k8sClient.Get(context.Background(), key, obj.(runtime.Object))).Should(Succeed())

				obj.GetLabels()["app"] = "foo"

				Expect(k8sClient.Update(context.Background(), obj.(runtime.Object))).Should(Succeed())

				assertState(key, current)
			}
		})
	})
})

func assertOwner(name string) GomegaMatcher {
	return WithTransform(func(obj metav1.Object) []metav1.OwnerReference { return obj.GetOwnerReferences() },
		And(
			HaveLen(1),
			WithTransform(func(rs []metav1.OwnerReference) string { return rs[0].Name }, Equal(name)),
			WithTransform(func(rs []metav1.OwnerReference) string { return rs[0].Kind }, Equal("ExternalService")),
		))
}

func mapContainsMap(mustContain map[string]string) GomegaMatcher {
	return WithTransform(func(m map[string]string) bool {
		for k, v := range mustContain {
			if otherV, ok := m[k]; !ok || otherV != v {
				return false
			}
		}

		return true
	}, BeTrue())
}

func assertLabels(target metav1.Object) GomegaMatcher {
	return WithTransform(func(obj metav1.Object) [2]map[string]string {
		return [2]map[string]string{obj.GetLabels(), obj.GetAnnotations()}
	},
		And(
			WithTransform(func(maps [2]map[string]string) map[string]string { return maps[0] }, mapContainsMap(target.GetLabels())),
			WithTransform(func(maps [2]map[string]string) map[string]string { return maps[1] }, mapContainsMap(target.GetAnnotations())),
		))
}

func assertState(key types.NamespacedName, es *v1.ExternalService) {
	const timeout = time.Second * 30
	const interval = time.Second * 1

	cTarget, cHash, err := configmap(es)
	Expect(err).To(BeNil())

	Eventually(func() *appsv1.Deployment {
		d := &appsv1.Deployment{}
		_ = k8sClient.Get(context.Background(), key, d)

		d.Spec.Replicas = nil

		return d
	}, timeout, interval).Should(And(
		WithTransform(func(d *appsv1.Deployment) appsv1.DeploymentSpec { return d.Spec }, Equal(deployment(es, cHash).Spec)),
		assertOwner(key.Name),
		assertLabels(deployment(es, cHash)),
	))

	Eventually(func() *networkingv1.NetworkPolicy {
		n := &networkingv1.NetworkPolicy{}
		_ = k8sClient.Get(context.Background(), key, n)

		return n
	}, timeout, interval).Should(And(
		WithTransform(func(d *networkingv1.NetworkPolicy) networkingv1.NetworkPolicySpec { return d.Spec }, Equal(networkPolicy(es).Spec)),
		assertOwner(key.Name),
		assertLabels(networkPolicy(es)),
	))

	Eventually(func() *corev1.Service {
		s := &corev1.Service{}
		_ = k8sClient.Get(context.Background(), key, s)
		s.Spec.ClusterIP = ""

		return s
	}, timeout, interval).Should(And(
		WithTransform(func(d *corev1.Service) corev1.ServiceSpec { return d.Spec }, Equal(service(es, true, nil).Spec)),
		assertOwner(key.Name),
		assertLabels(service(es, true, nil)),
	))

	Eventually(func() *corev1.ConfigMap {
		c := &corev1.ConfigMap{}
		_ = k8sClient.Get(context.Background(), key, c)

		return c
	}, timeout, interval).Should(And(
		WithTransform(func(d *corev1.ConfigMap) map[string]string { return d.Data }, Equal(cTarget.Data)),
		assertOwner(key.Name),
		assertLabels(cTarget),
	))

	Eventually(func() *autoscalingv1.HorizontalPodAutoscaler {
		h := &autoscalingv1.HorizontalPodAutoscaler{}
		_ = k8sClient.Get(context.Background(), key, h)

		return h
	}, timeout, interval).Should(And(
		WithTransform(func(d *autoscalingv1.HorizontalPodAutoscaler) autoscalingv1.HorizontalPodAutoscalerSpec {
			return d.Spec
		}, Equal(autoscaler(es).Spec)),
		assertOwner(key.Name),
		assertLabels(autoscaler(es)),
	))
}
