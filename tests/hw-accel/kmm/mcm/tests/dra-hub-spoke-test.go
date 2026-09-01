package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	moduleV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm/v1beta1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/mcm/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var _ = Describe("KMM-HUB", Ordered, Label(tsparams.LabelSuite), func() {
	Context("MCM DRA", Label("mcm-dra"), func() {
		image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
			kmmparams.LocalImageRegistry, kmmparams.KmmHubOperatorNamespace, "dra-hub-kmod")

		It("should accept MCM with valid DRA spec on hub (version gate skipped)",
			reportxml.ID("89711"), func() {
				By("Build DRA container config")

				draContainerCfg, err := kmm.NewDRAContainerBuilder(kmmparams.DRATestImage).
					WithCommand([]string{"dra-example-kubeletplugin"}).
					WithEnv("DRIVER_NAME", kmmparams.DRADriverName).
					GetDRAContainerConfig()
				Expect(err).ToNot(HaveOccurred(), "error creating DRA container config")

				By("Build Module spec with valid DRA")

				moduleSpec, err := define.ModuleSpec(APIClient, "dra-base",
					kmmparams.KmmHubOperatorNamespace, "dra-hub", image, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error building base module spec")

				moduleSpec.DRA = &moduleV1Beta1.DRASpec{
					DriverName:         kmmparams.DRADriverName,
					Container:          *draContainerCfg,
					ServiceAccountName: "dra-test-sa",
					DeviceClasses: []moduleV1Beta1.DeviceClassSpec{
						{Name: kmmparams.DRADeviceClassName},
					},
				}

				By("Create ManagedClusterModule targeting nonexistent spoke")

				mcm, err := kmm.NewManagedClusterModuleBuilder(APIClient, "test-hub-dra",
					kmmparams.KmmHubOperatorNamespace).
					WithModuleSpec(moduleSpec).
					WithSpokeNamespace(kmmparams.KmmOperatorNamespace).
					WithSelector(kmmparams.KmmNonexistentSpokeSelector).
					Create()
				Expect(err).ToNot(HaveOccurred(),
					"valid DRA MCM should be accepted by hub webhook (version gate skipped)")

				DeferCleanup(func() { _, _ = mcm.Delete() })

				By("Verify MCM has correct DRA configuration")

				Expect(mcm.Definition.Spec.ModuleSpec.DRA).ToNot(BeNil(),
					"DRA spec should be present in MCM")
				Expect(mcm.Definition.Spec.ModuleSpec.DRA.DriverName).To(
					Equal(kmmparams.DRADriverName),
					"DRA driverName should match")
				Expect(mcm.Definition.Spec.ModuleSpec.DRA.Container.Image).To(
					Equal(kmmparams.DRATestImage),
					"DRA container image should match")
				Expect(mcm.Definition.Spec.ModuleSpec.DRA.ServiceAccountName).To(
					Equal("dra-test-sa"),
					"DRA serviceAccountName should match")
				Expect(mcm.Definition.Spec.ModuleSpec.DRA.DeviceClasses).To(HaveLen(1),
					"DRA should have one DeviceClass")
				Expect(mcm.Definition.Spec.ModuleSpec.DRA.DeviceClasses[0].Name).To(
					Equal(kmmparams.DRADeviceClassName),
					"DeviceClass name should match")
			})

		It("should reject MCM with invalid DRA driverName",
			reportxml.ID("89711"), func() {
				By("Build Module spec with invalid DRA driverName")

				moduleSpec, err := define.ModuleSpec(APIClient, "dra-base",
					kmmparams.KmmHubOperatorNamespace, "dra-hub", image, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error building base module spec")

				moduleSpec.DRA = &moduleV1Beta1.DRASpec{
					DriverName: "INVALID DRIVER NAME WITH SPACES",
					Container:  moduleV1Beta1.CommonContainerSpec{Image: kmmparams.DRATestImage},
				}

				By("Create ManagedClusterModule")

				_, err = kmm.NewManagedClusterModuleBuilder(APIClient, "test-hub-invalid-driver",
					kmmparams.KmmHubOperatorNamespace).
					WithModuleSpec(moduleSpec).
					WithSpokeNamespace(kmmparams.KmmOperatorNamespace).
					WithSelector(kmmparams.KmmNonexistentSpokeSelector).
					Create()
				Expect(err).To(HaveOccurred(),
					"MCM with invalid DRA driverName should be rejected")
				Expect(err.Error()).To(ContainSubstring("driverName"),
					"error should mention driverName")
			})

		It("should reject MCM with duplicate DRA deviceClass names",
			reportxml.ID("89711"), func() {
				By("Build Module spec with duplicate deviceClass names")

				moduleSpec, err := define.ModuleSpec(APIClient, "dra-base",
					kmmparams.KmmHubOperatorNamespace, "dra-hub", image, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error building base module spec")

				moduleSpec.DRA = &moduleV1Beta1.DRASpec{
					DriverName: kmmparams.DRADriverName,
					Container:  moduleV1Beta1.CommonContainerSpec{Image: kmmparams.DRATestImage},
					DeviceClasses: []moduleV1Beta1.DeviceClassSpec{
						{Name: "my-class"},
						{Name: "my-class"},
					},
				}

				By("Create ManagedClusterModule")

				_, err = kmm.NewManagedClusterModuleBuilder(APIClient, "test-hub-dup-classes",
					kmmparams.KmmHubOperatorNamespace).
					WithModuleSpec(moduleSpec).
					WithSpokeNamespace(kmmparams.KmmOperatorNamespace).
					WithSelector(kmmparams.KmmNonexistentSpokeSelector).
					Create()
				Expect(err).To(HaveOccurred(),
					"MCM with duplicate deviceClass names should be rejected")
				Expect(err.Error()).To(ContainSubstring("duplicate"),
					"error should mention duplicate")
			})
	})
})
