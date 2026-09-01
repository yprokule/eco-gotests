package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/resource"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/modules/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("KMM", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity), func() {
	Context("DRA Scheduling", Label("dra", "dra-scheduling"), func() {
		nSpace := kmmparams.DRASchedulingTestNamespace
		moduleName := "dra-sched-test"
		kmodName := "schedmod"
		moduleServiceAccount := "sched-manager"
		draServiceAccount := "dra-driver-sa"
		deviceClassName := "test-scheduling-class"
		claimTemplateName := "test-claim"
		consumerPodName := "dra-consumer-test"
		draClusterRoleName := "dra-driver-89712"
		image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
			tsparams.LocalImageRegistry, nSpace, kmodName)
		buildArgValue := fmt.Sprintf("%s.o", kmodName)

		BeforeAll(func() {
			if kmmparams.DRADriverImage == "" {
				Skip("ECO_HWACCEL_KMM_DRA_DRIVER_IMAGE_REPO is not set")
			}

			By("Create Namespace")

			_, err := namespace.NewBuilder(APIClient, nSpace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

			By("Create module ServiceAccount and CRB")

			moduleSvcAccount, err := serviceaccount.
				NewBuilder(APIClient, moduleServiceAccount, nSpace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module serviceaccount")

			crbBuilder := define.ModuleCRB(*moduleSvcAccount, kmodName)
			_, err = crbBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module CRB")

			By("Create DRA driver ServiceAccount")

			draSvcAccount, err := serviceaccount.
				NewBuilder(APIClient, draServiceAccount, nSpace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating DRA driver serviceaccount")

			By("Create DRA driver privileged SCC CRB")

			draSccCRB := define.ModuleCRB(*draSvcAccount, "dra-driver")
			_, err = draSccCRB.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating DRA driver SCC CRB")

			By("Create DRA driver ClusterRole for resource.k8s.io RBAC")

			draClusterRole := define.DRADriverClusterRole(draClusterRoleName)
			_, err = draClusterRole.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating DRA driver ClusterRole")

			By("Create DRA driver ClusterRoleBinding for resource.k8s.io")

			draCRB := define.DRADriverCRB(draClusterRoleName+"-binding",
				draClusterRoleName, draServiceAccount, nSpace)
			_, err = draCRB.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating DRA driver CRB")

			By("Create ConfigMap")

			configmapContents := define.MultiStageConfigMapContent(kmodName)
			_, err = configmap.NewBuilder(APIClient, kmodName, nSpace).
				WithData(configmapContents).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating configmap")

			By("Create Module with moduleLoader and DRA (with DeviceClass selector)")

			module := define.UnstructuredModule(moduleName, nSpace, map[string]interface{}{
				"selector":     GeneralConfig.WorkerLabelMap,
				"moduleLoader": define.ModuleLoaderSpec(kmodName, image, buildArgValue, moduleServiceAccount),
				"dra":          define.DRASpecWithCELSelector(draServiceAccount, []string{deviceClassName}, nil),
			})

			err = APIClient.Create(context.TODO(), module)
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await build pod to complete")

			err = await.BuildPodCompleted(APIClient, nSpace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building module")

			By("Await module deployment")

			err = await.ModuleDeployment(APIClient, moduleName, nSpace,
				2*time.Minute, GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on module deployment")

			By("Await DRA deployment")

			err = await.DRADeployment(APIClient, moduleName, nSpace,
				3*time.Minute, GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on DRA deployment")
		})

		AfterAll(func() {
			By("Delete consumer pod")

			_, _ = pod.NewBuilder(APIClient, consumerPodName, nSpace,
				kmmparams.UBIMinimalImage).DeleteAndWait(time.Minute)

			By("Delete ResourceClaims")

			claims, listErr := APIClient.K8sClient.ResourceV1().ResourceClaims(nSpace).List(
				context.TODO(), metav1.ListOptions{})
			if listErr == nil {
				for _, claim := range claims.Items {
					_ = APIClient.K8sClient.ResourceV1().ResourceClaims(nSpace).Delete(
						context.TODO(), claim.Name, metav1.DeleteOptions{})
				}
			}

			By("Delete ResourceClaimTemplate")

			_, _ = resource.NewResourceClaimTemplateBuilder(
				APIClient, claimTemplateName, nSpace).Delete()

			By("Cleanup modules")

			err := await.CleanupModules(APIClient, []string{moduleName}, nSpace)
			Expect(err).ToNot(HaveOccurred(), "error cleaning up modules")

			By("Delete DRA driver ClusterRole and CRBs")

			draCRB := define.DRADriverCRB(draClusterRoleName+"-binding",
				draClusterRoleName, draServiceAccount, nSpace)
			_ = draCRB.Delete()

			draClusterRole := define.DRADriverClusterRole(draClusterRoleName)
			_ = draClusterRole.Delete()

			if moduleSvcAccount, pullErr := serviceaccount.Pull(
				APIClient, moduleServiceAccount, nSpace); pullErr == nil {
				moduleCRB := define.ModuleCRB(*moduleSvcAccount, kmodName)
				_ = moduleCRB.Delete()
			}

			if draSvcAccount, pullErr := serviceaccount.Pull(
				APIClient, draServiceAccount, nSpace); pullErr == nil {
				draSccCRB := define.ModuleCRB(*draSvcAccount, "dra-driver")
				_ = draSccCRB.Delete()
			}

			By("Delete Namespace")

			err = namespace.NewBuilder(APIClient, nSpace).DeleteAndWait(2 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error deleting namespace")
		})

		It("should schedule consumer pod on DRA-enabled node via ResourceClaimTemplate",
			reportxml.ID("89712"), func() {
				By("Verify ResourceSlices are published by the DRA driver")

				err := await.ResourceSlicesPublished(APIClient, kmmparams.DRADriverName, 3*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "DRA driver should publish ResourceSlices")

				By("Create ResourceClaimTemplate")

				rct, err := resource.NewResourceClaimTemplateBuilder(
					APIClient, claimTemplateName, nSpace).
					WithDeviceRequest("test-device", deviceClassName, 1).
					Create()
				Expect(err).ToNot(HaveOccurred(), "error creating ResourceClaimTemplate")
				Expect(rct.Object).ToNot(BeNil(), "ResourceClaimTemplate should be created")

				By("Create consumer pod with ResourceClaim")

				consumerPod, err := pod.NewBuilder(APIClient, consumerPodName, nSpace,
					kmmparams.UBIMinimalImage).
					WithCommand([]string{"sh", "-c", "echo 'DRA device allocated' && sleep 3600"}).
					WithResourceClaim("my-device", claimTemplateName).
					WithRestartPolicy(corev1.RestartPolicyNever).
					CreateAndWaitUntilRunning(5 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "consumer pod should be running")
				Expect(consumerPod.Exists()).To(BeTrue(),
					"consumer pod object should exist after reaching Running")
				Expect(consumerPod.Object.Status.Phase).To(Equal(corev1.PodRunning),
					"consumer pod should be in Running phase")

				By("Verify ResourceClaim is allocated")

				allocated, err := check.ResourceClaimAllocated(APIClient, nSpace)
				Expect(err).ToNot(HaveOccurred(), "error checking ResourceClaim allocation")
				Expect(allocated).To(BeTrue(),
					"at least one ResourceClaim should be allocated")
			})
	})
})
