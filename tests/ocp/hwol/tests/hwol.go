package tests

import (
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/hwolenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type networkStatusEntry struct {
	Name string   `json:"name"`
	IPs  []string `json:"ips"`
}

var _ = Describe(
	"HWOL",
	Ordered,
	ContinueOnFailure,
	func() {
		var (
			operatorNS string
			mcpLabel   string
			pfName     string
		)

		BeforeAll(func() {
			operatorNS = HwolOcpConfig.OcpHwolOperatorNamespace
			mcpLabel = HwolOcpConfig.MCPLabel

			By("Loading HWOL device configuration from ECO_OCP_HWOL_DEVICES")

			devices, err := HwolOcpConfig.GetHwolDevices()
			Expect(err).ToNot(HaveOccurred(), "Failed to get HWOL devices")
			Expect(devices).ToNot(BeEmpty(), "No HWOL devices configured")
			pfName = devices[0].InterfaceName

			vfNum, err := HwolOcpConfig.GetVFNum()
			Expect(err).ToNot(HaveOccurred(), "Failed to get VF count")

			By("Ensuring switchdev foundation (operator config, pool, policy, wait)")

			Expect(hwolenv.EnsureSwitchdevFoundation(
				operatorNS, mcpLabel, devices[0], vfNum,
			)).To(Succeed(), "Failed to ensure switchdev foundation")
		})

		AfterAll(func() {
			By("Cleaning HWOL resources")

			err := hwolenv.CleanupHwolResources(
				operatorNS,
				mcpLabel,
				tsparams.CleanupWaitTimeout,
				tsparams.DefaultStableDuration,
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to clean HWOL resources")
		})

		It("verifies switchdev mode and managed OVS bridge on MCP nodes",
			Label(tsparams.LabelSwitchdev),
			reportxml.ID("85001"),
			func() {
				By("Asserting eSwitchMode=switchdev and OVS bridge uplink for PF")

				Expect(hwolenv.AssertSwitchdevAndOVSBridge(operatorNS, mcpLabel, pfName)).To(Succeed())
			})

		DescribeTable("attach path",
			func(cniType sriovoperator.CNIType, networkName string) {
				By(fmt.Sprintf("Creating %s network %s", cniType, networkName))

				err := createAttachNetwork(cniType, networkName, operatorNS)
				Expect(err).ToNot(HaveOccurred(), "Failed to create attach network")

				DeferCleanup(func() {
					By(fmt.Sprintf("Deleting %s network %s", cniType, networkName))

					Expect(deleteAttachNetwork(cniType, networkName, operatorNS)).To(Succeed(),
						"Failed to delete attach network")
					Expect(sriovoperator.WaitForNADDeletion(
						APIClient, networkName, tsparams.TestNamespaceName, tsparams.DefaultTimeout,
					)).To(Succeed(), "NAD was not deleted")
				})

				By("Waiting for NAD creation")

				Expect(sriovoperator.WaitForNADCreation(
					APIClient, networkName, tsparams.TestNamespaceName, tsparams.DefaultTimeout,
				)).To(Succeed(), "NAD was not created")

				By("Asserting NAD CNI type and resource annotation")

				Expect(sriovoperator.AssertNAD(
					APIClient,
					networkName,
					tsparams.TestNamespaceName,
					cniType,
					tsparams.ResourceNamePrefix+tsparams.ResourceName,
				)).To(Succeed(), "NAD assertion failed")

				By("Creating sleep pod with secondary network")

				attachPod, err := createAttachPod(fmt.Sprintf("hwol-attach-%s", cniType), networkName)
				Expect(err).ToNot(HaveOccurred(), "Failed to create attach pod")

				DeferCleanup(func() {
					By("Deleting attach pod")

					Expect(attachPod.Delete()).To(Succeed(), "Failed to delete attach pod")
				})

				By("Asserting pod network-status includes the attach NAD with an IP")

				Expect(attachPod.Object.Annotations).To(HaveKey("k8s.v1.cni.cncf.io/network-status"))
				Expect(assertNetworkStatusHasIP(
					attachPod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"],
					networkName,
				)).To(Succeed(), "network-status should reference attach NAD with an assigned IP")
			},
			Entry("ovs CNI", Label(tsparams.LabelOvsNetwork), reportxml.ID("85002"),
				sriovoperator.CNITypeOVS, tsparams.OvsNetworkName),
			Entry("sriov CNI", Label(tsparams.LabelSriovNetwork), reportxml.ID("85003"),
				sriovoperator.CNITypeSriov, tsparams.SriovNetworkName),
		)
	})

func createAttachNetwork(cniType sriovoperator.CNIType, networkName, operatorNS string) error {
	switch cniType {
	case sriovoperator.CNITypeOVS:
		_, err := hwolenv.CreateOvsNetwork(
			networkName, operatorNS, tsparams.ResourceName, tsparams.TestNamespaceName,
			hwolenv.HostLocalIPAM)

		return err
	case sriovoperator.CNITypeSriov:
		_, err := hwolenv.CreateSriovNetwork(
			networkName, operatorNS, tsparams.ResourceName, tsparams.TestNamespaceName,
			hwolenv.HostLocalIPAM)

		return err
	default:
		return fmt.Errorf("unsupported CNI type %q", cniType)
	}
}

// assertNetworkStatusHasIP checks Multus network-status for an entry matching networkName
// with at least one assigned IP (IPAM-agnostic: host-local, NV-IPAM, etc.).
func assertNetworkStatusHasIP(networkStatusJSON, networkName string) error {
	var entries []networkStatusEntry
	if err := json.Unmarshal([]byte(networkStatusJSON), &entries); err != nil {
		return fmt.Errorf("failed to parse network-status: %w", err)
	}

	for _, entry := range entries {
		if entry.Name == networkName ||
			strings.HasSuffix(entry.Name, "/"+networkName) ||
			strings.Contains(entry.Name, networkName) {
			if len(entry.IPs) > 0 {
				return nil
			}

			return fmt.Errorf("network-status entry %q has no ips", entry.Name)
		}
	}

	return fmt.Errorf("network-status has no entry for network %q", networkName)
}

func deleteAttachNetwork(cniType sriovoperator.CNIType, networkName, operatorNS string) error {
	switch cniType {
	case sriovoperator.CNITypeOVS:
		builder := hwolenv.NewOvsNetworkBuilder(
			APIClient, networkName, operatorNS, tsparams.TestNamespaceName, tsparams.ResourceName)
		if builder == nil {
			return fmt.Errorf("failed to init OVSNetwork builder for delete")
		}

		return builder.Delete()
	case sriovoperator.CNITypeSriov:
		builder, err := sriov.PullNetwork(APIClient, networkName, operatorNS)
		if err != nil {
			return nil
		}

		return builder.Delete()
	default:
		return fmt.Errorf("unsupported CNI type %q", cniType)
	}
}

func createAttachPod(podName, networkName string) (*pod.Builder, error) {
	resName := corev1.ResourceName(tsparams.ResourceNamePrefix + tsparams.ResourceName)
	secNetwork := pod.StaticIPAnnotation(networkName, nil)

	if secNetwork == nil {
		return nil, fmt.Errorf("failed to build secondary network annotation for %s", networkName)
	}

	builder := pod.NewBuilder(
		APIClient,
		podName,
		tsparams.TestNamespaceName,
		HwolOcpConfig.OcpHwolTestContainer,
	).RedefineDefaultCMD([]string{"sleep", "infinity"}).
		WithSecondaryNetwork(secNetwork)

	if builder == nil || builder.Definition == nil || len(builder.Definition.Spec.Containers) == 0 {
		return nil, fmt.Errorf("failed to init attach pod builder")
	}

	builder.Definition.Spec.Containers[0].Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{resName: resource.MustParse("1")},
		Limits:   corev1.ResourceList{resName: resource.MustParse("1")},
	}

	return builder.CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
}
