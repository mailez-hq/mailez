import { chromium } from "playwright";
import { apiLogin } from "./helper.mjs";

const BASE = "http://127.0.0.1:8083";
const cookie = await apiLogin();
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
await page.context().addCookies([{ name: cookie.name, value: cookie.value, domain: "127.0.0.1", path: "/" }]);
await page.goto(`${BASE}/home`, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(6000);

// Sample the active nav tint under the default theme and again after
// switching the accent family, proving the highlight follows the theme.
const sample = () =>
  page.evaluate(() => {
    const els = [...document.querySelectorAll("aside a, nav a, aside button, nav button")];
    const active = els.find((b) => b.className.includes("inset_2px"));
    if (!active) return { found: false };
    return {
      found: true,
      bg: getComputedStyle(active).backgroundColor,
      color: getComputedStyle(active).color,
    };
  });

console.log("default:", JSON.stringify(await sample()));
await page.evaluate(() => document.documentElement.setAttribute("data-accent", "green"));
await page.waitForTimeout(300);
console.log("green:  ", JSON.stringify(await sample()));
await page.evaluate(() => document.documentElement.setAttribute("data-accent", "purple"));
await page.waitForTimeout(300);
console.log("purple: ", JSON.stringify(await sample()));

await browser.close();
