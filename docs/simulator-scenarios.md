# DRAForge Simulator Scenarios

The DRAForge simulator reads `SimulatedDevicePool` CRDs and translates them into real Kubernetes `ResourceSlice` objects. Scenarios are YAML files that define virtual device pools with configurable health modes, capacities, and topologies.

## Health Modes

Each `SimulatedDevicePool` supports one of four health modes via the `spec.health` field:

| Health              | Behavior                                                                 |
|---------------------|--------------------------------------------------------------------------|
| `healthy`           | All devices are published and available for allocation. Default.         |
| `unhealthy`         | Devices appear in the slice but are skipped by the allocation simulator. |
| `capacity-exhausted`| Slice is published with **zero** devices. Tests out-of-capacity paths.   |
| `disappear`         | No slice is created or existing slices are **deleted**. Node vanishes.   |

## Scenario Examples

### Basic GPU Pool (single node, 4 devices)

```yaml
apiVersion: draforge.oaslananka/v1alpha1
kind: SimulatedDevicePool
metadata:
  name: gpu-pool
spec:
  driverName: "sim.draforge.oaslananka"
  poolName: "gpu-pool"
  deviceCount: 4
  deviceType: "gpu"
  targetNodes: ["worker-1"]
  health: "healthy"
```

### Mixed Edge Devices (2 nodes, 2 device types)

```yaml
apiVersion: draforge.oaslananka/v1alpha1
kind: SimulatedDevicePool
metadata:
  name: edge-pool
spec:
  driverName: "sim.draforge.oaslananka"
  poolName: "edge-pool"
  deviceCount: 2
  deviceType: "fpga"
  targetNodes: ["edge-0", "edge-1"]
  health: "healthy"
```

### Multi-Node GPU (2 nodes, 1 GPU each)

`examples/scenarios/multi-node-gpu.yaml`:

```yaml
apiVersion: draforge.oaslananka/v1alpha1
kind: SimulatedDevicePool
metadata:
  name: multi-node-gpu
spec:
  driverName: "sim.draforge.oaslananka"
  poolName: "gpu-pool"
  deviceCount: 1
  deviceType: "gpu"
  targetNodes: ["node-0", "node-1"]
  health: "healthy"
```

### Unhealthy Device (fault injection)

`examples/scenarios/unhealthy-device.yaml`:

```yaml
apiVersion: draforge.oaslananka/v1alpha1
kind: SimulatedDevicePool
metadata:
  name: unhealthy-gpu
spec:
  driverName: "sim.draforge.oaslananka"
  poolName: "gpu-pool"
  deviceCount: 2
  deviceType: "gpu"
  targetNodes: ["node-0"]
  health: "unhealthy"
```

Devices are still visible in the ResourceSlice but the allocation simulator skips them.

### Capacity Exhausted

`examples/scenarios/capacity-exhausted.yaml`:

```yaml
apiVersion: draforge.oaslananka/v1alpha1
kind: SimulatedDevicePool
metadata:
  name: exhausted-pool
spec:
  driverName: "sim.draforge.oaslananka"
  poolName: "gpu-pool"
  deviceCount: 2
  deviceType: "gpu"
  targetNodes: ["node-0"]
  health: "capacity-exhausted"
```

Publishes zero devices — ResourceSlice exists but `spec.devices` is empty.

### Disappear (node failure simulation)

```yaml
apiVersion: draforge.oaslananka/v1alpha1
kind: SimulatedDevicePool
metadata:
  name: fail-node
spec:
  driverName: "sim.draforge.oaslananka"
  poolName: "gpu-pool"
  deviceCount: 2
  deviceType: "gpu"
  targetNodes: ["node-0"]
  health: "disappear"
```

Reconcile deletes all ResourceSlices owned by this pool.

## Dry-Run Validation

Validate any scenario before applying:

```bash
kubectl apply --dry-run=client -f examples/scenarios/basic-gpu.yaml
```

Or validate all examples at once:

```bash
for f in examples/scenarios/*.yaml; do
  echo "=== $f ==="
  kubectl apply --dry-run=client -f "$f" || exit 1
done
```

## Applying Scenarios

```bash
# Apply CRD first (once)
kubectl apply -f deploy/crds/simulateddevicepool-crd.yaml

# Apply scenario
kubectl apply -f examples/scenarios/basic-gpu.yaml

# Verify
kubectl get simpool
kubectl get resourceslices -o wide
```

## E2E Tests

End-to-end tests require a live cluster and the `DRAFORGE_E2E=1` environment variable:

```bash
DRAFORGE_E2E=1 go test -tags=e2e ./tests/e2e/ -v
```

The env guard prevents accidental execution against production clusters.

## Running Unit Tests

All tests use fake Kubernetes clients — no cluster required.

```bash
go test ./internal/simulator/ -v
```

The simulator test suite covers:
- All four health modes (healthy, unhealthy, capacity-exhausted, disappear)
- Idempotent reconciliation (same SDP → same state on multiple reconciles)
- Deterministic output (consistent device names across runs)
- Empty targetNodes (auto-discovery from cluster nodes)
- Multi-node pool distribution (one slice per node)
- Allocation: healthy device assignment
- Allocation: unhealthy slice skipped
- Allocation: no duplicate assignments
- Allocation: no matching device available
- Allocation: already-allocated claims left unchanged
