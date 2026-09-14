import { chromium } from "playwright";
import { apiLogin } from "./helper.mjs";

const B = "http://127.0.0.1:18082/admin"; // basePath /admin
const c = await apiLogin();
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
await page.context().addCookies([{ name: c.name, value: c.value, domain: "127.0.0.1", path: "/" }]);

const results = [];
const check = (name, ok) => { results.push(`${ok ? "PASS" : "FAIL"}  ${name}`); };

// 1. health page renders checks (admin UI defaults to English)
await page.goto(`${B}/health`, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(6000);
const healthText = await page.locator("main").innerText();
check("health page: title + domain card", /Health checks|健康体检/.test(healthText) && healthText.includes("example.com"));
check("health page: check rows (MX/SPF/DMARC)", ["MX", "SPF", "DMARC"].every((k) => healthText.includes(k)));
check("health page: system section", /System checks|系统检查/.test(healthText) && healthText.includes("Redis"));

// 2. domains page DNS wizard dialog
await page.goto(`${B}/domains`, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(4000);
await page.getByRole("button", { name: "DNS", exact: true }).first().click();
await page.waitForTimeout(4000);
const wizText = await page.locator("[role='dialog']").innerText().catch(() => "");
check("wizard dialog: records table", wizText.includes("MX") && wizText.includes("mail.example.com"));
check("wizard dialog: status badges", /Not configured|已生效|Active/.test(wizText));

// 3. config security tab (TOTP card)
await page.goto(`${B}/config`, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(3000);
await page.getByRole("tab", { name: /Security|安全/ }).click();
await page.waitForTimeout(3000);
const secText = await page.locator("main").innerText();
check("security tab: TOTP card + secret", /TOTP|两步验证/.test(secText) && /Secret|密钥/.test(secText));
check("security tab: state Disabled", /Disabled|未启用/.test(secText));

// 4. reports page (may be empty until samples accumulate)
await page.goto(`${B}/reports`, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(4000);
const repText = await page.locator("main").innerText();
check("reports page renders", /Traffic report|流量报表/.test(repText));

// 5. nav items present
const nav = await page.locator("aside").innerText();
check("nav: Health + Traffic entries", /Health|健康体检/.test(nav) && /Traffic|流量报表/.test(nav));

await page.screenshot({ path: "mail-e2e/artifacts/ops-features.png", fullPage: false });
console.log(results.join("\n"));
await browser.close();
process.exit(results.some((r) => r.startsWith("FAIL")) ? 1 : 0);
