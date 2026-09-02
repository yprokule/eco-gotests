package check

import (
	"context"
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/neuron"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/resource"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// DRAInClusterBuildConfigured verifies that the DeviceConfig omits a pre-built
// driver image and that the generated Module combines in-cluster build settings
// with DRA instead of the legacy device plugin.
func DRAInClusterBuildConfigured(apiClient *clients.Settings) (bool, error) {
	deviceConfig, err := neuron.Pull(
		apiClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
	if err != nil {
		return false, fmt.Errorf("pulling DeviceConfig: %w", err)
	}

	spec := deviceConfig.Object.Spec
	if spec.DriversImage != "" || spec.DriverVersion == "" || spec.DRADriverImage == "" {
		return false, nil
	}

	module, err := kmm.Pull(apiClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
	if err != nil {
		return false, fmt.Errorf("pulling KMM Module: %w", err)
	}

	moduleSpec := module.Object.Spec
	if moduleSpec.DRA == nil || moduleSpec.DevicePlugin != nil || moduleSpec.ModuleLoader == nil {
		return false, nil
	}

	container := moduleSpec.ModuleLoader.Container
	if container.Build != nil {
		return true, nil
	}

	for _, mapping := range container.KernelMappings {
		if mapping.Build != nil {
			return true, nil
		}
	}

	return false, nil
}

// DRADaemonSet returns the DRA driver DaemonSet from the operator namespace.
func DRADaemonSet(apiClient *clients.Settings) (*appsv1.DaemonSet, error) {
	klog.V(params.NeuronLogLevel).Info("Getting DRA DaemonSet")

	dsList, err := apiClient.K8sClient.AppsV1().DaemonSets(params.NeuronNamespace).List(
		context.TODO(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s",
				params.DRADaemonSetLabelKey, params.DRADaemonSetLabelValue),
		})
	if err != nil {
		return nil, err
	}

	if len(dsList.Items) == 0 {
		return nil, fmt.Errorf("no DRA DaemonSet found in namespace %s", params.NeuronNamespace)
	}

	return &dsList.Items[0], nil
}

// SmallestDRANode returns the node name and device count of the Neuron node
// with the fewest DRA devices, based on ResourceSlice data.
func SmallestDRANode(apiClient *clients.Settings) (string, int, error) {
	slices, err := resource.ListResourceSlicesByDriver(apiClient, params.DRADriverName)
	if err != nil {
		return "", 0, err
	}

	if len(slices) == 0 {
		return "", 0, fmt.Errorf("no ResourceSlices found for driver %s", params.DRADriverName)
	}

	targetNode := ""
	targetDeviceCount := 0

	for _, slice := range slices {
		if slice.Object.Spec.NodeName == nil {
			continue
		}

		devCount := len(slice.Object.Spec.Devices)
		if targetNode == "" || devCount < targetDeviceCount {
			targetNode = *slice.Object.Spec.NodeName
			targetDeviceCount = devCount
		}
	}

	if targetNode == "" {
		return "", 0, fmt.Errorf("no ResourceSlices with valid nodeName found")
	}

	klog.V(params.NeuronLogLevel).Infof("Smallest DRA node: %s with %d devices",
		targetNode, targetDeviceCount)

	return targetNode, targetDeviceCount, nil
}

// ResourceClaimAllocatedAndReserved reports whether a ResourceClaim in the
// namespace is both allocated and reserved for a consumer.
func ResourceClaimAllocatedAndReserved(apiClient *clients.Settings, namespace string) (bool, error) {
	claims, err := resource.ListResourceClaims(apiClient, namespace)
	if err != nil {
		return false, err
	}

	for _, claim := range claims {
		if claim.Object.Status.Allocation != nil && len(claim.Object.Status.ReservedFor) > 0 {
			return true, nil
		}
	}

	return false, nil
}

// PodHasNeuronDevices checks if a running pod has /dev/neuron* devices visible.
func PodHasNeuronDevices(apiClient *clients.Settings, name, namespace string) (bool, error) {
	runningPod, err := pod.Pull(apiClient, name, namespace)
	if err != nil {
		return false, err
	}

	output, err := runningPod.ExecCommand(
		[]string{"sh", "-c", "ls /dev/neuron* 2>/dev/null | head -1"})
	if err != nil {
		return false, err
	}

	found := strings.TrimSpace(output.String()) != ""

	klog.V(params.NeuronLogLevel).Infof("Pod %s neuron devices found: %v", name, found)

	return found, nil
}
