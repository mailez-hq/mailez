import { chromium } from "playwright";
import { apiLogin } from "./helper.mjs";

const BASE = "http://127.0.0.1:8083";
const cookie = await apiLogin();
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
await page.context().addCookies([{ name: cookie.name, value: cookie.value, domain: "127.0.0.1", path: "/" }]);
await page.goto(`${BASE}/mail/Inbox`, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(6000);

const row = page.locator(".mail-scroll [role='button']").first();
const h = await row.evaluate((el) => el.getBoundingClientRect().height);
const text = (await row.innerText()).replace(/\n/g, " | ").slice(0, 120);
console.log("row height (preview on):", h);
console.log("row text:", text);

// settings toggle: turn previews off
await page.goto(`${BASE}/mail/Inbox`, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(2000);
await page.evaluate(() => {
  const raw = localStorage.getItem("mailez.prefs");
  if (raw) localStorage.setItem("mailez.prefs", JSON.stringify({ ...JSON.parse(raw), listPreview: false }));
});
await page.reload({ waitUntil: "domcontentloaded" });
await page.waitForTimeout(6000);
const row2 = page.locator(".mail-scroll [role='button']").first();
const h2 = await row2.evaluate((el) => el.getBoundingClientRect().height);
console.log("row height (preview off):", h2);

await browser.close();
console.log("done");
