# DRAForge Simulator Scenarios

The DRAForge simulator reads `SimulatedDevicePool` CRDs and translates them into real Kubernetes `ResourceSlice` objects. Scenarios are YAML files that define virtual device pools with configurable health modes, capacities, and topologies.

## Scenario Catalog

Reusable scenario files live under `examples/scenarios/`.

| Scenario | File | Purpose |
|---|---|---|
| Success | `examples/scenarios/success.yaml` | Healthy pool with enough GPU capacity for a normal allocation path. |
| No match | `examples/scenarios/no-match.yaml` | Pool attributes differ from common high-end GPU selectors. |
| Capacity | `examples/scenarios/capacity.yaml` | Zero-device pool for capacity diagnostics. |
| Delayed binding | `examples/scenarios/delayed-binding.yaml` | Multi-node target list for delayed binding checks. |
| Multi-node | `examples/scenarios/multi-node.yaml` | Four simulated devices across two target nodes. |

## Health Modes

Each `SimulatedDevicePool` supports one of four health modes via the `spec.health` field:

| Health | Behavior |
|---|---|
| `healthy` | All devices are published and available for allocation. Default. |
| `unhealthy` | Devices appear in the slice but are skipped by the allocation simulator. |
| `capacity-exhausted` | Slice is published with zero devices. Tests out-of-capacity paths. |
| `disappear` | No slice is created or existing slices are deleted. Node vanishes. |

## Applying Scenarios

```bash
kubectl apply -f deploy/crds/simulateddevicepool-crd.yaml
kubectl apply -f examples/scenarios/success.yaml
kubectl get simpool
kubectl get resourceslices -o wide
```

Validate all example scenarios before using them:

```bash
for f in examples/scenarios/*.yaml; do
  echo "=== $f ==="
  kubectl apply --dry-run=client -f "$f" || exit 1
done
```

## Adding a Scenario

Create a focused YAML file in `examples/scenarios/` using the `SimulatedDevicePool` CRD. Give the resource and file a clear diagnostic intent, keep the scope small, and update the catalog table above.

## Allocation Selection Contract

The allocation simulator evaluates actual Kubernetes DRA objects rather than
pool, class, or product-name substrings. A pending claim must reference an
existing `DeviceClass`. Every class selector and request-level selector is
combined with logical AND through the exact-version Kubernetes DRA CEL
evaluator.

The current supported allocation subset is:

- `Exactly` requests and ordered `FirstAvailable` alternatives;
- default/`ExactCount` allocation mode, with an omitted count defaulting to one, and `All` mode over every matching device on one compatible node;
- typed scalar attributes, semantic-version attributes, `device.driver`, and
  quantity capacities inside CEL selectors;
- healthy simulator-managed devices that are not already allocated by the same
  `<driver, pool, device>` identity;
- one compatible `NodeName` across all results in a claim;
- stable scalar `MatchAttribute` constraints for string, integer, boolean, and
  semantic-version values, including all-request, request, and
  `<request>/<subrequest>` scopes.

`All` allocation requires complete ResourceSlice coverage for the latest
pool generation, excludes devices already allocated by exact identity, and
respects Kubernetes' 32-result claim limit. An empty or oversized `All`
alternative may fall through to the next ordered `FirstAvailable` alternative.
`MatchAttribute` uses deterministic, context-aware backtracking and a fixed
100,000-branch safety budget; exhausting that budget leaves the claim pending
with an explicit unsupported-request warning instead of approximating a result.

The following features are intentionally fail-closed until their complete
semantics are implemented: consumable-capacity requests/accounting, multiple
allocations, `DistinctAttribute`, list-valued constraint attributes, admin access,
device taints/tolerations, partitionable/per-device node
selection, binding conditions, shared counters, and node-allocatable mappings.
The claim remains pending and the controller emits a warning event such as
`SimulationUnsupportedRequest`, `SimulationSelectorError`, or
`SimulationDeviceClassNotFound`.

This behavior is useful for deterministic development scenarios, but it is not
a Kubernetes scheduler conformance oracle.

## CDI Output Modes

The Helm chart deliberately separates two node-plugin modes:

| Mode | Storage | Source of devices | Failure behavior |
|---|---|---|---|
| `demo` (default) | Pod-local `emptyDir` | Two explicit static demo devices | A write failure reports not-ready; no host path is used. |
| `node` | `/var/lib/kubelet/device-plugins/cdi` hostPath | Allocated ResourceClaim results matching the node | Kubernetes or CDI write failures report not-ready and preserve the last-known-good file. |

Enable host-integrated output only on a disposable or controlled node:

```bash
helm upgrade --install draforge deploy/helm/draforge \
  --namespace draforge-system \
  --create-namespace \
  --set nodePlugin.outputMode=node
```

The generated file is mode `0644`, owned by the writing process, and replaced atomically. The DaemonSet exposes process liveness at `/healthz` and dependency/output readiness at `/readyz` on container port `8083`.

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
- Idempotent reconciliation
- Deterministic output
- Empty targetNodes
- Multi-node pool distribution
- Allocation success and failure paths
- Typed DeviceClass and request-level CEL selector evaluation
- Ordered `FirstAvailable` fallback without product-name heuristics
- `All` allocation across complete latest-generation pools, including empty, oversized, and allocated-device cases
- Scalar `MatchAttribute` equality, request/subrequest scope, combination and cross-request backtracking, and bounded-search failure
- Fail-closed diagnostics for missing classes, invalid CEL, and unsupported request or constraint modes
