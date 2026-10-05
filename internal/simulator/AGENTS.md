# Simulator and Controller Instructions

These instructions apply to `internal/simulator/**` and supplement the root instructions.

This package owns simulated allocation/controller behavior and can mutate Kubernetes DRA state. Treat it as an authority boundary.

## Reconciliation

- Reconciliation must be deterministic and idempotent under retries.
- Preserve Kubernetes ownership/conflict semantics and never overwrite state owned by another controller without an explicit contract.
- Unknown, malformed, unsupported, or conflicting allocation facts must not be converted into a successful allocation.
- Keep capacity accounting bounded and consistent across retries.
- Preserve complete device identity and request/result relationships.

## Simulation truth

Simulation proves only the modeled behavior exercised by tests. Do not present simulated allocation as evidence of real hardware, vendor driver, or scheduler production behavior.

## Failure semantics

- Kubernetes API update failures must surface through the controller/error policy rather than silently succeeding.
- Preserve explicit status/condition behavior and useful diagnostics.
- Avoid unbounded retry loops, duplicate events, or repeated side effects.

## Verification

Run focused simulator/controller tests, including conflict, capacity, selector, sharing, ownership, and API-error cases. Relevant Kubernetes e2e/install tests are required when external behavior changes.
