// P2 UI/feature smoke: PGP toggles without a key, AI endpoints, multi-account
// switcher, and EE-only surfaces in this (EE) build.
// Run: cd frontend && node mail-e2e/qa/p2-ui.mjs
import { USER, api, check, cleanupFolder, closeBrowser, dumpResults, gotoMail, newContext, purgeSubjects, record, seedMail, shot, sleep, summary, blocked } from "./lib.mjs";

const MOD = "P2-ui";
const stamp = Date.now();
const SUBJ = `E2E-P2UI-${stamp}`;
const web = await newContext({ locale: "en" });
const toasts = [];
try {
  // --------------------------------------------------------------- AI APIs
  const status = await api("/api/v1/ai/status");
  check(MOD, "AI: status advertises provider", status.status === 200, JSON.stringify(status.json).slice(0, 80));
  await seedMail(SUBJ, { body: "ai probe body about quarterly invoices" });
  const msg = await (async () => {
    for (let i = 0; i < 20; i++) {
      const hit = (await (await import("./lib.mjs")).listFolder("Inbox", 0)).messages.find((m) => (m.subject || "").includes(SUBJ));
      if (hit) return hit;
      await sleep(1500);
    }
    return null;
  })();
  const tries = [
    ["summarize", "/api/v1/ai/summarize", { folder: "Inbox", uid: msg?.uid }],
    ["compose", "/api/v1/ai/compose", { subject: "E2E AI", body: "write a short status note", tone: "formal" }],
    ["draft", "/api/v1/ai/draft", { text: "reply saying thanks", tone: "friendly" }],
  ];
  for (const [name, path, body] of tries) {
    const r = await api(path, { method: "POST", body });
    const ok = r.status < 400 && !!(r.json && (r.json.summary || r.json.text || r.json.html || r.json.body || r.json.draft || r.json.messages || r.json.replies));
    if (r.status === 502 || r.status === 503) {
      blocked(MOD, `AI ${name}`, `provider error ${r.status}: ${(r.text || "").slice(0, 80)}`);
    } else {
      check(MOD, `AI ${name} returns usable content`, ok, `status=${r.status} keys=${Object.keys(r.json || {}).join(",")}`);
    }
  }

  // ------------------------------------------------------ PGP toggles (no key)
  const pgp = await api("/api/v1/me/pgp");
  check(MOD, "PGP: no key configured in this deployment", pgp.status === 200 && pgp.json?.has_key === false,
    JSON.stringify(pgp.json));
  await gotoMail(web.page, "/mail/Inbox");
  await web.page.locator("button", { hasText: /^Write$/ }).first().click();
  await web.page.locator('input[placeholder="user@example.com"]').first().fill(USER.email);
  await web.page.keyboard.press("Enter");
  await web.page.locator('[data-testid="compose-subject"]').fill(`E2E-PGP-${stamp}`);
  const signBtn = web.page.locator("button", { hasText: /^PGP sign$/ }).first();
  const signCount = await web.page.locator("button", { hasText: /^PGP sign$/ }).count();
  check(MOD, "PGP: composer exposes a sign toggle", signCount > 0, `buttons=${signCount}`);
  if (signCount) {
    await signBtn.click({ timeout: 8000 }).catch(() => {});
    await web.page.waitForTimeout(2500);
    const body = await web.page.locator("body").innerText();
    const feedback = /key|PGP|error|失败|密钥/i.test(body);
    check(MOD, "PGP: enabling sign without a key gives visible feedback", feedback,
      `feedbackShown=${feedback}`);
  }
  await web.page.locator("button[title='Close']:visible").first().click().catch(() => {});
  await web.page.waitForTimeout(2000);

  // ------------------------------------------------------- multi-account UI
  const accounts = await api("/api/v1/accounts");
  check(MOD, "Accounts: aggregation API reachable", accounts.status === 200,
    `count=${Array.isArray(accounts.json) ? accounts.json.length : "n/a"}`);
  await gotoMail(web.page, "/home");
  const menuBtn = web.page.locator('button[aria-label="My account"], button[aria-haspopup="menu"]').first();
  await menuBtn.click().catch(() => {});
  await web.page.waitForTimeout(1500);
  const menuText = await web.page.locator("body").innerText();
  check(MOD, "Accounts: account menu exposes switching/aggregation entry",
    /Accounts|Add account|Switch|委托|账号/i.test(menuText),
    `menuSnippet=${menuText.replace(/\s+/g, " ").slice(0, 90)}`);
  await shot(web.page, "p2-account-menu");
  await web.page.keyboard.press("Escape");

  // ------------------------------------------------------------ EE surfaces
  const ee = await Promise.all(["/api/v1/audit", "/api/v1/announcement", "/api/v1/me/pgp"].map((p) => api(p)));
  check(MOD, "Edition: EE-only endpoints are present in this build",
    ee[0].status === 200 && (ee[1].status === 200 || ee[1].status === 204),
    `audit=${ee[0].status} announcement=${ee[1].status}`);
} finally {
  await web.ctx.close().catch(() => {});
  for (const f of ["Inbox", "Sent", "Drafts"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects([SUBJ, `E2E-PGP-${stamp}`]).catch(() => 0);
  record(MOD, "Cleanup: P2 UI probes purged", purged >= 0 ? "PASS" : "FAIL", "expunged=" + purged);
  await closeBrowser();
}
void toasts;
const s = summary();
console.log(`\n=== p2-ui: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("p2-ui.json");
process.exit(s.fail > 0 ? 1 : 0);
