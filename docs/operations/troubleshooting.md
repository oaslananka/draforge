# Troubleshooting Guide

This guide provides solutions for common issues encountered when running DRAForge. It maps symptoms to diagnostic commands and potential fixes.

## General Diagnostics

If you suspect an issue with the DRA configuration or DRAForge components, your first step should be to run the built-in diagnostics tool:

```bash
draforge doctor
```

This command runs active cluster and driver diagnostic checks, including validating API availability, version compatibility, and checking for stale `ResourceSlice` objects.

## Common Symptoms & Solutions

### Symptom: ResourceClaims are stuck in a "Pending" state

**Description:** Pods requesting dynamic resources are not scheduling, and their associated `ResourceClaim` objects remain pending.

**Diagnostic Command:**
```bash
draforge explain <claim-name>
```

**Potential Causes & Fixes:**
- **Insufficient Capacity:** The explain output will highlight if there are no devices in the `SimulatedDevicePool` matching the selector, or if the pool is fully allocated.
  - *Fix:* Check pool capacities using `draforge discover`. Add more devices to the pool or free up existing allocations.
- **Node Affinity Mismatch:** The claim might have a node selector that doesn't match the nodes where devices are available.
  - *Fix:* Review the explain output to identify which nodes were rejected. Update the pod or claim configuration to target the correct nodes.
- **Device Health:** The explain engine verifies candidate device health.
  - *Fix:* If devices are reported as unhealthy, inspect the specific node or device logs for failures. The output will provide remediation hints.

### Symptom: Missing APIs or Feature Gates

**Description:** Errors indicating that `resource.k8s.io/v1` or `SimulatedDevicePool` resources cannot be found.

**Diagnostic Command:**
```bash
draforge doctor
```

**Potential Causes & Fixes:**
- **DynamicResourceAllocation Feature Gate Disabled:** The Kubernetes cluster must have the DRA feature gate enabled.
  - *Fix:* Ensure your cluster is launched with `--feature-gates=DynamicResourceAllocation=true`. See your cluster provider's documentation for instructions.
- **CRDs Not Installed:** The `SimulatedDevicePool` CRD is missing.
  - *Fix:* Apply the CRD manually (`kubectl apply -f deploy/crds/simulateddevicepool-crd.yaml`) or ensure the Helm chart was installed correctly.

### Symptom: Stale ResourceSlices

**Description:** Allocations appear incorrect, or the dashboard shows nodes that no longer exist.

**Diagnostic Command:**
```bash
draforge doctor
```

**Potential Causes & Fixes:**
- **Node Eviction/Deletion:** A node was removed from the cluster abruptly, and its `ResourceSlice` was not cleaned up.
  - *Fix:* The `doctor` command (`StaleResourceSliceCheck`) actively identifies stale slices. You can manually delete the stale slice using `kubectl delete resourceslice <slice-name>`.

### Symptom: Dashboard Connectivity Issues

**Description:** The React web dashboard is inaccessible or fails to update in real-time.

**Potential Causes & Fixes:**
- **Server Not Running:** Ensure the API server is active.
  - *Fix:* Check the deployment status: `kubectl get deployment -n draforge-system`.
- **SSE Stream Interruption:** Network issues might disrupt the Server-Sent Events stream.
  - *Fix:* Refresh the page. The dashboard is designed to rebuild the state from the current cluster state upon reconnection. Note that history is not persisted across reconnects.

## Gathering Support Information

If you need further assistance, gather the following information before opening an issue:

1. The output of `draforge version`.
2. The complete output of `draforge doctor`.
3. Relevant logs from the DRAForge controller:
   ```bash
   kubectl logs -l app.kubernetes.io/name=draforge-controller -n draforge-system
   ```