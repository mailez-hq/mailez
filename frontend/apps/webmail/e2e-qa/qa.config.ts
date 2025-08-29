import { defineConfig, devices } from "@playwright/test";

// QA deep-test suite against the dev servers (webmail :3001, admin :3000).
const baseURL = process.env.MAILWEB_QA_BASE_URL ?? "http://localhost:3001";

export default defineConfig({
  testDir: ".",
  testIgnore: "**/admin/**",
  timeout: 120_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  retries: 0,
  workers: 1,
  reporter: [["list"], ["json", { outputFile: "qa-results.json" }]],
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
