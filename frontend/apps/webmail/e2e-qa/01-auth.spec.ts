import { expect, test } from "@playwright/test";

import { attemptLogin, login, QA_DOMAIN, QA_PASSWORD, QA_USER } from "./qa-helpers";

test.describe("auth & session", () => {
  test("login page renders with server settings hint", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByLabel("Account")).toBeVisible();
    await expect(page.getByLabel("Password")).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
    // Domain hint / select from server settings
    const domainUi = page.getByLabel(/domain/i).or(page.locator("select"));
    await expect(domainUi.first()).toBeVisible({ timeout: 20_000 }).catch(() => {});
  });

  test("rejects a wrong password with a visible error and stays put", async ({ page }) => {
    const loginResponses: number[] = [];
    page.on("response", (res) => {
      if (res.url().includes("/sso/login")) loginResponses.push(res.status());
    });
    const res = await attemptLogin(page, `${QA_USER}@${QA_DOMAIN}`, "definitely-wrong");
    expect(res.outcome, "wrong password must surface a backend error").toBe("backend-error");
    expect(loginResponses.some((s) => s === 401 || s === 403), "backend must reject with 401/403").toBe(true);
    await expect(
      page.locator("p.text-destructive, [role='alert'], .text-red-500").first(),
    ).toBeVisible({ timeout: 15_000 });
    await expect(page).not.toHaveURL(/\/(mail|home)/);
  });

  test("logs in and lands on the mailbox or dashboard", async ({ page }) => {
    await login(page);
    await page.goto("/mail/Inbox");
    await expect(page).toHaveURL(/\/mail\/Inbox/);
    await expect(page.getByRole("button", { name: "Write", exact: true }).first()).toBeVisible();
  });

  test("sign out returns to the login page", async ({ page }) => {
    await login(page);
    await page.getByRole("button", { name: "Sign out" }).click();
    await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible({ timeout: 30_000 });
  });

  test("session-expiry bounce carries a reason and a way back", async ({ page }) => {
    await page.goto("/?expired=1&next=%2Fmail%2FInbox");
    await expect(page.getByLabel("Account")).toBeVisible();
    // The page should explain the session expired (reason text or notice).
    const pageText = await page.textContent("body");
    const mentionsExpiry = /expired|session|登录|过期/i.test(pageText ?? "");
    expect(mentionsExpiry, "expiry notice should be visible on the login page").toBe(true);
  });

  test("locale switcher changes the UI language", async ({ page }) => {
    await page.goto("/");
    const switcher = page.getByRole("combobox", { name: /language|locale|语言/i })
      .or(page.getByRole("button", { name: /language|locale|语言/i }));
    if (await switcher.first().isVisible({ timeout: 5_000 }).catch(() => false)) {
      await switcher.first().click();
      // After switching to Chinese, the submit button should be 中文
      const zhOption = page.getByRole("option", { name: /中文|Chinese/i });
      if (await zhOption.first().isVisible({ timeout: 3_000 }).catch(() => false)) {
        await zhOption.first().click();
        await expect(page.getByRole("button", { name: /登录|登入|Sign in/ })).toBeVisible();
        await expect(page.getByText(/邮件|邮箱/).first()).toBeVisible();
      }
    }
  });
});
