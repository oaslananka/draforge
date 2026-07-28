# DRAForge Dashboard

The web dashboard is a read-only React single-page application served by the DRAForge server. It visualizes cluster DRA resources, device pools, claims, allocation status, and the live resource relationship graph.

## Read-Only Model

The dashboard uses a strict **read-only** API model, but it is not public by default:

- All data is fetched via HTTP `GET` requests; the frontend performs no POST/PUT/PATCH/DELETE operations.
- Cluster modifications remain CLI-only and use the administrator's `kubeconfig`.
- A normal Helm install creates only internal services. Use `kubectl port-forward` for local operator access.
- The standalone binary defaults CORS to `*` for local use. Secure Helm exposure sets an explicit HTTPS origin. CORS is not an authentication boundary.

### Metadata visible to dashboard readers

An authenticated dashboard reader can observe resource names and namespaces, node identity/readiness, pod-to-claim relationships, ResourceClaims and allocation state, ResourceSlices, DeviceClasses, device attributes and capacities, simulated pools, diagnostic results, graph relationships, and selected Kubernetes event evidence. Treat this as operational cluster metadata and protect it accordingly.

## API Endpoints

| Endpoint | Method | Description |
|---|---|---|
| `/api/summary` | GET | Cluster-wide counts (pools, devices, claims, doctor status) |
| `/api/pools` | GET | Simulated device pools |
| `/api/devices` | GET | Discovered devices |
| `/api/claims` | GET | ResourceClaims with every request/class alternative and complete allocation identity |
| `/api/graph` | GET | Snapshot of the resource relationship graph |
| `/api/doctor` | GET | Diagnostics check results (PASS/WARN/FAIL) |
| `/api/explain?claim=X&namespace=default` | GET | Allocation explanation tree for a claim |
| `/api/stream` | SSE | Live graph updates via Server-Sent Events |

## SSE Stream

The `/api/stream` endpoint pushes `ResourceGraph` JSON updates every 5 seconds.

The dashboard handles the stream with controlled reconnection:

- Connection opens → status shows `connected`
- On error/close → status shows `reconnecting` with backoff: 1s → 2s → 5s (max)
- On successful reconnect → resets to 1s and shows `connected`
- Component unmount → EventSource closed, no memory leak

A stream status badge is visible in the dashboard header.

## Frontend architecture and tests

`App.tsx` is a small composition root. API/query state is isolated in `useDashboardData`, claim explanation requests are isolated in `useClaimExplanation`, SSE lifecycle remains in `useSSE`, and tab rendering is delegated to `DashboardViewRouter`. Header, footer, graph, and view state components are independently reviewable.

The Vitest + repository-owned linkedom environment + React Testing Library suite runs without Kubernetes or a live API server. Local `web/vendor` adapters supply the narrow CSSOM, HTML escaping, and property-hyphenation contracts required by linkedom without introducing scanner-flagged npm helper packages. Deterministic mocks cover:

- initial and partial API failures plus empty states;
- SSE connected, reconnecting, malformed-message, and cleanup behavior;
- namespace-qualified duplicate claim selection and explain requests;
- graph-to-explain navigation;
- diagnostics rendering and active-tab accessibility;
- keyboard selection of SVG graph nodes.

Run the same required CI gate locally with `pnpm --dir web test`. Lint and production build remain separate required checks.

## Empty Cluster Behavior

When no DRA resources are discovered, the dashboard shows clear messages:

- **Pools tab**: "No DRA resources discovered yet."
- **Devices tab**: "No DRA resources discovered yet."
- **Claims tab**: lists every requested DeviceClass and every `<request>=<driver>/<pool>/<device>@<node>` allocation identity. Missing node identity is shown as `unknown-node` rather than inferred from a pool name. Empty state: "No ResourceClaims found in the current cluster."
- **Graph tab**: represents each allocation result as its own orange `Allocation` node between the claim and resolved device, so repeated allocations to the same device identity remain visible.
- **Graph tab**: "No resource relationships to display. Deploy a SimulatedDevicePool scenario to populate the graph."
- **Doctor tab**: "Unable to load DRAForge diagnostics." (on error)

## Error Handling

API errors are displayed inline without raw stack traces:

- Connection errors show a warning banner at the top of the page.
- Individual tab errors show a per-section message.
- Structured errors `{ "error": { "message": "...", "code": "..." } }` are parsed safely.
- Legacy `{ "error": "..." }` format is also supported.

## Development

```bash
# Install dependencies
pnpm --dir web install

# Start dev server (port 3000, proxies /api to localhost:8080)
pnpm --dir web dev

# Build for production
pnpm --dir web build

# Lint
pnpm --dir web lint
```

The production build outputs to `web/dist/`, which is served by the Go server from `./web/dist/`.

## Troubleshooting

| Symptom | Likely Cause | Check |
|---|---|---|
| Dashboard shows "Disconnected" | Go server not running or /api/stream unreachable | Server logs, network |
| All data shows "No DRA resources" | No SimulatedDevicePool CRD applied | `kubectl get simpool` |
| Doctor tab shows no data | Server cannot connect to cluster | `kubectl cluster-info` |
| Graph empty | No resource relationships exist | Deploy a scenario |


## Browser and Accessibility Support

DRAForge uses a **rolling Playwright support policy**. Pull requests run the exact browser revisions bundled with the repository-pinned `@playwright/test` version against:

- Playwright Chromium on a desktop profile;
- Playwright Firefox on a desktop profile;
- Playwright WebKit on a desktop profile; and
- Playwright Chromium with the Pixel 7 mobile profile as the representative narrow viewport.

These are Playwright-maintained browser builds, not claims of support for every branded browser release. Updating `@playwright/test` and the lockfile updates the tested rolling browser revisions together. The compatibility gate mocks the dashboard REST and SSE boundaries deterministically, so it does not require Kubernetes, cloud credentials, or a running DRAForge backend.

The Chromium project additionally runs axe checks tagged for WCAG 2.0, 2.1, and 2.2 levels A and AA on Overview, Graph, Doctor, and the global error state. Serious or critical findings block the browser job. Automated checks cannot prove complete WCAG conformance; the manual keyboard review below remains required for interaction changes.

Run the browser gate locally from the repository root:

```bash
task web:test:browser:install
task web:test:browser
```

The install command downloads the exact Chromium, Firefox, and WebKit revisions and may install supported Linux system packages. Browser traces, screenshots, videos, and the HTML report are generated only for failed runs under `web/test-results/` and `web/playwright-report/`; both directories are gitignored.

### Manual keyboard checklist

For changes to navigation, graph interaction, status indicators, diagnostics, focus styles, or responsive layout:

1. Use only `Tab`, `Shift+Tab`, `Enter`, and `Space`; confirm every interactive control has a clearly visible focus indicator.
2. Navigate to Graph, select a ResourceClaim node with `Enter` or `Space`, and activate **Diagnose Allocation** without a pointer.
3. Confirm the live stream status is announced as a named status and the Doctor summary opens the diagnostics view.
4. Enable the operating system or browser reduced-motion preference; confirm graph nodes remain fixed and non-essential CSS animation is disabled.
5. At a narrow mobile viewport, confirm the navigation remains reachable and the page does not introduce horizontal document scrolling.
