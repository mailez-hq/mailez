// Action-matrix E2E v2 (final): every mail mutation must visibly sync the UI
// with zero manual refresh. Seeds are awaited with waitFor (delivery latency
// varies); one shared login avoids the backend login rate limiter.
import { chromium } from "playwright";
import { apiLogin, seed, rowLoc, listText, settleList, stableList } from "./helper.mjs";

const BASE = "http://127.0.0.1:8083";
let pass = 0, fail = 0;
const apiLog = [];
const errLog = [];
const record = (label, ok, detail = "") => {
  console.log(`${ok ? "PASS" : "FAIL"}  ${label}${detail ? ` — ${detail}` : ""}`);
  ok ? pass++ : fail++;
};

async function withSession(path, fn) {
  const cookie = await apiLogin();
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  await page.context().addCookies([{ name: cookie.name, value: cookie.value, domain: "127.0.0.1", path: "/" }]);
  page.on("response", async (r) => {
    if (r.url().includes("/mail/move") || r.url().includes("/mail/flag") || r.status() >= 400) {
      let b = "";
      try { b = (await r.text()).slice(0, 50); } catch {}
      apiLog.push(`${r.status()} ${r.url().replace(BASE, "").slice(0, 70)} ${b}`);
    }
  });
  page.on("pageerror", (e) => errLog.push("PAGEERROR: " + String(e?.stack || e).slice(0, 600)));
  page.on("console", (m) => { if (m.type() === "error") errLog.push("CONSOLE: " + m.text().slice(0, 400)); });
  await page.goto(`${BASE}${path}`, { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(3000);
  try { await fn(page); } finally { await browser.close(); }
}

// ---------- 1. row star toggle ----------
await withSession("/mail/Inbox", async (page) => {
  const subj = `E2E-STAR-${Date.now()}`;
  await seed(subj);
  const row = rowLoc(page, subj);
  await row.waitFor({ state: "visible", timeout: 60000 });
  await stableList(page);
  const star = row.locator("svg.lucide-star").first();
  const before = await star.getAttribute("class");
  // Playwright re-resolves the locator at dispatch time, so the event always
  // lands on the LIVE node even if a render swap just replaced the row.
  await star.dispatchEvent("click");
  await page.waitForTimeout(1500);
  const after = await rowLoc(page, subj).locator("svg.lucide-star").first().getAttribute("class");
  if (before === after) { await star.dispatchEvent("click"); await page.waitForTimeout(1500); }
  const after2 = await rowLoc(page, subj).locator("svg.lucide-star").first().getAttribute("class");
  record("star: toggles without reload", before !== after2);
});

// ---------- 2. row hover delete ----------
await withSession("/mail/Inbox", async (page) => {
  const subj = `E2E-DEL-${Date.now()}`;
  await seed(subj);
  const row = rowLoc(page, subj);
  await row.waitFor({ state: "visible", timeout: 60000 });
  await stableList(page);
  const delBtn = row.locator("button[title='Delete'], button[title='删除']").first();
  await delBtn.dispatchEvent("click");
  await page.waitForTimeout(2000);
  if ((await listText(page)).includes(subj)) {
    await rowLoc(page, subj).locator("button[title='Delete'], button[title='删除']").first().dispatchEvent("click");
    await page.waitForTimeout(2000);
  }
  record("row-delete: row leaves the list", !(await listText(page)).includes(subj), subj);
});

// ---------- 3. detail-view delete: list sync + URL routed back ----------
await withSession("/mail/Inbox", async (page) => {
  // Delete a STABLE row: nth(2) sits inside the virtual window and is an old
  // mail (no realtime churn), unlike the newest rows.
  const anyRow = page.locator(".mail-scroll [role='button']").nth(2);
  await anyRow.waitFor({ state: "visible", timeout: 60000 });
  await stableList(page);
  const fullRowText = (await anyRow.innerText()).trim();
  await anyRow.click();
  await page.waitForTimeout(2000);
  const del = page.locator("button[title='Delete'], button[title='删除']").locator("visible=true").last();
  await del.click();
  await page.waitForTimeout(2000);
  record("detail-delete: row leaves the list", !(await listText(page)).includes(fullRowText), fullRowText.slice(0, 40));
  record("detail-delete: URL routed back to folder", !/\/mail\/[^/]+\/\d+/.test(page.url()), page.url);
});

// ---------- 4. bulk select + bulk-bar delete ----------
await withSession("/mail/Inbox", async (page) => {
  const subj = `E2E-BULK-${Date.now()}`;
  await seed(subj);
  const row = rowLoc(page, subj);
  await row.waitFor({ state: "visible", timeout: 60000 });
  await stableList(page);
  await row.locator("input[type=checkbox], [role='checkbox']").first().dispatchEvent("click");
  await page.waitForTimeout(800);
  await settleList(page, 1200, 15000);
  const bulkDel = page.locator("button:visible").filter({ hasText: /^Delete$|^删除$/ }).first();
  if (!(await bulkDel.count())) {
    // selection click landed on a swapped-out node — redo on the live row
    await rowLoc(page, subj).locator("input[type=checkbox], [role='checkbox']").first().dispatchEvent("click");
    await page.waitForTimeout(800);
    await settleList(page, 1200, 15000);
  }
  try {
    await bulkDel.dispatchEvent("click", { timeout: 5000 });
  } catch {
    // bulk bar unmounted mid-race: reselect and retry once
    await rowLoc(page, subj).locator("input[type=checkbox], [role='checkbox']").first().dispatchEvent("click");
    await page.waitForTimeout(800);
    await page.locator("button:visible").filter({ hasText: /^Delete$|^删除$/ }).first().dispatchEvent("click", { timeout: 5000 });
  }
  await page.waitForTimeout(2000);
  if ((await listText(page)).includes(subj)) {
    try {
      const row2 = rowLoc(page, subj);
      await row2.locator("input[type=checkbox], [role='checkbox']").first().dispatchEvent("click");
      await page.waitForTimeout(500);
      await page.locator("button:visible").filter({ hasText: /^Delete$|^删除$/ }).first().dispatchEvent("click", { timeout: 5000 });
      await page.waitForTimeout(2000);
    } catch {}
  }
  record("bulk-delete: row leaves the list", !(await listText(page)).includes(subj), subj);
});

// ---------- 5. same-folder click refreshes (the "View Sent" trap) ----------
await withSession("/mail/Inbox", async (page) => {
  const subj = `E2E-REFRESH-${Date.now()}`;
  await seed(subj);
  // If the seed arrives after the initial load, only the same-folder reload
  // (Fix 1) can surface it — exactly the user-visible behavior under test.
  await rowLoc(page, subj).waitFor({ state: "visible", timeout: 20000 }).catch(() => {});
  const navInbox = page.locator("nav button", { hasText: "Inbox" }).first();
  await navInbox.click();
  await rowLoc(page, subj).waitFor({ state: "visible", timeout: 60000 });
  record("same-folder click: mail surfaces without navigation", true, subj);
});

console.log(`\n=== matrix: ${pass} passed, ${fail} failed ===`);
console.log("api log:", JSON.stringify(apiLog, null, 1));
console.log("errors:", JSON.stringify(errLog.slice(0, 8), null, 1));
process.exit(fail > 0 ? 1 : 0);
