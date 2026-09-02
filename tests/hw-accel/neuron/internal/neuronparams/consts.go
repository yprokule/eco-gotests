package neuronparams

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// PrometheusNamespace represents the namespace for Prometheus.
	PrometheusNamespace = "openshift-monitoring"
	// ThanosQuerierServiceName represents the Thanos querier service name.
	ThanosQuerierServiceName = "thanos-querier"
)

// ServiceMonitorGVR is the GroupVersionResource for ServiceMonitor.
var ServiceMonitorGVR = schema.GroupVersionResource{
	Group:    "monitoring.coreos.com",
	Version:  "v1",
	Resource: "servicemonitors",
}

// DeviceConfigGVR is the GroupVersionResource for Neuron DeviceConfigs.
var DeviceConfigGVR = schema.GroupVersionResource{
	Group:    "k8s.aws",
	Version:  "v1beta1",
	Resource: "deviceconfigs",
}

// HasPrefix checks if a string starts with a specific prefix.
func HasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
