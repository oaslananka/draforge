# Simulator Scenarios

DRAForge includes reusable simulator scenarios under examples/scenarios.

Catalog:

- success.yaml: healthy pool with enough GPU capacity.
- no-match.yaml: attributes differ from common high-end GPU selectors.
- capacity.yaml: zero-device pool for capacity diagnostics.
- delayed-binding.yaml: multi-node target list for delayed binding checks.
- multi-node.yaml: four simulated devices across two target nodes.

Apply a scenario:

```bash
kubectl apply -f examples/scenarios/success.yaml
```

Inspect pools, devices, claims, graph, and doctor output with the CLI or dashboard.

To add a scenario, create a YAML file in examples/scenarios using the SimulatedDevicePool CRD. Keep it focused on one diagnostic intent and update this catalog.
