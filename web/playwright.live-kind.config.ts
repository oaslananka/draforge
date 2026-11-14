import { defineConfig, devices } from '@playwright/test';

// Distinct from the mocked compatibility suite: the installed kind server
// serves the real frontend and live read-only Kubernetes REST/SSE endpoints.
export default defineConfig({
  testDir: './live-e2e',
  fullyParallel: false,
  forbidOnly: true,
  retries: 0, // the test mutates one fixture as the external operator; no retry
  workers: 1,
  timeout: 90_000,
  expect: { timeout: 45_000 },
  reporter: [['line']],
  outputDir: './test-results/live-kind',
  use: {
    ...devices['Desktop Chrome'],
    baseURL: process.env.DRAFORGE_INSTALL_E2E_BROWSER_URL ?? 'http://127.0.0.1:18080',
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'live-kind-chromium' }],
});
