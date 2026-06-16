name: Security Hardening Suggestion
description: Propose a security improvement for DRAForge.
labels: [security]
body:
  - type: markdown
    attributes:
      value: |
        Thank you for helping make DRAForge more secure.

        **If you are reporting a current vulnerability, do not use this form.**
        See [SECURITY.md](https://github.com/oaslananka/draforge/blob/main/SECURITY.md)
        for the responsible disclosure process.
  - type: dropdown
    id: area
    attributes:
      label: Area
      description: Which area does this suggestion relate to?
      options:
        - Web Dashboard
        - API Server
        - CLI
        - Helm Chart / RBAC
        - Dependency / Supply Chain
        - Infrastructure / Terraform
        - Secrets / Credential Handling
        - CI Pipeline
        - Other
    validations:
      required: true
  - type: textarea
    id: risk
    attributes:
      label: Risk Description
      description: What security risk or weakness have you identified?
      placeholder: e.g. "The dashboard API does not validate input on the SSE endpoint."
    validations:
      required: true
  - type: textarea
    id: mitigation
    attributes:
      label: Suggested Mitigation
      description: How would you propose addressing this risk?
    validations:
      required: true
  - type: dropdown
    id: backward_compat
    attributes:
      label: Backward Compatibility
      description: Would your suggested mitigation break existing behavior?
      options:
        - Yes (breaking change)
        - No (backward compatible)
        - Unknown / Needs discussion
    validations:
      required: true
  - type: checkboxes
    id: sensitive
    attributes:
      label: Sensitive Details
      description: >
        Does this suggestion include any sensitive details (e.g., tokens,
        internal infrastructure, unpatched vulnerabilities)?
      options:
        - label: >
            I confirm that this suggestion does **not** contain sensitive details
            that should be disclosed privately. If it does, I will send it via
            email to `oaslananka@gmail.com` instead.
          required: true
