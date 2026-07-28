import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';
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
  await page.keyboard.press('Tab');

  const claimNode = page.getByRole('button', {
    name: 'ResourceClaim shared in namespace team-b, status Allocated',
  });
  await expect(claimNode).toBeFocused();
  const focusedNodeStroke = await claimNode.locator('circle').evaluate((element) =>
    getComputedStyle(element).strokeWidth,
  );
  expect(Number.parseFloat(focusedNodeStroke)).toBeGreaterThanOrEqual(4);
  await page.keyboard.press('Enter');
  await expect(claimNode).toHaveAttribute('aria-pressed', 'true');

  await page.keyboard.press('Tab');
  const diagnose = page.getByRole('button', { name: 'Diagnose Allocation' });
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
    page.getByText('Reduced motion is active; graph nodes use fixed positions.'),
  ).toBeVisible();
  await expect(page.getByRole('region', { name: 'Resource relationship graph' })).toHaveAttribute(
    'data-motion',
    'reduced',
  );
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
