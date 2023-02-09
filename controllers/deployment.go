package controllers

import (
	"context"
	"os"
	"strconv"

	"github.com/golang/protobuf/proto"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	egressv1 "github.com/monzo/egress-operator/api/v1"
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

	img := "envoyproxy/envoy:v1.25.9"
	if i, ok := os.LookupEnv("ENVOY_IMAGE"); ok {
		img = i
	}

	labelSelector := metav1.SetAsLabelSelector(labelsToSelect(es))

	var tolerations []corev1.Toleration
	tk, kok := os.LookupEnv("TAINT_TOLERATION_KEY")
	tv, vok := os.LookupEnv("TAINT_TOLERATION_VALUE")
	if kok && vok {
		tolerations = append(tolerations, corev1.Toleration{
			Key:    tk,
			Value:  tv,
			Effect: corev1.TaintEffectNoSchedule,
		})
	}

	var nodeSelector map[string]string
	nk, kok := os.LookupEnv("NODE_SELECTOR_KEY")
	nv, vok := os.LookupEnv("NODE_SELECTOR_VALUE")
	if kok && vok {
		nodeSelector = map[string]string{
			nk: nv,
		}
	}

	var podTopologySpread []corev1.TopologySpreadConstraint
	topologyEnable, _ := os.LookupEnv("ENABLE_POD_TOPOLOGY_SPREAD")
	if topologyEnable == "true" {
		zoneSkew, zoneEnabled := os.LookupEnv("POD_TOPOLOGY_ZONE_MAX_SKEW")
		zoneKey, zoneKeyFound := os.LookupEnv("POD_TOPOLOGY_ZONE_MAX_SKEW_KEY")
		if zoneEnabled {
			maxSkew, err := strconv.Atoi(zoneSkew)
			if err != nil {
				maxSkew = 1
			}
			// Default zone key to the Kubernetes topology one if not specified
			if !zoneKeyFound {
				zoneKey = "topology.kubernetes.io/zone"
			}
			podTopologySpread = append(podTopologySpread, corev1.TopologySpreadConstraint{
				TopologyKey:       zoneKey,
				WhenUnsatisfiable: corev1.ScheduleAnyway,
				MaxSkew:           int32(maxSkew),
				LabelSelector:     labelSelector,
			})
		}
		hostnameSkew, hostnameEnabled := os.LookupEnv("POD_TOPOLOGY_HOSTNAME_MAX_SKEW")
		hostnameKey, hostnameKeyFound := os.LookupEnv("POD_TOPOLOGY_HOSTNAME_MAX_SKEW_KEY")
		if hostnameEnabled {
			maxSkew, err := strconv.Atoi(hostnameSkew)
			if err != nil {
				maxSkew = 1
			}
			// Default zone key to the Kubernetes topology one if not specified
			if !hostnameKeyFound {
				hostnameKey = "kubernetes.io/hostname"
			}
			podTopologySpread = append(podTopologySpread, corev1.TopologySpreadConstraint{
				TopologyKey:       hostnameKey,
				WhenUnsatisfiable: corev1.ScheduleAnyway,
				MaxSkew:           int32(maxSkew),
				LabelSelector:     labelSelector,
			})
		}
	}

	maxUnavailableStr := lookupEnvOr("ROLLING_UPDATE_MAX_UNAVAILABLE", "25%")
	maxSurgeStr := lookupEnvOr("ROLLING_UPDATE_MAX_SURGE", "25%")

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

	maxUnavailable := intstr.FromString(maxUnavailableStr)
	maxSurge := intstr.FromString(maxSurgeStr)

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
					MaxUnavailable: &maxUnavailable,
					MaxSurge:       &maxSurge,
				},
			},
			Selector: labelSelector,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels(es),
					Annotations: a,
				},
				Spec: corev1.PodSpec{
					Tolerations:               tolerations,
					NodeSelector:              nodeSelector,
					TopologySpreadConstraints: podTopologySpread,
					Containers: []corev1.Container{
						{
							Name:            "gateway",
							Image:           img,
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
								PreStop: &corev1.LifecycleHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sleep", "25"},
									},
								},
							},
							TerminationMessagePath:   corev1.TerminationMessagePathDefault,
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
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
							Env: []corev1.EnvVar{
								{
									Name:  "ENVOY_UID",
									Value: "0",
								},
							},
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

func lookupEnvOr(envKey, envDefaultValue string) string {
	valueStr, isSet := os.LookupEnv(envKey)
	if !isSet || len(valueStr) == 0 {
		return envDefaultValue
	}
	return valueStr
}
