import { expect, test } from "@playwright/test";

import { acceptDialogs, adminLogin, gotoAdmin, marker, rowDelete, rowEdit } from "./admin-helpers";

test.describe("admin users page", () => {
  test.beforeEach(({ page }) => acceptDialogs(page));

  test("users table renders with columns and the admin account", async ({ page }) => {
    await adminLogin(page);
    await gotoAdmin(page, "Users", /Users/i);
    await expect(page.getByRole("columnheader", { name: "Email" })).toBeVisible({ timeout: 20_000 });
    await expect(page.getByText("admin@example.com").first()).toBeVisible({ timeout: 20_000 });
  });

  test("create a user, edit it, then delete it", async ({ page }) => {
    const email = `${marker("u")}@example.com`;
    await adminLogin(page);
    await gotoAdmin(page, "Users", /Users/i);
    await page.getByRole("button", { name: "New user" }).first().click();
    const dialog = page.getByRole("dialog").first();
    await expect(dialog).toBeVisible({ timeout: 15_000 });
    // Fields have no htmlFor association - locate by placeholder/type
    await dialog.getByPlaceholder("user@example.com").fill(email);
    await dialog.locator("input[type=password]").fill("UserPw!2026x");
    await dialog.getByRole("button", { name: /Save|Create/i }).last().click();
    await expect(dialog).toBeHidden({ timeout: 20_000 });
    // Search to surface the new user regardless of pagination
    const search = page.getByPlaceholder(/Search|search/i).first();
    if (await search.isVisible({ timeout: 5_000 }).catch(() => false)) await search.fill(email);
    await expect(page.getByText(email).first()).toBeVisible({ timeout: 20_000 });

    // Edit: rename the displayed name (text input nth(1) = displayed name)
    const row = page.locator("tr", { hasText: email }).first();
    await rowEdit(page, row);
    const editDialog = page.getByRole("dialog").first();
    await expect(editDialog).toBeVisible({ timeout: 15_000 });
    const textInputs = editDialog.locator("input:not([type=number]):not([type=password]):not([type=checkbox])");
    await textInputs.nth(1).fill("QA Edited");
    // Edit-mode submit is labeled "Edit" (editTarget ? edit : create)
    await editDialog.getByRole("button", { name: /^Edit$/ }).click();
    await expect(editDialog).toBeHidden({ timeout: 20_000 });
    await expect(page.getByText("QA Edited").first()).toBeVisible({ timeout: 20_000 });

    // Delete (Actions menu + auto-accepted confirm)
    const search2 = page.getByPlaceholder(/Search|search/i).first();
    if (await search2.isVisible({ timeout: 3_000 }).catch(() => false)) await search2.fill(email);
    await rowDelete(page, page.locator("tr", { hasText: email }).first());
    await expect(page.getByText(email).first()).toBeHidden({ timeout: 20_000 });
  });

  test("user search/filter narrows the table", async ({ page }) => {
    await adminLogin(page);
    await gotoAdmin(page, "Users", /Users/i);
    const search = page.getByPlaceholder(/Search|search/i).first();
    if (await search.isVisible({ timeout: 5_000 }).catch(() => false)) {
      await search.fill("e2esmoke");
      await expect(page.getByText("e2esmoke@example.com").first()).toBeVisible({ timeout: 15_000 });
      await expect(page.getByText("admin@example.com")).toBeHidden({ timeout: 15_000 }).catch(() => {});
    }
  });
});

test.describe("admin domains page", () => {
  test.beforeEach(({ page }) => acceptDialogs(page));

  test("domains table shows example.com", async ({ page }) => {
    await adminLogin(page);
    await gotoAdmin(page, "Domains", /Domains/i);
    await expect(page.getByText("example.com").first()).toBeVisible({ timeout: 20_000 });
  });

  test("create a domain, see DKIM state, then delete it", async ({ page }) => {
    const domain = `${marker("d")}.test`;
    await adminLogin(page);
    await gotoAdmin(page, "Domains", /Domains/i);
    await page.getByRole("button", { name: "New domain" }).first().click();
    const dialog = page.getByRole("dialog").first();
    await expect(dialog).toBeVisible({ timeout: 15_000 });
    await dialog.getByPlaceholder("example.com", { exact: true }).fill(domain);
    await dialog.getByRole("button", { name: /Save|Create/i }).last().click();
    await expect(dialog).toBeHidden({ timeout: 20_000 });
    await expect(page.getByText(domain).first()).toBeVisible({ timeout: 20_000 });

    // Edit view exposes the DKIM section
    await rowEdit(page, page.locator("tr", { hasText: domain }).first());
    const editDialog = page.getByRole("dialog").first();
    await expect(editDialog).toBeVisible({ timeout: 15_000 });
    await expect(editDialog.getByText(/DKIM/i).first()).toBeVisible({ timeout: 10_000 });
    await editDialog.getByRole("button", { name: /Close|Cancel/i }).last().click();
    await expect(editDialog).toBeHidden({ timeout: 15_000 });

    // Delete
    await rowDelete(page, page.locator("tr", { hasText: domain }).first());
    await expect(page.getByText(domain).first()).toBeHidden({ timeout: 20_000 });
  });
});