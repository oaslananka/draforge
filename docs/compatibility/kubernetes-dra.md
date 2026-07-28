# Kubernetes DRA Compatibility

> **Last updated:** 2026-07-28
> **Kubernetes reference:** v1.35 through v1.36
> **DRAForge version:** 0.2.0

---

## Compatibility Position

DRAForge is provider-neutral and targets Kubernetes **Dynamic Resource Allocation (DRA)** using the
`resource.k8s.io/v1` structured-parameters API. Compatibility is defined by Kubernetes API and behavior, not by cloud vendor. As of the current upstream
Kubernetes documentation, core DRA is **stable in Kubernetes v1.35** and is
enabled by default.

DRAForge therefore treats **Kubernetes v1.35+** as the recommended compatibility
baseline. Kubernetes v1.32 through v1.34 may expose parts of the same API shape
in earlier maturity states, but those versions should be treated as legacy or
transition targets and must be validated per distribution.

| Kubernetes version | Upstream DRA status | DRAForge stance |
| --- | --- | --- |
| v1.36 | Stable core DRA plus additional alpha/beta extensions | Supported target, with extension gaps documented below |
| v1.35 | Stable core DRA, enabled by default | Recommended baseline |
| v1.34 | Transition-era DRA support | Best-effort only; validate API availability |
| v1.32-v1.33 | Earlier DRA API maturity | Best-effort only; feature gates and API shape must be checked |
| < v1.32 | Legacy / incompatible API shape | Not supported |

> DRAForge assumes that `resource.k8s.io/v1` is served by the cluster. If the
> API group or version is not present, the doctor and discovery layers must
> surface that as a partial or unavailable DRA state, not as an empty cluster.

---

## Tested Kubernetes Versions

| Distribution | Version | Test status |
| --- | --- | --- |
| GitHub Actions / static CI | n/a | Go, web, Helm, Terraform, workflow, and E2E harness contracts |
| kind pull-request gate | v1.35.5 | Complete chart install and scenario-to-dashboard verification on every pull request |
| kind full gate | v1.35.5 and v1.36.1 | Weekly, manual, release-candidate, and final-release matrix |
| DOKS | distribution-supported v1 API | Optional provider-adapter smoke target; not a general release gate |

`tests/install-e2e/kubernetes-versions.json` is the source of truth for the pinned kind and Kubernetes node-image matrix. The full install suite verifies the chart CRD, server, controller, sim-driver, Services, RBAC, NetworkPolicies, scenario, ResourceClaim allocation, consumer Pod association, API, explain output, metrics, readiness, graph, and SSE identity.


---

## Upstream DRA Feature Status

The table below captures the compatibility model DRAForge should use as of
2026-06-24. It is intentionally conservative: stable core APIs are considered
baseline; alpha/beta extensions are discovered or explained only when the code
explicitly supports them.

| Capability | Upstream status as of v1.36 docs | DRAForge 0.2.0 support |
| --- | --- | --- |
| Core Dynamic Resource Allocation | Stable since v1.35 | Supported for discovery and visualization |
| DeviceClass | Stable core DRA | Discovered; class selectors use the exact-version Kubernetes DRA CEL evaluator |
| ResourceClaim / ResourceClaimTemplate | Stable core DRA | Claims discovered; templates read-only / limited |
| ResourceSlice | Stable core DRA | Discovered and used for graph/doctor views |
| CEL device filtering | Stable core behavior | Supported for driver identity, typed scalar attributes, semantic versions and quantity capacities; unsupported feature-gated values fail closed |
| ResourceClaim device status | Observability feature | Partial display only |
| Device health monitoring | Observability feature | Partial; custom health label fallback remains |
| Admin access | Beta extension | Not implemented |
| Granular status authorization | Beta extension | Not implemented |
| Extended resource allocation by DRA | Alpha extension | Not implemented |
| Partitionable devices | Alpha extension | Not implemented |
| Consumable capacity | Alpha extension | Partially displayed; no allocation scoring |
| Device taints and tolerations | Alpha extension | Not implemented |
| Resource pool status | Alpha extension | Not implemented |
| Device binding conditions | Alpha extension | Not implemented |
| Node allocatable resources | Alpha extension | Not implemented |
| DRA device metadata in containers | Alpha extension | Not implemented |
| List type attributes | Alpha extension | Not implemented |

---

## DRAForge Support Matrix

| Feature | Status | Notes |
| --- | --- | --- |
| ResourceSlice discovery | Supported | Uses `resource.k8s.io/v1` and should expose API errors as partial state |
| ResourceClaim discovery | Supported | Reads status/allocation data and maps claims to Pods where possible |
| DeviceClass discovery | Supported | DeviceClasses are listed and core selectors use `k8s.io/dynamic-resource-allocation/cel` pinned to the repository Kubernetes version |
| Claim allocation explain | Partial | DeviceClass selectors use the shared Kubernetes evaluator; request-level selectors, taints, binding and consumable-capacity reasoning remain incomplete |
| Simulator allocation | Partial | Supports existing DeviceClasses, class/request selectors, ordered `FirstAvailable`, default/`ExactCount`, `All` over complete latest-generation pools, health, complete device identity, one-node selection and the 32-result limit; remaining advanced features fail closed |
| Device health | Partial | Custom label fallback exists; native health status support is incomplete |
| Consumable capacity | Partial | Values can be displayed; capacity-aware allocation and exhaustion scoring are missing |
| CDI output | Partial | The simulator can produce CDI-oriented output, but Helm deployment modes must clearly separate demo and host-integrated operation |
| Helm CRD packaging | Supported | The SimulatedDevicePool CRD is packaged under chart `crds/` for Helm install |
| RBAC least privilege | Supported | Server and node-plugin roles are read-only; controller writes are resource/verb scoped, including the Kubernetes v1.36 `resourceclaims/binding` authorization required for allocation updates |
| Install-level E2E coverage | Supported | Reduced v1.35 gate on pull requests; full v1.35-v1.36 gate on schedule and releases |

---

## Known Limitations

1. **The supported selector subset is explicit.** DeviceClass selectors and
   simulator request selectors use the Kubernetes DRA CEL implementation pinned
   to the same `v0.36.2` module version as the repository APIs. Driver identity,
   typed scalar attributes, semantic versions and quantity capacities are
   available. List-valued attributes and feature-gated extensions remain
   unsupported and produce a fail-closed diagnostic instead of an approximation.

2. **Simulator allocation is not scheduler-equivalent.** It supports
   `Exactly`, ordered `FirstAvailable`, default/`ExactCount`, and `All` requests
   over healthy simulator-managed devices on one compatible node. `All` requires
   complete latest-generation pool slices, excludes already allocated identities,
   and respects the 32-result claim limit. Consumable capacity accounting, claim
   constraints, admin access, taints/tolerations,
   partitionable devices, binding conditions and node-allocatable mappings remain
   unsupported. Unsupported input leaves the claim pending and emits a warning
   event; the simulator must not be used as a scheduler conformance oracle.

3. **Native health and status features are incomplete.** DRAForge still relies
   on custom labels or simplified status interpretation in some paths.

4. **CDI mode must be explicit.** Demo-local output and host-integrated kubelet
   CDI output are different operational modes and should not be presented as the
   same production behavior. This is configured via the `nodePlugin.outputMode`
   Helm chart value:
   - `demo` (default): Uses an `emptyDir`, writes explicit static demo devices, runs as non-root, and never modifies the host.
   - `node`: Mounts the kubelet CDI `hostPath`, runs as UID 0 with privilege escalation disabled and all Linux capabilities dropped, and derives devices only from Kubernetes allocations for the current node. Directory, API, or atomic write failures are fail-closed: readiness becomes false and the last-known-good document remains in place.

5. **Granular authorization is not implemented.** DRAForge currently assumes
   broad enough read access to observe DRA objects. Per-device status visibility
   and admin-access flows are future work.

6. **Alpha/beta DRA extensions are not guaranteed.** v1.36-era alpha features
   such as partitionable devices, consumable capacity, device taints, binding
   conditions and node allocatable resources require explicit support before
   DRAForge can diagnose them accurately.

---

## API Surface

DRAForge uses or plans around the following Kubernetes APIs.

| Resource | API group | Current use |
| --- | --- | --- |
| Pods | `core/v1` | Read-only discovery and owner mapping |
| Nodes | `core/v1` | Read-only node and graph context |
| Events | `core/v1` | Diagnostics and controller events |
| ResourceClaims | `resource.k8s.io/v1` | Discovery, explain and simulator status paths |
| ResourceClaimTemplates | `resource.k8s.io/v1` | Read-only discovery |
| ResourceSlices | `resource.k8s.io/v1` | Discovery, graph, simulator output |
| DeviceClasses | `resource.k8s.io/v1` | Discovery and shared core CEL selector evaluation |
| SimulatedDevicePools | `draforge.oaslananka/v1alpha1` | DRAForge simulator CRD |

DRAForge does **not** aim to support legacy non-v1 DRA API groups by default.
If a distribution serves only older APIs, users should upgrade the cluster or add
an explicit compatibility adapter.

---

## Quick Troubleshooting

### `resource.k8s.io/v1` is missing

Symptoms include errors such as:

```text
the server could not find the requested resource
resource.k8s.io/v1 API group is not registered
```

Check:

```bash
kubectl api-resources --api-group=resource.k8s.io
kubectl version
```

Use Kubernetes v1.35+ for the default compatibility path. For older clusters,
verify feature gates and served API versions before using DRAForge.

### SimulatedDevicePool CRD is missing

The Helm chart now packages the CRD under `deploy/helm/draforge/crds/`. For
manual installs, the standalone CRD remains available:

```bash
kubectl apply -f deploy/crds/simulateddevicepool-crd.yaml
```

For Helm installs, use:

```bash
helm install draforge deploy/helm/draforge --namespace draforge-system --create-namespace
```

### Discovery shows no DRA objects

An empty graph can mean either “no DRA objects exist” or “the DRA API is not
available.” The discovery layer should preserve those as different states. If the
current UI shows only an empty result, run the doctor command and inspect API
availability warnings.

---

## RBAC Guidance

### Server

The server should remain read-only:

```yaml
apiGroups: [""]
resources: [pods, nodes, namespaces, events]
verbs: [get, list, watch]

apiGroups: [resource.k8s.io]
resources: [resourceclaims, resourceclaimtemplates, resourceslices, deviceclasses]
verbs: [get, list, watch]

apiGroups: [draforge.oaslananka]
resources: [simulateddevicepools]
verbs: [get, list, watch]
```

### Controller

The controller should use the narrowest write access possible:

```yaml
apiGroups: [""]
resources: [pods, nodes, namespaces]
verbs: [get, list, watch]

apiGroups: [""]
resources: [events]
verbs: [create, patch]

apiGroups: [resource.k8s.io]
resources: [resourceslices]
verbs: [get, list, watch, create, update, patch, delete]

apiGroups: [resource.k8s.io]
resources: [resourceclaims]
verbs: [get, list, watch, patch]

apiGroups: [resource.k8s.io]
resources: [resourceclaims/status]
verbs: [get, update, patch]

apiGroups: [resource.k8s.io]
resources: [resourceclaims/binding]
verbs: [update]

apiGroups: [resource.k8s.io]
resources: [deviceclasses, resourceclaimtemplates]
verbs: [get, list, watch]

apiGroups: [draforge.oaslananka]
resources: [simulateddevicepools]
verbs: [get, list, watch]

apiGroups: [draforge.oaslananka]
resources: [simulateddevicepools/status]
verbs: [get, update, patch]
```

Kubernetes v1.36 introduces granular authorization for ResourceClaim allocation
and reservation changes. Updating `status.allocation` or `status.reservedFor`
requires the synthetic `resourceclaims/binding` resource in addition to
`resourceclaims/status`. DRAForge uses `UpdateStatus`, so the chart grants only
`update` on `resourceclaims/binding`; it does not grant `patch`, wildcard
resources, or wildcard verbs.

---

## Release Readiness Implications

Before DRAForge is called release-ready for Kubernetes DRA, the following must be
true:

- The compatibility matrix must be checked against current upstream Kubernetes
  docs during every minor release.
- CI must run Go, web, Helm, Terraform and GoReleaser checks.
- Helm installs must include the simulator CRD.
- The doctor command must distinguish missing APIs from empty clusters.
- The simulator and explain engine must clearly mark unsupported DRA features as
  unsupported rather than silently approximating them.
- The reduced install-level kind gate must pass on every pull request.
- The full v1.35-v1.36 install matrix must pass before release candidates or final releases publish artifacts.
- API and SSE assertions must preserve the same namespace-qualified claim and complete allocation identity.

---

## References

- [Kubernetes Dynamic Resource Allocation](https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/)
- [Set Up DRA in a Cluster](https://kubernetes.io/docs/tasks/configure-pod-container/assign-resources/set-up-dra-cluster/)
- [ResourceClaim v1 API](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/resource-claim-v1/)
- [ResourceSlice v1 API](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/resource-slice-v1/)
- [DeviceClass v1 API](https://kubernetes.io/docs/reference/kubernetes-api/resource/device-class-v1/)
