# OCP HWOL Test Suite

Validates hardware offload (HWOL / switchdev) on OpenShift using the SR-IOV
Network Operator. This is a standalone suite under `tests/ocp/hwol/` — select it
locally with `ECO_TEST_FEATURES=hwol` (`make run-tests`).

## Prerequisites

- OCP cluster with the SR-IOV Network Operator installed and healthy
- At least one worker node with an mlx5-capable NIC (BF-2 / BF-3 NIC mode / CX-6 Dx for plumbing)
- `KUBECONFIG` pointing at the cluster

Lab note (BOS2 ANL): Cluster 5 topology is provisioner **143** + BM workers
**141** / **142** (BF-2). BF-3 NIC mode is required later for customer proof
(RHELBU-4108); BF-2 is fine for skeleton and plumbing.

## Required environment variables

| Variable | Description | Example |
|----------|-------------|---------|
| `KUBECONFIG` | Path to kubeconfig | `/path/to/kubeconfig` |
| `ECO_OCP_HWOL_DEVICES` | Device list (`name:deviceID:vendor:interface`, comma-separated) | `bf2:a2d6:15b3:ens7f0` |

Devices are **not** configured in YAML. `ECO_OCP_HWOL_DEVICES` is required.

## Optional environment variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ECO_OCP_HWOL_OPERATOR_NAMESPACE` | SR-IOV operator namespace | `openshift-sriov-network-operator` |
| `ECO_OCP_HWOL_TEST_CONTAINER` | Workload container image | see `internal/ocphwolconfig/default.yaml` |
| `ECO_OCP_HWOL_MCP_LABEL` | Machine config pool label | `sriov` |
| `ECO_OCP_HWOL_VF_NUM` | Number of VFs to configure | `2` |
| `ECO_WORKER_LABEL` | Worker node label selector | from shared ocpconfig |

## Directory structure

```text
tests/ocp/hwol/
├── hwol_suite_test.go              # Ginkgo suite entrypoint
├── README.md
├── tests/
│   └── setup.go                    # Precondition smoke
└── internal/
    ├── ocphwolconfig/              # ECO_OCP_HWOL_* config + default.yaml
    ├── ocphwolinittools/           # APIClient + HwolOcpConfig globals
    └── tsparams/                   # Labels, timeouts, reporter dumps
```

## Test labels

| Label | Description |
|-------|-------------|
| `ocphwol` | Suite label — all HWOL tests |
| `setup` | Precondition smoke (operator, devices, workers) |

## Running tests

### Compile / dry-run (no cluster)

```bash
go test -c ./tests/ocp/hwol
ginkgo --dry-run ./tests/ocp/hwol
```

### Live smoke

```bash
export KUBECONFIG=/path/to/kubeconfig
export ECO_OCP_HWOL_DEVICES="bf2:a2d6:15b3:ens7f0"
export ECO_GOTESTS_PACKAGE_PATH=./tests/ocp/hwol
export ECO_TEST_LABELS="ocphwol"
make run-tests
```

### Setup smoke only

```bash
export ECO_TEST_LABELS="setup"
make run-tests
```

## Current coverage

| Spec | What it checks |
|------|----------------|
| HWOL setup smoke | SR-IOV operator deployed, `ECO_OCP_HWOL_DEVICES` parses, ≥1 worker |

Switchdev policy, OVS HWOL, and OVN feature tests are follow-on work.
