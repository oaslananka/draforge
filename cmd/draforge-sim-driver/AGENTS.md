# Sim Driver and CDI Boundary Instructions

These instructions apply to `cmd/draforge-sim-driver/**` and supplement the root instructions.

The simulator node plugin may write CDI documents and, in explicit node mode, cross a host filesystem boundary.

## Modes

- Demo mode remains isolated and non-root where documented.
- Host-integrated `nodePlugin.outputMode=node` is explicit opt-in and must fail closed when Kubernetes state or the host CDI directory is unavailable.
- Do not broaden host mounts, Linux capabilities, privilege, or service-account authority for convenience.

## CDI integrity

- Preserve atomic same-directory write/replace semantics, fsync/rename durability, and last-known-good behavior.
- Validate device identity and node allocation before writing CDI output.
- Do not partially publish a document after an upstream/API/write failure.
- Readiness must reflect whether required output can be maintained; liveness remains process-oriented as documented.

## Security

Keep the workload controls documented in `SECURITY.md`: no privilege escalation, RuntimeDefault seccomp, read-only root filesystem where supported, bounded writable volumes, and minimum capabilities.

## Verification

Run focused sim-driver tests and the workload/CDI verification scripts. Node-mode changes also require install/e2e evidence appropriate to the host integration.
