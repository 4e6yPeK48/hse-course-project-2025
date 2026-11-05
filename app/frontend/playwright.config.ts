import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests/browser',
  timeout: 180000,
  expect: { timeout: 10000 },
  use: { baseURL: process.env.BASE_URL || 'http://localhost:8080', trace: 'retain-on-failure' },
  workers: 1,
  reporter: 'list'
});
