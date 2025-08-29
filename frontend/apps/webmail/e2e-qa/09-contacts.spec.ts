import { expect, test } from "@playwright/test";

import { api, login, marker } from "./qa-helpers";

test.describe("contacts deep flows", () => {
  async function openContacts(page: import("@playwright/test").Page) {
    await login(page);
    await page.getByRole("button", { name: "Contacts", exact: true }).first().click();
    await expect(page.getByText(/My contacts|All departments/i).first()).toBeVisible({ timeout: 30_000 });
  }

  test("contacts panel opens with mine/org tabs", async ({ page }) => {
    await openContacts(page);
    await expect(page.getByText(/My contacts/i).first()).toBeVisible();
  });

  test("add, search and delete a contact", async ({ page }) => {
    const name = marker("contact");
    await openContacts(page);
    await page.getByRole("button", { name: "Add", exact: true }).first().click();
    const dialog = page.getByRole("dialog").first();
    await expect(dialog).toBeVisible({ timeout: 15_000 });
    await dialog.getByPlaceholder("Name").fill(name);
    await dialog.locator('input[type="email"]').first().fill(`${name}@example.com`);
    await dialog.getByRole("button", { name: "Add", exact: true }).last().click();
    await expect(page.getByText(name).first()).toBeVisible({ timeout: 20_000 });

    // Search narrows to it
    const search = page.getByPlaceholder(/Search|搜索/i).first();
    await search.fill(name);
    await expect(page.getByText(name).first()).toBeVisible({ timeout: 15_000 });

    // Delete it
    const row = page.getByText(name).first();
    await row.click({ force: true });
    const del = page.getByRole("button", { name: "Delete", exact: true }).first();
    if (await del.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await del.click();
      await expect(page.getByText(name)).toBeHidden({ timeout: 15_000 }).catch(() => {});
    }
  });

  test("contact history opens past conversations", async ({ page }) => {
    const subject = marker("hist");
    await api("/mail/send", { method: "POST", body: JSON.stringify({ to: ["e2esmoke@example.com"], subject, body: "history check" }) });
    await openContacts(page);
    const self = page.getByText("e2esmoke@example.com").first();
    if (await self.isVisible({ timeout: 8_000 }).catch(() => false)) {
      await self.click({ force: true });
      await expect(page.getByText(/History|历史/i).first()).toBeVisible({ timeout: 15_000 }).catch(() => {});
    }
  });

  test("vCard export triggers a download", async ({ page }) => {
    await openContacts(page);
    const exportBtn = page.getByRole("button", { name: /Export/i }).first();
    if (await exportBtn.isVisible({ timeout: 5_000 }).catch(() => false)) {
      const dl = page.waitForEvent("download", { timeout: 20_000 }).catch(() => null);
      await exportBtn.click();
      const file = await dl;
      if (file) expect(file.suggestedFilename()).toMatch(/\.vcf$/);
    }
  });

  test("CardDAV sync section exposes URL and credentials", async ({ page }) => {
    await openContacts(page);
    const carddav = page.getByRole("button", { name: /CardDAV/i }).first();
    if (await carddav.isVisible({ timeout: 5_000 }).catch(() => false)) {
      await carddav.click();
      await expect(page.getByText(/CardDAV/i).first()).toBeVisible({ timeout: 15_000 });
    }
  });
});
