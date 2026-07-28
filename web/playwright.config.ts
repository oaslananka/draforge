import { defineConfig, devices } from '@playwright/test';

const isCI = Boolean(process.env.CI);

export default defineConfig({
  testDir: './e2e',
  outputDir: './test-results',
  fullyParallel: false,
  forbidOnly: isCI,
  retries: isCI ? 1 : 0,
  workers: 1,
  reporter: isCI
    ? [['line'], ['html', { outputFolder: 'playwright-report', open: 'never' }]]
    : 'list',
  use: {
    baseURL: 'http://127.0.0.1:4173',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  webServer: {
    command: 'node ./node_modules/vite/bin/vite.js preview --host 127.0.0.1 --port 4173',
    url: 'http://127.0.0.1:4173',
    reuseExistingServer: !isCI,
    timeout: 30_000,
  },
  projects: [
    {
      name: 'chromium',
      grep: /@desktop|@axe|@motion/,
      use: { ...devices['Desktop Chrome'] },
    },
    {
      name: 'firefox',
      grep: /@desktop/,
      use: { ...devices['Desktop Firefox'] },
    },
    {
      name: 'webkit',
      grep: /@desktop/,
      use: { ...devices['Desktop Safari'] },
    },
    {
      name: 'chromium-mobile',
      grep: /@mobile/,
      use: { ...devices['Pixel 7'] },
    },
  ],
});
