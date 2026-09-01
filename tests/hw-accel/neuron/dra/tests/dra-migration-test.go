package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/neuron"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/resource"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/dra/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/do"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronhelpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const (
	migrationTestNS      = "neuron-dra-migration-test"
	migrationClaimTpl    = "neuron-migration-claim"
	migrationTimeout     = 10 * time.Minute
	migrationPollTimeout = 5 * time.Minute
)

var _ = Describe("Neuron DRA Migration Tests", Ordered,
	Label(params.Label, params.DRALabel, params.DRAMigrationLabel), func() {
		// DRA-8: Device-Plugin to DRA Migration
		Context("Device-plugin to DRA migration", Label(tsparams.LabelSuite), func() {
			neuronCfg := neuronconfig.NewNeuronConfig()

			BeforeAll(func() {
				if !neuronCfg.IsDRAMigrationConfigured() {
					Skip("DRA migration not configured - requires both device-plugin and DRA images")
				}

				By("Verifying all required operators are ready")

				var options *neuronhelpers.NeuronInstallConfigOptions
				if neuronCfg.CatalogSource != "" {
					options = &neuronhelpers.NeuronInstallConfigOptions{
						CatalogSource: neuronhelpers.StringPtr(neuronCfg.CatalogSource),
					}
				}

				Expect(neuronhelpers.AreAllOperatorsReady(APIClient, options)).To(BeTrue(),
					"All operators (NFD, KMM, Neuron) must be pre-installed and ready")

				By("Verifying Neuron nodes exist")

				exists, err := check.NeuronNodesExist(APIClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue(), "At least one Neuron node must exist")

				By("Cleaning up any existing DeviceConfig")

				if existingDC, _ := neuron.Pull(
					APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace); existingDC != nil {
					_, err := existingDC.Delete()
					Expect(err).ToNot(HaveOccurred())

					Eventually(func() bool {
						_, pullErr := neuron.Pull(
							APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)

						return pullErr != nil
					}, migrationPollTimeout, 5*time.Second).Should(BeTrue(),
						"Existing DeviceConfig should be deleted")
				}

				By("Creating device-plugin mode DeviceConfig")

				builder := neuron.NewBuilder(
					APIClient,
					params.DefaultDeviceConfigName,
					params.NeuronNamespace,
					neuronCfg.DriversImage,
					neuronCfg.DriverVersion,
					neuronCfg.DevicePluginImage,
				).WithSelector(map[string]string{
					params.NeuronNFDLabelKey: params.NeuronNFDLabelValue,
				}).WithScheduler(
					neuronCfg.SchedulerImage,
					neuronCfg.SchedulerExtensionImage,
				).WithNodeMetricsImage(neuronCfg.NodeMetricsImage)

				if neuronCfg.ImageRepoSecretName != "" {
					builder = builder.WithImageRepoSecret(neuronCfg.ImageRepoSecretName)
				}

				_, err = builder.Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create device-plugin mode DeviceConfig")

				By("Waiting for device-plugin mode to be fully ready")

				err = neuronhelpers.WaitForClusterStabilityAfterDeviceConfig(APIClient)
				Expect(err).ToNot(HaveOccurred())

				err = await.DevicePluginDeployment(
					APIClient, params.NeuronNamespace, migrationTimeout)
				Expect(err).ToNot(HaveOccurred(), "Device-plugin DaemonSet should be ready")

				err = await.SchedulerDeployment(
					APIClient, params.NeuronNamespace, migrationTimeout)
				Expect(err).ToNot(HaveOccurred(), "Custom scheduler should be ready")
			})

			It("should have device-plugin and scheduler running before migration",
				reportxml.ID("90500"), func() {
					By("Verifying device-plugin DaemonSet exists")

					dsList, err := APIClient.K8sClient.AppsV1().DaemonSets(
						params.NeuronNamespace).List(
						context.TODO(), metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred())

					dpFound := false

					for _, ds := range dsList.Items {
						if strings.HasPrefix(ds.Name, params.DevicePluginDaemonSetPrefix) {
							dpFound = true
							Expect(int(ds.Status.NumberReady)).To(BeNumerically(">", 0),
								"Device-plugin DaemonSet should have ready pods")
						}
					}

					Expect(dpFound).To(BeTrue(), "Device-plugin DaemonSet should exist")

					By("Verifying custom scheduler deployment exists")

					deployList, err := APIClient.K8sClient.AppsV1().Deployments(
						params.NeuronNamespace).List(
						context.TODO(), metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred())

					schedulerFound := false

					for _, deploy := range deployList.Items {
						if strings.Contains(deploy.Name, "scheduler") {
							schedulerFound = true
						}
					}

					Expect(schedulerFound).To(BeTrue(), "Custom scheduler should exist")

					By("Verifying no DRA DaemonSet exists")

					draDSList, err := APIClient.K8sClient.AppsV1().DaemonSets(
						params.NeuronNamespace).List(
						context.TODO(), metav1.ListOptions{
							LabelSelector: fmt.Sprintf("%s=%s",
								params.DRADaemonSetLabelKey, params.DRADaemonSetLabelValue),
						})
					Expect(err).ToNot(HaveOccurred())
					Expect(draDSList.Items).To(BeEmpty(),
						"DRA DaemonSet should not exist in device-plugin mode")
				})

			It("should migrate to DRA mode by updating DeviceConfig",
				reportxml.ID("90501"), func() {
					neuronCfg := neuronconfig.NewNeuronConfig()

					By("Pulling DeviceConfig and switching to DRA mode")

					dcBuilder, err := neuron.Pull(
						APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
					Expect(err).ToNot(HaveOccurred())

					dcBuilder.Definition.Spec.DevicePluginImage = ""
					dcBuilder.Definition.Spec.CustomSchedulerImage = ""
					dcBuilder.Definition.Spec.SchedulerExtensionImage = ""
					dcBuilder.Definition.Spec.DRADriverImage = neuronCfg.DRADriverImage

					_, err = dcBuilder.Update(false)
					Expect(err).ToNot(HaveOccurred(), "Failed to update DeviceConfig to DRA mode")

					By("Waiting for device-plugin DaemonSet to be removed")

					err = await.DevicePluginDaemonSetGone(
						APIClient, params.NeuronNamespace, migrationPollTimeout)
					Expect(err).ToNot(HaveOccurred(),
						"Device-plugin DaemonSet should be removed after migration")

					By("Waiting for scheduler deployments to be removed")

					err = await.NoSchedulerDeployments(
						APIClient, params.NeuronNamespace, migrationPollTimeout)
					Expect(err).ToNot(HaveOccurred(),
						"Scheduler deployments should be removed after migration")

					By("Waiting for DRA DaemonSet to be ready")

					err = await.DRADaemonSet(
						APIClient, params.NeuronNamespace, migrationTimeout)
					Expect(err).ToNot(HaveOccurred(),
						"DRA DaemonSet should be ready after migration")

					klog.V(params.NeuronLogLevel).Info(
						"Migration to DRA mode complete")
				})

			It("should have DeviceClass created after migration to DRA",
				reportxml.ID("90502"), func() {
					By("Waiting for DeviceClass to exist")

					err := await.DeviceClassExists(
						APIClient, params.DRADefaultDeviceClassName, tsparams.DeviceClassTimeout)
					Expect(err).ToNot(HaveOccurred(),
						"DeviceClass %s should exist after migration", params.DRADefaultDeviceClassName)

					By("Verifying DeviceClass has KMM ownership labels")

					dcBuilder, err := resource.PullDeviceClass(
						APIClient, params.DRADefaultDeviceClassName)
					Expect(err).ToNot(HaveOccurred())

					labels := dcBuilder.Object.Labels
					Expect(labels).To(HaveKeyWithValue(
						"kmm.node.kubernetes.io/module.name", params.DefaultDeviceConfigName))
				})

			It("should have ResourceSlices published after migration to DRA",
				reportxml.ID("90503"), func() {
					By("Listing ResourceSlices for neuron.aws.com driver")

					slices, err := resource.ListResourceSlicesByDriver(
						APIClient, params.DRADriverName)
					Expect(err).ToNot(HaveOccurred())
					Expect(slices).ToNot(BeEmpty(),
						"ResourceSlices should be published after migration to DRA")

					klog.V(params.NeuronLogLevel).Infof(
						"Found %d ResourceSlices after migration", len(slices))
				})

			It("should allocate a device via DRA after migration",
				reportxml.ID("90504"), func() {
					By("Creating test namespace and ResourceClaimTemplate")

					nsBuilder := namespace.NewBuilder(APIClient, migrationTestNS)
					if !nsBuilder.Exists() {
						_, err := nsBuilder.Create()
						Expect(err).ToNot(HaveOccurred())
					}

					rctBuilder := resource.NewResourceClaimTemplateBuilder(
						APIClient, migrationClaimTpl, migrationTestNS).
						WithDeviceRequest("neuron-device", params.DRADefaultDeviceClassName, 1)
					Expect(rctBuilder).ToNot(BeNil())

					if !rctBuilder.Exists() {
						_, err := rctBuilder.Create()
						Expect(err).ToNot(HaveOccurred())
					}

					By("Creating DRA consumer pod")

					err := do.CreateDRAConsumerPodAndWait(
						APIClient, "migration-consumer", migrationTestNS,
						migrationClaimTpl, migrationPollTimeout)
					Expect(err).ToNot(HaveOccurred(),
						"DRA consumer pod should be Running after migration")

					By("Verifying Neuron device is visible")

					hasDevices, err := check.PodHasNeuronDevices(
						APIClient, "migration-consumer", migrationTestNS)
					Expect(err).ToNot(HaveOccurred())
					Expect(hasDevices).To(BeTrue(),
						"Pod should have /dev/neuron* device after migration to DRA")

					By("Cleaning up test namespace")

					nsBuilder = namespace.NewBuilder(APIClient, migrationTestNS)
					if nsBuilder.Exists() {
						err = nsBuilder.DeleteAndWait(migrationPollTimeout)
						Expect(err).ToNot(HaveOccurred())
					}
				})
		})

		// DRA-10: DRA to Device-Plugin Rollback
		Context("DRA to device-plugin rollback", Label(tsparams.LabelSuite), func() {
			neuronCfg := neuronconfig.NewNeuronConfig()

			BeforeAll(func() {
				if !neuronCfg.IsDRAMigrationConfigured() {
					Skip("DRA migration not configured - requires both device-plugin and DRA images")
				}

				By("Verifying DeviceConfig is in DRA mode from previous context")

				dcBuilder, err := neuron.Pull(
					APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
				Expect(err).ToNot(HaveOccurred(), "DeviceConfig must exist")
				Expect(dcBuilder.Definition.Spec.DRADriverImage).ToNot(BeEmpty(),
					"DeviceConfig must be in DRA mode (set by previous migration context)")

				By("Verifying DRA DaemonSet is running")

				err = await.DRADaemonSet(
					APIClient, params.NeuronNamespace, migrationTimeout)
				Expect(err).ToNot(HaveOccurred(), "DRA DaemonSet must be ready before rollback")
			})

			It("should rollback to device-plugin mode by updating DeviceConfig",
				reportxml.ID("90505"), func() {
					By("Pulling DeviceConfig and switching to device-plugin mode")

					dcBuilder, err := neuron.Pull(
						APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
					Expect(err).ToNot(HaveOccurred())

					dcBuilder.Definition.Spec.DRADriverImage = ""
					dcBuilder.Definition.Spec.DevicePluginImage = neuronCfg.DevicePluginImage
					dcBuilder.Definition.Spec.CustomSchedulerImage = neuronCfg.SchedulerImage
					dcBuilder.Definition.Spec.SchedulerExtensionImage = neuronCfg.SchedulerExtensionImage

					_, err = dcBuilder.Update(false)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to update DeviceConfig to device-plugin mode")

					By("Waiting for DRA DaemonSet to be removed")

					err = await.DRADaemonSetGone(
						APIClient, params.NeuronNamespace, migrationPollTimeout)
					Expect(err).ToNot(HaveOccurred(),
						"DRA DaemonSet should be removed after rollback")

					By("Waiting for DeviceClass to be removed")

					err = await.DeviceClassGone(
						APIClient, params.DRADefaultDeviceClassName, tsparams.DeviceClassTimeout)
					Expect(err).ToNot(HaveOccurred(),
						"DeviceClass should be removed after rollback")

					By("Waiting for ResourceSlices to be cleaned up")

					err = await.ResourceSlicesGone(
						APIClient, params.DRADriverName, migrationPollTimeout)
					Expect(err).ToNot(HaveOccurred(),
						"ResourceSlices should be removed after rollback")

					klog.V(params.NeuronLogLevel).Info("DRA resources cleaned up after rollback")
				})

			It("should have device-plugin DaemonSet running after rollback",
				reportxml.ID("90506"), func() {
					By("Waiting for device-plugin DaemonSet to be ready")

					err := await.DevicePluginDeployment(
						APIClient, params.NeuronNamespace, migrationTimeout)
					Expect(err).ToNot(HaveOccurred(),
						"Device-plugin DaemonSet should be ready after rollback")

					By("Verifying device-plugin pods are running on Neuron nodes")

					neuronNodes, err := check.GetNeuronNodes(APIClient)
					Expect(err).ToNot(HaveOccurred())

					dsList, err := APIClient.K8sClient.AppsV1().DaemonSets(
						params.NeuronNamespace).List(
						context.TODO(), metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred())

					dpFound := false

					for _, ds := range dsList.Items {
						if strings.HasPrefix(ds.Name, params.DevicePluginDaemonSetPrefix) {
							dpFound = true
							Expect(int(ds.Status.NumberReady)).To(Equal(len(neuronNodes)),
								"Device-plugin should have one ready pod per Neuron node")
						}
					}

					Expect(dpFound).To(BeTrue(),
						"Device-plugin DaemonSet should exist after rollback")
				})

			It("should have custom scheduler running after rollback",
				reportxml.ID("90507"), func() {
					By("Waiting for scheduler deployment to be ready")

					err := await.SchedulerDeployment(
						APIClient, params.NeuronNamespace, migrationTimeout)
					Expect(err).ToNot(HaveOccurred(),
						"Custom scheduler should be ready after rollback")

					By("Verifying scheduler extension exists")

					deployList, err := APIClient.K8sClient.AppsV1().Deployments(
						params.NeuronNamespace).List(
						context.TODO(), metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred())

					extensionFound := false

					for _, deploy := range deployList.Items {
						if strings.HasPrefix(deploy.Name,
							params.SchedulerExtensionDeploymentPrefix) {
							extensionFound = true
						}
					}

					Expect(extensionFound).To(BeTrue(),
						"Scheduler extension deployment should exist after rollback")
				})

			It("should have Neuron device resources on nodes after rollback",
				reportxml.ID("90508"), func() {
					By("Waiting for all Neuron nodes to have device resources")

					err := await.AllNeuronNodesResourceAvailable(
						APIClient, migrationTimeout)
					Expect(err).ToNot(HaveOccurred(),
						"Neuron device resources should be available after rollback")
				})

			It("should NOT have any DRA resources after rollback",
				reportxml.ID("90509"), func() {
					By("Verifying no DRA DaemonSet exists")

					draDSList, err := APIClient.K8sClient.AppsV1().DaemonSets(
						params.NeuronNamespace).List(
						context.TODO(), metav1.ListOptions{
							LabelSelector: fmt.Sprintf("%s=%s",
								params.DRADaemonSetLabelKey, params.DRADaemonSetLabelValue),
						})
					Expect(err).ToNot(HaveOccurred())
					Expect(draDSList.Items).To(BeEmpty(),
						"DRA DaemonSet should not exist after rollback")

					By("Verifying no DeviceClass exists")

					_, err = APIClient.K8sClient.ResourceV1().DeviceClasses().Get(
						context.TODO(), params.DRADefaultDeviceClassName, metav1.GetOptions{})
					Expect(err).To(HaveOccurred(),
						"DeviceClass should not exist after rollback")

					By("Verifying no ResourceSlices for neuron driver")

					sliceList, err := APIClient.K8sClient.ResourceV1().ResourceSlices().List(
						context.TODO(), metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred())

					for idx := range sliceList.Items {
						Expect(sliceList.Items[idx].Spec.Driver).ToNot(Equal(params.DRADriverName),
							"No ResourceSlice should have driver %s after rollback",
							params.DRADriverName)
					}
				})
		})
	})
