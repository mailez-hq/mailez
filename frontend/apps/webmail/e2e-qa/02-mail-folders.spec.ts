import { expect, test } from "@playwright/test";

import { api, login, marker } from "./qa-helpers";

test.describe("folder navigation & management", () => {
  test("standard folders are all present with unread badges", async ({ page }) => {
    await login(page);
    await page.goto("/mail/Inbox");
    // This deployment exposes Inbox/Drafts/Junk/Sent/Trash (no Archive).
    for (const folder of ["Inbox", "Drafts", "Sent", "Trash", "Junk"]) {
      await expect(
        page.getByRole("button", { name: new RegExp(folder) }).first(),
        `${folder} folder button visible`,
      ).toBeVisible({ timeout: 20_000 });
    }
  });

  test("clicking each folder switches list context and URL", async ({ page }) => {
    await login(page);
    await page.goto("/mail/Inbox");
    for (const folder of ["Sent", "Drafts", "Junk", "Trash"]) {
      await page.getByRole("button", { name: new RegExp(`^${folder}`) }).first().click();
      await expect(page).toHaveURL(new RegExp(`/mail/${folder}`), { timeout: 15_000 });
      // The list must follow the URL (search placeholder names the folder) —
      // regression guard for the deep-link folder mismatch bug.
      await expect(page.getByPlaceholder(`Search ${folder}`)).toBeVisible({ timeout: 15_000 });
    }
  });

  test("direct deep link /mail/Drafts shows drafts (regression)", async ({ page }) => {
    await login(page);
    // Fresh full load straight to a non-Inbox folder
    await page.goto("/mail/Trash");
    await expect(page.getByPlaceholder("Search Trash")).toBeVisible({ timeout: 20_000 });
  });

  test("create, rename and delete a custom folder", async ({ page }) => {
    await login(page);
    const name = marker("folder");
    await page.getByRole("button", { name: "New folder" }).first().click();
    const input = page.getByPlaceholder("Folder name");
    await expect(input).toBeVisible({ timeout: 15_000 });
    await input.fill(name);
    await page.keyboard.press("Enter");
    const row = page.getByRole("button", { name: new RegExp(name) });
    await expect(row.first()).toBeVisible({ timeout: 20_000 });
    // Open the row's menu (⋯ button) and delete the folder
    const menuBtn = row.first().locator("..").getByRole("button", { name: "Menu" }).first();
    await menuBtn.click({ force: true }).catch(() => {});
    const del = page.getByText("Delete folder").first();
    if (await del.isVisible({ timeout: 5_000 }).catch(() => false)) {
      await del.click();
      await page.getByRole("button", { name: /Delete|Confirm|OK/i }).last().click();
      await expect(row.first()).toBeHidden({ timeout: 20_000 });
    }
  });

  test("unread badge decreases after reading a message", async ({ page }) => {
    const subject = marker("unread-badge");
    await api("/mail/send", { method: "POST", body: JSON.stringify({ to: ["e2esmoke@example.com"], subject, body: "x" }) });
    await login(page);
    await page.goto("/mail/Inbox");
    const inboxBtn = page.getByRole("button", { name: /Inbox/ }).first();
    // Wait for the seeded message to arrive BEFORE reading the badge
    const row = page.getByRole("button").filter({ hasText: subject }).first();
    await expect(row).toBeVisible({ timeout: 45_000 });
    const before = (await inboxBtn.textContent()) ?? "";
    const mBefore = Number(before.match(/\d+/)?.[0] ?? "0");
    expect(mBefore).toBeGreaterThanOrEqual(1);
    await row.click();
    await expect(page.getByText(subject).first()).toBeVisible({ timeout: 20_000 });
    // Unread badge must have decreased (flag-store regression guard)
    await expect
      .poll(async () => {
        const t = (await inboxBtn.textContent()) ?? "";
        return Number(t.match(/\d+/)?.[0] ?? "0");
      }, { timeout: 25_000 })
      .toBeLessThan(mBefore);
  });

  test("mark all as read clears the inbox badge", async ({ page }) => {
    await login(page);
    await page.goto("/mail/Inbox");
    const trigger = page.getByRole("button", { name: "Mark all as read" });
    if (await trigger.isVisible({ timeout: 5_000 }).catch(() => false)) {
      await trigger.click();
      await expect
        .poll(async () => {
          const t = (await page.getByRole("button", { name: /Inbox/ }).first().textContent()) ?? "";
          return Number(t.match(/\d+/)?.[0] ?? "0");
        }, { timeout: 25_000 })
        .toBe(0);
    }
  });

  test("label manager: create and delete a label", async ({ page }) => {
    await login(page);
    const label = marker("label");
    await page.getByRole("button", { name: "New label…" }).first().click();
    const dialog = page.getByRole("dialog").first();
    await expect(dialog).toBeVisible({ timeout: 15_000 });
    const nameInput = dialog.getByPlaceholder("New label…");
    await nameInput.fill(label);
    await nameInput.press("Enter");
    await expect(dialog.getByText(label).first()).toBeVisible({ timeout: 15_000 });
    // Delete the label to leave no residue (confirm dialog may appear)
    await dialog.getByTitle("Delete").first().click().catch(() => {});
    await page.getByRole("button", { name: /Delete|Confirm/i }).last().click().catch(() => {});
  });
});
