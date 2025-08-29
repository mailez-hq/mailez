import { expect, type Locator, type Page } from "@playwright/test";

export const ADMIN_BASE = process.env.ADMIN_QA_BASE_URL ?? "http://localhost:3000";
export const ADMIN_EMAIL = process.env.ADMIN_QA_EMAIL ?? "admin@example.com";
export const ADMIN_PASSWORD = process.env.ADMIN_QA_PASSWORD ?? "MailezDemo2026!";
export const API_BASE = process.env.QA_API_BASE_URL ?? "http://localhost:8080";

/** fill() that survives pre-hydration resets: fill, verify, retry. */
export async function fillSticky(locator: Locator, value: string): Promise<void> {
  await expect(async () => {
    await locator.fill("");
    await locator.fill(value);
    await expect(locator).toHaveValue(value, { timeout: 2_000 });
  }).toPass({ timeout: 45_000 });
}

/** Submit the admin login form with self-healing retries. */
export async function adminLogin(page: Page): Promise<void> {
  await page.goto("/");
  await page.locator("form").first().waitFor({ state: "visible", timeout: 20_000 }).catch(() => {});
  const email = page.getByLabel(/Email/i).first();
  const password = page.getByLabel(/Password/i).first();
  const deadline = Date.now() + 60_000;
  let first = true;
  while (Date.now() < deadline) {
    if (!first) await page.waitForTimeout(1_500);
    first = false;
    await fillSticky(email, ADMIN_EMAIL);
    await fillSticky(password, ADMIN_PASSWORD);
    await page.getByRole("button", { name: "Sign in" }).click();
    // Success = the admin sidebar rendered (Overview link). A URL-based
    // check is unreliable here: "http://" defeats login-excluding regexes,
    // and Next may keep "/" while the client router settles.
    const navigated = await page
      .getByRole("link", { name: /Overview/i })
      .first()
      .waitFor({ state: "visible", timeout: 30_000 }) // full-suite runs face dev-mode cold compiles
      .then(() => true)
      .catch(() => false);
    if (navigated) {
      await page.waitForLoadState("networkidle").catch(() => {});
      return;
    }
  }
  throw new Error(`admin login did not navigate; still on ${page.url()}`);
}

export function marker(prefix = "qa"): string {
  return `${prefix}-${Date.now()}-${Math.floor(Math.random() * 10_000)}`;
}

/** Navigate via the sidebar and assert the page header appears. */
export async function gotoAdmin(page: Page, navName: string, headerPattern: RegExp): Promise<void> {
  await page.getByRole("link", { name: navName }).first().click();
  await expect(page.getByRole("heading", { name: headerPattern })).toBeVisible({ timeout: 30_000 });
}

/**
 * Accept native confirm()/alert() dialogs — admin delete flows use window.confirm
 * and Playwright dismisses them by default, which would cancel every deletion.
 */
export function acceptDialogs(page: Page): void {
  page.on("dialog", (d) => d.accept().catch(() => {}));
}

let cachedAdminCookie = "";
export async function adminCookie(): Promise<string> {
  if (cachedAdminCookie) return cachedAdminCookie;
  const res = await fetch(`${API_BASE}/api/v1/sso/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email: ADMIN_EMAIL, pw: ADMIN_PASSWORD }),
  });
  if (!res.ok) throw new Error(`admin API login failed: ${res.status}`);
  cachedAdminCookie = (res.headers.get("set-cookie") ?? "").split(";")[0];
  return cachedAdminCookie;
}

export async function adminApi<T>(path: string, init?: RequestInit): Promise<{ status: number; data: T }> {
  const c = await adminCookie();
  const res = await fetch(`${API_BASE}/api/v1${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", Cookie: c, ...(init?.headers ?? {}) },
  });
  const text = await res.text();
  const data = text ? (JSON.parse(text) as T) : (undefined as unknown as T);
  return { status: res.status, data };
}

/** Rows expose edit/delete behind the ⋯ "Actions" dropdown (RowActions). */
export async function rowEdit(page: Page, row: ReturnType<Page["locator"]>): Promise<void> {
  await row.getByRole("button", { name: "Actions" }).click();
  await page.getByRole("menuitem", { name: "Edit" }).click();
}

export async function rowDelete(page: Page, row: ReturnType<Page["locator"]>): Promise<void> {
  await row.getByRole("button", { name: "Actions" }).click();
  await page.getByRole("menuitem", { name: "Delete" }).click();
  // window.confirm is auto-accepted via acceptDialogs
}
