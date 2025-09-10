import { chromium } from "playwright";
import { apiLogin } from "./helper.mjs";

const BASE = "http://127.0.0.1:8083";
const cookie = await apiLogin();
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
await page.context().addCookies([{ name: cookie.name, value: cookie.value, domain: "127.0.0.1", path: "/" }]);
await page.goto(`${BASE}/mail/Inbox`, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(4000);
await page.evaluate(() => {
  const raw = localStorage.getItem("mailez.prefs");
  const p = raw ? JSON.parse(raw) : {};
  p.listPreview = false;
  localStorage.setItem("mailez.prefs", JSON.stringify(p));
});
await page.reload({ waitUntil: "domcontentloaded" });
await page.waitForTimeout(6000);
const stored = await page.evaluate(() => JSON.parse(localStorage.getItem("mailez.prefs")));
console.log("stored listPreview:", stored.listPreview, "density:", stored.density);
const row = page.locator(".mail-scroll [role='button']").first();
console.log("row height:", await row.evaluate((el) => el.getBoundingClientRect().height));
await browser.close();
