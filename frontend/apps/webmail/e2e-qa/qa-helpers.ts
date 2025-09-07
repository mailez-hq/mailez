import { expect, type Page } from "@playwright/test";

// QA suite helpers — comprehensive page-driven tests against the dev server.
// Dev-mode hydration is slow: fill() before React attaches resets controlled
// inputs, so every fill must be verified to stick and retried otherwise.

export const QA_USER = process.env.MAILWEB_QA_USER ?? "e2esmoke";
export const QA_DOMAIN = process.env.MAILWEB_QA_DOMAIN ?? "example.com";
export const QA_PASSWORD = process.env.MAILWEB_QA_PASSWORD ?? "E2eSmoke2026!";
export const QA_ADMIN = process.env.MAILWEB_QA_ADMIN ?? "admin";
export const QA_ADMIN_PASSWORD = process.env.MAILWEB_QA_ADMIN_PASSWORD ?? "MailezDemo2026!";
export const WEBMAIL_BASE = process.env.MAILWEB_QA_BASE_URL ?? "http://localhost:3001";
export const ADMIN_BASE = process.env.ADMIN_QA_BASE_URL ?? "http://localhost:3000";
export const API_BASE = process.env.QA_API_BASE_URL ?? "http://localhost:8080";

/** fill() that survives pre-hydration resets: fill, verify, retry. */
export async function fillSticky(
  locator: Parameters<typeof expect>[0] extends never ? never : import("@playwright/test").Locator,
  value: string,
): Promise<void> {
  await expect(async () => {
    await locator.fill("");
    await locator.fill(value);
    await expect(locator).toHaveValue(value, { timeout: 2_000 });
  }).toPass({ timeout: 45_000 });
}

/** Submit the login form with self-healing retries; reports the outcome. */
export async function attemptLogin(
  page: Page,
  user: string,
  pw: string,
): Promise<{ outcome: "navigated"; finalUrl: string } | { outcome: "backend-error"; error: string }> {
  await page.goto("/");
  // Settings arrive async and mount the domain picker, remounting the account
  // input (controlled state resets). Wait for that settle, then self-heal:
  // click, check navigation, refill + re-click if a remount wiped a field.
  await page.locator("select").first().waitFor({ state: "visible", timeout: 20_000 }).catch(() => {});
  const account = page.getByLabel("Account");
  const password = page.getByLabel("Password");
  const deadline = Date.now() + 60_000;
  let first = true;
  while (Date.now() < deadline) {
    if (!first) await page.waitForTimeout(1_500);
    first = false;
    await fillSticky(account, user);
    await fillSticky(password, pw);
    await page.getByRole("button", { name: "Sign in" }).click();
    const navigated = await page
      .waitForURL(/\/(home|mail)/, { timeout: 8_000 })
      .then(() => true)
      .catch(() => false);
    if (navigated) {
      if (!page.url().includes("/mail")) await page.goto("/mail/Inbox");
      // The signed-in shell is ready once the top-right account avatar in the
      // global app header is visible (the exit moved from the sidebar footer
      // into that avatar's menu, mirroring mainstream webmail).
      await expect(page.getByRole("button", { name: "My account" })).toBeVisible({ timeout: 45_000 });
      await page.waitForLoadState("networkidle").catch(() => {});
      return { outcome: "navigated", finalUrl: page.url() };
    }
    const err = await page.locator("p.text-destructive").textContent().catch(() => "");
    if (err && /incorrect|invalid|wrong|错误|失败/i.test(err)) {
      return { outcome: "backend-error", error: err };
    }
  }
  throw new Error(`login did not complete after retries; still on ${page.url()}`);
}

/** Log in through the UI and wait for the mailbox shell to be interactive. */
export async function login(page: Page, user = `${QA_USER}@${QA_DOMAIN}`, pw = QA_PASSWORD): Promise<void> {
  const res = await attemptLogin(page, user, pw);
  if (res.outcome === "backend-error") throw new Error(`login rejected by backend: ${res.error}`);
}

/** Open the compose dialog via the sidebar "Write" button. */
export async function openCompose(page: Page): Promise<void> {
  await page.getByRole("button", { name: "Write", exact: true }).first().click();
  await expect(page.getByRole("heading", { name: "New message" })).toBeVisible({ timeout: 30_000 });
}

/** Type into the ProseMirror editor (fill() does not work on it). */
export async function typeBody(page: Page, text: string): Promise<void> {
  const editor = page.locator('[contenteditable="true"]').first();
  await editor.click();
  await page.keyboard.type(text);
}

/** Marker unique per test run for subjects / names. */
export function marker(prefix = "qa"): string {
  return `${prefix}-${Date.now()}-${Math.floor(Math.random() * 10_000)}`;
}

/** API session cookie for seeding data directly (bypasses UI speed). */
let cachedCookie = "";
export async function apiCookie(user = `${QA_USER}@${QA_DOMAIN}`, pw = QA_PASSWORD): Promise<string> {
  if (cachedCookie) return cachedCookie;
  const res = await fetch(`${API_BASE}/api/v1/sso/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email: user, pw }),
  });
  if (!res.ok) throw new Error(`API login failed: ${res.status}`);
  const setCookie = res.headers.get("set-cookie") ?? "";
  cachedCookie = setCookie.split(";")[0];
  return cachedCookie;
}

export async function api<T>(path: string, init?: RequestInit, cookie?: string): Promise<{ status: number; data: T }> {
  const c = cookie ?? (await apiCookie());
  const res = await fetch(`${API_BASE}/api/v1${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", Cookie: c, ...(init?.headers ?? {}) },
  });
  const text = await res.text();
  const data = text ? (JSON.parse(text) as T) : (undefined as unknown as T);
  return { status: res.status, data };
}

/** Send a mail through the API and wait for it to reach the recipient. */
export async function seedMail(
  to: string,
  subject: string,
  body = "QA seeded body.",
): Promise<void> {
  const { status } = await api<{ status?: number }>("/mail/send", {
    method: "POST",
    body: JSON.stringify({ to: [to], subject, body }),
  });
  if (status !== 204 && status !== 200) throw new Error(`seed send failed: ${status}`);
}
