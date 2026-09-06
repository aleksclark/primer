import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  testMatch: 's17-collaboration.spec.mjs',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 180_000,
  outputDir: '../../.paseo-e2e/s17-collaboration/playwright',
  use: { trace: 'off' }, // Never record fixture cookies in traces.
  reporter: [['list']],
});
