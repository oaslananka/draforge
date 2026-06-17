name: Feature Request
description: Suggest an idea for DRAForge.
labels: [enhancement]
body:
  - type: markdown
    attributes:
      value: |
        Thank you for suggesting a feature! Please describe the use case and how it
        relates to DRAForge's scope.
  - type: textarea
    id: use_case
    attributes:
      label: Use Case
      description: What problem are you trying to solve? Who would benefit from this feature?
    validations:
      required: true
  - type: textarea
    id: dra_relevance
    attributes:
      label: DRA Relevance
      description: How does this feature relate to Dynamic Resource Allocation,
        observability, simulation, or diagnostics?
    validations:
      required: true
  - type: textarea
    id: proposed
    attributes:
      label: Proposed Behavior
      description: A clear and concise description of what you want to happen.
    validations:
      required: true
  - type: textarea
    id: alternatives
    attributes:
      label: Alternatives Considered
      description: Any alternative solutions, workarounds, or features you have considered.
    validations:
      required: false
  - type: textarea
    id: impact
    attributes:
      label: Compatibility and Security Impact
      description: Would this change affect existing CLI commands, API endpoints,
        or the security model? Update expectations?
    validations:
      required: false
