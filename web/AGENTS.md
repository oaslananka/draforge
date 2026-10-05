# Web Dashboard Instructions

Applies to `web/**`.

The dashboard is a Vite/React consumer of DRAForge API/SSE data. It must not become a second Kubernetes decision engine.

- Treat API/SSE/project text as untrusted UI input and preserve escaping/safe rendering.
- Preserve deterministic tests for navigation, query state, SSE, graph selection and diagnostics.
- Keep accessibility gates meaningful: keyboard navigation, visible focus, reduced motion and narrow viewport behavior are product contracts.
- Do not weaken browser/axe checks or replace real interaction coverage with snapshots only.
- Keep frontend dependency maturity, peer, lifecycle-script and high-severity audit policies intact.
- Do not expose unauthenticated production-public semantics simply because the demo showcase profile permits them.

Run `pnpm --dir web test`, `lint`, `build` and browser tests when UI/accessibility behavior changes.
