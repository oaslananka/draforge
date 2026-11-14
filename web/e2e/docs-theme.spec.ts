import { test, expect, type Page, type Route } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const docs = resolve(process.cwd(), '../docs');
const base = 'http://127.0.0.1:4173/site-acceptance/';
const readDoc = (file: string) => readFileSync(resolve(docs, file), 'utf8');
const jekyllPaths = (source: string) =>
  source.replace(/\{\{\s*'(\/[^']+)'\s*\|\s*relative_url\s*\}\}/g, '$1');

const head = jekyllPaths(readDoc('_includes/head-custom.html'));
const landing = jekyllPaths(
  readDoc('index.md').replace(/^---\n[\s\S]*?\n---\n/, ''),
);
const innerContent = [
  '<h2>Compatibility reference</h2>',
  '<p>This page contains a standard Markdown-like table and command block.</p>',
  '<table><thead><tr><th>Kubernetes</th><th>DRA API</th></tr></thead>',
  '<tbody><tr><td>v1.35</td><td>resource.k8s.io/v1</td></tr>',
  '<tr><td>v1.36</td><td>resource.k8s.io/v1</td></tr></tbody></table>',
  '<pre><code>kubectl get resourceclaims --all-namespaces --output json --request-timeout=15s</code></pre>',
].join('');

function htmlFor(content: string) {
  return [
    '<!doctype html><html lang="en"><head><meta charset="utf-8">',
    '<meta name="viewport" content="width=device-width,initial-scale=1">',
    '<title>DRAForge documentation theme test</title>',
    '<style>',
    'body{margin:0;background:white;color:#24292e}',
    '.markdown-body{box-sizing:border-box;max-width:1100px;margin:0 auto;padding:24px;color:#24292e}',
    '.markdown-body table{border-collapse:collapse;width:100%}',
    '.markdown-body td,.markdown-body th{padding:8px;border:1px solid #d8e1eb}',
    '.markdown-body pre{background:#f6f8fa;padding:12px;overflow:auto}',
    '</style>',
    head,
    '</head><body><main class="markdown-body">',
    '<h1><a href="/site-acceptance/landing">draforge</a></h1>',
    content,
    '</main></body></html>',
  ].join('');
}

async function setupDocs(page: Page) {
  const files: Record<string, string> = {
    '/assets/css/docs-theme.css': 'assets/css/docs-theme.css',
    '/assets/css/landing.css': 'assets/css/landing.css',
    '/assets/js/docs-theme.js': 'assets/js/docs-theme.js',
    '/assets/dashboard-simulation.webp': 'assets/dashboard-simulation.webp',
  };

  await page.route('**/assets/css/docs-theme.css', serveFile);
  await page.route('**/assets/css/landing.css', serveFile);
  await page.route('**/assets/js/docs-theme.js', serveFile);
  await page.route('**/assets/dashboard-simulation.webp', serveFile);

  async function serveFile(route: Route) {
    const pathname = new URL(route.request().url()).pathname;
    const file = files[pathname];
    if (!file) throw new Error('Unexpected asset: ' + pathname);
    await route.fulfill({
      status: 200,
      contentType: file.endsWith('.js') ? 'text/javascript'
        : file.endsWith('.webp') ? 'image/webp' : 'text/css',
      body: file.endsWith('.webp') ? readFileSync(resolve(docs, file)) : readDoc(file),
    });
  }

  await page.route('**/site-acceptance/**', (route) => {
    const isInner = new URL(route.request().url()).pathname.endsWith('/inner');
    return route.fulfill({
      status: 200,
      contentType: 'text/html',
      body: htmlFor(isInner ? innerContent : landing),
    });
  });
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
