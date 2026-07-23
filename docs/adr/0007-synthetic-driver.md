# ADR-0007: Synthetic DRA Driver Design

- **Status**: Approved
- **Context**: Testing DRA requires physical hardware (GPUs) which is expensive and unavailable.
- **Decision**: Implement a synthetic DRA driver (draforge-sim-driver) that publishes mock ResourceSlices and handles claims via custom resource definitions (SimulatedDevicePool).
- **Alternatives**: Mocking the Kubernetes API server itself (doesn't run real scheduling).
- **Consequences**: Can simulate GPUs, cameras, FPGAs, and high-speed NICs.
- **Security Considerations**: Demo mode is non-root and isolated. Explicit node mode mounts only the kubelet CDI directory and uses UID 0 solely for the root-owned host path while remaining non-privileged, with privilege escalation disabled, a read-only root filesystem, seccomp RuntimeDefault, and all Linux capabilities dropped.
- **Operational Considerations**: Demo mode writes static test devices to `emptyDir`. Node mode uses complete driver/pool/device allocation identity, atomic CDI replacement, last-known-good preservation, and separate liveness/readiness reporting.
