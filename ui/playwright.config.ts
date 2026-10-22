import { defineConfig, devices } from '@playwright/test';

// Exercise the production bundle, including the shipped DS CSS/font assets.
// API calls are mocked in the tests; no backend or real credentials are needed.
export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  use: {
    baseURL: 'http://127.0.0.1:4173',
    ...devices['Desktop Chrome'],
    trace: 'retain-on-failure',
    launchOptions: {
      executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || undefined,
    },
  },
  webServer: {
    command: 'npm run preview -- --host 127.0.0.1 --port 4173 --strictPort',
    url: 'http://127.0.0.1:4173/ui/',
    reuseExistingServer: false,
  },
});
