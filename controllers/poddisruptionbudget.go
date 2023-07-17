package controllers

import (
	"context"

	egressv1 "github.com/monzo/egress-operator/api/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *ExternalServiceReconciler) reconcilePodDisruptionBudget(ctx context.Context, req ctrl.Request, es *egressv1.ExternalService) error {
	desired := pdb(es)
	if err := ctrl.SetControllerReference(es, desired, r.Scheme); err != nil {
		return err
	}

	pdb := &policyv1beta1.PodDisruptionBudget{}
	if err := r.Get(ctx, req.NamespacedName, pdb); err != nil {
		if apierrs.IsNotFound(err) {
			return r.Client.Create(ctx, desired)
		}
		return err
	}

	patched := pdb.DeepCopy()
	mergeMap(desired.Labels, patched.Labels)
	mergeMap(desired.Annotations, patched.Annotations)
	patched.Spec = desired.Spec

	return ignoreNotFound(r.patchIfNecessary(ctx, patched, client.MergeFrom(pdb)))
}

func pdb(es *egressv1.ExternalService) *policyv1beta1.PodDisruptionBudget {
	return &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:        es.Name,
			Namespace:   namespace,
			Labels:      labels(es),
			Annotations: annotations(es),
		},
		Spec: policyv1beta1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels(es),
			},
			MaxUnavailable: &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "25%",
			},
		},
	}
}
