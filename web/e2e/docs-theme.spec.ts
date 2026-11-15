import { test, expect, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { resolve } from 'node:path';

const base = 'http://127.0.0.1:4173/site-acceptance/';
const fixtures = resolve(process.cwd(), 'e2e/fixtures');
const docs = resolve(process.cwd(), '../docs');

async function setupDocs(page: Page) {
  // Serve committed, static HTML fixtures. No dynamic raw-HTML interpolation:
  // only repository-owned CSS/JS source is loaded into the browser.
  await page.route('**/site-acceptance/landing', (route) =>
    route.fulfill({ path: resolve(fixtures, 'docs-theme-landing.html'), contentType: 'text/html' }),
  );
  await page.route('**/site-acceptance/inner', (route) =>
    route.fulfill({ path: resolve(fixtures, 'docs-theme-inner.html'), contentType: 'text/html' }),
  );
  await page.route('**/assets/css/docs-theme.css', (route) =>
    route.fulfill({ path: resolve(docs, 'assets/css/docs-theme.css'), contentType: 'text/css' }),
  );
  await page.route('**/assets/css/landing.css', (route) =>
    route.fulfill({ path: resolve(docs, 'assets/css/landing.css'), contentType: 'text/css' }),
  );
  await page.route('**/assets/js/docs-theme.js', (route) =>
    route.fulfill({ path: resolve(docs, 'assets/js/docs-theme.js'), contentType: 'text/javascript' }),
  );
}

test('@desktop docs: system dark, manual light and navigation persistence', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'dark' });
  await setupDocs(page);
  await page.goto(base + 'landing');
  await expect(page.locator('html')).toHaveAttribute('data-docs-theme', 'dark');
  const toggle = page.getByRole('button', { name: 'Switch to light mode' });
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');
  const darkBackground = await page.locator('body').evaluate((node) => getComputedStyle(node).backgroundColor);
  expect(darkBackground).toBe('rgb(13, 21, 34)');

  await toggle.click();
  await expect(page.locator('html')).toHaveAttribute('data-docs-theme', 'light');
  await expect(page.getByRole('button', { name: 'Switch to dark mode' }))
    .toHaveAttribute('aria-pressed', 'false');

  await page.goto(base + 'inner');
  await expect(page.locator('html')).toHaveAttribute('data-docs-theme', 'light');
  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('data-docs-theme', 'light');
  expect(await page.evaluate(() => localStorage.getItem('draforge-docs-theme'))).toBe('light');
});

test('@desktop docs: follow OS until keyboard override and style code/tables', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' });
  await setupDocs(page);
  await page.goto(base + 'inner');
  await expect(page.locator('html')).toHaveAttribute('data-docs-theme', 'light');
  await page.emulateMedia({ colorScheme: 'dark' });
  await expect(page.locator('html')).toHaveAttribute('data-docs-theme', 'dark');
  const tableColor = await page.locator('table tbody tr').first()
    .evaluate((el) => getComputedStyle(el).backgroundColor);
  const codeColor = await page.locator('pre').first()
    .evaluate((el) => getComputedStyle(el).backgroundColor);
  expect(tableColor).not.toBe('rgb(255, 255, 255)');
  expect(codeColor).toBe('rgb(19, 36, 57)');

  const toggle = page.getByRole('button', { name: 'Switch to light mode' });
  await toggle.focus();
  await expect(toggle).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page.locator('html')).toHaveAttribute('data-docs-theme', 'light');
  await expect(page.getByRole('button', { name: 'Switch to dark mode' }))
    .toHaveAttribute('aria-pressed', 'false');
});

test('@mobile docs: responsive task cards and code without document overflow', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.emulateMedia({ colorScheme: 'dark' });
  await setupDocs(page);
  await page.goto(base + 'landing');
  await expect(page.locator('html')).toHaveAttribute('data-docs-theme', 'dark');
  const sizes = await page.evaluate(() => {
    const button = document.getElementById('docs-theme-toggle');
    const grid = document.querySelector('.df-path-grid');
    const path = document.querySelector('.df-path');
    const code = path?.querySelector('pre');
    return {
      viewport: innerWidth,
      page: document.documentElement.scrollWidth,
      buttonHeight: button?.getBoundingClientRect().height ?? 0,
      columns: grid ? getComputedStyle(grid).gridTemplateColumns.split(' ').length : 0,
      codeScrollable: Boolean(code && code.scrollWidth > code.clientWidth),
    };
  });
  expect(sizes.page).toBeLessThanOrEqual(sizes.viewport);
  expect(sizes.buttonHeight).toBeGreaterThanOrEqual(44);
  expect(sizes.columns).toBe(1);
  expect(sizes.codeScrollable).toBe(true);
  await page.getByRole('button', { name: 'Switch to light mode' }).click();
  await expect(page.locator('html')).toHaveAttribute('data-docs-theme', 'light');
});


test('@axe docs theme: no serious or critical violations in either theme', async ({ page }) => {
  await setupDocs(page);
  for (const scheme of ['dark', 'light'] as const) {
    await page.emulateMedia({ colorScheme: scheme });
    for (const route of ['landing', 'inner']) {
      await page.goto(base + route);
      await expect(page.locator('html')).toHaveAttribute('data-docs-theme', scheme);
      const results = await new AxeBuilder({ page }).analyze();
      const blocking = results.violations
        .filter((issue) => issue.impact === 'serious' || issue.impact === 'critical')
        .map((issue) => ({
          rule: issue.id,
          targets: issue.nodes.flatMap((node) => node.target),
        }));
      expect(blocking).toEqual([]);
    }
  }
});
