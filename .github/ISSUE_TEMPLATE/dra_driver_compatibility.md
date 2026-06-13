name: DRA Driver Compatibility
description: Report compatibility results or issues with a specific DRA driver.
labels: [compatibility]
body:
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
  - type: textarea
    id: compatibility_details
    attributes:
      label: Compatibility Details
      description: What worked? What didn't work? Provide CLI outputs or logs.
    validations:
      required: true
