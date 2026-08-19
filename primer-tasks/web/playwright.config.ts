import { defineConfig, devices } from "@playwright/test";

const baseURL = process.env.PRIMER_TASKS_BASE_URL;
if (!baseURL) throw new Error("PRIMER_TASKS_BASE_URL must point at the real Stacklane web origin");

export default defineConfig({
  testDir: "./e2e",
  timeout: 30_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL,
    ...devices["Desktop Chrome"],
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  reporter: [["list"], ["json", { outputFile: "test-artifacts/playwright-phase1.json" }]],
});
