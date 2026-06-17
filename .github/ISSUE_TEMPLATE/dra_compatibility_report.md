name: DRA Driver Compatibility Report
description: Report compatibility results or issues with a specific DRA driver.
labels: [compatibility]
body:
  - type: markdown
    attributes:
      value: |
        Use this template to report how a specific DRA driver works (or doesn't work)
        with DRAForge. This helps us maintain the [compatibility matrix](https://github.com/oaslananka/draforge/blob/main/docs/compatibility/kubernetes-dra.md).
  - type: input
    id: k8s_version
    attributes:
      label: Kubernetes Version
      description: Output of `kubectl version --short`.
      placeholder: e.g. v1.32.0
    validations:
      required: true
  - type: input
    id: dra_api_version
    attributes:
      label: DRA API Version
      description: The DRA API group/version in use (e.g. `resource.k8s.io/v1` or `v1alpha3`).
      placeholder: e.g. resource.k8s.io/v1
    validations:
      required: true
  - type: input
    id: driver_name
    attributes:
      label: Driver Name
      placeholder: e.g. nvidia-k8s-dra-driver
    validations:
      required: true
  - type: input
    id: driver_version
    attributes:
      label: Driver Version
      placeholder: e.g. v0.1.0
    validations:
      required: true
  - type: input
    id: cluster_provider
    attributes:
      label: Cluster Provider
      description: Where is the cluster running?
      placeholder: e.g. Kind, DOKS, AKS, EKS, GKE, on-prem
    validations:
      required: true
  - type: textarea
    id: resource_details
    attributes:
      label: Resource Details
      description: |
        If applicable, describe the ResourceSlice, ResourceClaim, and DeviceClass
        objects involved. Output from `kubectl get` is helpful.
      placeholder: e.g. "3 ResourceSlices, 1 DeviceClass 'gpu.nvidia.com', 2 pending claims"
    validations:
      required: false
  - type: textarea
    id: expected_support
    attributes:
      label: Expected Support Level
      description: |
        What DRAForge features do you expect to work with this driver?
        (e.g. discover, doctor, explain, graph, simulation)
    validations:
      required: true
  - type: textarea
    id: observed_issue
    attributes:
      label: Observed Issue
      description: What is not working as expected? Include CLI outputs, logs, or
        error messages (redacted).
    validations:
      required: true
