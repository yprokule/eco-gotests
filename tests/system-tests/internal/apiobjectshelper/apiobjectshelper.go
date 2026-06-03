package apiobjectshelper

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/csv"
	"k8s.io/apimachinery/pkg/util/wait"
)

// VerifyNamespaceExists asserts specific namespace exists.
func VerifyNamespaceExists(apiClient *clients.Settings, nsname string, timeout time.Duration) error {
	glog.V(90).Infof("Verify namespace %q exists", nsname)

	err := wait.PollUntilContextTimeout(context.TODO(), time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			_, pullErr := namespace.Pull(apiClient, nsname)
			if pullErr != nil {
				glog.V(90).Infof("Failed to pull in namespace %q - %v", nsname, pullErr)

				return false, nil
			}

			return true, nil
		})

	if err != nil {
		return fmt.Errorf("failed to pull in %s namespace", nsname)
	}

	return nil
}

// VerifyOperatorDeployment assert that specific deployment succeeded.
func VerifyOperatorDeployment(apiClient *clients.Settings,
	subscriptionName, deploymentName, nsname string, timeout time.Duration) error {
	glog.V(90).Infof("Verify deployment %s in namespace %s", deploymentName, nsname)

	if deploymentName == "" {
		return fmt.Errorf("operator deployment name have to be provided")
	}

	if subscriptionName != "" {
		csvName, err := csv.GetCurrentCSVNameFromSubscription(apiClient, subscriptionName, nsname)

		if err != nil {
			return fmt.Errorf("csv %s not found in namespace %s", csvName, nsname)
		}

		csvObj, err := olm.PullClusterServiceVersion(apiClient, csvName, nsname)

		if err != nil {
			return fmt.Errorf("failed to pull %q csv from the %s namespace", csvName, nsname)
		}

		isSuccessful, err := csvObj.IsSuccessful()

		if err != nil {
			return fmt.Errorf("failed to verify csv %s in the namespace %s status", csvName, nsname)
		}

		if !isSuccessful {
			return fmt.Errorf("failed to deploy %s; the csv %s in the namespace %s status %v",
				subscriptionName, csvName, nsname, isSuccessful)
		}
	}

	glog.V(90).Infof("Confirm that operator %s is running in namespace %s", deploymentName, nsname)

	err := await.WaitUntilDeploymentReady(apiClient, deploymentName, nsname, timeout)

	if err != nil {
		return fmt.Errorf("deployment %s not found in %s namespace; %w", deploymentName, nsname, err)
	}

	return nil
}

// CreateServiceAccount creates the service account and verifies it was created.
func CreateServiceAccount(apiClient *clients.Settings, saName, nsName string) error {
	glog.V(100).Infof(fmt.Sprintf("Creating ServiceAccount %q in %q namespace",
		saName, nsName))
	glog.V(100).Infof("Creating SA %q in %q namespace", saName, nsName)

	deploySa := serviceaccount.NewBuilder(apiClient, saName, nsName)

	err := wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*15,
		time.Minute,
		true,
		func(ctx context.Context) (bool, error) {
			deploySa, err := deploySa.Create()

			if err != nil {
				glog.V(100).Infof("Error creating SA %q in %q namespace: %v", saName, nsName, err)

				return false, nil
			}

			glog.V(100).Infof("Created SA %q in %q namespace",
				deploySa.Definition.Name, deploySa.Definition.Namespace)

			return true, nil
		})

	if err != nil {
		return fmt.Errorf("failed to create ServiceAccount %q in %q namespace", saName, nsName)
	}

	return nil
}

// CreateClusterRBAC creates the RBAC and verifies it was created.
func CreateClusterRBAC(
	apiClient *clients.Settings,
	rbacName, clusterRole, saName, nsName string) error {
	glog.V(100).Infof("Creating RBAC for SA %s", saName)

	glog.V(100).Infof("Creating ClusterRoleBinding %q", rbacName)
	crbSa := rbac.NewClusterRoleBindingBuilder(
		apiClient,
		rbacName,
		clusterRole,
		rbacv1.Subject{
			Name:      saName,
			Kind:      "ServiceAccount",
			Namespace: nsName,
		})

	err := wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*15,
		time.Minute,
		true,
		func(ctx context.Context) (bool, error) {
			crbSa, err := crbSa.Create()
			if err != nil {
				glog.V(100).Infof(
					"Error Creating ClusterRoleBinding %q : %v", crbSa.Definition.Name, err)

				return false, nil
			}

			glog.V(100).Infof("ClusterRoleBinding %q created:\n\t%v",
				crbSa.Definition.Name, crbSa)

			return true, nil
		})

	if err != nil {
		return fmt.Errorf("failed to create ClusterRoleBinding '%s' during timeout %v; %w",
			rbacName, time.Minute, err)
	}

	return nil
}

// DeleteService deletes the service and verifies it was removed.
func DeleteService(apiClient *clients.Settings, svcName, nsName string) error {
	glog.V(100).Infof("Delete service %q from namespace %s", svcName, nsName)

	if svcObj, err := service.Pull(
		apiClient, svcName, nsName); err == nil {
		glog.V(100).Infof("Service %q found in %q nsname", svcName, nsName)
		glog.V(100).Infof("Deleting service %q in %q nsname", svcName, nsName)

		err = wait.PollUntilContextTimeout(
			context.TODO(),
			time.Second*15,
			time.Minute,
			true,
			func(ctx context.Context) (bool, error) {
				err := svcObj.Delete()

				if err != nil {
					glog.V(100).Infof("Error deleting service %q in %q nsname: %v",
						svcName, nsName, err)

					return false, nil
				}

				glog.V(100).Infof("Deleted service %q in %q nsname", svcName, nsName)

				return true, nil
			})

		if err != nil {
			return fmt.Errorf("failed to delete service %q from %q ns", svcName, nsName)
		}
	} else {
		glog.V(100).Infof("service %q not found in %q nsname", svcName, nsName)
	}

	return nil
}

// DeleteClusterRBAC deletes the RBAC and verifies it was removed.
func DeleteClusterRBAC(apiClient *clients.Settings, rbacName string) error {
	glog.V(100).Infof("Deleting Cluster RBAC")

	glog.V(100).Infof("Assert ClusterRoleBinding %q exists", rbacName)

	crbSa, err := rbac.PullClusterRoleBinding(apiClient, rbacName)

	if err != nil {
		glog.V(100).Infof("ClusterRoleBinding %q not found; %v", rbacName, err)

		return nil
	}

	glog.V(100).Infof("ClusterRoleBinding %q found. Deleting...", rbacName)

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*15,
		time.Minute,
		true,
		func(ctx context.Context) (bool, error) {
			err = crbSa.Delete()

			if err != nil {
				glog.V(100).Infof("Error deleting ClusterRoleBinding %q : %v", rbacName, err)

				return false, nil
			}

			glog.V(100).Infof("Deleted ClusterRoleBinding %q", rbacName)

			return true, nil
		})

	if err != nil {
		return fmt.Errorf("failed to delete Cluster RBAC %q", rbacName)
	}

	return nil
}

// DeleteServiceAccount deletes the service account and verifies it was removed.
func DeleteServiceAccount(apiClient *clients.Settings, saName, nsName string) error {
	glog.V(100).Infof("Removing Service Account")
	glog.V(100).Infof("Assert SA %q exists in %q namespace", saName, nsName)

	if deploySa, err := serviceaccount.Pull(
		apiClient, saName, nsName); err == nil {
		glog.V(100).Infof("ServiceAccount %q found in %q namespace", saName, nsName)
		glog.V(100).Infof("Deleting ServiceAccount %q in %q namespace", saName, nsName)

		err = wait.PollUntilContextTimeout(
			context.TODO(),
			time.Second*15,
			time.Minute,
			true,
			func(ctx context.Context) (bool, error) {
				err := deploySa.Delete()

				if err != nil {
					glog.V(100).Infof("Error deleting ServiceAccount %q in %q namespace: %v",
						saName, nsName, err)

					return false, nil
				}

				glog.V(100).Infof("Deleted ServiceAccount %q in %q namespace", saName, nsName)

				return true, nil
			})

		if err != nil {
			return fmt.Errorf("failed to delete ServiceAccount %q from %q ns", saName, nsName)
		}
	} else {
		glog.V(100).Infof("ServiceAccount %q not found in %q namespace", saName, nsName)
	}

	return nil
}

// DeleteDeployment deletes the deployment and verifies it and all related pods were removed.
func DeleteDeployment(
	apiClient *clients.Settings,
	deploymentName, nsName string) error {
	glog.V(100).Infof("Removing test deployment %q from %q ns", deploymentName, nsName)

	if deploymentObj, err := deployment.Pull(apiClient, deploymentName, nsName); err == nil {
		glog.V(100).Infof("Deleting deployment %q from %q namespace", deploymentName, nsName)

		err = deploymentObj.DeleteAndWait(300 * time.Second)

		if err != nil {
			glog.V(100).Infof("Error deleting deployment %q from %q namespace: %v",
				deploymentName, nsName, err)

			return fmt.Errorf("failed to delete deployment %q from %q namespace: %w",
				deploymentName, nsName, err)
		}
	} else {
		glog.V(100).Infof("deployment %q not found in %q namespace", deploymentName, nsName)
	}

	return nil
}

// logStuckPodDiagnostics logs detailed diagnostic information about stuck pods.
func logStuckPodDiagnostics(stuckPods []*pod.Builder) {
	glog.V(100).Infof("Pod cleanup diagnostics - %d pods still present after 11min timeout:", len(stuckPods))

	for _, stuckPod := range stuckPods {
		glog.V(100).Infof("  Pod: %s", stuckPod.Definition.Name)
		glog.V(100).Infof("    Phase: %s", stuckPod.Object.Status.Phase)
		glog.V(100).Infof("    DeletionTimestamp: %v", stuckPod.Object.DeletionTimestamp)

		// Scenario 1: Finalizers blocking deletion
		if len(stuckPod.Object.Finalizers) > 0 {
			glog.V(100).Infof("    STUCK SCENARIO (Finalizers): Finalizers blocking deletion: %v",
				stuckPod.Object.Finalizers)
		}

		// Scenario 2: PreStop hook timeout (inferred from long deletion time)
		if stuckPod.Object.DeletionTimestamp != nil {
			deletionDuration := time.Since(stuckPod.Object.DeletionTimestamp.Time)
			if deletionDuration > 5*time.Minute {
				glog.V(100).Infof("    STUCK SCENARIO (PreStop): PreStop hook timeout suspected (deletion in progress for %v)",
					deletionDuration)
			}
		}

		// Scenario 3: CNI cleanup issues (inferred from network-related containers)
		for _, containerStatus := range stuckPod.Object.Status.ContainerStatuses {
			if containerStatus.State.Terminated == nil && containerStatus.State.Running == nil {
				if containerStatus.State.Waiting != nil &&
					(containerStatus.State.Waiting.Reason == "ContainerCreating" ||
						containerStatus.State.Waiting.Reason == "PodInitializing") {
					glog.V(100).Infof("    STUCK SCENARIO (CNI): CNI/network setup issue suspected (container in %s state)",
						containerStatus.State.Waiting.Reason)
				}
			}
		}

		// Log all data regardless
		glog.V(100).Infof("    Finalizers: %v", stuckPod.Object.Finalizers)

		// Log container states
		for _, containerStatus := range stuckPod.Object.Status.ContainerStatuses {
			glog.V(100).Infof("    Container: %s, Ready: %v, State: %+v",
				containerStatus.Name, containerStatus.Ready, containerStatus.State)
		}

		// Log conditions
		for _, cond := range stuckPod.Object.Status.Conditions {
			glog.V(100).Infof("    Condition: Type=%s, Status=%s, Reason=%s, Message=%s",
				cond.Type, cond.Status, cond.Reason, cond.Message)
		}
	}
}

// forceDeleteStuckPods attempts force deletion of stuck pods and waits for completion.
func forceDeleteStuckPods(
	apiClient *clients.Settings,
	nsName string,
	podLabel string,
	stuckPods []*pod.Builder,
	startTime time.Time) error {
	glog.V(100).Infof("Attempting force deletion for %d stuck pods", len(stuckPods))

	for _, stuckPod := range stuckPods {
		glog.V(100).Infof("Force deleting pod: %s", stuckPod.Definition.Name)

		// Use DeleteImmediate for force deletion (GracePeriodSeconds: 0)
		// This is safe for test cleanup after 11min timeout as:
		// 1. Pods are already terminating for >11min
		// 2. Test workloads don't require graceful shutdown
		// 3. Allows test suite to proceed rather than hanging indefinitely
		_, delErr := stuckPod.DeleteImmediate()
		if delErr != nil {
			glog.V(100).Infof("  Force deletion failed: %v", delErr)
		} else {
			glog.V(100).Infof("  Force deletion initiated successfully")
		}
	}

	// Poll for force deletion completion instead of blocking sleep
	glog.V(100).Infof("Waiting for force deletions to complete...")

	pollErr := wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*2,
		time.Second*30,
		true,
		func(ctx context.Context) (bool, error) {
			checkPods, _ := pod.List(apiClient, nsName, metav1.ListOptions{LabelSelector: podLabel})
			if len(checkPods) == 0 {
				glog.V(100).Infof("Force deletion succeeded - all pods removed in %v", time.Since(startTime))

				return true, nil
			}

			return false, nil
		})

	if pollErr == nil {
		// Force deletion succeeded
		return nil
	}

	// Verify final state after polling timeout
	remainingPods, verifyErr := pod.List(apiClient, nsName, metav1.ListOptions{LabelSelector: podLabel})
	if verifyErr != nil {
		glog.V(100).Infof("Failed to verify force deletion results: %v", verifyErr)

		return fmt.Errorf("pods matching label(%q) still present in namespace %q after force deletion, "+
			"verification failed: %w", podLabel, nsName, verifyErr)
	}

	if len(remainingPods) == 0 {
		glog.V(100).Infof("Force deletion succeeded - all pods removed")

		return nil
	}

	glog.V(100).Infof("Force deletion incomplete - %d pods still remain after 30s", len(remainingPods))

	return fmt.Errorf("pods matching label(%q) still present in namespace %q after force deletion attempt",
		podLabel, nsName)
}

// EnsureAllPodsRemoved Ensure all deployment pods in namespace with the specific pod label were removed.
func EnsureAllPodsRemoved(
	apiClient *clients.Settings,
	nsName, podLabel string) error {
	glog.V(100).Infof("Starting pod cleanup for label %q in namespace %q", podLabel, nsName)

	startTime := time.Now()

	err := wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*3,
		time.Minute*11, // Extended timeout for SRIOV pod-level bond workloads
		true,
		func(ctx context.Context) (bool, error) {
			oldPods, _ := pod.List(apiClient, nsName,
				metav1.ListOptions{LabelSelector: podLabel})

			return len(oldPods) == 0, nil
		})

	if err != nil {
		glog.V(100).Infof("Pod cleanup failed after %v: %v", time.Since(startTime), err)

		// Retrieve stuck pods for diagnostics
		stuckPods, listErr := pod.List(apiClient, nsName, metav1.ListOptions{LabelSelector: podLabel})
		if listErr != nil {
			glog.V(100).Infof("Failed to retrieve stuck pods for diagnostics: %v", listErr)

			return fmt.Errorf("pods matching label(%q) still present in namespace %q, "+
				"and failed to retrieve pod details: %w", podLabel, nsName, listErr)
		}

		if len(stuckPods) > 0 {
			logStuckPodDiagnostics(stuckPods)

			return forceDeleteStuckPods(apiClient, nsName, podLabel, stuckPods, startTime)
		}

		return fmt.Errorf("pods matching label(%q) still present in namespace %q", podLabel, nsName)
	}

	glog.V(100).Infof("Pod cleanup succeeded in %v", time.Since(startTime))

	return nil
}
