# OCP HWOL Test Suite

Validates hardware offload (HWOL / switchdev) on OpenShift using the SR-IOV
Network Operator. This is a standalone suite under `tests/ocp/hwol/` — select it
locally with `ECO_TEST_FEATURES=hwol` (`make run-tests`).

## Prerequisites

- OCP cluster with the SR-IOV Network Operator installed and healthy
- At least one worker with an mlx5-capable NIC in a mode that supports host
  eswitch switchdev (BF-3 NIC mode / CX-6 Dx; BF-2 DPU mode is **not** enough)
- MCP matching `ECO_OCP_HWOL_MCP_LABEL` (default `sriov`) with workers labeled
  `node-role.kubernetes.io/<mcp_label>=""`
- Host IOMMU enabled (`intel_iommu=on iommu=pt`); Dell/BF often also need
  `pci=realloc` (and sometimes BIOS MMIO High Size) before VF create works
- `KUBECONFIG` pointing at the cluster
- For the **ovs-network** attach path: Subscription must set `OVS_CNI_IMAGE` to a
  pullable ovs-cni image, with registry credentials configured on the cluster
  as needed

## Required environment variables

| Variable | Description | Example |
|----------|-------------|---------|
| `KUBECONFIG` | Path to kubeconfig | `/path/to/kubeconfig` |
| `ECO_OCP_HWOL_DEVICES` | Device list (`name:deviceID:vendor:interface`, comma-separated) | `bf2:a2d6:15b3:ens7f0np0` |

Devices are **not** configured in YAML. `ECO_OCP_HWOL_DEVICES` is required.

## Optional environment variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ECO_OCP_HWOL_OPERATOR_NAMESPACE` | SR-IOV operator namespace | `openshift-sriov-network-operator` |
| `ECO_OCP_HWOL_TEST_CONTAINER` | Workload container image | see `internal/ocphwolconfig/default.yaml` |
| `ECO_OCP_HWOL_MCP_LABEL` | Machine config pool name / role label | `sriov` |
| `ECO_OCP_HWOL_VF_NUM` | Number of VFs to configure (≥2; VF0 reserved) | `2` |
| `ECO_WORKER_LABEL` | Worker node label selector | from shared ocpconfig |

If the default test image (`quay.io/ocp-edge-qe/...`) cannot be pulled, set
`ECO_OCP_HWOL_TEST_CONTAINER` to a public image the cluster can reach
(e.g. `registry.access.redhat.com/ubi9/ubi-minimal:latest`).

## Directory structure

```text
tests/ocp/hwol/
├── hwol_suite_test.go              # Ginkgo suite entrypoint
├── README.md
├── tests/
│   ├── setup.go                    # Precondition smoke
│   └── hwol.go                     # Ordered: foundation, switchdev It, attach table
└── internal/
    ├── hwolenv/                    # Foundation, OVSNetwork, policy helpers
    ├── ocphwolconfig/              # ECO_OCP_HWOL_* config + default.yaml
    ├── ocphwolinittools/           # APIClient + HwolOcpConfig globals
    └── tsparams/                   # Labels, timeouts, reporter dumps
```

Shared NAD wait/assert helpers live in `tests/internal/sriovoperator/nad.go`.

## Test labels

| Label | Description |
|-------|-------------|
| `ocphwol` | Suite label — all HWOL tests |
| `setup` | Precondition smoke (operator, devices, workers) |
| `switchdev` | Switchdev mode + managed OVS bridge assert |
| `ovs-network` | Attach via `OVSNetwork` (ovs CNI) + NAD + pod |
| `sriov-network` | Attach via `SriovNetwork` (sriov CNI) + NAD + pod |

Foundation (operator config, pool, switchdev policy, MCP wait) runs once in the
Ordered `BeforeAll` for the HWOL Describe. Filtering with `ECO_TEST_LABELS`
still runs that foundation when an attach or switchdev `It`/`Entry` is selected.

## Running tests

### Compile / dry-run (no cluster)

```bash
go test -c ./tests/ocp/hwol
ginkgo --dry-run ./tests/ocp/hwol
```

### Live smoke (local make runner)

```bash
export KUBECONFIG=/path/to/kubeconfig
export ECO_TEST_FEATURES=hwol
export ECO_TEST_LABELS=setup
export ECO_OCP_HWOL_DEVICES="bf2:a2d6:15b3:ens7f0np0"
make run-tests
```

### switchdev bridge assert

```bash
export ECO_TEST_FEATURES=hwol
export ECO_TEST_LABELS=switchdev
export ECO_OCP_HWOL_DEVICES="bf2:a2d6:15b3:ens7f0np0"
make run-tests
```

### ovs CNI attach (requires OVS_CNI_IMAGE on the Subscription)

```bash
export ECO_TEST_FEATURES=hwol
export ECO_TEST_LABELS=ovs-network
export ECO_OCP_HWOL_DEVICES="bf2:a2d6:15b3:ens7f0np0"
export ECO_OCP_HWOL_TEST_CONTAINER=registry.access.redhat.com/ubi9/ubi-minimal:latest
make run-tests
```

### sriov CNI attach

```bash
export ECO_TEST_FEATURES=hwol
export ECO_TEST_LABELS=sriov-network
export ECO_OCP_HWOL_DEVICES="bf2:a2d6:15b3:ens7f0np0"
export ECO_OCP_HWOL_TEST_CONTAINER=registry.access.redhat.com/ubi9/ubi-minimal:latest
make run-tests
```

The Ordered HWOL spec ensures `SriovOperatorConfig` (`disablePlugins: [mellanox]`,
`featureGates.manageSoftwareBridges`, `enableOvsOffload`), creates
`SriovNetworkPoolConfig` (OvsHardwareOffload only — no parallel
nodeSelector/maxUnavailable), applies switchdev policy with `bridge.ovs: {}`,
and waits for MCP/node-state sync. The `switchdev` It asserts
`eSwitchMode=switchdev` and a managed OVS bridge uplink. Attach table Entries
create the network CR, assert NAD type/resource, and run a sleep pod.

`AfterAll` removes HWOL OVSNetwork/SriovNetwork objects, policies, and the pool
config — run on a dedicated lab MCP, not a shared production-like pool. Cleanup
waits up to `CleanupWaitTimeout` (15m). If that times out (often switchdev reset
stuck with `device or resource busy`), reboot the MCP-labeled HWOL node before
re-running.

## Current coverage

| Spec | What it checks |
|------|----------------|
| HWOL setup smoke | SR-IOV operator deployed, `ECO_OCP_HWOL_DEVICES` parses, ≥1 worker |
| HWOL switchdev | Foundation + OVS bridge status on MCP nodes |
| HWOL attach (ovs / sriov) | NAD `type` + resource annotation + pod secondary network |

Attach networks default to host-local IPAM (configurable on the create helpers);
NV-IPAM is not a suite prerequisite.

OVN feature tables and offload flow assertions (`ovs-appctl dpctl/dump-flows
type=offloaded`) are follow-on work.
