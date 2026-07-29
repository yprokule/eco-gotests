// Package tsparams provides test suite parameters and constants for OCP HWOL tests.
package tsparams

import "time"

const (
	// TestNamespaceName is the namespace where HWOL test cases are performed.
	TestNamespaceName = "hwol-tests"
	// LabelSuite represents the HWOL suite label for test selection.
	LabelSuite = "ocphwol"
	// LabelSetup represents the setup/smoke test label for filtering.
	LabelSetup = "setup"

	// DefaultTimeout represents default timeout for general operations.
	DefaultTimeout = 300 * time.Second
	// WaitTimeout represents default timeout for most waiting operations.
	WaitTimeout = 20 * time.Minute
	// RetryInterval represents retry interval for ginkgo Eventually functions.
	RetryInterval = 3 * time.Second

	// MlxVendorID is the Mellanox/NVIDIA PCI vendor ID.
	MlxVendorID = "15b3"

	// TestResourceLabelKey is the label key used to identify test-created resources.
	TestResourceLabelKey = "eco-gotests.openshift.io/test"
	// TestResourceLabelValue is the label value for test-created resources.
	TestResourceLabelValue = "ocp-hwol"
)
