// Shared E2E helpers. Logs in ONCE per process and reuses the session to
// avoid tripping the backend's login rate limiter.
import { chromium } from "playwright";

const BASE = "http://127.0.0.1:8083";
export const USER = { email: "admin@example.com", pw: "MailezDemo2026!" };
let cookieCache = null;

export async function apiLogin() {
  if (cookieCache) return cookieCache;
  for (let attempt = 0; attempt < 3; attempt++) {
    const res = await fetch(`${BASE}/api/v1/sso/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(USER),
    });
    if (res.status === 429) {
      const wait = 30000 * (attempt + 1);
      console.log(`  (login rate-limited, waiting ${wait / 1000}s)`);
      await new Promise((r) => setTimeout(r, wait));
      continue;
    }
    const cookie = (res.headers.get("set-cookie") || "").split(";")[0];
    if (!cookie || !cookie.includes("=")) throw new Error(`login failed: ${res.status}`);
    const eq = cookie.indexOf("=");
    cookieCache = { name: cookie.slice(0, eq), value: cookie.slice(eq + 1) };
    return cookieCache;
  }
  throw new Error("login rate-limited repeatedly; aborting");
}

export async function withSession(path, fn) {
  const cookie = await apiLogin();
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  await page.context().addCookies([{ name: cookie.name, value: cookie.value, domain: "127.0.0.1", path: "/" }]);
  await page.goto(`${BASE}${path}`, { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(3000);
  try { await fn(page); } finally { await browser.close(); }
}

export const rowLoc = (page, subj) =>
  page.locator(".mail-scroll [role='button']", { hasText: subj }).first();
export const listText = (page) => page.locator(".mail-scroll").last().innerText();

// Wait until the list has stopped refetching (realtime arrival triggers
// several silent reloads; clicking mid-churn hits shifted rows).
export async function settleList(page, quietMs = 1500, capMs = 30000) {
  let last = Date.now();
  const done = () => Date.now() - last >= quietMs;
  const onResp = () => { last = Date.now(); };
  page.on("response", onResp);
  const start = Date.now();
  while (!done() && Date.now() - start < capMs) {
    await new Promise((r) => setTimeout(r, 200));
  }
  page.off("response", onResp);
}

// Wait until the row count is stable across two samples — arrivals may land
// long after the send returns (queue latency), so network quiet is not enough.
export async function stableList(page, capMs = 45000) {
  const count = () => page.locator(".mail-scroll [role='button']").count();
  const start = Date.now();
  let a = await count();
  for (;;) {
    await page.waitForTimeout(1500);
    const b = await count();
    if (a === b && Date.now() - start > 2500) return b;
    a = b;
    if (Date.now() - start > capMs) return a;
  }
}

export async function seed(subject) {
  const cookie = await apiLogin();
  const res = await fetch(`${BASE}/api/v1/mail/send`, {
    method: "POST",
    headers: { "Content-Type": "application/json", cookie: `${cookie.name}=${cookie.value}` },
    body: JSON.stringify({ to: [USER.email], cc: [], bcc: [], subject, body: "seed body" }),
  });
  if (!res.ok && res.status !== 204) throw new Error(`seed failed ${res.status}`);
}

export function check(label, ok, detail = "") {
  console.log(`${ok ? "PASS" : "FAIL"}  ${label}${detail ? ` — ${detail}` : ""}`);
  return ok;
}
