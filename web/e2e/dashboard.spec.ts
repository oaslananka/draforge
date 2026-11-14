import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';
import type { Locator, Page } from '@playwright/test';
import {
  blockingViolations,
  installDashboardMocks,
} from './dashboard-fixture';

const wcagTags = [
  'wcag2a',
  'wcag2aa',
  'wcag21a',
  'wcag21aa',
  'wcag22a',
  'wcag22aa',
];

// SVG link focus order can differ across browser engines. Verify the target
// is genuinely keyboard-reachable rather than assuming it is the next Tab stop.
async function tabUntilFocused(page: Page, target: Locator, maxTabs = 24): Promise<void> {
  for (let i = 0; i < maxTabs; i += 1) {
    await page.keyboard.press('Tab');
    if (await target.evaluate((element) => element === document.activeElement)) {
      return;
    }
  }
  expect(false, 'Expected interactive graph control to be reachable by keyboard Tab navigation').toBe(true);
}

test.beforeEach(async ({ page }) => {
  await installDashboardMocks(page);
});

test('@desktop critical dashboard flow is keyboard operable with visible focus and named graph controls', async ({ page }) => {
  await page.goto('/');

  await expect(page.getByRole('heading', { name: 'DRAForge' })).toBeVisible();
  await expect(
    page.getByRole('status', { name: 'Live stream status' }),
  ).toContainText('Stream Connected');

  const overview = page.getByRole('button', { name: 'OVERVIEW' });
  await overview.focus();
  for (const name of ['POOLS', 'DEVICES', 'CLAIMS', 'GRAPH']) {
    await page.keyboard.press('Tab');
    await expect(page.getByRole('button', { name })).toBeFocused();
  }

  const graphTab = page.getByRole('button', { name: 'GRAPH' });
  const graphFocus = await graphTab.evaluate((element) => {
    const style = getComputedStyle(element);
    return { style: style.outlineStyle, width: style.outlineWidth };
  });
  expect(graphFocus.style).not.toBe('none');
  expect(Number.parseFloat(graphFocus.width)).toBeGreaterThanOrEqual(2);

  await page.keyboard.press('Enter');
  await expect(
    page.getByRole('heading', { name: 'Resource Relationship Graph' }),
  ).toBeVisible();

  const resetView = page.getByRole('button', { name: 'Reset View' });
  await resetView.focus();

  const claimNode = page.getByRole('button', {
    name: 'ResourceClaim shared in namespace team-b, status Allocated',
  });
  await tabUntilFocused(page, claimNode);
  await expect(claimNode).toBeFocused();
  const focusedNodeStroke = await claimNode.locator('circle').evaluate((element) =>
    getComputedStyle(element).strokeWidth,
  );
  expect(Number.parseFloat(focusedNodeStroke)).toBeGreaterThanOrEqual(4);
  await page.keyboard.press('Enter');
  await expect(claimNode).toHaveAttribute('aria-pressed', 'true');

  const diagnose = page.getByRole('button', { name: 'Diagnose Allocation' });
  await tabUntilFocused(page, diagnose);
  await expect(diagnose).toBeFocused();
  await page.keyboard.press('Enter');

  await expect(
    page.getByRole('heading', { name: 'Allocation Explanation Engine' }),
  ).toBeVisible();
  await expect(page.getByText('Claim is allocated.')).toBeVisible();

  const doctor = page.getByRole('button', { name: /Doctor: 1 Failure/i });
  await doctor.focus();
  await page.keyboard.press('Enter');
  await expect(
    page.getByRole('heading', { name: 'Cluster Diagnostics (Doctor)' }),
  ).toBeVisible();
  await expect(page.getByText('ResourceClaims API is unavailable.')).toBeVisible();
});

test('@desktop claim recovery updates the visible dashboard and explanation without a reload', async ({ page }) => {
  let status = 'Pending';
  let claimReads = 0;
  await page.route('**/api/claims', async (route) => {
    claimReads += 1;
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify([{
        name: 'shared',
        namespace: 'team-b',
        status,
        requests: [],
        allocations: status === 'Allocated' ? [{
          request: 'gpu',
          driverName: 'driver.example',
          poolName: 'recovery-pool',
          deviceName: 'gpu-0',
          nodeName: 'worker-a',
        }] : [],
      }]),
    });
  });
  await page.route('**/api/explain?**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        targetName: 'shared',
        targetType: 'claim',
        allocated: status === 'Allocated',
        reasonTree: {
          message: status === 'Allocated' ? 'Claim is allocated.' : 'Claim could not be allocated.',
          confidence: 'confirmed',
          evidence: 'Observed from the current claim state.',
          sourceType: 'ResourceClaim',
        },
        remedy: [],
      }),
    });
  });

  await page.goto('/');
  await expect(page.getByRole('status', { name: 'Live stream status' })).toContainText('Stream Connected');
  await page.getByRole('button', { name: 'CLAIMS' }).click();
  const claimRow = page.getByRole('row', { name: /shared team-b/ });
  await expect(claimRow).toContainText('Pending');

  // The mock stream starts with a baseline frame. A real claim allocation
  // should produce a changed SSE frame and refresh the read-only views.
  status = 'Allocated';
  const graph = {
    nodes: [{ id: 'claim/team-b/shared', type: 'ResourceClaim', label: 'shared', metadata: { namespace: 'team-b', status: 'Allocated', allocationCount: 1 } }],
    edges: [],
  };
  await page.evaluate((next) => {
    window.dispatchEvent(new CustomEvent('draforge-test-graph', { detail: next }));
  }, graph);
  await expect(claimRow).toContainText('recovery-pool');
  await expect(claimRow).toContainText('Allocated');
  await page.getByRole('button', { name: 'EXPLAIN' }).click();
  await expect(page.getByText('Claim is allocated.')).toBeVisible();

  const afterRecoveryReads = claimReads;
  await page.evaluate((next) => {
    window.dispatchEvent(new CustomEvent('draforge-test-graph', { detail: next }));
  }, graph);
  expect(claimReads).toBe(afterRecoveryReads);
});

test('@axe selected critical views have no serious or critical WCAG 2.2 violations', async ({
  page,
}) => {

  await page.goto('/');
  await expect(
    page.getByRole('status', { name: 'Live stream status' }),
  ).toContainText('Stream Connected');

  for (const view of ['OVERVIEW', 'GRAPH', 'DOCTOR']) {
    await page.getByRole('button', { name: view, exact: true }).click();
    const results = await new AxeBuilder({ page }).withTags(wcagTags).analyze();
    expect(blockingViolations(results.violations), `${view} accessibility violations`).toEqual([]);
  }
});

test('@axe global error state has no serious or critical accessibility violations', async ({
  page,
}) => {

  await page.unroute('**/api/**');
  await installDashboardMocks(page, { summaryError: true });
  await page.goto('/');

  await expect(page.getByRole('alert')).toContainText('summary unavailable');
  const results = await new AxeBuilder({ page }).withTags(wcagTags).analyze();
  expect(blockingViolations(results.violations)).toEqual([]);
});

test('@motion keeps graph positions static and exposes the reduced-motion state', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/');
  await expect.poll(() => page.evaluate(() =>
    window.matchMedia('(prefers-reduced-motion: reduce)').matches,
  )).toBe(true);
  await page.getByRole('button', { name: 'GRAPH', exact: true }).click();

  await expect(
    page.getByText('Reduced motion is active; graph nodes use fixed, separated positions. Scroll inside the graph to reach more nodes.'),
  ).toBeVisible();
  await expect(page.getByRole('region', { name: 'Resource relationship graph' })).toHaveAttribute(
    'data-motion',
    'reduced',
  );
});

test('@motion dense static graph keeps every node separately pointer-selectable', async ({ page }) => {
  await page.setViewportSize({ width: 1600, height: 900 });
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/');
  await expect(page.getByRole('status', { name: 'Live stream status' }))
    .toContainText('Stream Connected');
  await page.getByRole('button', { name: 'GRAPH' }).click();

  const claim = {
    id: 'claim/draforge-e2e/e2e-recovery-claim',
    type: 'ResourceClaim',
    label: 'e2e-recovery-claim',
    metadata: { namespace: 'draforge-e2e', status: 'Allocated' },
  };
  const graph = {
    nodes: [
      claim,
      ...Array.from({ length: 18 }, (_, index) => ({
        id: 'resource/synthetic-' + index,
        type: ['Allocation', 'Device', 'DeviceClass', 'ResourcePool', 'Node', 'Pod', 'Driver', 'Namespace'][index % 8],
        label: 'resource-' + index,
        metadata: { status: 'healthy' },
      })),
    ],
    edges: [],
  };
  await page.evaluate((payload) => {
    window.dispatchEvent(new CustomEvent('draforge-test-graph', { detail: payload }));
  }, graph);

  const nodes = page.locator('.graph-node');
  await expect(nodes).toHaveCount(19);
  // A one-node mock passes even when other graph nodes obscure the target.
  // Check the actual browser pointer hit for every node in a dense graph.
  for (let index = 0; index < 19; index += 1) {
    const node = nodes.nth(index);
    await node.scrollIntoViewIfNeeded();
    const targetIsTopmost = await node.evaluate((element) => {
      const circle = element.querySelector('circle');
      if (!circle) return false;
      const bounds = circle.getBoundingClientRect();
      const hit = document.elementFromPoint(
        bounds.x + bounds.width / 2,
        bounds.y + bounds.height / 2,
      );
      return hit?.closest('.graph-node') === element;
    });
    expect(targetIsTopmost, 'Graph node index ' + index + ' must be independently clickable')
      .toBe(true);
  }

  const recoveryClaim = page.getByRole('button', {
    name: 'ResourceClaim e2e-recovery-claim in namespace draforge-e2e, status Allocated',
  });
  await recoveryClaim.locator('circle').click();
  await expect(recoveryClaim).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('button', { name: 'Diagnose Allocation' })).toBeVisible();

  // A reordered SSE graph must preserve the same fixed layout, without jitter.
  const before = await recoveryClaim.locator('g[transform]').first().getAttribute('transform');
  await page.evaluate((payload) => {
    window.dispatchEvent(new CustomEvent('draforge-test-graph', { detail: payload }));
  }, { ...graph, nodes: [...graph.nodes].reverse() });
  await expect(recoveryClaim.locator('g[transform]').first()).toHaveAttribute('transform', before ?? '');
});

test('@mobile reduced-motion dense graph scrolls internally without shrinking nodes', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/');
  await expect(page.getByRole('status', { name: 'Live stream status' }))
    .toContainText('Stream Connected');
  await page.getByRole('button', { name: 'GRAPH' }).click();

  const graph = {
    nodes: Array.from({ length: 75 }, (_, index) => ({
      id: 'device/synthetic-' + index,
      type: 'Device',
      label: 'dev-' + index,
      metadata: { status: 'healthy' },
    })),
    edges: [],
  };
  await page.evaluate((payload) => {
    window.dispatchEvent(new CustomEvent('draforge-test-graph', { detail: payload }));
  }, graph);
  await expect(page.locator('.graph-node')).toHaveCount(75);

  const sizes = await page.getByRole('region', { name: 'Resource relationship graph' })
    .evaluate((region) => {
      const svg = region.querySelector('svg');
      if (!svg) return null;
      return {
        viewport: document.documentElement.clientWidth,
        documentWidth: document.documentElement.scrollWidth,
        internalWidth: region.scrollWidth,
        internalHeight: region.scrollHeight,
        visibleWidth: region.clientWidth,
        visibleHeight: region.clientHeight,
        actualSvgWidth: svg.getBoundingClientRect().width,
      };
    });
  if (sizes === null) {
    throw new Error('Graph SVG is missing from the reduced-motion panel');
  }
  expect(sizes.documentWidth).toBeLessThanOrEqual(sizes.viewport);
  expect(sizes.internalWidth).toBeGreaterThan(sizes.visibleWidth);
  expect(sizes.internalHeight).toBeGreaterThan(sizes.visibleHeight);
  expect(sizes.actualSvgWidth).toBeGreaterThanOrEqual(800);
});

test('@mobile representative mobile viewport keeps navigation and main content within the page', async ({
  page,
}) => {

  await page.goto('/');
  await expect(page.getByRole('navigation', { name: 'Dashboard sections' })).toBeVisible();

  const dimensions = await page.evaluate(() => ({
    viewport: window.innerWidth,
    document: document.documentElement.scrollWidth,
    body: document.body.scrollWidth,
  }));
  expect(dimensions.document).toBeLessThanOrEqual(dimensions.viewport);
  expect(dimensions.body).toBeLessThanOrEqual(dimensions.viewport);

  await page.getByRole('button', { name: 'GRAPH' }).click();
  await expect(
    page.getByRole('heading', { name: 'Resource Relationship Graph' }),
  ).toBeVisible();
});
