import { expect, test } from "@playwright/test";

import { E2E_DOMAIN, E2E_USER, login, openCompose } from "./helpers";

// A unique marker per run keeps assertions (list rows, search results)
// independent of whatever else is in the mailbox.
function uniqueSubject(): string {
  return `e2e smoke ${Date.now()}`;
}

test.describe("mailbox main chain", () => {
  test("compose a self-addressed mail, find it in Sent, open it", async ({ page }) => {
    const subject = uniqueSubject();
    await login(page);
    await openCompose(page);

    // Recipients: the input invites user@example.com; the selector covers
    // both plain and full-address forms accepted by the backend.
    await page.getByPlaceholder("user@example.com").fill(`${E2E_USER}@${E2E_DOMAIN}`);
    await page.getByTestId("compose-subject").fill(subject);
    // ProseMirror ignores programmatic fills — click and type for real.
    const editor = page.locator('[contenteditable="true"]').first();
    await editor.click();
    await page.keyboard.type("Hello from the mailez e2e suite.");

    await page.getByRole("button", { name: "Send", exact: true }).click();

    // Success feedback = the compose dialog closes.
    await expect(page.getByRole("heading", { name: "New message" })).toBeHidden({ timeout: 20_000 });

    // The message shows up in the Sent folder. The Sent copy is appended
    // shortly after the send API returns, so refresh until the row lands.
    await page.goto("/mail/Sent");
    const row = page.getByRole("button").filter({ hasText: subject }).first();
    await expect(async () => {
      if (!(await row.isVisible().catch(() => false))) {
        await page.getByRole("button", { name: "Refresh" }).click();
      }
      await expect(row).toBeVisible({ timeout: 3_000 });
    }).toPass({ timeout: 45_000 });

    // Opening the row shows the reading pane with the same subject.
    await row.click();
    await expect(page.getByText(subject).first()).toBeVisible({ timeout: 20_000 });
    await expect(page.getByText("Hello from the mailez e2e suite.").first()).toBeVisible();
  });

  test("keyword search narrows the list to matching messages", async ({ page }) => {
    const subject = uniqueSubject();
    await login(page);

    // Seed a message through the same UI path, then find it via search.
    await openCompose(page);
    await page.getByPlaceholder("user@example.com").fill(`${E2E_USER}@${E2E_DOMAIN}`);
    await page.getByTestId("compose-subject").fill(subject);
    // ProseMirror ignores programmatic fills — click and type for real.
    const editor = page.locator('[contenteditable="true"]').first();
    await editor.click();
    await page.keyboard.type("Searchable body.");
    await page.getByRole("button", { name: "Send", exact: true }).click();
    await expect(page.getByRole("heading", { name: "New message" })).toBeHidden({ timeout: 20_000 });

    // The search box lives in the mailbox view — go to Sent.
    await page.goto("/mail/Sent");
    const searchBox = page.getByPlaceholder(/Search /);
    const row = page.getByRole("button").filter({ hasText: subject }).first();
    // Search executes on Enter (see the "Search ↵" affordance); retry the
    // whole query while the Sent copy settles.
    await expect(async () => {
      await searchBox.fill(subject);
      await searchBox.press("Enter");
      await expect(row).toBeVisible({ timeout: 3_000 });
    }).toPass({ timeout: 45_000 });

    // Clearing search restores the plain folder list.
    await page.getByTitle("Clear search").click();
    await expect(searchBox).toHaveValue("");
  });

  test("folder navigation switches the list", async ({ page }) => {
    await login(page);
    await page.goto("/mail/Inbox");
    await expect(page).toHaveURL(/\/mail\/Inbox/);

    // Folder buttons may carry an unread badge ("Inbox 3"), so match loosely.
    await page.getByRole("button", { name: /Trash/ }).first().click();
    await expect(page).toHaveURL(/\/mail\/Trash/);

    await page.getByRole("button", { name: /Inbox/ }).first().click();
    await expect(page).toHaveURL(/\/mail\/Inbox/);
  });
});
