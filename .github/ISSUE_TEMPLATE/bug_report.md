name: Bug report
description: Create a report to help us improve.
labels: [bug]
body:
  - type: markdown
    attributes:
      value: |
        Thank you for reporting a bug! Please fill in the details below.
  - type: textarea
    id: description
    attributes:
      label: Bug Description
      description: A clear and concise description of what the bug is.
    validations:
      required: true
  - type: textarea
    id: reproduction
    attributes:
      label: Steps to Reproduce
      description: How do we reproduce this behavior?
    validations:
      required: true
  - type: textarea
    id: environment
    attributes:
      label: Environment Info
      description: |
        Kubernetes version, DRAForge version, OS, etc.
    validations:
      required: true
