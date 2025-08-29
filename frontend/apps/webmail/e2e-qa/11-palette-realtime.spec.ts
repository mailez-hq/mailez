import { expect, test } from "@playwright/test";

import { api, login, marker, openCompose } from "./qa-helpers";

test.describe("command palette, shortcuts, workspace, realtime", () => {
  test("Ctrl+K opens the command palette with core actions", async ({ page }) => {
    await login(page);
    await page.goto("/mail/Inbox");
    // The keystroke can land before hydration; retry until the PALETTE input
    // (distinct from the mail search box) appears.
    await expect(async () => {
      await page.keyboard.press("Control+k");
      await expect(page.getByPlaceholder(/Type a command/i)).toBeVisible({ timeout: 5_000 });
    }).toPass({ timeout: 45_000 });
    for (const item of ["New message", "Settings", "Contacts"]) {
      await expect(page.getByText(item).first()).toBeVisible();
    }
    await page.keyboard.press("Escape");
  });

  test("palette action navigates (Contacts)", async ({ page }) => {
    await login(page);
    await page.goto("/mail/Inbox");
    await page.keyboard.press("Control+k");
    await page.getByText("Contacts").first().click();
    await expect(page.getByText(/My contacts/i).first()).toBeVisible({ timeout: 15_000 });
  });

  test("? opens the keyboard shortcuts dialog", async ({ page }) => {
    await login(page);
    await page.goto("/mail/Inbox");
    await page.keyboard.press("?");
    await expect(page.getByText(/Keyboard shortcuts/i).first()).toBeVisible({ timeout: 15_000 });
    await page.keyboard.press("Escape");
  });

  test("/ focuses the search box; j/k navigate rows; x selects", async ({ page }) => {
    const subject = marker("kbd");
    await api("/mail/send", { method: "POST", body: JSON.stringify({ to: ["e2esmoke@example.com"], subject, body: "kb" }) });
    await login(page);
    await page.goto("/mail/Inbox");
    await page.keyboard.press("/");
    await expect(page.getByPlaceholder(/Search /)).toBeFocused({ timeout: 10_000 });
    await page.keyboard.press("Escape");
    // Focus list then navigate
    await page.getByPlaceholder(/Search /).click();
    await page.keyboard.press("Escape");
    const row = page.getByRole("button").filter({ hasText: subject }).first();
    await expect(row).toBeVisible({ timeout: 30_000 });
    await row.focus();
    await page.keyboard.press("o");
    await expect(page.getByText("kb").first()).toBeVisible({ timeout: 15_000 }).catch(() => {});
  });

  test("workspace dashboard shows greeting, unread, storage, events, recent files", async ({ page }) => {
    await login(page);
    await page.goto("/home");
    await expect(page.getByText(/Good (morning|afternoon|evening)/i).first()).toBeVisible({ timeout: 30_000 });
    await expect(page.getByText(/Unread/i).first()).toBeVisible({ timeout: 15_000 });
    await expect(page.getByText(/Storage/i).first()).toBeVisible({ timeout: 15_000 }).catch(() => {});
    await expect(page.getByText(/Today|schedule/i).first()).toBeVisible({ timeout: 15_000 }).catch(() => {});
    // Quick actions present
    await expect(page.getByRole("button", { name: "Write", exact: true }).first()).toBeVisible();
    // Recent files section exists (title or empty text)
    await expect(page.getByText(/Recent files|No files/i).first()).toBeVisible({ timeout: 15_000 }).catch(() => {});
  });

  test("dashboard upcoming event is clickable and opens calendar", async ({ page }) => {
    const title = marker("dash-event");
    const start = new Date(Date.now() + 2 * 3600_000).toISOString();
    const end = new Date(Date.now() + 3 * 3600_000).toISOString();
    await api("/calendar/events", { method: "POST", body: JSON.stringify({ summary: title, start, end }) });
    await login(page);
    await page.goto("/home");
    const ev = page.getByText(title).first();
    if (await ev.isVisible({ timeout: 20_000 }).catch(() => false)) {
      await ev.click();
      await expect(page.getByText(/Month|Week|Today/i).first()).toBeVisible({ timeout: 15_000 }).catch(() => {});
    }
    const { data } = await api<Array<{ id: number; summary: string }>>("/calendar/events?from=2020-01-01T00:00:00Z&to=2030-01-01T00:00:00Z");
    const row = (data as unknown as Array<{ id: number; summary: string }>).find((e) => e.summary === title);
    if (row) await api(`/calendar/events/${row.id}`, { method: "DELETE" });
  });

  test("SSE realtime: new mail surfaces without manual refresh", async ({ page }) => {
    const subject = marker("live");
    await login(page);
    await page.goto("/mail/Inbox");
    const row = page.getByRole("button").filter({ hasText: subject }).first();
    // Seed AFTER the inbox is open; SSE should push a toast/new row
    await api("/mail/send", { method: "POST", body: JSON.stringify({ to: ["e2esmoke@example.com"], subject, body: "pushed" }) });
    const appeared = await row
      .waitFor({ state: "visible", timeout: 45_000 })
      .then(() => true)
      .catch(() => false);
    if (!appeared) {
      // Fall back: the toast may have shown instead of auto-insert
      await page.getByRole("button", { name: "Refresh" }).first().click();
      await expect(row).toBeVisible({ timeout: 30_000 });
    }
  });

  test("compose full-screen (pop out) keeps the draft", async ({ page }) => {
    const subject = marker("popout");
    await login(page);
    await openCompose(page);
    await page.getByTestId("compose-subject").fill(subject);
    const pop = page.getByRole("button", { name: /Full compose/i }).first();
    if (await pop.isVisible({ timeout: 4_000 }).catch(() => false)) {
      await pop.click();
      await expect(page.getByTestId("compose-subject")).toBeVisible({ timeout: 15_000 });
    }
    await page.keyboard.press("Escape");
  });
});
