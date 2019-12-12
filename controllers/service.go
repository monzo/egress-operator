package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	egressv1 "github.com/monzo/egress-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;patch

func (r *ExternalServiceReconciler) reconcileService(ctx context.Context, req ctrl.Request, es *egressv1.ExternalService) error {
	desired := service(es)
	if err := ctrl.SetControllerReference(es, desired, r.Scheme); err != nil {
		return err
	}
	s := &corev1.Service{}
	if err := r.Get(ctx, req.NamespacedName, s); err != nil {
		if apierrs.IsNotFound(err) {
			return r.Client.Create(ctx, desired)
		}
		return err
	}

	patched := s.DeepCopy()
	patched.Labels = desired.Labels
	patched.Annotations = desired.Annotations
	patched.Spec = desired.Spec
	patched.Spec.ClusterIP = s.Spec.ClusterIP

	return ignoreNotFound(r.patchIfNecessary(ctx, patched, client.MergeFrom(s)))
}

func servicePorts(es *egressv1.ExternalService) (ports []corev1.ServicePort) {
	for _, port := range es.Spec.Ports {
		var p corev1.Protocol
		if port.Protocol == nil {
			p = corev1.ProtocolTCP
		} else {
			p = *port.Protocol
		}

		ports = append(ports, corev1.ServicePort{
			Name:       fmt.Sprintf("%s-%s-%s", es.Name, strings.ToLower(string(p)), strconv.Itoa(int(port.Port))),
			Protocol:   p,
			Port:       port.Port,
			TargetPort: intstr.FromInt(int(port.Port)),
		})
	}

	return
}

func service(es *egressv1.ExternalService) *corev1.Service {
	l := labels(es)
	if es.Spec.HijackDns {
		l["egress.monzo.com/hijack-dns"] = "true"
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        es.Name,
			Namespace:   namespace,
			Labels:      l,
			Annotations: annotations(es),
		},
		Spec: corev1.ServiceSpec{
			Selector:        labelsToSelect(es),
			Ports:           servicePorts(es),
			SessionAffinity: corev1.ServiceAffinityNone,
			Type:            corev1.ServiceTypeClusterIP,
		},
	}
}
