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

3. **Verify Component Versions:**
   After the upgrade completes, verify the running versions:
   ```bash
   kubectl get pods -n draforge-system -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.containers[0].image}{"\n"}{end}'
   ```

## Observability

DRAForge provides native support for monitoring and logging to assist operators in maintaining cluster health.

### Metrics

Both the API server and the reconciler expose Prometheus metrics via a `/metrics` handler.

- **Endpoint:** The metrics are exposed on the components' respective HTTP ports (typically port 8080) at the `/metrics` path.
- **Integration:** You can configure your cluster's Prometheus operator to scrape these endpoints. Example `ServiceMonitor` configurations are not included by default but can be added alongside the Helm deployment to target the `draforge-controller` and `draforge-api-server` services.

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

### Showase / Cloud Resources Cleanup

If you deployed the DOKS showcase using the provided Terraform modules, refer to the [Cost Control Guide](cost-control.md) for instructions on tearing down billable infrastructure using `task demo:down` or `terraform destroy`.