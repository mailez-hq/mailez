import { defineConfig, devices } from "@playwright/test";

// The e2e suite runs against a fully deployed mailez stack (docker compose):
// webmail is served at :8083 by the nginx gateway, same-origin with the API.
// Point MAILWEB_E2E_BASE_URL at a different deployment to test it instead.
const baseURL = process.env.MAILWEB_E2E_BASE_URL ?? "http://localhost:8083";

export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL,
    trace: "on-first-retry",
    locale: "en-US",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
