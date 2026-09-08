import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e-mocks",
  timeout: 30_000,
  expect: { timeout: 10_000 },
  workers: 1,
  use: {
    baseURL: "http://127.0.0.1:6006",
    locale: "en-US",
    timezoneId: "UTC",
    colorScheme: "dark",
    reducedMotion: "reduce",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  webServer: {
    command: "npm run storybook -- --ci --no-open --host 127.0.0.1",
    url: "http://127.0.0.1:6006",
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
  reporter: [["list"], ["html", { outputFolder: "test-artifacts/ui-mocks-report", open: "never" }]],
});
