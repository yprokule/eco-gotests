// Package tests contains the HWOL test cases for OCP.
package tests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe(
	"HWOL setup",
	Ordered,
	Label(tsparams.LabelSetup),
	func() {
		It("verifies operator, devices config, and worker nodes", func() {
			Expect(HwolOcpConfig).ToNot(BeNil(),
				"HwolOcpConfig is nil: config init failed (see NewHwolOcpConfig logs)")

			By("Checking the SR-IOV operator is running")

			err := sriovoperator.IsSriovDeployed(APIClient, HwolOcpConfig.OcpHwolOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "SR-IOV operator is not running")

			By("Loading HWOL device configuration from ECO_OCP_HWOL_DEVICES")

			devices, err := HwolOcpConfig.GetHwolDevices()
			Expect(err).ToNot(HaveOccurred(), "Failed to get HWOL devices")
			Expect(len(devices)).To(BeNumerically(">", 0), "No HWOL devices configured")

			By("Discovering worker nodes")

			workerNodes, err := nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: HwolOcpConfig.WorkerLabel})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover nodes")
			Expect(len(workerNodes)).To(BeNumerically(">=", 1), "No worker nodes found")
		})
	})
