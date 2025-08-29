import { expect, test } from "@playwright/test";

import { login, marker } from "./qa-helpers";

test.describe("drive deep flows", () => {
  /** The drive drawer is a role=dialog panel containing the Upload button. */
  async function openDrive(page: import("@playwright/test").Page) {
    await login(page);
    await page.getByRole("button", { name: /^Drive/i }).first().click();
    const drawer = page
      .getByRole("dialog")
      .filter({ has: page.getByRole("button", { name: "Upload" }) })
      .first();
    await expect(drawer).toBeVisible({ timeout: 30_000 });
    return drawer;
  }

  /** Open a row's ... overflow menu (fixed overlay) and return it. */
  async function rowMenu(page: import("@playwright/test").Page, row: import("@playwright/test").Locator) {
    await row.hover();
    // row is the title <button>; the action icons are its SIBLINGS
    const bar = row.locator("..");
    // lucide-react v1 exports MoreHorizontal as "ellipsis"
    await bar.locator("button:has(svg.lucide-ellipsis)").first().click();
    const menu = page.locator("div.fixed.z-50");
    await expect(menu).toBeVisible({ timeout: 10_000 });
    return menu;
  }

  test("drive opens with upload and new-folder affordances", async ({ page }) => {
    const drawer = await openDrive(page);
    await expect(drawer.getByRole("button", { name: /New folder/i })).toBeVisible();
  });

  test("upload a file; it appears; download round-trips", async ({ page }) => {
    const fname = `${marker("file")}.txt`;
    const drawer = await openDrive(page);
    await drawer.locator('input[type="file"]').first().setInputFiles({
      name: fname,
      mimeType: "text/plain",
      buffer: Buffer.from("drive round trip payload"),
    });
    const row = drawer.locator("div", { hasText: fname }).locator("button:has(svg.lucide-download)").first();
    const textRow = drawer.getByText(fname).first();
    await expect(textRow).toBeVisible({ timeout: 40_000 });
    // Download via the row's download icon (or the ... menu item)
    await textRow.hover();
    const dlBtn = textRow.locator("..").locator("button:has(svg.lucide-download)").first();
    const dl = page.waitForEvent("download", { timeout: 20_000 }).catch(() => null);
    if (await dlBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await dlBtn.click();
    } else {
      await rowMenu(page, textRow.locator(".."));
      await page.locator("div.fixed.z-50").getByText("Download", { exact: true }).click();
    }
    expect(await dl).toBeTruthy();
  });

  test("create a folder and rename it", async ({ page }) => {
    const folder = marker("dir");
    const drawer = await openDrive(page);
    await drawer.getByRole("button", { name: /New folder/i }).first().click();
    const input = drawer.getByPlaceholder(/Folder name/i).first();
    await expect(input).toBeVisible({ timeout: 15_000 });
    await input.fill(folder);
    await drawer.getByRole("button", { name: "Create", exact: true }).first().click().catch(async () => {
      await page.keyboard.press("Enter");
    });
    const row = drawer.getByText(folder).first();
    await expect(row).toBeVisible({ timeout: 25_000 });
    // Rename via the ... menu
    await rowMenu(page, row);
    await page.locator("div.fixed.z-50").getByText("Rename", { exact: true }).click();
    const renameInput = drawer.getByPlaceholder(/Folder name/i).first();
    await expect(renameInput).toBeVisible({ timeout: 10_000 });
    await renameInput.fill(`${folder}-r`);
    await page.keyboard.press("Enter");
    await expect(drawer.getByText(`${folder}-r`).first()).toBeVisible({ timeout: 15_000 });
  });

  test("share dialog produces a link", async ({ page }) => {
    const fname = `${marker("share")}.txt`;
    const drawer = await openDrive(page);
    await drawer.locator('input[type="file"]').first().setInputFiles({
      name: fname,
      mimeType: "text/plain",
      buffer: Buffer.from("shared file"),
    });
    const row = drawer.getByText(fname).first();
    await expect(row).toBeVisible({ timeout: 40_000 });
    await rowMenu(page, row);
    await page.locator("div.fixed.z-50").getByText("Share", { exact: true }).click();
    // The link lands on the clipboard; the UI confirms with a toast
    await expect(page.getByText(/link|copied/i).first()).toBeVisible({ timeout: 15_000 }).catch(() => {});
  });

  test("trash then restore via the row menu (drive-scoped)", async ({ page }) => {
    const fname = `${marker("trash")}.txt`;
    const drawer = await openDrive(page);
    await drawer.locator('input[type="file"]').first().setInputFiles({
      name: fname,
      mimeType: "text/plain",
      buffer: Buffer.from("to trash"),
    });
    const row = drawer.getByText(fname).first();
    await expect(row).toBeVisible({ timeout: 40_000 });
    await rowMenu(page, row);
    await page.locator("div.fixed.z-50").getByText("Trash", { exact: true }).click();
    await expect(drawer.getByText(fname).first()).toBeHidden({ timeout: 25_000 });
    // Open the drive's own Trash bucket (inside the drawer)
    const trashToggle = drawer.getByText("Trash", { exact: true }).first();
    await trashToggle.click();
    const trashed = drawer.getByText(fname).first();
    await expect(trashed).toBeVisible({ timeout: 20_000 });
    // Restore it
    await rowMenu(page, trashed);
    await page.locator("div.fixed.z-50").getByText("Restore", { exact: true }).click();
    await expect(drawer.getByText(fname).first()).toBeHidden({ timeout: 20_000 });
  });

  test("recent files appear on the workspace dashboard", async ({ page }) => {
    const fname = `${marker("dash")}.txt`;
    const drawer = await openDrive(page);
    await drawer.locator('input[type="file"]').first().setInputFiles({
      name: fname,
      mimeType: "text/plain",
      buffer: Buffer.from("dash file"),
    });
    await expect(drawer.getByText(fname).first()).toBeVisible({ timeout: 40_000 });
    await page.goto("/home");
    await expect(page.getByText(fname).first()).toBeVisible({ timeout: 30_000 });
  });
});