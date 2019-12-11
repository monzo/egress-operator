package controllers

import (
	"context"

	egressv1 "github.com/monzo/egress-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;patch

func (r *ExternalServiceReconciler) reconcileDeployment(ctx context.Context, req ctrl.Request, es *egressv1.ExternalService) error {
	desired := deployment(es)
	if err := ctrl.SetControllerReference(es, desired, r.Scheme); err != nil {
		return err
	}
	d := &appsv1.Deployment{}
	if err := r.Get(ctx, req.NamespacedName, d); err != nil {
		if apierrs.IsNotFound(err) {
			return r.Client.Create(ctx, desired)
		}
		return err
	}

	patched := d.DeepCopy()
	patched.Spec = desired.Spec

	return ignoreNotFound(r.Client.Patch(ctx, patched, client.MergeFrom(d)))
}

func replicas(es *egressv1.ExternalService) int32 {
	if es.Spec.Replicas > 0 {
		return es.Spec.Replicas
	}

	return 3
}

func deploymentPorts(es *egressv1.ExternalService) (ports []corev1.ContainerPort) {
	for _, port := range es.Spec.Ports {
		var p corev1.Protocol
		if port.Protocol == nil {
			p = corev1.ProtocolTCP
		} else {
			p = *port.Protocol
		}

		ports = append(ports, corev1.ContainerPort{
			Protocol:      p,
			ContainerPort: port.Port,
		})
	}

	return
}

func deployment(es *egressv1.ExternalService) *appsv1.Deployment {
	r := replicas(es)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        es.Name,
			Namespace:   namespace,
			Labels:      labels(es),
			Annotations: annotations(es),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: metav1.SetAsLabelSelector(labelsToSelect(es)),
			Replicas: &r,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels(es),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							// TODO: readiness check
							Name: "gateway",
							// TODO this version doesn't actually support UDP, we need 1.13 which isn't stable
							Image: "envoyproxy/envoy-alpine:v1.12.2",
							Ports: deploymentPorts(es),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "envoy-config",
									MountPath: "/etc/envoy",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"cpu":    resource.MustParse("100m"),
									"memory": resource.MustParse("50Mi"),
								},
								Limits: corev1.ResourceList{
									"cpu":    resource.MustParse("2"),
									"memory": resource.MustParse("1Gi"),
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "envoy-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: es.Name,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
