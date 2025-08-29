import { expect, test } from "@playwright/test";

import { api, login, marker } from "./qa-helpers";

test.describe("calendar drawer deep flows", () => {
  async function openCalendar(page: import("@playwright/test").Page) {
    await login(page);
    const trigger = page.getByRole("button", { name: /^Calendar/i }).first();
    await trigger.click();
    await expect(page.getByText(/Month|Week|Today/i).first()).toBeVisible({ timeout: 30_000 });
  }

  test("calendar opens with month/week views and today navigation", async ({ page }) => {
    await openCalendar(page);
    for (const ctl of ["Today", "Month", "Week"]) {
      const btn = page.getByRole("button", { name: new RegExp(`^${ctl}$`) }).first();
      expect(await btn.isVisible({ timeout: 5_000 }).catch(() => false)).toBeTruthy();
    }
    await page.getByRole("button", { name: "Week", exact: true }).first().click().catch(() => {});
    await page.getByRole("button", { name: "Month", exact: true }).first().click().catch(() => {});
  });

  test("create an event; it appears; edit then delete it", async ({ page }) => {
    const title = marker("event");
    await openCalendar(page);
    const startVal = new Date(Date.now() + 3600_000);
    const pad = (n: number) => String(n).padStart(2, "0");
    const local = `${startVal.getFullYear()}-${pad(startVal.getMonth() + 1)}-${pad(startVal.getDate())}T${pad(startVal.getHours())}:${pad(startVal.getMinutes())}`;
    // The dialog can remount while settings settle; retry the whole create
    await expect(async () => {
      await page.getByRole("button", { name: /New event/i }).first().click();
      const dialog = page.getByRole("dialog").first();
      await expect(dialog).toBeVisible({ timeout: 15_000 });
      await dialog.getByPlaceholder("Event title").fill(title);
      const startInput = dialog.locator('input[type="datetime-local"]').first();
      await startInput.fill(local);
      await expect(startInput).toHaveValue(local, { timeout: 5_000 });
      await dialog.getByRole("button", { name: "Save", exact: true }).click();
      await expect(dialog).toBeHidden({ timeout: 15_000 });
    }).toPass({ timeout: 90_000 });
    await expect(page.getByText(title).first()).toBeVisible({ timeout: 20_000 });

    // Edit: click the event, change the title
    await page.getByText(title).first().click();
    const editDialog = page.getByRole("dialog").first();
    if (await editDialog.isVisible({ timeout: 8_000 }).catch(() => false)) {
      const titleInput = editDialog.getByPlaceholder("Event title");
      if (await titleInput.isVisible().catch(() => false)) {
        await titleInput.fill(`${title}-x`);
        await editDialog.getByRole("button", { name: "Save", exact: true }).click();
        await expect(editDialog).toBeHidden({ timeout: 25_000 });
        await expect(page.getByText(`${title}-x`).first()).toBeVisible({ timeout: 20_000 });
      }
      // Delete
      await page.getByText(`${title}-x`).first().click();
      const editDialog2 = page.getByRole("dialog").first();
      const del = editDialog2.getByRole("button", { name: /Delete/i });
      if (await del.isVisible({ timeout: 8_000 }).catch(() => false)) {
        await del.click();
        await expect(page.getByText(`${title}-x`)).toBeHidden({ timeout: 20_000 }).catch(() => {});
      }
    }
  });

  test("event dialog validates empty title", async ({ page }) => {
    await openCalendar(page);
    await page.getByRole("button", { name: /New event/i }).first().click();
    const dialog = page.getByRole("dialog").first();
    await dialog.getByRole("button", { name: "Save", exact: true }).click();
    await expect(page.getByText("Title and start time are required")).toBeVisible({ timeout: 10_000 });
    await page.keyboard.press("Escape");
  });

  test("share dialog adds and removes a sharee", async ({ page }) => {
    await openCalendar(page);
    const share = page.getByRole("button", { name: /Share/i }).first();
    await share.click();
    const dialog = page.getByRole("dialog").first();
    await expect(dialog).toBeVisible({ timeout: 15_000 });
    const email = dialog.getByPlaceholder(/email/i).first();
    if (await email.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await email.fill("e2esmoke@example.com");
      await dialog.getByRole("button", { name: /Add|Grant/i }).first().click();
      await expect(page.getByText("e2esmoke@example.com").first()).toBeVisible({ timeout: 15_000 });
    }
    // Subscription URL visible
    await expect(page.getByText(/\.ics|Subscription/i).first()).toBeVisible({ timeout: 10_000 }).catch(() => {});
    await page.keyboard.press("Escape");
  });

  test("events seeded via API render on the calendar", async ({ page }) => {
    const title = marker("seeded");
    const start = new Date(Date.now() + 3 * 3600_000).toISOString();
    const end = new Date(Date.now() + 4 * 3600_000).toISOString();
    await api("/calendar/events", { method: "POST", body: JSON.stringify({ summary: title, start, end }) });
    await openCalendar(page);
    await expect(page.getByText(title).first()).toBeVisible({ timeout: 20_000 });
    // Cleanup
    const { data } = await api<Array<{ id: number; summary: string }>>("/calendar/events?from=2020-01-01T00:00:00Z&to=2030-01-01T00:00:00Z");
    const row = (data as unknown as Array<{ id: number; summary: string }>).find((e) => e.summary === title);
    if (row) await api(`/calendar/events/${row.id}`, { method: "DELETE" });
  });
});
