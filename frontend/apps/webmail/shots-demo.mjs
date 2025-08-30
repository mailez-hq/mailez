// Marketing screenshot generator: drives the real webmail/admin dev servers
// with a zh-CN context, mocks only the mail store (no engine containers on
// this machine), and captures the hero shots for the website.
// Usage: node shots-demo.mjs  (webmail :3001, admin :3002, backend :8080)
import { chromium } from "@playwright/test";
import { mkdirSync } from "node:fs";

const OUT = "../../../mailez-website/static/img/shots";
mkdirSync(OUT, { recursive: true });

const zhList = [
  {
    uid: 104, id: "m104", seq: 4,
    subject: "Re: 邮件系统二期验收时间",
    from: [{ name: "王峰", email: "wangfeng@huadong-gov.cn" }],
    to: [{ name: "陈静", email: "chen@example.com" }],
    date: "2026-08-30T09:12:00+08:00",
    flags: ["\\Unseen"], has_attachment: true,
    thread_id: "t-88", thread_count: 4, thread_latest: true,
    category: "work",
  },
  {
    uid: 103, id: "m103", seq: 3,
    subject: "询价函：邮件系统授权与部署服务",
    from: [{ name: "刘颖", email: "liuying@cvte-example.com.cn" }],
    to: [{ name: "陈静", email: "chen@example.com" }],
    date: "2026-08-30T08:47:00+08:00",
    flags: ["\\Unseen"], has_attachment: true, category: "work",
  },
  {
    uid: 102, id: "m102", seq: 2,
    subject: "本周项目周报（第 35 周）",
    from: [{ name: "项目管理", email: "pm@example.com" }],
    to: [{ name: "陈静", email: "chen@example.com" }],
    date: "2026-08-29T18:02:00+08:00",
    flags: [], has_attachment: false, category: "work",
  },
  {
    uid: 101, id: "m101", seq: 1,
    subject: "关于国庆假期值班安排的通知",
    from: [{ name: "人力资源部", email: "hr@example.com" }],
    to: [{ name: "全体同事", email: "all@example.com" }],
    date: "2026-08-29T16:30:00+08:00",
    flags: [], has_attachment: false, category: "work",
  },
  {
    uid: 100, id: "m100", seq: 0,
    subject: "设计稿确认：发布会主视觉",
    from: [{ name: "陈静", email: "chen@example.com" }],
    to: [{ name: "设计部", email: "design@example.com" }],
    date: "2026-08-29T11:20:00+08:00",
    flags: ["\\Flagged"], has_attachment: true, category: "work",
  },
  {
    uid: 99, id: "m99", seq: 5,
    subject: "InfoQ 精选：Go 高性能网关设计实战",
    from: [{ name: "InfoQ 编辑部", email: "infoq-editorial@subscribe.example" }],
    to: [{ name: "陈静", email: "chen@example.com" }],
    date: "2026-08-29T08:00:00+08:00",
    flags: ["\\Seen"], has_attachment: false, category: "newsletter",
  },
];

const detail = {
  ...zhList[0],
  to: [{ name: "陈静", email: "chen@example.com" }],
  cc: [{ name: "李文博", email: "liwenbo@mailez.dev" }],
  text_body: "陈静你好：\n\n二期验收初步定在 9 月中旬，需要贵方提供：等保三级材料、引擎切换灰度方案与 180 天审计日志留存证明。附件是会议纪要与验收清单，请查收。\n\n王峰",
  html_body: "<p>陈静你好：</p><p>二期验收初步定在 <b>9 月中旬</b>，需要贵方提供：等保三级材料、引擎切换灰度方案与 180 天审计日志留存证明。</p><p>附件是会议纪要与验收清单，请查收。</p><p style='color:#666'>王峰<br/>华东政务云项目负责人</p>",
  attachments: [
    { filename: "二期验收会议纪要.pdf", content_type: "application/pdf", size: 384_512 },
    { filename: "验收清单-引擎切换.xlsx", content_type: "application/vnd.ms-excel", size: 96_128 },
  ],
};

function mailRoute(route) {
  const url = route.request().url();
  if (url.includes("/mail/messages")) {
    return route.fulfill({ json: zhList, headers: { "X-Total-Messages": String(zhList.length) } });
  }
  if (url.includes("/mail/message?")) return route.fulfill({ json: detail });
  if (url.includes("/mail/folders")) return route.fulfill({ json: ["Inbox", "Sent", "Drafts", "Trash", "Archive", "Junk"] });
  if (url.includes("/mail/unseen")) return route.fulfill({ json: { Inbox: 2 } });
  if (url.includes("/mail/labels")) return route.fulfill({ json: [] });
  if (url.includes("/mail/snoozed")) return route.fulfill({ json: [] });
  if (url.includes("/mail/saved-searches")) return route.fulfill({ json: [] });
  if (url.includes("/mail/announcements")) return route.fulfill({ json: [] });
  return route.fulfill({ json: {} });
}

async function login(page, account, password) {
  await page.goto("http://localhost:3001/");
  const zh = page.getByText("中文", { exact: true }).first();
  if (await zh.isVisible().catch(() => false)) { await zh.click(); await page.waitForTimeout(600); }
  await page.getByLabel(/账号|Account/).fill(account);
  await page.getByLabel(/密码|Password/).fill(password);
  await page.getByRole("button", { name: /登录|Sign in/ }).click();
  await page.waitForTimeout(3000);
}

async function shot(page, name) {
  await page.waitForTimeout(900);
  await page.screenshot({ path: `${OUT}/${name}.jpg`, type: "jpeg", quality: 88 });
  console.log("shot:", name);
}

const browser = await chromium.launch();

// ---------- webmail (chen) with mocked mail store ----------
const ctx = await browser.newContext({
  viewport: { width: 1440, height: 900 },
  deviceScaleFactor: 2,
  locale: "zh-CN",
});
const page = await ctx.newPage();
await page.route(/\/api\/v1\/mail\//, mailRoute);

await page.goto("http://localhost:3001/");
await page.waitForTimeout(2500);
await shot(page, "01-login");

await login(page, "chen", "ChenDemo2026!");
await shot(page, "01b-home");
await page.goto("http://localhost:3001/mail/INBOX");
await page.waitForTimeout(2500);
await shot(page, "02-inbox");

const first = page.getByText(zhList[0].subject).first();
await first.click({ timeout: 15000 });
await page.waitForTimeout(1800);
await shot(page, "03-reading");

// calendar: real seeded data
await page.getByRole("button", { name: /日历|Calendar/ }).first().click();
await page.waitForTimeout(2000);
await shot(page, "04-calendar");
await page.keyboard.press("Escape");

await page.getByRole("button", { name: /通讯录|Contacts/ }).first().click();
await page.waitForTimeout(1600);
await shot(page, "05-contacts");
await page.keyboard.press("Escape");

await page.getByRole("button", { name: /云盘|Drive/ }).first().click();
await page.waitForTimeout(1800);
await shot(page, "06-drive");

await ctx.close();

// ---------- admin (admin@example.com), real data ----------
const actx = await browser.newContext({
  viewport: { width: 1440, height: 900 },
  deviceScaleFactor: 2,
  locale: "zh-CN",
});
const ap = await actx.newPage();
await ap.goto("http://localhost:3002/");
await ap.waitForTimeout(2000);
await ap.getByLabel(/账号|Account/).fill("admin");
await ap.getByLabel(/密码|Password/).fill("MailezDemo2026!");
await ap.getByRole("button", { name: /登录|Sign in/ }).click();
await ap.waitForTimeout(2500);
await shot(ap, "07-admin-overview");

await ap.goto("http://localhost:3002/domains");
await ap.waitForTimeout(1800);
await shot(ap, "08-admin-domains");

await ap.goto("http://localhost:3002/audit");
await ap.waitForTimeout(1800);
await shot(ap, "09-admin-audit");

await actx.close();
await browser.close();
console.log("done");
