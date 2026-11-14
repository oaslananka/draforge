import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { expect, test } from '@playwright/test';

const recoveryFixture = fileURLToPath(
  new URL('../../tests/install-e2e/recovery-fix.yaml', import.meta.url),
);

test('real kind dashboard diagnoses and observes claim recovery without reloading', async ({ page }) => {
  // No request routes, mocked EventSource, or synthesized API responses.
  // The browser speaks directly to the installed DRAForge server in kind.
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'DRAForge' })).toBeVisible();
  await expect(page.getByRole('status', { name: 'Live stream status' }))
    .toContainText('Stream Connected');

  await page.getByRole('button', { name: 'CLAIMS', exact: true }).click();
  const claimRow = page.getByRole('row').filter({
    has: page.getByRole('cell', { name: 'e2e-recovery-claim', exact: true }),
  });
  await expect(claimRow).toContainText('Pending');

  await page.getByRole('button', { name: 'EXPLAIN', exact: true }).click();
  const claimSelector = page.getByRole('combobox', { name: /Select ResourceClaim to diagnose/ });
  await claimSelector.selectOption('draforge-e2e/e2e-recovery-claim');
  await expect(page.getByText(/draforge-e2e-recovery-gpu.*does not exist/i))
    .toBeVisible();

  // Wait for the *real* SSE baseline to contain this pending claim before
  // changing anything. Otherwise a first post-repair frame could masquerade
  // as the initial snapshot and the dashboard would not refresh its panels.
  await page.getByRole('button', { name: 'GRAPH', exact: true }).click();
  const pendingGraphNode = page.getByRole('button', {
    name: 'ResourceClaim e2e-recovery-claim in namespace draforge-e2e, status Pending',
  });
  await expect(pendingGraphNode).toHaveCount(1);

  // Operator-side repair only: the browser UI never obtains write access.
  execFileSync('kubectl', ['apply', '-f', recoveryFixture], { timeout: 20_000 });

  await expect(page.getByRole('button', {
    name: 'ResourceClaim e2e-recovery-claim in namespace draforge-e2e, status Allocated',
  })).toHaveCount(1, { timeout: 60_000 });

  await page.getByRole('button', { name: 'CLAIMS', exact: true }).click();
  await expect(claimRow).toContainText('Allocated');
  await expect(claimRow).toContainText('e2e-recovery-pool');

  await page.getByRole('button', { name: 'EXPLAIN', exact: true }).click();
  await expect(claimSelector).toHaveValue('draforge-e2e/e2e-recovery-claim');
  await expect(page.getByText('Claim is allocated.')).toBeVisible();

  // This entire transition occurs in one browser page, without reload.
  expect(new URL(page.url()).origin).toBe(
    new URL(process.env.DRAFORGE_INSTALL_E2E_BROWSER_URL ?? 'http://127.0.0.1:18080').origin,
  );
});
