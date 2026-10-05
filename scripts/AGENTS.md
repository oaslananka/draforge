# Policy Script Instructions

Applies to `scripts/**`.

These scripts implement repository policy for E2E orchestration, release provenance, image/chart verification, workload security, frontend dependency policy, CI privilege boundaries and documentation contracts.

- Fail closed when required evidence is missing, malformed or contradictory.
- Do not weaken thresholds, allowlists, image/platform checks, security settings or release ancestry checks merely to pass CI.
- Keep path/process handling portable and bounded.
- Shell scripts must preserve `set -euo pipefail` style failure semantics where already used.
- A verifier change needs focused self-test/fixture coverage where the repository provides it.
- Do not update expected fixtures solely to hide a regression.
- External/provider commands must keep cost/destructive behavior explicit.

Run the script's own test/self-test plus the Task/CI gate that consumes it.
