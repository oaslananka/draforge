# Kubernetes DRA Compatibility

> **Last updated:** 2026-06-16  
> **Kubernetes reference:** v1.34 / v1.35 / v1.36  
> **DRAForge version:** 0.1.0

---

## Supported Kubernetes Versions

DRAForge targets the Kubernetes **Dynamic Resource Allocation (DRA) structured-parameters**
API (`resource.k8s.io/v1`), which is **General Availability (GA) since Kubernetes v1.34**.

| K8s Version | DRA Status | DRAForge Support       |
|-------------|------------|------------------------|
| v1.32       | Beta       | Compatible (v1 API)    |
| v1.33       | Beta       | Compatible (v1 API)    |
| **v1.34**   | **GA**     | **Recommended**        |
| v1.35       | GA         | Compatible             |
| v1.36       | GA         | Compatible             |
| < v1.32     | Classic    | Not compatible         |

> **Assumption:** `resource.k8s.io/v1` API group is served by the cluster.
> Older clusters with DRA classic (pre-v1.32) use a different API shape and are
> **not supported** by DRAForge.

### Tested Kubernetes Versions

| Distribution | Version | DRAForge Tested |
|-------------|---------|-----------------|
| kind (local) | v1.32 (with DRA feature gate) | Manual |
| DOKS | v1.34 (DRA GA) | CI + Manual |

---

## API Surface

DRAForge uses the following `resource.k8s.io/v1` API endpoints:

| Resource       | API Group              | Verbs Used     | DRA Status |
|----------------|------------------------|----------------|------------|
| ResourceClaim  | `resource.k8s.io/v1`   | `get, list`    | GA         |
| ResourceSlice  | `resource.k8s.io/v1`   | `get, list`    | GA         |
| DeviceClass    | `resource.k8s.io/v1`   | `get, list`    | GA         |
| Pod            | `v1`                   | `get, list`    | GA         |
| Node           | `v1`                   | `get, list`    | GA         |

DRAForge **does not use** the following legacy API groups:
- `resource.k8s.io/v1beta1` (K8s ≤1.30, removed in v1.34)
- `resource.k8s.io/v1beta2` (K8s ≤1.32, removed in v1.34)
- `resource.k8s.io/v1alpha3` (alpha features)

---

## DRA Feature Status

The table below shows the **upstream Kubernetes DRA feature status** as of K8s v1.36.
This is informational; DRAForge may not implement all features.

| Feature                     | K8s v1.34 | K8s v1.35 | K8s v1.36 |
|-----------------------------|-----------|-----------|-----------|
| Core DRA (structured params)| **GA**    | **GA**    | **GA**    |
| ResourceHealthStatus        | Alpha     | Beta      | Beta      |
| Partitionable Devices       | Alpha     | Beta      | Beta      |
| Device Taints               | Alpha     | Beta      | Beta      |
| Device Taint Rules          | —         | Alpha     | Alpha     |
| Admin Access                | Alpha     | Beta      | Beta      |
| Extended Resource           | Alpha     | Beta      | **GA**    |
| Prioritized List            | —         | Alpha     | Beta      |
| DRAWorkloadResourceClaims   | Alpha     | Alpha     | Alpha     |
| Device Binding Conditions   | Alpha     | Alpha     | Alpha     |
| Node Allocatable Resources  | —         | —         | Alpha     |

Sources:
- [Kubernetes DRA concept docs](https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/)
- [Kubernetes v1.34: DRA has graduated to GA](https://kubernetes.io/blog/2025/09/01/kubernetes-v1-34-dra-updates/)
- [Kubernetes v1.36: DRA updates](https://kubernetes.io/blog/2026/05/07/kubernetes-v1-36-dra-136-updates/)

---

## DRAForge Support Matrix

| Feature                       | Kubernetes Status | DRAForge Status             | Notes |
|-------------------------------|-------------------|-----------------------------|-------|
| ResourceSlice discovery       | GA (v1.34+)       | supported                   | Full listing via `resource.k8s.io/v1` |
| ResourceClaim discovery       | GA (v1.34+)       | supported                   | Status, allocation, owner pod mapping |
| DeviceClass discovery         | GA (v1.34+)       | partial                     | Listed via discovery but not used in explain engine |
| Claim allocation explain      | GA (v1.34+)       | supported                   | Explain engine uses K8s events + conditions |
| Device health                 | Beta (v1.35+, default-on) | partial           | Reads `draforge.oaslananka/health` label from ResourceSlice; does **not** consume native `ResourceHealthStatus` |
| Consumable capacity           | GA (v1.34+)       | partial                     | Capacity values read; no capacity-exhaustion scoring |
| Device taints/tolerations     | Beta (v1.35+, default-on) | unsupported        | No taint reading or evaluation in DRAForge |
| Granular status authorization | Beta (v1.35+, default-on) | unsupported        | Admin-access subresources not implemented |
| CDI simulation                | GA (v1.34+)       | partial                     | CDI volume mounted in Helm chart; no CDI file generation |
| Partitionable devices         | Beta (v1.35+, default-on) | unsupported        | Not implemented |
| Prioritized device requests   | Beta (v1.35+, default-on) | unsupported        | Not implemented |
| Extended resource integration | GA (v1.37+)       | unsupported                 | Not implemented |
| DRAWorkloadResourceClaims     | Alpha (v1.34+)    | unsupported                 | Out of scope for 0.1.0 |

---

## Known Limitations

1. **DeviceClass discovery**: DRAForge lists DeviceClasses via the API but does
   **not** use them in the explain engine or doctor checks. DeviceClass selectors
   and CEL expressions are not evaluated.

2. **Device health**: DRAForge reads health from a custom label
   (`draforge.oaslananka/health`) on ResourceSlices, not from the upstream
   `ResourceHealthStatus` feature. Until DRAForge migrates to the standard
   health reporting, device health visibility depends on the driver populating
   this label.

3. **Device taints/tolerations**: Neither taints defined on ResourceSlice devices
   nor tolerations on ResourceClaims are inspected. The explain engine does not
   account for device-level taints.

4. **Granular status authorization**: The upstream DRA Admin Access feature
   allows per-device status visibility. DRAForge currently reads all
   ResourceSlices/ResourceClaims cluster-wide via its read-only ClusterRole.

5. **CDI output**: The node plugin mounts the CDI staging directory but does not
   generate CDI specification files. Real DRA drivers are expected to produce
   their own CDI output.

6. **Capacity scoring**: DRAForge reads and displays device capacity values but
   does not implement capacity-exhaustion scoring or prioritization across
   pools.

7. **Version assumptions**: DRAForge assumes `resource.k8s.io/v1` is served.
   Clusters that only serve v1beta1/v1beta2 (K8s < v1.34 with legacy API groups)
   require explicit API group configuration.

---

## RBAC Requirements

### Server (Read-Only)

The DRAForge server requires **read-only** access to present the dashboard and API:

```yaml
apiGroups: [""]
resources: [pods, nodes, namespaces, events]
verbs: [get, list, watch]

apiGroups: [resource.k8s.io]
resources: [resourceclaims, resourceslices, deviceclasses, resourceclaimtemplates]
verbs: [get, list, watch]

apiGroups: [draforge.oaslananka]
resources: [simulateddevicepools]
verbs: [get, list, watch]
```

### Controller (Simulation Write)

The DRAForge controller creates ResourceSlices and manages ResourceClaims
for simulation scenarios:

```yaml
apiGroups: [""]
resources: [pods, nodes, namespaces]
verbs: [get, list, watch]

apiGroups: [""]
resources: [events]
verbs: [create, patch]

apiGroups: [resource.k8s.io]
resources: [resourceclaims, resourceclaims/status, resourceslices]
verbs: [*]

apiGroups: [resource.k8s.io]
resources: [deviceclasses, resourceclaimtemplates]
verbs: [get, list, watch]

apiGroups: [draforge.oaslananka]
resources: [simulateddevicepools, simulateddevicepools/status]
verbs: [*]
```

### Node Plugin

The node plugin daemon only needs to read claims for its node:

```yaml
apiGroups: [""]
resources: [nodes, pods]
verbs: [get, list, watch]

apiGroups: [resource.k8s.io]
resources: [resourceclaims]
verbs: [get, list, watch]
```

### Separation Principle

- **Server** → never writes to any K8s resource. All mutations are CLI-only and
  use the operator's local `kubeconfig`.
- **Controller** → minimal write access: only `resourceclaims/status`,
  `resourceslices`, `events`, and its own CRD.
- **Node plugin** → scoped to read-only for its own node.

---

## Operational Notes

- **Feature gates**: DRAForge only requires the core `DynamicResourceAllocation`
  feature gate (GA and enabled by default since v1.34). Optional beta/alpha
  features (health, taints, etc.) are not required for basic functionality.
- **API priority**: DRAForge uses `resource.k8s.io/v1` exclusively. For clusters
  that only serve `v1beta2` (v1.32–v1.33), DRAForge will detect this and warn
  via the doctor check but is functionally compatible.
- **Kubelet plugin CDI path**: The Helm chart mounts
  `/var/lib/kubelet/device-plugins/cdi` for the node plugin. This path must
  exist on the host or be consistent with the kubelet CDI configuration.
- **ResourceClaimTemplate**: Read but not used for allocation by DRAForge.

---

## Future Work

The following improvements are planned for future releases:

- [ ] Migrate device health from custom label to `ResourceHealthStatus`
- [ ] Add DeviceClass selector evaluation to explain engine
- [ ] Add device taint/toleration awareness to explain engine
- [ ] Implement capacity-exhaustion scoring for pools
- [ ] Generate CDI spec files in the node plugin
- [ ] Support granular status authorization (Admin Access)
- [ ] Add E2E tests against real K8s v1.34+ clusters with a DRA driver

---

## References

- [Kubernetes DRA Documentation](https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/)
- [Set Up DRA in a Cluster](https://kubernetes.io/docs/tasks/configure-pod-container/assign-resources/set-up-dra-cluster/)
- [ResourceClaim v1 API](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/resource-claim-v1/)
- [ResourceSlice v1 API](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/resource-slice-v1/)
- [DeviceClass v1 API](https://kubernetes.io/docs/reference/kubernetes-api/resource/device-class-v1/)
- [K8s v1.34: DRA GA blog post](https://kubernetes.io/blog/2025/09/01/kubernetes-v1-34-dra-updates/)
- [K8s v1.36: DRA updates blog post](https://kubernetes.io/blog/2026/05/07/kubernetes-v1-36-dra-136-updates/)
- [DRA promotion to GA tracking issue](https://github.com/kubernetes/kubernetes/issues/131903)
