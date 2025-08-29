import { defineConfig, devices } from "@playwright/test";

// Admin QA suite against the admin dev server on :3000.
const baseURL = process.env.ADMIN_QA_BASE_URL ?? "http://localhost:3000";

export default defineConfig({
  testDir: "./admin",
  timeout: 120_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  retries: 0,
  workers: 1,
  reporter: [["list"], ["json", { outputFile: "qa-admin-results.json" }]],
  use: {
    baseURL,
    trace: "retain-on-failure",
    locale: "en-US",
    viewport: { width: 1440, height: 900 },
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
