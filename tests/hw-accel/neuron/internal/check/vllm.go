package check

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const defaultSchedulerName = "default-scheduler"

// VLLMDeploymentUsesResourceClaim reports whether the cluster deployment uses
// the requested DRA claim and has no legacy Neuron extended-resource request.
func VLLMDeploymentUsesResourceClaim(
	apiClient *clients.Settings, name, namespace, claimName, claimTemplateName string) (bool, error) {
	vllmDeployment, err := deployment.Pull(apiClient, name, namespace)
	if err != nil {
		return false, err
	}

	podSpec := vllmDeployment.Object.Spec.Template.Spec
	hasPodClaim := false

	for _, claim := range podSpec.ResourceClaims {
		if claim.Name == claimName && claim.ResourceClaimTemplateName != nil &&
			*claim.ResourceClaimTemplateName == claimTemplateName {
			hasPodClaim = true

			break
		}
	}

	if !hasPodClaim {
		return false, nil
	}

	for _, container := range podSpec.Containers {
		if _, found := container.Resources.Limits[corev1.ResourceName(params.NeuronCapacityID)]; found {
			return false, nil
		}

		if _, found := container.Resources.Requests[corev1.ResourceName(params.NeuronCapacityID)]; found {
			return false, nil
		}

		for _, claim := range container.Resources.Claims {
			if claim.Name == claimName {
				return true, nil
			}
		}
	}

	return false, nil
}

// VLLMPodsUseDefaultScheduler reports whether all matching scheduled vLLM pods
// use the Kubernetes default scheduler.
func VLLMPodsUseDefaultScheduler(
	apiClient *clients.Settings, namespace string, podLabels map[string]string) (bool, error) {
	pods, err := pod.List(apiClient, namespace, metav1.ListOptions{
		LabelSelector: labels.Set(podLabels).String(),
	})
	if err != nil {
		return false, err
	}

	if len(pods) == 0 {
		return false, nil
	}

	for _, vllmPod := range pods {
		if vllmPod.Object.Spec.NodeName == "" ||
			(vllmPod.Object.Spec.SchedulerName != "" &&
				vllmPod.Object.Spec.SchedulerName != defaultSchedulerName) {
			return false, nil
		}
	}

	return true, nil
}
