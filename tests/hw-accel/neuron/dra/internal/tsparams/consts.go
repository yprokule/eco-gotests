package tsparams

import "time"

const (
	// LabelSuite represents the DRA test suite label.
	LabelSuite = "neuron-dra"

	// LabelValidation represents the DRA CRD validation test label.
	LabelValidation = "dra-validation"

	// DRAInClusterBuildLabel selects the DRA in-cluster build scenario.
	DRAInClusterBuildLabel = "dra-in-cluster-build"

	// DRADeployTimeout is the timeout for DRA DaemonSet to become ready.
	DRADeployTimeout = 10 * time.Minute

	// DeviceConfigTimeout is the timeout for a DRA DeviceConfig to become available.
	DeviceConfigTimeout = time.Minute

	// DeviceClassTimeout is the timeout for DeviceClass operations.
	DeviceClassTimeout = 5 * time.Minute

	// DRATestNamespace is the namespace for DRA consumer test pods.
	DRATestNamespace = "neuron-dra-test"

	// DRAInClusterBuildTestNamespace is used by the DRA in-cluster build consumer.
	DRAInClusterBuildTestNamespace = "neuron-dra-inclusterbuild-test"

	// DRAInClusterBuildClaimTemplate is the in-cluster build consumer claim template.
	DRAInClusterBuildClaimTemplate = "neuron-claim"

	// DRAInClusterBuildConsumerPod is the pod that validates the built driver end-to-end.
	DRAInClusterBuildConsumerPod = "dra-inclusterbuild-consumer"

	// DRAInClusterBuildTimeout allows KMM to build and deploy the Neuron driver.
	DRAInClusterBuildTimeout = 30 * time.Minute

	// DRAVLLMTestNamespace is the namespace for DRA vLLM inference tests.
	DRAVLLMTestNamespace = "neuron-dra-vllm-test"

	// DRAVLLMClaimTemplate is the ResourceClaimTemplate used by the vLLM workload.
	DRAVLLMClaimTemplate = "neuron-vllm-claim"

	// DRAVLLMStartupTimeout allows for model download and Neuron compilation.
	DRAVLLMStartupTimeout = 45 * time.Minute

	// DRAVLLMClaimTimeout is the timeout for claim allocation and pod scheduling.
	DRAVLLMClaimTimeout = 5 * time.Minute

	// DRAVLLMInferenceTimeout includes the first Neuron inference compilation.
	DRAVLLMInferenceTimeout = 10 * time.Minute
)
