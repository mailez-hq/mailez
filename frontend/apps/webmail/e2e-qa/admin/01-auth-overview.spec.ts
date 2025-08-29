import { expect, test } from "@playwright/test";

import { adminLogin, gotoAdmin, marker } from "./admin-helpers";

test.describe("admin auth & shell", () => {
  test("login page renders", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByText("Sign in to Mailez")).toBeVisible({ timeout: 20_000 });
    await expect(page.getByLabel(/Email/i)).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
  });

  test("rejects wrong password", async ({ page }) => {
    await page.goto("/");
    await page.getByLabel(/Email/i).fill("admin@example.com");
    await page.getByLabel(/Password/i).fill("wrong-password");
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByText("Sign-in failed").or(page.locator(".text-destructive, [role='alert']").first())).toBeVisible({ timeout: 20_000 });
    await expect(page).not.toHaveURL(/overview/);
  });

  test("login lands on overview; logout returns to sign-in", async ({ page }) => {
    await adminLogin(page);
    await expect(page.getByText(/Overview|System overview/i).first()).toBeVisible({ timeout: 20_000 });
    await page.getByRole("button", { name: "Logout" }).or(page.getByText("Logout")).first().click();
    await expect(page.getByText("Sign in to Mailez")).toBeVisible({ timeout: 20_000 });
  });

  test("all 14 sections are reachable from the sidebar", async ({ page }) => {
    await adminLogin(page);
    const sections: Array<[string, RegExp]> = [
      ["Overview", /System overview|Overview/i],
      ["Users", /Users/i],
      ["Domains", /Domains/i],
      ["Groups", /Distribution Groups/i],
      ["Aliases", /Aliases/i],
      ["Relays", /Relays/i],
      ["Fetches", /Fetches/i],
      ["Tokens", /Tokens/i],
      ["Anon aliases", /Anonymous aliases/i],
      ["Archive", /Email Archive/i],
      ["Audit", /Audit log/i],
      ["Approval / DLP", /Approval & Content Filter|Approval/i],
      ["Config", /Configuration/i],
      ["Announcement", /Announcement/i],
    ];
    for (const [nav, header] of sections) {
      await gotoAdmin(page, nav, header);
    }
  });

  test("unauthenticated access bounces to sign-in", async ({ page }) => {
    await page.goto("/users");
    await expect(page.getByText("Sign in to Mailez").or(page.getByLabel(/Email/i)).first()).toBeVisible({ timeout: 20_000 });
  });
});

test.describe("admin overview", () => {
  test("overview shows license, infrastructure and usage cards", async ({ page }) => {
    await adminLogin(page);
    await gotoAdmin(page, "Overview", /System overview|Overview/i);
    const body = await page.textContent("body");
    expect(body).toMatch(/Users|Domains/i);
    expect(body).toMatch(/Mail engine|Infrastructure|License/i);
  });
});
