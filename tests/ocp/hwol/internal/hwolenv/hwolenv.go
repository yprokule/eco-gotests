// Package hwolenv provides helpers for OCP HWOL switchdev / OVS bridge setup.
package hwolenv

import (
	"fmt"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	hwolconfig "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

// MCPNodeSelector returns the node-role label selector for the HWOL MCP pool.
func MCPNodeSelector(mcpLabel string) map[string]string {
	return map[string]string{fmt.Sprintf("node-role.kubernetes.io/%s", mcpLabel): ""}
}

// EnsureHwolOperatorConfig verifies SriovOperatorConfig is set for the
// manageSoftwareBridges + mlx5 path (disable mellanox plugin, enable OVS offload).
func EnsureHwolOperatorConfig(operatorNS string) error {
	opCfg, err := sriov.PullOperatorConfig(APIClient, operatorNS)
	if err != nil {
		return fmt.Errorf("failed to pull SriovOperatorConfig: %w", err)
	}

	changed := false

	if !hasDisablePlugin(opCfg.Definition.Spec.DisablePlugins, tsparams.MellanoxPlugin) {
		opCfg = opCfg.WithDisablePlugins([]string{tsparams.MellanoxPlugin})
		changed = true
	}

	if opCfg.Definition.Spec.FeatureGates == nil {
		opCfg.Definition.Spec.FeatureGates = map[string]bool{}
	}

	if !opCfg.Definition.Spec.FeatureGates[tsparams.ManageSoftwareBridgesGate] {
		opCfg.Definition.Spec.FeatureGates[tsparams.ManageSoftwareBridgesGate] = true
		changed = true
	}

	if !opCfg.Definition.Spec.EnableOvsOffload {
		opCfg.Definition.Spec.EnableOvsOffload = true
		changed = true
	}

	if !changed {
		return nil
	}

	_, err = opCfg.Update()
	if err != nil {
		return fmt.Errorf("failed to update SriovOperatorConfig for HWOL: %w", err)
	}

	return nil
}

func hasDisablePlugin(plugins sriovv1.PluginNameSlice, name string) bool {
	for _, plugin := range plugins {
		if string(plugin) == name {
			return true
		}
	}

	return false
}

// EnsureSwitchdevFoundation configures operator, pool, and switchdev policy and waits for
// SR-IOV/MCP stability. It does not create SriovNetwork or OVSNetwork CRs.
func EnsureSwitchdevFoundation(
	operatorNS, mcpLabel string,
	device hwolconfig.DeviceConfig,
	vfNum int,
) error {
	if err := EnsureHwolOperatorConfig(operatorNS); err != nil {
		return err
	}

	mcpNodes, err := ListMCPWorkerNodes(mcpLabel)
	if err != nil {
		return fmt.Errorf("failed to list MCP nodes: %w", err)
	}

	if len(mcpNodes) == 0 {
		return fmt.Errorf("no nodes with label node-role.kubernetes.io/%s", mcpLabel)
	}

	if _, err := CreateOvsOffloadPoolConfig(tsparams.PoolConfigName, operatorNS, mcpLabel); err != nil {
		return err
	}

	if _, err := CreateSwitchdevPolicy(
		tsparams.PolicyName,
		operatorNS,
		tsparams.ResourceName,
		device,
		vfNum,
		MCPNodeSelector(mcpLabel),
	); err != nil {
		return err
	}

	return WaitForSriovAndMCPStable(
		operatorNS,
		mcpLabel,
		tsparams.MCOWaitTimeout,
		tsparams.DefaultStableDuration,
	)
}

// CreateOvsOffloadPoolConfig creates SriovNetworkPoolConfig with only
// ovsHardwareOffloadConfig.name set to the MCP name.
// Do not set nodeSelector/maxUnavailable together with OvsHardwareOffload — webhook rejects it.
func CreateOvsOffloadPoolConfig(name, operatorNS, mcpName string) (*sriov.PoolConfigBuilder, error) {
	builder := sriov.NewPoolConfigBuilder(APIClient, name, operatorNS)
	if builder == nil {
		return nil, fmt.Errorf("failed to init SriovNetworkPoolConfig builder")
	}

	builder.Definition.Spec.OvsHardwareOffloadConfig = sriovv1.OvsHardwareOffloadConfig{
		Name: mcpName,
	}

	if builder.Exists() {
		builder.Definition.ResourceVersion = builder.Object.ResourceVersion

		updated, err := builder.Update()
		if err != nil {
			return nil, fmt.Errorf("failed to update SriovNetworkPoolConfig %s: %w", name, err)
		}

		return updated, nil
	}

	created, err := builder.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create SriovNetworkPoolConfig %s: %w", name, err)
	}

	return created, nil
}

// CreateSwitchdevPolicy creates a switchdev SriovNetworkNodePolicy with OVS bridge management.
// VF0 is reserved as the management port (pfNames range starts at 1).
func CreateSwitchdevPolicy(
	name, operatorNS, resourceName string,
	device hwolconfig.DeviceConfig,
	vfNum int,
	nodeSelector map[string]string,
) (*sriov.PolicyBuilder, error) {
	if vfNum < 2 {
		return nil, fmt.Errorf("vfNum must be >= 2 to reserve VF0 as management port, got %d", vfNum)
	}

	pfSelector := fmt.Sprintf("%s#1-%d", device.InterfaceName, vfNum-1)

	policy := sriov.NewPolicyBuilder(
		APIClient,
		name,
		operatorNS,
		resourceName,
		vfNum,
		[]string{pfSelector},
		nodeSelector,
	).WithDevType("netdevice").WithMTU(1500)

	policy.Definition.Spec.NicSelector.Vendor = device.Vendor
	policy.Definition.Spec.NicSelector.DeviceID = device.DeviceID
	policy.Definition.Spec.EswitchMode = tsparams.SwitchdevMode
	policy.Definition.Spec.Bridge = sriovv1.Bridge{
		OVS: &sriovv1.OVSConfig{},
	}

	created, err := policy.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create SriovNetworkNodePolicy %s: %w", name, err)
	}

	return created, nil
}

// CreateSriovNetwork creates a SriovNetwork for the HWOL resource in the test namespace.
// If ipam is empty, HostLocalIPAM is used. Callers may pass other IPAM JSON later
// (for example NV-IPAM) without changing this helper.
func CreateSriovNetwork(
	name, operatorNS, resourceName, networkNS, ipam string,
) (*sriov.NetworkBuilder, error) {
	network := sriov.NewNetworkBuilder(APIClient, name, operatorNS, networkNS, resourceName)
	if network == nil {
		return nil, fmt.Errorf("failed to init SriovNetwork builder")
	}

	if ipam == "" {
		ipam = HostLocalIPAM
	}

	network.Definition.Spec.IPAM = ipam

	created, err := network.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create SriovNetwork %s: %w", name, err)
	}

	return created, nil
}

// WaitForSriovAndMCPStable waits for SR-IOV node states and the HWOL MCP to stabilize.
func WaitForSriovAndMCPStable(operatorNS, mcpName string, timeout, stableDuration time.Duration) error {
	return sriovoperator.WaitForSriovAndMCPStable(
		APIClient, timeout, stableDuration, mcpName, operatorNS)
}

// ListMCPWorkerNodes returns nodes matching the HWOL MCP role label.
func ListMCPWorkerNodes(mcpLabel string) ([]*nodes.Builder, error) {
	selector := labels.Set(MCPNodeSelector(mcpLabel)).String()

	return nodes.List(APIClient, metav1.ListOptions{LabelSelector: selector})
}

// AssertSwitchdevAndOVSBridge checks SriovNetworkNodeState for switchdev mode and a managed OVS bridge
// on the given PF for each MCP-labeled node.
func AssertSwitchdevAndOVSBridge(operatorNS, mcpLabel, pfName string) error {
	workerNodes, err := ListMCPWorkerNodes(mcpLabel)
	if err != nil {
		return fmt.Errorf("failed to list MCP nodes: %w", err)
	}

	if len(workerNodes) == 0 {
		return fmt.Errorf("no nodes found with label node-role.kubernetes.io/%s", mcpLabel)
	}

	for _, node := range workerNodes {
		nodeName := node.Object.Name
		state := sriov.NewNetworkNodeStateBuilder(APIClient, nodeName, operatorNS)

		if err := state.Discover(); err != nil {
			return fmt.Errorf("failed to discover SriovNetworkNodeState for %s: %w", nodeName, err)
		}

		if state.Objects.Status.SyncStatus != "Succeeded" {
			return fmt.Errorf("node %s syncStatus is %q, want Succeeded",
				nodeName, state.Objects.Status.SyncStatus)
		}

		iface, err := findStatusInterface(state.Objects.Status.Interfaces, pfName)
		if err != nil {
			return fmt.Errorf("node %s: %w", nodeName, err)
		}

		if iface.EswitchMode != tsparams.SwitchdevMode {
			return fmt.Errorf("node %s interface %s eSwitchMode is %q, want %s",
				nodeName, pfName, iface.EswitchMode, tsparams.SwitchdevMode)
		}

		if err := assertOVSBridgeForPF(state.Objects.Status.Bridges, pfName, iface.PciAddress); err != nil {
			return fmt.Errorf("node %s: %w", nodeName, err)
		}

		klog.V(90).Infof("HWOL switchdev+OVS bridge OK on node %s pf %s pci %s",
			nodeName, pfName, iface.PciAddress)
	}

	return nil
}

func findStatusInterface(ifaces sriovv1.InterfaceExts, pfName string) (*sriovv1.InterfaceExt, error) {
	for i := range ifaces {
		if ifaces[i].Name == pfName {
			return &ifaces[i], nil
		}
	}

	return nil, fmt.Errorf("interface %s not found in SriovNetworkNodeState status", pfName)
}

func assertOVSBridgeForPF(bridges sriovv1.Bridges, pfName, pciAddress string) error {
	if len(bridges.OVS) == 0 {
		return fmt.Errorf("no OVS bridges in SriovNetworkNodeState status for PF %s", pfName)
	}

	for _, bridge := range bridges.OVS {
		for _, uplink := range bridge.Uplinks {
			if uplink.Name == pfName || uplink.PciAddress == pciAddress {
				return nil
			}
		}
	}

	return fmt.Errorf("no OVS bridge uplink matching PF %s (%s)", pfName, pciAddress)
}

// CleanupHwolResources removes switchdev-created networks, policies, and pool config, then waits for stability.
func CleanupHwolResources(operatorNS, mcpName string, timeout, stableDuration time.Duration) error {
	klog.V(90).Infof("Cleaning HWOL test resources in namespace %s", operatorNS)

	ovsNet := NewOvsNetworkBuilder(
		APIClient, tsparams.OvsNetworkName, operatorNS, tsparams.TestNamespaceName, tsparams.ResourceName)
	if ovsNet != nil {
		if err := ovsNet.Delete(); err != nil {
			return fmt.Errorf("failed to delete OVSNetwork %s: %w", tsparams.OvsNetworkName, err)
		}
	}

	if err := sriovoperator.RemoveAllSriovNetworks(APIClient, operatorNS, tsparams.DefaultTimeout); err != nil {
		return err
	}

	if err := sriov.CleanAllNetworkNodePolicies(APIClient, operatorNS); err != nil {
		return err
	}

	if pool, err := sriov.PullPoolConfig(APIClient, tsparams.PoolConfigName, operatorNS); err == nil {
		if delErr := pool.Delete(); delErr != nil {
			return fmt.Errorf("failed to delete SriovNetworkPoolConfig %s: %w", tsparams.PoolConfigName, delErr)
		}
	}

	if err := WaitForSriovAndMCPStable(operatorNS, mcpName, timeout, stableDuration); err != nil {
		return fmt.Errorf(
			"HWOL cleanup timed out waiting for SR-IOV/MCP stable: SNNS may be stuck resetting "+
				"switchdev (device or resource busy); reboot the MCP-labeled HWOL node before re-run: %w",
			err)
	}

	return nil
}
