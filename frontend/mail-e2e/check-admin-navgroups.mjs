import { chromium } from "playwright";
import { apiLogin } from "./helper.mjs";

const B = "http://127.0.0.1:18082";
const c = await apiLogin();
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
await page.context().addCookies([{ name: c.name, value: c.value, domain: "127.0.0.1", path: "/" }]);
await page.goto(`${B}/overview`, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(4000);

const nav = await page.locator("aside").innerText();
const lines = nav.split("\n").map((s) => s.trim()).filter(Boolean);
console.log("=== nav order ===");
console.log(lines.join(" | "));

const expect = [
  "Overview & Monitoring", "Overview", "Health", "Traffic",
  "Organization", "Domains", "Users", "Groups", "Aliases",
  "Mail Services", "Relays", "Fetches", "Tokens", "Anon aliases",
  "Compliance & Audit", "Approval / DLP", "Archive", "Audit",
  "System", "Announcement", "Config",
];
let ok = true;
for (const k of expect) {
  if (!nav.includes(k)) { console.log("MISSING:", k); ok = false; }
}
// Order sanity: first group label before Domains, Compliance before Audit, System last-ish
const idx = (s) => lines.findIndex((l) => l === s || l.startsWith(s));
const ordered =
  idx("Overview & Monitoring") < idx("Overview") &&
  idx("Organization") < idx("Domains") &&
  idx("Mail Services") < idx("Relays") &&
  idx("Compliance & Audit") < idx("Approval / DLP") &&
  idx("System") < idx("Announcement") &&
  idx("Announcement") < idx("Config");
console.log(ok && ordered ? "PASS nav grouped + ordered" : `FAIL (labels=${ok} order=${ordered})`);

await page.screenshot({ path: "mail-e2e/artifacts/admin-nav-groups.png" });
await browser.close();
process.exit(ok && ordered ? 0 : 1);
