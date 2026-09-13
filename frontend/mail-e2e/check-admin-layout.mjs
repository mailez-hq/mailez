import { chromium } from "playwright";
import { apiLogin } from "./helper.mjs";

const B = "http://127.0.0.1:18082/admin"; // basePath /admin
const c = await apiLogin();
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, locale: "zh-CN" });
await page.context().addCookies([{ name: c.name, value: c.value, domain: "127.0.0.1", path: "/" }]);
await page.goto(`${B}/users`, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(6000);

const m = await page.evaluate(() => {
  const aside = document.querySelector("aside");
  const main = document.querySelector("main");
  const footer = aside?.lastElementChild;
  if (!aside || !main || !footer) return { found: false };
  const fr = footer.getBoundingClientRect();
  return {
    found: true,
    viewportH: window.innerHeight,
    asideH: aside.getBoundingClientRect().height,
    footerBottom: Math.round(fr.bottom),
    footerVisible: fr.bottom <= window.innerHeight + 1,
    mainScrollable: main.scrollHeight > main.clientHeight,
    docScrollH: document.documentElement.scrollHeight,
  };
});
console.log(JSON.stringify(m, null, 1));
console.log(m.found && m.footerVisible && m.mainScrollable && m.asideH <= m.viewportH + 1 ? "PASS: sidebar footer visible, main scrolls internally" : "FAIL");
await browser.close();
