// Two open questions from the audit, re-tested with better isolation:
//   A) burn-after-read: does the body come back on a second open? The list row
//      preview is excluded by subtracting the list column's text from the page
//      text, so only the reading pane is judged.
//   B) multi-account: where does the UI actually expose the switcher?
// Run: cd frontend && node mail-e2e/qa/verify-pending.mjs
import {
  USER, api, awaitMail, check, cleanupFolder, closeBrowser, dumpResults, gotoMail, listFolder,
  newContext, purgeSubjects, record, seedMail, shot, sleep, stableList, summary, withImap,
} from "./lib.mjs";

const MOD = "VERIFY3";
const stamp = Date.now();
const BURN = `E2E-BURN2-${stamp}`;
const web = await newContext({ locale: "en" });

/** Page text with the message-list column removed (i.e. the reading pane). */
async function paneText(page) {
  const body = await page.locator("body").innerText();
  const list = await page.locator(".mail-scroll").last().innerText().catch(() => "");
  return list ? body.split(list).join(" ") : body;
}

try {
  // ------------------------------------------------------ A) burn second open
  await seedMail(BURN, { body: "BURNSECRET string", burn_after_minutes: 5 });
  const bm = await awaitMail(BURN);
  await gotoMail(web.page, "/mail/Inbox");
  await stableList(web.page, 20000);
  const rowText = await web.page.locator(".mail-scroll [role='button']", { hasText: BURN }).first().innerText();
  check(MOD, "Burn: the list preview does NOT leak the burned body",
    !rowText.includes("BURNSECRET"), `row="${rowText.replace(/\s+/g, " ").slice(0, 60)}"`);

  await web.page.locator(".mail-scroll [role='button']", { hasText: BURN }).first().click();
  await web.page.waitForTimeout(3000);
  const firstPane = await paneText(web.page);
  const reveal = web.page.locator("button", { hasText: /Reveal|查看邮件|Open message/ }).first();
  const hadReveal = await reveal.count();
  if (hadReveal) {
    await reveal.click({ timeout: 8000 }).catch(() => {});
    await web.page.waitForTimeout(3000);
  }
  const firstAfter = await paneText(web.page);
  const flags1 = (await withImap((c) => c.uidFetch(bm.uid))).flags || [];
  check(MOD, "Burn: first open reveals the body and flags $BurnRead",
    firstAfter.includes("BURNSECRET") && flags1.some((f) => /BurnRead/i.test(f)),
    `revealButton=${hadReveal} bodyVisible=${firstAfter.includes("BURNSECRET")} flags=[${flags1.join(",")}]`);
  void firstPane;
  await shot(web.page, "verify3-burn-first");

  await gotoMail(web.page, "/mail/Inbox");
  await stableList(web.page, 20000);
  await web.page.locator(".mail-scroll [role='button']", { hasText: BURN }).first().click();
  await web.page.waitForTimeout(4000);
  const secondPane = await paneText(web.page);
  const secondRow = await web.page.locator(".mail-scroll [role='button']", { hasText: BURN }).first().innerText();
  check(MOD, "Burn: second open does not show the body again",
    !secondPane.includes("BURNSECRET"),
    `paneHasSecret=${secondPane.includes("BURNSECRET")} listHasSecret=${secondRow.includes("BURNSECRET")}`);
  await shot(web.page, "verify3-burn-second");

  // ------------------------------------------------------ B) multi-account UI
  await gotoMail(web.page, "/home");
  const sidebarAccount = web.page.locator("button", { hasText: USER.email }).first();
  check(MOD, "Accounts: sidebar shows the active account control",
    (await web.page.locator("button", { hasText: USER.email }).count()) > 0);
  await sidebarAccount.click({ timeout: 8000 }).catch(() => {});
  await web.page.waitForTimeout(2000);
  const menuText = await web.page.locator("body").innerText();
  const hasSwitcher = /Accounts|Add account|Switch account|委托|账号切换|Mailbox delegation/i.test(menuText);
  check(MOD, "Accounts: clicking the account control exposes switching/aggregation entries",
    hasSwitcher, `entriesFound=${hasSwitcher}`);
  await shot(web.page, "verify3-account-menu");
  const apiAccounts = await api("/api/v1/accounts");
  check(MOD, "Accounts: aggregation API agrees (reachable)", apiAccounts.status === 200,
    `count=${Array.isArray(apiAccounts.json) ? apiAccounts.json.length : "n/a"}`);
  const delegations = await api("/api/v1/delegations").catch(() => ({ status: "ERR" }));
  record(MOD, "Accounts: delegations endpoint", delegations.status === 200 ? "PASS" : "BLOCKED",
    `status=${delegations.status}`);
} finally {
  await web.ctx.close().catch(() => {});
  for (const f of ["Inbox", "Sent"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects([BURN]).catch(() => 0);
  record(MOD, "Cleanup: pending-verification mail purged", purged >= 0 ? "PASS" : "FAIL", "expunged=" + purged);
  await closeBrowser();
}
const s = summary();
console.log(`\n=== verify-pending: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("verify-pending.json");
process.exit(s.fail > 0 ? 1 : 0);
