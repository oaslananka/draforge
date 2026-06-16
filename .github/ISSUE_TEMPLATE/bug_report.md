name: Bug Report
description: Report a bug to help us improve DRAForge.
labels: [bug]
body:
  - type: markdown
    attributes:
      value: |
        Thank you for reporting a bug! Please fill in the details below.
        For security vulnerabilities, **do not** use this form — see [SECURITY.md](https://github.com/oaslananka/draforge/blob/main/SECURITY.md).
  - type: input
    id: version
    attributes:
      label: DRAForge Version
      description: Output of `draforge version` or the release tag you are using.
      placeholder: e.g. v0.1.0, main@abc1234
    validations:
      required: true
  - type: input
    id: k8s_version
    attributes:
      label: Kubernetes Version
      description: Output of `kubectl version --short`.
      placeholder: e.g. v1.32.0
    validations:
      required: true
  - type: dropdown
    id: area
    attributes:
      label: Area
      description: Which component is affected?
      options:
        - CLI (draforge)
        - Web Dashboard
        - Controller
        - Simulator / Scenarios
        - Helm Chart
        - Documentation
        - Other
    validations:
      required: true
  - type: textarea
    id: description
    attributes:
      label: Bug Description
      description: A clear and concise description of what the bug is.
    validations:
      required: true
  - type: textarea
    id: steps
    attributes:
      label: Steps to Reproduce
      description: How can we reproduce this behavior?
      placeholder: |
        1. Run '...'
        2. Click '...'
        3. See error
    validations:
      required: true
  - type: textarea
    id: expected
    attributes:
      label: Expected Behavior
      description: What did you expect to happen?
    validations:
      required: true
  - type: textarea
    id: actual
    attributes:
      label: Actual Behavior
      description: What actually happened?
    validations:
      required: true
  - type: textarea
    id: logs
    attributes:
      label: Logs and Output (redacted)
      description: Relevant CLI output, server logs, or browser console errors.
        Redact any sensitive information (tokens, kubeconfig contents, IPs).
    validations:
      required: false
  - type: textarea
    id: environment
    attributes:
      label: Environment
      description: |
        OS, cluster provider (Kind, DOKS, etc.), DRA feature gate status, and any
        relevant configuration.
      placeholder: |
        OS: macOS 14.5
        Cluster: Kind v0.27.0
        DRA Feature Gate: Enabled
    validations:
      required: false
