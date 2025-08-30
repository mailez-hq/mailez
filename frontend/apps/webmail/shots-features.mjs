// Feature screenshot generator: drives the real webmail dev server (which
// proxies to the live community Docker stack) and captures the conversation
// view + quoted-reply fold shots for the website.
// Usage: node shots-features.mjs   (webmail :3001, stack backend :8081)
import { chromium } from "@playwright/test";
import { mkdirSync } from "node:fs";

// Four levels up from frontend/apps/webmail lands on mailez-hq, where the
// website repo lives beside the product repo.
const OUT = "../../../../mailez-website/static/img/shots";
mkdirSync(OUT, { recursive: true });

async function shot(page, name) {
  await page.waitForTimeout(900);
  await page.screenshot({ path: `${OUT}/${name}.jpg`, type: "jpeg", quality: 88 });
  console.log("shot:", name);
}

const browser = await chromium.launch();
const ctx = await browser.newContext({
  viewport: { width: 1440, height: 900 },
  deviceScaleFactor: 2,
  locale: "zh-CN",
});
const page = await ctx.newPage();

await page.goto("http://localhost:3001/");
await page.waitForTimeout(2000);
// Switch the UI to Chinese (the marketing site's primary language).
const zh = page.getByText("中文", { exact: true }).first();
if (await zh.isVisible().catch(() => false)) {
  await zh.click();
  await page.waitForTimeout(800);
}
await page.getByLabel(/账号|Account/).fill("admin@example.com");
await page.getByLabel(/密码|Password/).fill("MailezDemo2026!");
await page.getByRole("button", { name: /登录|Sign in/ }).click();
await page.waitForTimeout(3000);

// --- Conversation view: the Q3 planning thread (grouped list + timeline) ---
await page.goto("http://localhost:3001/mail/Inbox");
await page.waitForTimeout(2500);
await shot(page, "13-conversation-list");

const thread = page.getByText(/Q3 产品规划讨论/).first();
await thread.click({ timeout: 15000 });
await page.waitForTimeout(2000);
await shot(page, "14-conversation-thread");

// --- Quoted-text fold: collapsed gradient block with an expand affordance ---
await page.goto("http://localhost:3001/mail/Inbox");
await page.waitForTimeout(2200);
const quoted = page.getByText(/项目周报（第 35 周）|引用示例/).first();
await quoted.click({ timeout: 15000 });
await page.waitForTimeout(2200);
await shot(page, "15-quote-folded");

// Expand the quote for the before/after pair.
const expander = page.getByRole("button", { name: /展开引用|Expand quote/i }).first();
if (await expander.isVisible().catch(() => false)) {
  await expander.click();
  await page.waitForTimeout(1200);
  await shot(page, "16-quote-expanded");
} else {
  console.log("expander not visible; skipped 16");
}

await ctx.close();
await browser.close();
console.log("done");
