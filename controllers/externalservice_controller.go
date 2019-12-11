/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	egressv1 "github.com/monzo/egress-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const namespace = "egress-operator-system"

// ExternalServiceReconciler reconciles a ExternalService object
type ExternalServiceReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=egress.monzo.com,resources=externalservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=egress.monzo.com,resources=externalservices/status,verbs=get;update;patch

func (r *ExternalServiceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("externalservice", req.NamespacedName)

	es := &egressv1.ExternalService{}
	if err := r.Get(ctx, req.NamespacedName, es); err != nil {
		log.Error(err, "unable to fetch ExternalService")
		return ctrl.Result{}, ignoreNotFound(err)
	}

	req.Namespace = namespace

	if err := r.reconcileService(ctx, req, es); err != nil {
		log.Error(err, "unable to reconcile Service")
		return ctrl.Result{}, err
	}

	if err := r.reconcileNetworkPolicy(ctx, req, es); err != nil {
		log.Error(err, "unable to reconcile NetworkPolicy")
		return ctrl.Result{}, err
	}

	if err := r.reconcileConfigMap(ctx, req, es); err != nil {
		log.Error(err, "unable to reconcile ConfigMap")
		return ctrl.Result{}, err
	}

	if err := r.reconcileDeployment(ctx, req, es); err != nil {
		log.Error(err, "unable to reconcile Deployment")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func labels(es *egressv1.ExternalService) map[string]string {
	return map[string]string{
		"app":                      "egress-gateway",
		"egress.monzo.com/gateway": es.Name,
	}
}

func labelsToSelect(es *egressv1.ExternalService) map[string]string {
	return map[string]string{
		"egress.monzo.com/gateway": es.Name,
	}
}

func (r *ExternalServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&egressv1.ExternalService{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}

func ignoreNotFound(err error) error {
	if apierrs.IsNotFound(err) {
		return nil
	}
	return err
}
