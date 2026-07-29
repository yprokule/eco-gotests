// Package tsparams provides test suite parameters and constants for OCP HWOL tests.
package tsparams

import (
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/openshift-kni/k8sreporter"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolinittools"
)

var (
	// Labels represent the suite-level labels applied to all tests in the suite.
	Labels = []string{LabelSuite}

	// ReporterCRDsToDump tells the reporter what CRDs to dump on failure.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &sriovv1.SriovNetworkNodePolicyList{}},
		{Cr: &sriovv1.SriovNetworkList{}},
		{Cr: &sriovv1.SriovNetworkNodeStateList{}},
		{Cr: &sriovv1.SriovOperatorConfigList{}},
	}

	// ReporterNamespacesToDump tells the reporter what namespaces to dump on failure.
	ReporterNamespacesToDump = map[string]string{
		HwolOcpConfig.OcpHwolOperatorNamespace: HwolOcpConfig.OcpHwolOperatorNamespace,
		TestNamespaceName:                      "other",
	}
)
