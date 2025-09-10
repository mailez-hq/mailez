import { chromium } from "playwright";
import { apiLogin } from "./helper.mjs";
import { mkdirSync } from "node:fs";

const BASE = "http://127.0.0.1:8083";
const ADMIN = "http://127.0.0.1:8082";
const OUT = "shots-theme";
mkdirSync(OUT, { recursive: true });

const cookie = await apiLogin();

const browser = await chromium.launch();
for (const mode of ["light", "dark"]) {
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: mode });
  const page = await ctx.newPage();
  await page.context().addCookies([{ name: cookie.name, value: cookie.value, domain: "127.0.0.1", path: "/" }]);
  const tag = mode;

  // Landing / login
  await page.goto(`${BASE}/`, { waitUntil: "networkidle" }).catch(() => {});
  await page.waitForTimeout(1500);
  await page.screenshot({ path: `${OUT}/${tag}-landing.png` });

  // Mailbox: list + reader
  await page.goto(`${BASE}/mail/Inbox`, { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(6000);
  const row = page.locator(".mail-scroll [role='button']").first();
  if (await row.count()) {
    await row.click().catch(() => {});
    await page.waitForTimeout(3500);
  }
  await page.screenshot({ path: `${OUT}/${tag}-mailbox.png` });

  // Admin
  await page.goto(`${ADMIN}/`, { waitUntil: "networkidle" }).catch(() => {});
  await page.waitForTimeout(2000);
  await page.screenshot({ path: `${OUT}/${tag}-admin.png` });

  await ctx.close();
}
await browser.close();
console.log("done");
