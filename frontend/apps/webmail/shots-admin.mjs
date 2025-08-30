import { chromium } from "@playwright/test";
import { mkdirSync } from "node:fs";

const OUT = "../../../mailez-website/static/img/shots";
mkdirSync(OUT, { recursive: true });

const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 2, locale: "zh-CN" });
const page = await ctx.newPage();

await page.goto("http://localhost:3002/");
await page.waitForTimeout(1500);
const zh = page.getByText("中文", { exact: true }).first();
if (await zh.isVisible().catch(() => false)) { await zh.click(); await page.waitForTimeout(600); }
await page.getByLabel(/邮箱|Email/).fill("admin@example.com");
await page.getByLabel(/密码|Password/).fill("MailezDemo2026!");
await page.getByRole("button", { name: /登录|Sign in/ }).click();
await page.waitForTimeout(3000);
await page.screenshot({ path: `${OUT}/07-admin-overview.jpg`, type: "jpeg", quality: 88 });
console.log("shot: 07-admin-overview | url:", page.url());

for (const [path, name] of [["/domains", "08-admin-domains"], ["/users", "09-admin-users"], ["/audit", "10-admin-audit"]]) {
  await page.goto(`http://localhost:3002${path}`);
  await page.waitForTimeout(2200);
  await page.screenshot({ path: `${OUT}/${name}.jpg`, type: "jpeg", quality: 88 });
  console.log("shot:", name);
}
await browser.close();
console.log("done");
