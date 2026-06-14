# ADR-0007: Synthetic DRA Driver Design

- **Status**: Approved
- **Context**: Testing DRA requires physical hardware (GPUs) which is expensive and unavailable.
- **Decision**: Implement a synthetic DRA driver (draforge-sim-driver) that publishes mock ResourceSlices and handles claims via custom resource definitions (SimulatedDevicePool).
- **Alternatives**: Mocking the Kubernetes API server itself (doesn't run real scheduling).
- **Consequences**: Can simulate GPUs, cameras, FPGAs, and high-speed NICs.
- **Security Considerations**: Driver does not mount actual host devices; runs as non-privileged.
- **Operational Considerations**: Simulates allocation via CDI-injected files or environment variables.
