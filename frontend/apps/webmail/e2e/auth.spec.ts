import { expect, test } from "@playwright/test";

import { E2E_DOMAIN, E2E_PASSWORD, E2E_USER, login } from "./helpers";

test.describe("authentication", () => {
  test("shows the login page with server settings", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByLabel("Account")).toBeVisible();
    await expect(page.getByLabel("Password")).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
  });

  test("rejects a wrong password", async ({ page }) => {
    await page.goto("/");
    await page.getByLabel("Account").fill(E2E_USER);
    await page.getByLabel("Password").fill("definitely-wrong");
    await page.getByRole("button", { name: "Sign in" }).click();
    // The backend error surfaces in the destructive error paragraph and the
    // user stays on the login page.
    await expect(page.locator("p.text-destructive")).toBeVisible({ timeout: 15_000 });
    await expect(page).not.toHaveURL(/\/(mail|home)/);
  });

  test("logs in, reaches the mailbox and signs out", async ({ page }) => {
    await login(page);

    // The mailbox view loads: folders and the compose entry point.
    await page.goto("/mail/Inbox");
    await expect(page).toHaveURL(/\/mail\/Inbox/);
    await expect(page.getByRole("button", { name: "Write", exact: true }).first()).toBeVisible();

    // Sign out returns to the login page.
    await page.getByRole("button", { name: "Sign out" }).click();
    await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
    void E2E_DOMAIN;
    void E2E_PASSWORD;
  });
});
