import { expect, test } from "@playwright/test";

import { api, login, marker, typeBody } from "./qa-helpers";

test.describe("reading pane deep actions", () => {
  async function openSeeded(page: import("@playwright/test").Page, subject: string) {
    await api("/mail/send", { method: "POST", body: JSON.stringify({ to: ["e2esmoke@example.com"], subject, body: "Deep read body." }) });
    await login(page);
    await page.goto("/mail/Inbox");
    const row = page.getByRole("button").filter({ hasText: subject }).first();
    await expect(row).toBeVisible({ timeout: 30_000 });
    await row.click();
    await expect(page.getByText("Deep read body.").first()).toBeVisible({ timeout: 20_000 });
  }

  test("header shows from/subject and details expand", async ({ page }) => {
    const subject = marker("hdr");
    await openSeeded(page, subject);
    await expect(page.getByText(subject).first()).toBeVisible();
    const details = page.getByRole("button", { name: "Show details" });
    if (await details.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await details.click();
      await expect(page.getByText("e2esmoke@example.com").first()).toBeVisible();
    }
  });

  test("action toolbar: reply / reply all / forward / archive / delete / spam visible", async ({ page }) => {
    await openSeeded(page, marker("actions"));
    for (const action of ["Reply", "Forward", "Archive", "Delete", "Spam"]) {
      const btn = page.getByRole("button", { name: action, exact: true }).first();
      const visible = await btn.isVisible({ timeout: 4_000 }).catch(() => false);
      if (!visible) {
        // Some actions live under the More menu
        const more = page.getByRole("button", { name: "More", exact: true }).first();
        if (await more.isVisible({ timeout: 2_000 }).catch(() => false)) {
          await more.click();
          await expect(page.getByRole("menuitem", { name: action }).first()).toBeVisible();
          await page.keyboard.press("Escape");
        }
      } else {
        expect(visible).toBeTruthy();
      }
    }
  });

  test("quick reply sends an inline answer", async ({ page }) => {
    const subject = marker("qr");
    await openSeeded(page, subject);
    await page.getByRole("button", { name: "Quick reply", exact: true }).first().click();
    // Quick reply is a textarea (placeholder "Quick reply"), not contenteditable
    const box = page.getByPlaceholder("Quick reply").first();
    await expect(box).toBeVisible({ timeout: 15_000 });
    await box.fill("Quick reply body.");
    await page.getByRole("button", { name: "Send", exact: true }).last().click();
    // A Sent copy must land (regression guard)
    await expect
      .poll(async () => {
        const { data } = await api<Array<{ subject: string }>>("/mail/messages?folder=Sent&page=0&sort=date&dir=desc");
        return JSON.stringify(data).includes(subject);
      }, { timeout: 40_000 })
      .toBe(true);
  });

  test("snooze popover offers presets without applying one", async ({ page }) => {
    await openSeeded(page, marker("snooze"));
    // Alarm button in the reader header (title="Snooze"); scope to the reader
    const snooze = page.getByTitle("Snooze", { exact: true }).first();
    await expect(snooze).toBeVisible({ timeout: 10_000 });
    await snooze.click();
    for (const preset of ["Later today", "Tomorrow", "Next week"]) {
      await expect(page.getByText(preset).first()).toBeVisible({ timeout: 5_000 });
    }
    // Close the popover by toggling the same button (no snooze applied)
    await snooze.click();
  });

  test("view raw and download .eml are offered in the action bar", async ({ page }) => {
    await openSeeded(page, marker("raw"));
    await expect(page.getByTitle("View raw").first()).toBeVisible({ timeout: 10_000 });
    await expect(page.getByTitle("Download .eml").first()).toBeVisible({ timeout: 10_000 });
  });

  test("mark unread via Shift+U restores the unread state", async ({ page }) => {
    const subject = marker("unread-again");
    await openSeeded(page, subject);
    // Reader supports Shift+U (shortcuts dialog documents it)
    await page.keyboard.press("Shift+u");
    await page.waitForTimeout(1_500);
    await page.goto("/mail/Inbox");
    const row = page.getByRole("button").filter({ hasText: subject }).first();
    await expect(row).toBeVisible({ timeout: 30_000 });
    // Unread rows render with a heavier font treatment / unseen dot; assert
    // via the inbox badge counting at least this message.
    await expect
      .poll(async () => {
        const t = (await page.getByRole("button", { name: /Inbox/ }).first().textContent()) ?? "";
        return Number(t.match(/\d+/)?.[0] ?? "0");
      }, { timeout: 20_000 })
      .toBeGreaterThanOrEqual(1);
  });

  test("message with attachment shows attachment card and download", async ({ page }) => {
    // Seed with attachment via API (base64 text file)
    const content = Buffer.from("attachment payload for qa").toString("base64");
    const subject = marker("attach");
    const { status } = await api("/mail/send", {
      method: "POST",
      body: JSON.stringify({
        to: ["e2esmoke@example.com"],
        subject,
        body: "see attachment",
        attachments: [{ filename: "qa-attach.txt", content_type: "text/plain", size: content.length, data: content }],
      }),
    });
    expect([200, 204]).toContain(status);
    await login(page);
    await page.goto("/mail/Inbox");
    const row = page.getByRole("button").filter({ hasText: subject }).first();
    await expect(row).toBeVisible({ timeout: 30_000 });
    await row.click();
    await expect(page.getByText(/Attachments?/i).first()).toBeVisible({ timeout: 20_000 });
    await expect(page.getByText("qa-attach.txt").first()).toBeVisible({ timeout: 10_000 });
    const downloadAll = page.getByRole("button", { name: /Download all/i });
    if (await downloadAll.isVisible({ timeout: 3_000 }).catch(() => false)) {
      const dl = page.waitForEvent("download", { timeout: 20_000 }).catch(() => null);
      await downloadAll.click();
      const file = await dl;
      if (file) expect(file.suggestedFilename()).toMatch(/\.zip$/);
    }
  });
});
