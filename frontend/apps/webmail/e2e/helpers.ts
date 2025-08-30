import { expect, type Page } from "@playwright/test";

// Dedicated e2e account — NOT the seeded admin. A fresh user gets a fresh
// engine mailbox store, which keeps runs isolated from (and non-destructive
// to) real local data. CI creates it right after seeding; locally:
//   scripts/e2e-user.ps1   (see e2e/README.md)
export const E2E_USER = process.env.MAILWEB_E2E_USER ?? "e2esmoke";
export const E2E_DOMAIN = process.env.MAILWEB_E2E_DOMAIN ?? "example.com";
export const E2E_PASSWORD = process.env.MAILWEB_E2E_PASSWORD ?? "E2eSmoke2026!";

/** Log in through the UI and wait for the mailbox shell to be interactive. */
export async function login(page: Page): Promise<void> {
  await page.goto("/");
  await page.getByLabel("Account").fill(E2E_USER);
  await page.getByLabel("Password").fill(E2E_PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  // The landing page depends on the user's "after sign-in" preference
  // (/home dashboard or the mailbox), so wait for the post-login sidebar
  // instead of a specific URL.
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible({ timeout: 20_000 });
}

/** Open the compose dialog via the sidebar "Write" button. */
export async function openCompose(page: Page): Promise<void> {
  await page.getByRole("button", { name: "Write", exact: true }).first().click();
  await expect(page.getByRole("heading", { name: "New message" })).toBeVisible();
}
