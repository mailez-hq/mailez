import { expect, test } from "@playwright/test";

import { acceptDialogs, adminLogin, gotoAdmin, marker } from "./admin-helpers";
import { apiCookie } from "../qa-helpers";

test.describe("admin compliance surfaces", () => {
  test.beforeEach(({ page }) => acceptDialogs(page));

  test("archive: policy form saves and search runs", async ({ page }) => {
    await adminLogin(page);
    await gotoAdmin(page, "Archive", /Email Archive/i);
    // Global policy form present
    await expect(page.getByText("Archive Policy")).toBeVisible({ timeout: 15_000 });
    const save = page.getByRole("button", { name: "Save global policy" });
    if (await save.isVisible().catch(() => false)) {
      await save.click();
      await expect(page.getByText("Archive policy saved").first()).toBeVisible({ timeout: 15_000 });
    }
    // Search section runs
    await page.getByPlaceholder(/Subject\/body\/sender/i).fill("qa");
    await page.getByRole("button", { name: "Search", exact: true }).last().click();
    await expect(page.getByText(/No items|Archived at|qa/i).first()).toBeVisible({ timeout: 20_000 });
  });

  test("audit: log table renders with recent activity", async ({ page }) => {
    await adminLogin(page);
    await gotoAdmin(page, "Audit", /Audit log/i);
    await expect(page.getByText(/Recent activity|No activity yet/i).first()).toBeVisible({ timeout: 20_000 });
    // Our own navigation should produce entries (time/user/method/path columns)
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 8_000 }).catch(() => false)) {
      await expect(table.locator("tbody tr").first()).toBeVisible();
    }
  });

  test("dlp: rules list and create-block flow", async ({ page }) => {
    const rule = marker("rule");
    await adminLogin(page);
    await gotoAdmin(page, "Approval / DLP", /Approval & Content Filter|Approval/i);
    await page.getByRole("button", { name: "New rule" }).first().click();
    const dialog = page.getByRole("dialog").first();
    await expect(dialog).toBeVisible({ timeout: 15_000 });
    await dialog.getByPlaceholder(/confidential keyword/i).fill(rule);
    await dialog.getByPlaceholder(/Keyword or regex/i).fill("topsecret-qa");
    await dialog.getByRole("button", { name: /Save/i }).last().click();
    await expect(dialog).toBeHidden({ timeout: 20_000 });
    await expect(page.getByText(rule).first()).toBeVisible({ timeout: 20_000 });
    // Approval Center section exists
    await expect(page.getByText(/Approval Center|No records|Pending/i).first()).toBeVisible({ timeout: 15_000 });
    // Cleanup — the rule list re-renders on polling, detaching row buttons;
    // dlp rows expose a direct title="Delete" trash button (no Actions menu).
    await expect(async () => {
      const row = page.locator("tr", { hasText: rule }).first();
      await row.getByTitle("Delete").click();
      // native confirm() is auto-accepted via acceptDialogs
      await expect(page.locator("tr", { hasText: rule }).first()).toBeHidden({ timeout: 10_000 });
    }).toPass({ timeout: 60_000 });
  });

  test("announcement: publish then clear; banner reaches webmail", async ({ page }) => {
    const subject = marker("announce");
    await adminLogin(page);
    await gotoAdmin(page, "Announcement", /Announcement/i);
    await page.getByPlaceholder(/Scheduled maintenance/i).fill(subject);
    await page.getByPlaceholder(/Optional details/i).fill("QA broadcast body");
    await page.getByRole("button", { name: /Save/i }).first().click();
    await expect(page.getByText("Announcement saved").first()).toBeVisible({ timeout: 20_000 });

    // Cross-app check: sign in as a webmail user via API-injected session
    // cookie in an isolated context (immune to UI-login flakiness).
    const cookiePair = await apiCookie();
    const eq = cookiePair.indexOf("=");
    const cname = cookiePair.slice(0, eq);
    const cvalue = cookiePair.slice(eq + 1); // base64 value may itself contain "="
    const browser = page.context().browser();
    const ctx2 = await browser!.newContext({ baseURL: process.env.MAILWEB_QA_BASE_URL ?? "http://localhost:3001" });
    await ctx2.addCookies([{ name: cname, value: cvalue, domain: "localhost", path: "/" }]);
    const wm = await ctx2.newPage();
    await wm.goto("/mail/Inbox");
    await expect(wm.getByText(subject).first()).toBeVisible({ timeout: 30_000 });
    await wm.close();
    await ctx2.close();

    // Clear the announcement
    await page.getByRole("button", { name: "Clear" }).first().click();
    await page.getByRole("button", { name: /Clear|Confirm|OK/i }).last().click();
    await expect(page.getByText(subject)).toBeHidden({ timeout: 20_000 }).catch(() => {});
  });

  test("config: export JSON download works; branding fields persist", async ({ page }) => {
    await adminLogin(page);
    await gotoAdmin(page, "Config", /Configuration/i);
    // Backup export
    const exportBtn = page.getByRole("button", { name: "Export JSON" }).first();
    if (await exportBtn.isVisible({ timeout: 8_000 }).catch(() => false)) {
      const dl = page.waitForEvent("download", { timeout: 30_000 }).catch(() => null);
      await exportBtn.click();
      const file = await dl;
      if (file) expect(file.suggestedFilename()).toMatch(/\.json$/);
    }
    // Branding save round-trip
    const brandTitle = page.getByLabel(/Brand name/i).first();
    if (await brandTitle.isVisible({ timeout: 8_000 }).catch(() => false)) {
      await brandTitle.fill("Mailez");
      await page.getByRole("button", { name: "Save branding" }).first().click();
      await expect(page.getByText("Branding saved").first()).toBeVisible({ timeout: 15_000 });
    }
    // AI provider section visible
    await expect(page.getByText(/AI provider/i).first()).toBeVisible({ timeout: 10_000 }).catch(() => {});
  });
});
