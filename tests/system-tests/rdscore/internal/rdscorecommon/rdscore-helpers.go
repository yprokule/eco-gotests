package rdscorecommon

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	uncordonNodeInterval = 15 * time.Second
	uncordonNodeTimeout  = 3 * time.Minute

	// Drain operation constants - used as minimum fallback values.
	// Actual timeouts are calculated dynamically based on pod grace periods.
	drainNodeTimeout      = 25 * time.Minute // Minimum total drain timeout
	drainNodeGracePeriod  = 600              // Minimum pod termination grace period (10 min)
	drainNodeSkipWait     = 300              // Minimum skip wait for stuck pods (5 min)
	drainNodeRetryTimeout = 2 * time.Minute  // Retry window for transient failures
)

// UncordonNode uncordons a node referenced by nodeToUncordon parameter.
// It retries uncordoning for the specified timeout duration at regular intervals.
// Returns error if uncordon fails after timeout to allow caller to handle appropriately.
func UncordonNode(nodeToUncordon *nodes.Builder, interval, timeout time.Duration) error {
	By(fmt.Sprintf("Uncordoning node %q", nodeToUncordon.Definition.Name))

	err := wait.PollUntilContextTimeout(context.TODO(), interval, timeout, true,
		func(context.Context) (bool, error) {
			err := nodeToUncordon.Uncordon()
			if err != nil {
				errorMsg := err.Error()
				// Log retryable errors differently from permanent failures
				if strings.Contains(errorMsg, "ManagedNode infra config cache not synchronized") ||
					strings.Contains(errorMsg, "connection reset") ||
					strings.Contains(errorMsg, "connection refused") {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
						"Uncordon failed with retryable error, will retry: %v", err)

					return false, nil
				}

				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Failed to uncordon %q: %v", nodeToUncordon.Definition.Name, err)

				return false, err
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Successfully uncordon %q", nodeToUncordon.Definition.Name)

			return true, nil
		})
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to uncordon %q after %v: %v",
			nodeToUncordon.Definition.Name, timeout, err)

		return fmt.Errorf("failed to uncordon %q within %v: %w", nodeToUncordon.Definition.Name, timeout, err)
	}

	return nil
}

// calculateDrainTimeouts scans all pods on the target node and returns recommended
// drain timeout values based on the maximum terminationGracePeriodSeconds found.
// It ensures drain timeouts accommodate even the slowest-terminating pods.
// Returns (gracePeriod, skipWait, totalTimeout, error).
func calculateDrainTimeouts(
	apiClient *clients.Settings,
	nodeName string,
) (int, int, time.Duration, error) {
	klog.V(100).Infof("Calculating drain timeouts for node %q", nodeName)

	// List all pods on the target node across all namespaces
	listOptions := metav1.ListOptions{
		FieldSelector: fields.SelectorFromSet(
			fields.Set{"spec.nodeName": nodeName}).String(),
	}

	pods, err := pod.ListInAllNamespaces(apiClient, listOptions)
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Failed to list pods on node %q: %v - using default timeouts", nodeName, err)

		return drainNodeGracePeriod, drainNodeSkipWait, drainNodeTimeout, err
	}

	klog.V(100).Infof("Found %d pods on node %q", len(pods), nodeName)

	// Handle empty node case
	if len(pods) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Node %q has no pods, using minimum timeout values", nodeName)

		return drainNodeGracePeriod, drainNodeSkipWait, drainNodeTimeout, nil
	}

	// Find maximum terminationGracePeriodSeconds among all pods
	maxGracePeriod := int64(30) // Kubernetes default

	var podsWithMaxGracePeriod []string

	for _, podBuilder := range pods {
		var gracePeriod int64

		if podBuilder.Object.Spec.TerminationGracePeriodSeconds != nil {
			gracePeriod = *podBuilder.Object.Spec.TerminationGracePeriodSeconds
		} else {
			gracePeriod = 30 // Kubernetes default
		}

		if gracePeriod > maxGracePeriod {
			maxGracePeriod = gracePeriod
			podsWithMaxGracePeriod = []string{fmt.Sprintf("%s/%s", podBuilder.Object.Namespace, podBuilder.Object.Name)}
		} else if gracePeriod == maxGracePeriod {
			podsWithMaxGracePeriod = append(podsWithMaxGracePeriod,
				fmt.Sprintf("%s/%s", podBuilder.Object.Namespace, podBuilder.Object.Name))
		}
	}

	// Calculate timeout values with appropriate buffer
	// Grace period: use max found, but enforce minimum of 600s (10 min)
	calculatedGracePeriod := int(maxGracePeriod)
	if calculatedGracePeriod < drainNodeGracePeriod {
		calculatedGracePeriod = drainNodeGracePeriod
	}

	// Skip wait: half of grace period (reasonable for stuck pods)
	calculatedSkipWait := calculatedGracePeriod / 2

	// Total timeout: grace period + 5 minute buffer, minimum 25 minutes
	calculatedTimeout := time.Duration(calculatedGracePeriod)*time.Second + 5*time.Minute
	if calculatedTimeout < drainNodeTimeout {
		calculatedTimeout = drainNodeTimeout
	}

	// Add diagnostic logging
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Maximum terminationGracePeriodSeconds on node %q: %ds", nodeName, maxGracePeriod)

	// Limit to first 5 pods to avoid log spam
	podList := podsWithMaxGracePeriod
	if len(podList) > 5 {
		podList = append(podList[:5], fmt.Sprintf("... and %d more", len(podsWithMaxGracePeriod)-5))
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Pods with %ds grace period on node %q: %v", maxGracePeriod, nodeName, podList)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Calculated drain timeouts for node %q - gracePeriod=%ds, skipWait=%ds, totalTimeout=%v",
		nodeName, calculatedGracePeriod, calculatedSkipWait, calculatedTimeout)

	return calculatedGracePeriod, calculatedSkipWait, calculatedTimeout, nil
}

// DrainNodeWithRetry drains a node with retry logic for transient failures.
// It configures drain with production-appropriate timeouts and logs drain duration.
func DrainNodeWithRetry(ctx context.Context, nodeToDrain *nodes.Builder, apiClient *clients.Settings) error {
	// Calculate dynamic timeouts based on pod grace periods
	gracePeriod, skipWait, totalTimeout, calcErr := calculateDrainTimeouts(
		apiClient, nodeToDrain.Definition.Name)
	if calcErr != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Failed to calculate dynamic timeouts for node %q, using defaults: %v",
			nodeToDrain.Definition.Name, calcErr)
		// Fall back to static constants
		gracePeriod = drainNodeGracePeriod
		skipWait = drainNodeSkipWait
		totalTimeout = drainNodeTimeout
	}

	By(fmt.Sprintf("Draining node %q with timeout=%v",
		nodeToDrain.Definition.Name, totalTimeout))

	// Configure drain with calculated timeout
	nodeToDrain.SetDrainHelper(
		true,         // force: allow standalone pods
		true,         // ignoreDaemonsets: required for OpenShift
		true,         // deleteLocalData: required for emptyDir volumes
		gracePeriod,  // gracePeriod: calculated from pod grace periods
		skipWait,     // skipWaitForDelete: calculated as gracePeriod/2
		totalTimeout, // timeout: calculated as gracePeriod + 5min buffer
	)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Drain configuration for node %q - timeout=%v, gracePeriod=%ds, skipWait=%ds (dynamically calculated)",
		nodeToDrain.Definition.Name, totalTimeout, gracePeriod, skipWait)

	startTime := time.Now()

	var lastErr error

	// Retry for transient failures (gRPC keepalive timeouts, etc.)
	err := wait.PollUntilContextTimeout(ctx, 15*time.Second,
		drainNodeRetryTimeout, true,
		func(context.Context) (bool, error) {
			lastErr = nodeToDrain.Drain()
			if lastErr == nil {
				duration := time.Since(startTime)
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Successfully drained node %q in %v",
					nodeToDrain.Definition.Name, duration)

				return true, nil
			}

			// Check if error is retryable (gRPC/network issues)
			errorMsg := lastErr.Error()
			if strings.Contains(errorMsg, "keepalive") ||
				strings.Contains(errorMsg, "Unavailable") ||
				strings.Contains(errorMsg, "connection refused") {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Drain failed with retryable error, will retry: %v", lastErr)

				return false, nil // Retry
			}

			// Non-retryable error - fail immediately
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Drain failed with non-retryable error: %v", lastErr)

			return false, lastErr
		})
	if err != nil {
		duration := time.Since(startTime)

		return fmt.Errorf("failed to drain node %q after %v: %w",
			nodeToDrain.Definition.Name, duration, lastErr)
	}

	return lastErr
}
