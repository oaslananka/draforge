import type { Page, Route } from '@playwright/test';

const graph = {
  nodes: [
    {
      id: 'claim/team-b/shared',
      type: 'ResourceClaim',
      label: 'shared',
      metadata: { namespace: 'team-b', status: 'Allocated' },
    },
  ],
  edges: [],
};

const responses: Record<string, unknown> = {
  '/api/summary': {
    poolsCount: 1,
    devicesCount: 1,
    claimsCount: 1,
    doctorStatus: { PASS: 1, WARN: 0, FAIL: 1 },
    timestamp: '2026-07-28T07:00:00Z',
  },
  '/api/version': { version: 'v0.3.0', commit: 'browser-test' },
  '/api/pools': [
    {
      name: 'pool-a',
      driverName: 'driver.example',
      nodeName: 'worker-a',
      deviceType: 'gpu',
      health: 'Healthy',
    },
  ],
  '/api/devices': [
    {
      id: 'device/driver.example/pool-a/gpu-0',
      name: 'gpu-0',
      type: 'gpu',
      nodeName: 'worker-a',
      poolName: 'pool-a',
      isSynthetic: true,
      status: 'Healthy',
    },
  ],
  '/api/claims': [
    {
      name: 'shared',
      namespace: 'team-b',
      status: 'Allocated',
      requests: [
        {
          name: 'gpu',
          mode: 'Exactly',
          alternatives: [
            {
              deviceClassName: 'gpu-class',
              allocationMode: 'ExactCount',
              count: 1,
            },
          ],
        },
      ],
      allocations: [
        {
          request: 'gpu',
          driverName: 'driver.example',
          poolName: 'pool-a',
          deviceName: 'gpu-0',
          nodeName: 'worker-a',
        },
      ],
      ownerPodName: 'consumer-a',
    },
  ],
  '/api/doctor': {
    timestamp: '2026-07-28T07:00:00Z',
    summary: { PASS: 1, WARN: 0, FAIL: 1 },
    results: [
      {
        id: 'DRA-001',
        name: 'Resource API availability',
        category: 'cluster',
        status: 'FAIL',
        severity: 'high',
        evidence: 'ResourceClaims API is unavailable.',
        remediation: 'Enable the resource.k8s.io/v1 API.',
        docReference: 'docs/operations/troubleshooting.md',
      },
    ],
  },
  '/api/explain': {
    targetName: 'shared',
    targetType: 'claim',
    allocated: true,
    reasonTree: {
      message: 'Claim is allocated.',
      confidence: 'confirmed',
      evidence: 'The claim references driver.example/pool-a/gpu-0.',
      sourceType: 'ResourceClaim',
    },
    remedy: [],
  },
};

export interface DashboardMockOptions {
  readonly summaryError?: boolean;
}

async function fulfillApi(route: Route, options: DashboardMockOptions) {
  const url = new URL(route.request().url());
  if (options.summaryError && url.pathname === '/api/summary') {
    await route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({
        error: { message: 'summary unavailable', code: 'unavailable' },
      }),
    });
    return;
  }

  const response = responses[url.pathname];
  if (response === undefined) {
    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ error: { message: 'not found' } }),
    });
    return;
  }

  await route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(response),
  });
}

export async function installDashboardMocks(
  page: Page,
  options: DashboardMockOptions = {},
) {
  await page.addInitScript((initialGraph) => {
    class MockEventSource {
      static readonly CONNECTING = 0;
      static readonly OPEN = 1;
      static readonly CLOSED = 2;

      readonly CONNECTING = 0;
      readonly OPEN = 1;
      readonly CLOSED = 2;
      readonly url: string;
      readonly withCredentials = false;
      readyState = MockEventSource.CONNECTING;
      onopen: ((event: Event) => void) | null = null;
      onmessage: ((event: MessageEvent<string>) => void) | null = null;
      onerror: ((event: Event) => void) | null = null;

      constructor(url: string | URL) {
        this.url = String(url);
        window.setTimeout(() => {
          this.readyState = MockEventSource.OPEN;
          this.onopen?.(new Event('open'));
          this.onmessage?.(
            new MessageEvent('message', { data: JSON.stringify(initialGraph) }),
          );
        }, 0);
      }

      close() {
        this.readyState = MockEventSource.CLOSED;
      }

    }

    Object.defineProperty(window, 'EventSource', {
      configurable: true,
      value: MockEventSource,
      writable: true,
    });
  }, graph);

  await page.route('**/api/**', (route) => fulfillApi(route, options));
}

export function blockingViolations(
  violations: ReadonlyArray<{
    readonly id: string;
    readonly impact: string | null;
    readonly help: string;
    readonly nodes: ReadonlyArray<{ readonly target: readonly string[] }>;
  }>,
) {
  return violations
    .filter((violation) =>
      violation.impact === 'serious' || violation.impact === 'critical',
    )
    .map((violation) => ({
      id: violation.id,
      impact: violation.impact,
      help: violation.help,
      targets: violation.nodes.flatMap((node) => node.target),
    }));
}
