package controllers

import (
	"context"

	"github.com/golang/protobuf/proto"
	egressv1 "github.com/monzo/egress-operator/api/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:namespace=egress-operator-system,groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;patch

func (r *ExternalServiceReconciler) reconcileAutoscaler(ctx context.Context, req ctrl.Request, es *egressv1.ExternalService) error {
	desired := autoscaler(es)
	if err := ctrl.SetControllerReference(es, desired, r.Scheme); err != nil {
		return err
	}
	d := &autoscalingv1.HorizontalPodAutoscaler{}
	if err := r.Get(ctx, req.NamespacedName, d); err != nil {
		if apierrs.IsNotFound(err) {
			return r.Client.Create(ctx, desired)
		}
		return err
	}

	patched := d.DeepCopy()
	mergeMap(desired.Labels, patched.Labels)
	mergeMap(desired.Annotations, patched.Annotations)
	patched.Spec = desired.Spec

	return ignoreNotFound(r.patchIfNecessary(ctx, patched, client.MergeFrom(d)))
}

func autoscaler(es *egressv1.ExternalService) *autoscalingv1.HorizontalPodAutoscaler {
	min := es.Spec.MinReplicas
	if min == nil {
		min = proto.Int(3)
	}

	max := es.Spec.MaxReplicas
	if max == nil {
		max = proto.Int(12)
	}

	target := es.Spec.TargetCPUUtilizationPercentage
	if target == nil {
		target = proto.Int(50)
	}

	return &autoscalingv1.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:        es.Name,
			Namespace:   namespace,
			Labels:      labels(es),
			Annotations: annotations(es),
		},
		Spec: autoscalingv1.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       es.Name,
			},
			MinReplicas:                    min,
			MaxReplicas:                    *max,
			TargetCPUUtilizationPercentage: target,
		},
	}
}
