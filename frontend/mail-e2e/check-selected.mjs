import { chromium } from "playwright";
import { apiLogin } from "./helper.mjs";

const BASE = "http://127.0.0.1:8083";
const cookie = await apiLogin();
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
await page.context().addCookies([{ name: cookie.name, value: cookie.value, domain: "127.0.0.1", path: "/" }]);
await page.goto(`${BASE}/mail/Inbox`, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(6000);

// Open the first row (selected state), sample backgrounds of row 1 vs row 2.
const row = page.locator(".mail-scroll [role='button']").first();
await row.click();
await page.waitForTimeout(2500);

const sample = async (idx) =>
  page.locator(".mail-scroll [role='button']").nth(idx).evaluate((el) => {
    // Walk up to the row wrapper carrying the bg (the [role=button] root).
    const target = el;
    return getComputedStyle(target).backgroundColor;
  });

const sel = await sample(0);
const plain = await sample(1);
console.log("selected bg:", sel, "| plain bg:", plain);

// Bulk-select row 2 via its checkbox and sample it too.
await page.locator(".mail-scroll [role='button']").nth(1).locator("input[type=checkbox]").first().dispatchEvent("click");
await page.waitForTimeout(800);
const bulk = await sample(1);
console.log("bulk-checked bg:", bulk);

// Pixel-delta sanity: selected tint must differ from plain by a real margin.
const rgb = (s) => s.match(/\d+/g).slice(0, 3).map(Number);
const [r1, g1, b1] = rgb(sel);
const [r2, g2, b2] = rgb(plain);
const delta = Math.abs(r1 - r2) + Math.abs(g1 - g2) + Math.abs(b1 - b2);
console.log("channel delta selected-vs-plain:", delta);

await browser.close();
