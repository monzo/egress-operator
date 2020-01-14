package controllers

import (
	"context"
	"strconv"

	"github.com/golang/protobuf/proto"
	egressv1 "github.com/monzo/egress-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:namespace=egress-operator-system,groups=apps,resources=deployments,verbs=get;list;watch;create;patch

func (r *ExternalServiceReconciler) reconcileDeployment(ctx context.Context, req ctrl.Request, es *egressv1.ExternalService, configHash string) error {
	desired := deployment(es, configHash)
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
	mergeMap(desired.Labels, patched.Labels)
	mergeMap(desired.Annotations, patched.Annotations)
	patched.Spec = desired.Spec
	patched.Spec.Replicas = d.Spec.Replicas

	return ignoreNotFound(r.patchIfNecessary(ctx, patched, client.MergeFrom(d)))
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

func deployment(es *egressv1.ExternalService, configHash string) *appsv1.Deployment {
	adPort := adminPort(es)
	a := annotations(es)
	a["egress.monzo.com/config-hash"] = configHash
	a["egress.monzo.com/admin-port"] = strconv.Itoa(int(adPort))
	a["prometheus.io/port"] = "11000"
	a["prometheus.io/scrape"] = "true"

	var resources corev1.ResourceRequirements
	if es.Spec.Resources != nil {
		resources = *es.Spec.Resources
	} else {
		resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				"cpu":    resource.MustParse("100m"),
				"memory": resource.MustParse("50Mi"),
			},
			Limits: corev1.ResourceList{
				"cpu":    resource.MustParse("2"),
				"memory": resource.MustParse("1Gi"),
			},
		}
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        es.Name,
			Namespace:   namespace,
			Labels:      labels(es),
			Annotations: annotations(es),
		},
		Spec: appsv1.DeploymentSpec{
			ProgressDeadlineSeconds: proto.Int(600),
			RevisionHistoryLimit:    proto.Int(10),
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: intstr.ValueOrDefault(nil, intstr.FromString("25%")),
					MaxSurge:       intstr.ValueOrDefault(nil, intstr.FromString("25%")),
				},
			},
			Selector: metav1.SetAsLabelSelector(labelsToSelect(es)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels(es),
					Annotations: a,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "gateway",
							// TODO this version doesn't actually support UDP, we need 1.13 which isn't stable
							Image:           "envoyproxy/envoy-alpine:v1.12.2",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports:           deploymentPorts(es),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "envoy-config",
									MountPath: "/etc/envoy",
								},
							},
							// Copying istio; don't try drain outbound listeners, but after going into terminating state,
							// wait 25 seconds for connections to naturally close before going ahead with stop.
							Lifecycle: &corev1.Lifecycle{
								PreStop: &corev1.Handler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sleep", "25"},
									},
								},
							},
							TerminationMessagePath:   corev1.TerminationMessagePathDefault,
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							ReadinessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/ready",
										Port:   intstr.FromInt(int(adPort)),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								FailureThreshold: 3,
								PeriodSeconds:    10,
								SuccessThreshold: 1,
								TimeoutSeconds:   1,
							},
							Resources: resources,
						},
					},
					RestartPolicy:                 corev1.RestartPolicyAlways,
					SchedulerName:                 corev1.DefaultSchedulerName,
					SecurityContext:               &corev1.PodSecurityContext{},
					TerminationGracePeriodSeconds: proto.Int64(30),
					DNSPolicy:                     corev1.DNSDefault,
					Volumes: []corev1.Volume{
						{
							Name: "envoy-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									DefaultMode: proto.Int(420),
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
