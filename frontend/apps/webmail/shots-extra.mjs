import { chromium } from "@playwright/test";

const OUT = "../../../mailez-website/static/img/shots";
const browser = await chromium.launch();

// webmail compose panel with a filled-in draft
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 2, locale: "zh-CN" });
const page = await ctx.newPage();
await page.route(/\/api\/v1\/mail\//, (route) => {
  const url = route.request().url();
  if (url.includes("/mail/folders")) return route.fulfill({ json: ["Inbox", "Sent", "Drafts", "Trash", "Archive", "Junk"] });
  if (url.includes("/mail/messages")) return route.fulfill({ json: [], headers: { "X-Total-Messages": "0" } });
  return route.fulfill({ json: [] });
});
await page.goto("http://localhost:3001/");
const zh = page.getByText("中文", { exact: true }).first();
if (await zh.isVisible().catch(() => false)) { await zh.click(); await page.waitForTimeout(600); }
await page.getByLabel(/账号|Account/).fill("chen");
await page.getByLabel(/密码|Password/).fill("ChenDemo2026!");
await page.getByRole("button", { name: /登录|Sign in/ }).click();
await page.waitForTimeout(2500);
await page.getByRole("button", { name: "写邮件" }).first().click();
await page.waitForTimeout(1200);
await page.locator("input, textarea").filter({ hasText: "" }).first();
// fill the draft so the panel reads naturally (recipient input sits inside
// the panel with placeholder "user@example.com"; subject follows the label)
const to = page.getByPlaceholder("user@example.com").first();
await to.fill("wangfeng@huadong-gov.cn");
const subj = page.getByPlaceholder(/主题|Subject|无主题/).first();
if (await subj.isVisible().catch(() => false)) await subj.fill("Re: 二期验收材料——等保清单已整理");
const editor = page.locator(".tiptap, [contenteditable=true]").first();
if (await editor.isVisible().catch(() => false)) {
  await editor.click();
  await editor.type("王峰你好：等保三级材料清单已整理好，见附件。引擎切换灰度方案周五前同步。");
}
await page.waitForTimeout(900);
await page.screenshot({ path: `${OUT}/11-compose.jpg`, type: "jpeg", quality: 88 });
console.log("shot: 11-compose");
await ctx.close();

// admin aliases
const actx = await browser.newContext({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 2, locale: "zh-CN" });
const ap = await actx.newPage();
await ap.goto("http://localhost:3002/");
await ap.waitForTimeout(1200);
const azh = ap.getByText("中文", { exact: true }).first();
if (await azh.isVisible().catch(() => false)) { await azh.click(); await ap.waitForTimeout(600); }
await ap.getByLabel(/邮箱|Email/).fill("admin@example.com");
await ap.getByLabel(/密码|Password/).fill("MailezDemo2026!");
await ap.getByRole("button", { name: /登录|Sign in/ }).click();
await ap.waitForTimeout(2500);
await ap.goto("http://localhost:3002/aliases");
await ap.waitForTimeout(2000);
await ap.screenshot({ path: `${OUT}/12-admin-aliases.jpg`, type: "jpeg", quality: 88 });
console.log("shot: 12-admin-aliases");
await actx.close();
await browser.close();
console.log("done");
