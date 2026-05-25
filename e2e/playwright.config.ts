import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  timeout: 30_000,
  retries: process.env.CI ? 2 : 0,
  workers: 1, // serial — tests share server state
  reporter: [['list'], ['html', { open: 'never' }]],

  use: {
    baseURL: process.env.YAAMON_URL ?? 'http://localhost:8080',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },

  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
    {
      name: 'mobile',
      use: { ...devices['iPhone 12'] },
      testMatch: ['**/dashboard.spec.ts', '**/auth.spec.ts'],
    },
  ],
});
