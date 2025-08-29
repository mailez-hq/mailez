import { expect, test } from "@playwright/test";

import { acceptDialogs, adminApi, adminLogin, gotoAdmin, marker, rowDelete, rowEdit } from "./admin-helpers";

test.describe("admin mail objects", () => {
  test.beforeEach(({ page }) => acceptDialogs(page));

  test("aliases: create forward alias and delete it", async ({ page }) => {
    const addr = `${marker("al")}@example.com`;
    await adminLogin(page);
    await gotoAdmin(page, "Aliases", /Aliases/i);
    await page.getByRole("button", { name: "New alias" }).first().click();
    const dialog = page.getByRole("dialog").first();
    await expect(dialog).toBeVisible({ timeout: 15_000 });
    await dialog.getByPlaceholder("info@example.com").fill(addr);
    await dialog.getByPlaceholder("user@example.com").fill("e2esmoke@example.com");
    await dialog.getByRole("button", { name: /Save|Create/i }).last().click();
    await expect(dialog).toBeHidden({ timeout: 20_000 });
    await expect(page.getByText(addr).first()).toBeVisible({ timeout: 20_000 });
    await rowDelete(page, page.locator("tr", { hasText: addr }).first());
    await expect(page.getByText(addr).first()).toBeHidden({ timeout: 20_000 });
  });

  test("groups: create distribution group with a member", async ({ page }) => {
    const addr = `${marker("g")}@example.com`;
    await adminLogin(page);
    await gotoAdmin(page, "Groups", /Distribution Groups/i);
    await page.getByRole("button", { name: "New group" }).first().click();
    const dialog = page.getByRole("dialog").first();
    await expect(dialog).toBeVisible({ timeout: 15_000 });
    await dialog.getByPlaceholder("team@example.com").fill(addr);
    await dialog.getByPlaceholder(/user@example\.com or external/i).fill("e2esmoke@example.com");
    await dialog.getByRole("button", { name: /Save|Create/i }).last().click();
    await expect(dialog).toBeHidden({ timeout: 20_000 });
    await expect(page.getByText(addr).first()).toBeVisible({ timeout: 20_000 });
    await rowDelete(page, page.locator("tr", { hasText: addr }).first());
    await expect(page.getByText(addr).first()).toBeHidden({ timeout: 20_000 });
  });

  test("relays: create external relay domain and delete it", async ({ page }) => {
    const domain = `${marker("r")}.test`;
    await adminLogin(page);
    await gotoAdmin(page, "Relays", /Relays/i);
    await page.getByRole("button", { name: "New relay" }).first().click();
    const dialog = page.getByRole("dialog").first();
    await expect(dialog).toBeVisible({ timeout: 15_000 });
    await dialog.getByPlaceholder("example.com", { exact: true }).fill(domain);
    await dialog.getByPlaceholder("smtp.example.com").fill("mx.partner.test");
    await dialog.getByRole("button", { name: /Save|Create/i }).last().click();
    await expect(dialog).toBeHidden({ timeout: 20_000 });
    await expect(page.getByText(domain).first()).toBeVisible({ timeout: 20_000 });
    await rowDelete(page, page.locator("tr", { hasText: domain }).first());
    await expect(page.getByText(domain).first()).toBeHidden({ timeout: 20_000 });
  });

  test("tokens: create app token; secret shown once; delete", async ({ page }) => {
    await adminLogin(page);
    await gotoAdmin(page, "Tokens", /Tokens/i);
    await page.getByRole("button", { name: "New token" }).first().click();
    const dialog = page.getByRole("dialog").first();
    await expect(dialog).toBeVisible({ timeout: 15_000 });
    const select = dialog.locator("select").first();
    await select.selectOption({ index: 1 }); // 0 = "Select user..." placeholder
    await dialog.getByRole("button", { name: /Save|Create/i }).last().click();
    // One-time secret display then Done
    const done = page.getByRole("button", { name: "Done" });
    await expect(done).toBeVisible({ timeout: 20_000 });
    await done.click();
    // The newest token row for the user exists (newest-first ordering)
    const row = page.locator("tr", { hasText: "@example.com" }).first();
    await expect(row).toBeVisible({ timeout: 15_000 });
    await rowDelete(page, row);
  });

  test("fetches: create external fetch account and delete it", async ({ page }) => {
    const host = `pop-${marker("f").slice(3, 12)}.test`;
    await adminLogin(page);
    await gotoAdmin(page, "Fetches", /Fetches/i);
    await page.getByRole("button", { name: "New fetch" }).first().click();
    const dialog = page.getByRole("dialog").first();
    await expect(dialog).toBeVisible({ timeout: 15_000 });
    await dialog.locator("select").first().selectOption({ index: 1 });
    // Field order: host, port(number), username, password, folders
    const texts = dialog.locator("input:not([type=number]):not([type=password]):not([type=checkbox])");
    await texts.nth(0).fill(host);
    await dialog.locator("input[type=number]").first().fill("995");
    await texts.nth(1).fill("qa-external");
    await dialog.locator("input[type=password]").first().fill("ExternalPw!1");
    await dialog.getByRole("button", { name: /Save|Create/i }).last().click();
    await expect(dialog).toBeHidden({ timeout: 20_000 });
    const row = page.locator("tr", { hasText: host }).first();
    await expect(row).toBeVisible({ timeout: 15_000 });
    await rowDelete(page, row);
  });

  test("anon aliases: enable feature, generate, then clean up", async ({ page }) => {
    // The trigger stays disabled while no domain has anon-mail enabled -
    // correct UX. Turn it on for example.com for this test, restore after.
    const put = await adminApi("/domains/example.com", {
      method: "PUT",
      body: JSON.stringify({
        max_users: -1, max_aliases: -1, max_quota_bytes: 0,
        signup_enabled: false, anonmail_enabled: true, comment: "",
      }),
    });
    if (put.status >= 300) throw new Error(`enable anonmail failed: ${put.status}`);
    try {
      await adminLogin(page);
      await gotoAdmin(page, "Anon aliases", /Anonymous aliases/i);
      const newBtn = page.getByRole("button", { name: "New alias" }).first();
      // The first domains fetch can race with session settling; reload once.
      await newBtn
        .waitFor({ state: "visible", timeout: 8_000 })
        .catch(() => page.reload().then(() => page.waitForLoadState("domcontentloaded")));
      await expect(newBtn).toBeEnabled({ timeout: 20_000 });
      // The dialog remounts while state settles — retry the whole open+fill+submit
      await expect(async () => {
        await newBtn.click();
        const dialog = page.getByRole("dialog").first();
        await expect(dialog).toBeVisible({ timeout: 15_000 });
        // domain select defaults to its first option; just fill the name
        await dialog.getByPlaceholder("my-shop").fill("qa-disposable");
        await dialog.getByRole("button", { name: "Create", exact: true }).click();
        await expect(dialog).toBeHidden({ timeout: 15_000 });
      }).toPass({ timeout: 90_000 });
      // The generated alias list gains a row with the chosen display name
      const row = page.locator("tr", { hasText: "qa-disposable" }).first();
      await expect(row).toBeVisible({ timeout: 20_000 });
      await rowDelete(page, row);
    } finally {
      await adminApi("/domains/example.com", {
        method: "PUT",
        body: JSON.stringify({
          max_users: -1, max_aliases: -1, max_quota_bytes: 0,
          signup_enabled: false, anonmail_enabled: false, comment: "",
        }),
      });
    }
  });
});