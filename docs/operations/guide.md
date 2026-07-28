# Operations Guide

This guide covers day-two operations for managing a DRAForge deployment, including upgrades, observability (metrics and logs), and cleanup procedures.

## Upgrades

Upgrading DRAForge primarily involves updating the Helm release.

1. **Update the Helm Repository/Chart:**
   Ensure you have the latest chart definitions. If you are using a remote repository, run `helm repo update`. If using the source directory, ensure you have pulled the latest changes.

2. **Run the Upgrade:**
   ```bash
   helm upgrade draforge deploy/helm/draforge \
     --namespace draforge-system
   ```

> **v0.3 exposure change:** External Gateway/Ingress resources are now disabled by default. Before upgrading an intentionally public deployment, prepare a secure exposure values file with TLS, an authentication proxy Service, and restrictive HTTPS CORS. The local-demo profile is explicitly unauthenticated and non-production.

3. **Verify Component Versions:**
   After the upgrade completes, verify the running versions:
   ```bash
   kubectl get pods -n draforge-system -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.containers[0].image}{"\n"}{end}'
   ```

## Runtime Health and Shutdown

Both the API server and simulation controller expose separate liveness and readiness endpoints:

- `/healthz` is process-only liveness and remains HTTP `200` during transient Kubernetes API outages.
- `/readyz` performs a read-only Kubernetes namespace list with a bounded request context. It returns a degraded HTTP `200` during the configured failure grace period and HTTP `503` if the dependency remains unavailable beyond that period.

The public response uses stable reason codes and never includes raw Kubernetes client errors, credentials, API tokens, kubeconfig paths, or cluster URLs.

The Helm chart configures both workloads with these defaults:

```yaml
server:
  lifecycle:
    readinessTimeout: 2s
    readinessGracePeriod: 15s
    shutdownTimeout: 5s
controller:
  lifecycle:
    readinessTimeout: 2s
    readinessGracePeriod: 15s
    shutdownTimeout: 5s
```

Set `readinessGracePeriod: 0` to fail readiness immediately. Timeout and shutdown values must be positive Go-style durations using `ms`, `s`, `m`, or `h`. Keep `shutdownTimeout` below the Pod termination grace period (Kubernetes defaults it to 30 seconds) so the process can complete its graceful shutdown before kubelet sends `SIGKILL`.

`draforge serve` listens for `SIGINT` and `SIGTERM`. On shutdown it stops accepting new connections, cancels active request contexts and SSE streams, waits up to `shutdownTimeout`, and force-closes remaining HTTP connections if the graceful deadline expires. The controller uses the same signal-aware readiness and shutdown settings for its runtime server.

For direct binary use, the corresponding flags are:

```text
--readiness-timeout
--readiness-grace-period
--shutdown-timeout
```

### Controller leader election and replicas

The controller uses a namespaced Kubernetes `Lease` by default. Every replica keeps
its `/healthz`, `/readyz`, and `/metrics` endpoints available, but only the current
lease holder starts reconciliation and simulated-allocation loops. The controller
Pod name is used as the lease identity through the Downward API.

```yaml
controller:
  replicaCount: 2
  leaderElection:
    enabled: true
    leaseDuration: 15s
    renewDeadline: 10s
    retryPeriod: 2s
```

The chart rejects `replicaCount` values above one when leader election is disabled.
The corresponding binary flags are `--leader-elect`,
`--leader-election-lease-name`, `--leader-election-lease-namespace`,
`--leader-election-identity`, `--leader-election-lease-duration`,
`--leader-election-renew-deadline`, and `--leader-election-retry-period`.

`draforge_controller_leader` is `1` only on the active replica and `0` on standby
replicas. A process that loses leadership stops its active loops and exits so the
Deployment can restart it as a fresh candidate.

Leader leases are not released early during shutdown. This prevents an old leader
and a replacement from overlapping while in-flight work drains. During an upgrade,
rollback, abrupt Pod loss, or node disruption, the replacement may therefore wait
up to `leaseDuration` before becoming active. Keep the lease duration short enough
for the required recovery objective, while preserving `leaseDuration >
renewDeadline > retryPeriod * 1.2`. Roll back by restoring the previous chart/image
version; the same Lease name is retained, so only one version can be active at a
time.

### Event-driven reconciliation

The active controller uses Kubernetes shared informers instead of fixed polling
intervals. Add, update, and delete events are coalesced into two typed workqueues:

- SimulatedDevicePool, Node, and ResourceSlice events trigger an idempotent pool
  synchronization and orphan cleanup pass.
- ResourceClaim, ResourceSlice, and DeviceClass events trigger an idempotent
  allocation synchronization pass.

Each queue has one worker, so pool passes are serialized with other pool passes
and allocation passes are serialized with other allocation passes; the two
pipelines can progress independently. A top-level synchronization error returned
by a worker increments `draforge_controller_reconcile_errors_total` and is retried
with exponential backoff starting at 100 milliseconds and capped at 30 seconds.
A successful pass clears the item's retry history. The current workers re-read
authoritative API state on each coalesced pass; informer events are the trigger,
not a scheduler-equivalent cached allocation model.

## Observability

DRAForge provides native support for monitoring and logging to assist operators in maintaining cluster health.

### Metrics

The API server exposes Prometheus metrics on its HTTP service. The controller exposes runtime health and metrics on the named `runtime` port, which defaults to TCP `8082` and serves `/metrics`.

The controller metrics Service is enabled by default, but the controller NetworkPolicy keeps all ingress closed until a monitoring peer is explicitly selected. This prevents unrelated namespaces from scraping the controller merely because the Service exists.

#### Allow a restricted monitoring peer

Use a namespace selector and, preferably, a pod selector:

```yaml
controller:
  metrics:
    networkPolicy:
      enabled: true
      namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: monitoring
      podSelector:
        matchLabels:
          app.kubernetes.io/name: prometheus
```

Install or upgrade with the values file:

```bash
helm upgrade --install draforge deploy/helm/draforge \
  --namespace draforge-system \
  --create-namespace \
  -f monitoring-values.yaml
```

The generated NetworkPolicy permits only matching pods in matching namespaces to reach the controller container metrics port. Verify namespace labels with:

```bash
kubectl get namespace monitoring --show-labels
```

Leaving `podSelector` empty allows every pod in the selected namespace, so production deployments should normally provide both selectors. Disabling `controller.metrics.service.enabled` removes the Service and keeps the controller ingress policy closed even if the metrics NetworkPolicy values remain configured.

#### Optional Prometheus Operator ServiceMonitor

A `ServiceMonitor` can be rendered when the Prometheus Operator CRDs are already installed:

```yaml
controller:
  metrics:
    serviceMonitor:
      enabled: true
      namespace: monitoring
      labels:
        release: prometheus
      interval: 30s
      scrapeTimeout: 10s
```

The ServiceMonitor selects the DRAForge controller Service in the Helm release namespace and scrapes the named `runtime` port at `/metrics`. It is disabled by default and schema validation rejects enabling it while the metrics Service is disabled. Enabling the ServiceMonitor does not open NetworkPolicy ingress; configure the restricted monitoring peer shown above as part of the same values profile.

### Logging

DRAForge components log operational details to standard output, making them compatible with standard Kubernetes log aggregators (e.g., Fluent Bit, Promtail).

- **Controller Logs:**
  The controller manages reconciliation and simulated allocations.
  ```bash
  kubectl logs -l app.kubernetes.io/name=draforge-controller -n draforge-system
  ```

- **API Server Logs:**
  The API server handles CLI and dashboard requests.
  ```bash
  kubectl logs -l app.kubernetes.io/name=draforge-api-server -n draforge-system
  ```

*Tip: Adjust log verbosity by setting the appropriate environment variables or flags defined in the component configurations if deeper debugging is required.*

## Cleanup Procedures

When you need to remove DRAForge or clean up test scenarios, follow these steps to ensure all resources are properly deleted.

### Removing Scenarios

Before uninstalling the core components, it's good practice to remove any applied scenarios (like SimulatedDevicePools and ResourceClaims) to allow for graceful termination.

```bash
kubectl delete -f examples/scenarios/basic-gpu.yaml
# Or, clean up specific pools
kubectl delete simulateddevicepools --all
```

### Uninstalling the Helm Release

To completely remove the DRAForge deployment from your cluster:

```bash
helm uninstall draforge -n draforge-system
```

### Removing CRDs

Helm does not automatically remove CRDs when a release is uninstalled. To fully clean up the cluster, you must remove the CRDs manually.
*Warning: This will delete all custom resources of this type in the cluster.*

```bash
kubectl delete -f deploy/crds/simulateddevicepool-crd.yaml
```

### Optional Showcase / Cloud Resources Cleanup

If you deployed the DOKS showcase using the provided Terraform modules, refer to the [Cost Control Guide](cost-control.md) for instructions on tearing down billable infrastructure using `task demo:down` or `terraform destroy`.
