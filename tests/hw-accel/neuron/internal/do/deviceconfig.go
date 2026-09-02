package do

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/neuron"
	neuronscheme "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/neuron/v1beta1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// DeviceConfigState contains the DeviceConfig that existed before a temporary
// replacement. A non-nil state with a nil Original means no DeviceConfig
// existed and allows cleanup to distinguish that from a failed snapshot.
type DeviceConfigState struct {
	Original  *neuronscheme.DeviceConfig
	Name      string
	Namespace string
}

// CreateDeviceConfigUnstructured creates a DeviceConfig with the given spec using the dynamic client.
// This bypasses eco-goinfra builder validation, useful for testing CRD-level CEL validation.
func CreateDeviceConfigUnstructured(
	apiClient *clients.Settings, name string, spec map[string]interface{}) error {
	klog.V(params.NeuronLogLevel).Infof("Creating unstructured DeviceConfig %s", name)

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k8s.aws/v1beta1",
			"kind":       "DeviceConfig",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": params.NeuronNamespace,
			},
			"spec": spec,
		},
	}

	_, err := apiClient.Resource(neuronparams.DeviceConfigGVR).
		Namespace(params.NeuronNamespace).
		Create(context.TODO(), obj, metav1.CreateOptions{})

	return err
}

// DeleteDeviceConfigIfExists deletes a DeviceConfig by name, ignoring not-found errors.
func DeleteDeviceConfigIfExists(apiClient *clients.Settings, name string) {
	klog.V(params.NeuronLogLevel).Infof("Deleting DeviceConfig %s if it exists", name)

	_ = apiClient.Resource(neuronparams.DeviceConfigGVR).
		Namespace(params.NeuronNamespace).
		Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// ReplaceDeviceConfig snapshots and removes the current DeviceConfig before
// creating the supplied replacement. The returned state is safe to pass to
// RestoreDeviceConfig even when replacement creation fails.
func ReplaceDeviceConfig(apiClient *clients.Settings, replacement *neuron.Builder,
	timeout time.Duration) (*DeviceConfigState, error) {
	if apiClient == nil {
		return nil, fmt.Errorf("api client is nil")
	}

	if replacement == nil {
		return nil, fmt.Errorf("replacement DeviceConfig builder is nil")
	}

	if replacement.Definition == nil {
		return nil, fmt.Errorf("replacement DeviceConfig definition is nil")
	}

	if err := apiClient.AttachScheme(neuronscheme.AddToScheme); err != nil {
		return nil, fmt.Errorf("attaching DeviceConfig scheme: %w", err)
	}

	state := &DeviceConfigState{
		Name:      replacement.Definition.Name,
		Namespace: replacement.Definition.Namespace,
	}
	current := &neuronscheme.DeviceConfig{}

	err := apiClient.Get(context.TODO(), ctrlclient.ObjectKey{
		Name: state.Name, Namespace: state.Namespace,
	}, current)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("capturing existing DeviceConfig: %w", err)
	}

	if err == nil {
		state.Original = current.DeepCopy()

		if deleteErr := apiClient.Delete(context.TODO(), current); deleteErr != nil {
			return state, fmt.Errorf("deleting existing DeviceConfig: %w", deleteErr)
		}

		if waitErr := await.DeviceConfigDeleted(
			apiClient, current.Name, current.Namespace, timeout); waitErr != nil {
			return state, fmt.Errorf("waiting for existing DeviceConfig deletion: %w", waitErr)
		}
	}

	if _, err = replacement.Create(); err != nil {
		return state, fmt.Errorf("creating replacement DeviceConfig: %w", err)
	}

	return state, nil
}

// RestoreDeviceConfig deletes the temporary DeviceConfig and recreates the
// exact DeviceConfig definition captured by ReplaceDeviceConfig. If no original
// existed, this leaves the cluster without a DeviceConfig.
func RestoreDeviceConfig(apiClient *clients.Settings, state *DeviceConfigState,
	timeout time.Duration) error {
	if state == nil {
		return nil
	}

	if apiClient == nil {
		return fmt.Errorf("api client is nil")
	}

	if err := apiClient.AttachScheme(neuronscheme.AddToScheme); err != nil {
		return fmt.Errorf("attaching DeviceConfig scheme: %w", err)
	}

	current := &neuronscheme.DeviceConfig{}

	err := apiClient.Get(context.TODO(), ctrlclient.ObjectKey{
		Name: state.Name, Namespace: state.Namespace,
	}, current)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("getting temporary DeviceConfig: %w", err)
	}

	if err == nil {
		if deleteErr := apiClient.Delete(context.TODO(), current); deleteErr != nil {
			return fmt.Errorf("deleting temporary DeviceConfig: %w", deleteErr)
		}

		if waitErr := await.DeviceConfigDeleted(
			apiClient, current.Name, current.Namespace, timeout); waitErr != nil {
			return fmt.Errorf("waiting for temporary DeviceConfig deletion: %w", waitErr)
		}
	}

	if state.Original == nil {
		return nil
	}

	restored := state.Original.DeepCopy()
	restored.ResourceVersion = ""
	restored.UID = ""
	restored.Generation = 0
	restored.CreationTimestamp = metav1.Time{}
	restored.DeletionTimestamp = nil
	restored.DeletionGracePeriodSeconds = nil
	restored.ManagedFields = nil
	restored.Status = neuronscheme.DeviceConfigStatus{}

	if err = apiClient.Create(context.TODO(), restored); err != nil {
		return fmt.Errorf("restoring original DeviceConfig: %w", err)
	}

	return nil
}
