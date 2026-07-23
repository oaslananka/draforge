# DRAForge - Architectural Design & Object Model

This document explains the core Dynamic Resource Allocation (DRA) object relationship model and details how DRAForge monitors, simulates, and diagnoses accelerator scheduling.

## The DRA Object Model

Kubernetes DRA introduces a flexible model where resource requests are handled by specific drivers. Below is the hierarchical relationship model implemented by Kubernetes DRA and observed by DRAForge:

```
+------------------+
|       Pod        |
+------------------+
         | (claims pod-resource claims)
         v
+------------------+
|  ResourceClaim   |
+------------------+
         | (requests allocation of a device class)
         v
+------------------+
|   DeviceClass    |
+------------------+
         | (allocated from)
         v
+------------------+
|   ResourceSlice  | (cluster-scoped, published per node)
+------------------+
         | (contains)
         v
+------------------+
|      Device      | (represented by Attributes & Capacity)
+------------------+
         | (part of)
         v
+------------------+
|    DevicePool    | (managed by Driver)
+------------------+
```

### 1. DevicePool & ResourceSlice
A **DevicePool** is a logical set of accelerator hardware devices (such as GPUs, FPGAs, or edge devices) managed by a specific DRA driver on a node. The driver publishes this pool state to the Kubernetes API server using the **ResourceSlice** API.
- In `resource.k8s.io/v1`, ResourceSlices are cluster-scoped.
- Each ResourceSlice contains a list of `Devices`, including their unique names, attributes (e.g. model, vendor, driver version), and capacities (e.g. device memory).

### 2. DeviceClass
A **DeviceClass** defines the parameters for selecting and configuring a device. It serves as a template, defining which driver to use and specifying selection filters (CEL expressions or label selectors) for matching target devices.

### 3. ResourceClaim
A **ResourceClaim** represents a pod's request for resource allocation. The claim specifies which `DeviceClass` it requires and outlines details of the request (e.g. "exactly 1 GPU" or "first-available GPU with memory > 80Gi").

### 4. Pod Claim Binding
When a Pod is scheduled, its `spec.resourceClaims` reference the `ResourceClaim`. The scheduler and the DRA driver collaborate to allocate a specific device from a `ResourceSlice` to the claim. Once allocated, the claim status is updated with the device identifier, pool details, and a node selector binding the pod to the node where the hardware resides.

---

## DRAForge Observation & Simulation

```
           +-----------------------------+
           |         Vite Web UI         |
           +-----------------------------+
                          ^
                          | (Server-Sent Events)
                          v
+--------------------------------------------------------+
|                    DRAForge Server                     |
|  - Real-time Graph Builder                             |
|  - REST API & SSE Streaming                            |
+--------------------------------------------------------+
    ^
    | (Query / Watch)
    v
+--------------------------------------------------------+
|                  Kubernetes API Server                 |
|  - ResourceSlices (v1)                                 |
|  - ResourceClaims (v1)                                 |
|  - DeviceClasses (v1)                                  |
|  - SimulatedDevicePools (v1alpha1 CRD)                 |
+--------------------------------------------------------+
    ^                                               ^
    | (Reconcile / Allocate)                        | (Publish spec)
    v                                               v
+-------------------------+             +------------------------+
|   DRAForge Controller   |             |   DRAForge Node Plugin |
|  - Allocation Simulator |             |   - Dynamic CDI Specs  |
+-------------------------+             +------------------------+
```

### 1. DRAForge Server
The Server queries the Kubernetes API for active DRA resources and constructs a relationship graph of all nodes. It exposes this data through a REST API and streams updates via Server-Sent Events (SSE) to the frontend React dashboard.

### 2. DRAForge Simulation Controller
The Controller watches `SimulatedDevicePool` custom resources (applied by scenarios) and generates corresponding cluster-scoped `ResourceSlice` objects to simulate hardware. It also acts as a fallback allocator: it watches for pending `ResourceClaims` and allocates simulated devices by updating the claim status with node-affinity details.

### 3. Node Plugin DaemonSet
The Node Plugin runs on each node and generates CDI (Container Device Interface) specification JSONs. In the default `demo` mode it writes an explicit static document to an isolated `emptyDir`. In opt-in `node` mode it lists allocated `ResourceClaim` devices for the current node and atomically replaces the kubelet-visible CDI document using a same-directory temporary file, file and directory fsync, and rename. Kubernetes API or write failures preserve the last-known-good document and make `/readyz` return `503`; `/healthz` remains process-only liveness. Device deduplication uses the complete `<driver>/<pool>/<device>` identity.
