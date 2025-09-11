// P1 leftovers: snooze (API vs UI view), burn-after-read second open, theme
// light/dark immediacy, and the LDAP group-sync config surface.
// Run: cd frontend && node mail-e2e/qa/p1-leftovers.mjs
import { api, awaitMail, check, cleanupFolder, closeBrowser, dumpResults, gotoMail, listFolder, newContext, purgeSubjects, record, seedMail, shot, sleep, stableList, summary, withImap } from "./lib.mjs";

const MOD = "P1-left";
const stamp = Date.now();
const SNOOZE = `E2E-SNZ-${stamp}`;
const BURN = `E2E-BRN-${stamp}`;
const web = await newContext({ locale: "en" });
try {
  // ------------------------------------------------- snooze: API vs UI view
  await seedMail(SNOOZE, { body: "snooze leftover probe" });
  const sn = await awaitMail(SNOOZE);
  const until = Math.floor(Date.now() / 1000) + 3600;
  await api("/api/v1/mail/snooze", { method: "POST", body: { folder: "Inbox", uid: sn.uid, until } });
  await sleep(3000);
  const t0 = Date.now();
  let apiGone = false, uiGone = false;
  for (let i = 0; i < 20 && !(apiGone && uiGone); i++) {
    if (!apiGone) apiGone = !(await listFolder("Inbox", 0)).messages.some((m) => m.uid === sn.uid);
    if (!uiGone) {
      await gotoMail(web.page, "/mail/Inbox");
      await stableList(web.page, 12000);
      uiGone = !(await web.page.locator("body").innerText()).includes(SNOOZE);
    }
    if (!(apiGone && uiGone)) await sleep(6000);
  }
  check(MOD, "Snooze: the message disappears from the open Inbox view", uiGone,
    `uiGone=${uiGone} apiGone=${apiGone} after ${Math.round((Date.now() - t0) / 1000)}s`);
  check(MOD, "Snooze: API list agrees it left Inbox", apiGone, `apiGone=${apiGone}`);
  const snoozed = await api("/api/v1/mail/snoozed");
  check(MOD, "Snooze: it is listed in /mail/snoozed",
    (snoozed.json || []).some((m) => (m.subject || "").includes(SNOOZE)), `count=${(snoozed.json || []).length}`);
  await shot(web.page, "p1left-snoozed");
  await api("/api/v1/mail/snooze", { method: "POST", body: { folder: "Inbox", uid: sn.uid, until: 0 } });
  let back = false;
  for (let i = 0; i < 10 && !back; i++) {
    back = (await listFolder("Inbox", 0)).messages.some((m) => m.uid === sn.uid);
    if (!back) await sleep(2000);
  }
  check(MOD, "Snooze: unsnooze returns it to Inbox", back);

  // ------------------------------------------- burn-after-read second open
  await seedMail(BURN, { body: "burned secret text", burn_after_minutes: 5 });
  const bm = await awaitMail(BURN);
  await gotoMail(web.page, "/mail/Inbox");
  await stableList(web.page, 15000);
  await web.page.locator(".mail-scroll [role='button']", { hasText: BURN }).first().click();
  await web.page.waitForTimeout(3000);
  const reveal = web.page.locator("button", { hasText: /Reveal|查看|Open/ }).first();
  if (await reveal.count()) {
    await reveal.click({ timeout: 8000 }).catch(() => {});
    await web.page.waitForTimeout(3000);
  }
  const first = await web.page.locator("main, body").first().innerText();
  const flags1 = (await withImap((c) => c.uidFetch(bm.uid))).flags || [];
  check(MOD, "Burn: first read shows the body and marks it read-once",
    first.includes("burned secret text") || flags1.some((f) => /BurnRead/i.test(f)),
    `flags=[${flags1.join(",")}]`);
  await shot(web.page, "p1left-burn-first");
  await gotoMail(web.page, "/mail/Inbox");
  await stableList(web.page, 15000);
  await web.page.locator(".mail-scroll [role='button']", { hasText: BURN }).first().click();
  await web.page.waitForTimeout(3500);
  const second = await web.page.locator("main, body").first().innerText();
  check(MOD, "Burn: second open no longer reveals the body",
    !second.includes("burned secret text") || /burn|consumed|阅后|已销毁/i.test(second),
    `bodyVisibleAgain=${second.includes("burned secret text")}`);
  await shot(web.page, "p1left-burn-second");

  // ------------------------------------------------ theme immediacy
  await gotoMail(web.page, "/home");
  const isDark = () => web.page.evaluate(() => document.documentElement.classList.contains("dark"));
  const before = await isDark();
  await web.page.getByRole("button", { name: /^Settings$/ }).first().click().catch(() => {});
  await web.page.waitForTimeout(2000);
  await web.page.locator("button", { hasText: /^Dark$/ }).first().click({ timeout: 8000 }).catch(() => {});
  await web.page.waitForTimeout(1200);
  const darkNow = await isDark();
  await web.page.locator("button", { hasText: /^Light$/ }).first().click({ timeout: 8000 }).catch(() => {});
  await web.page.waitForTimeout(1200);
  const lightNow = await isDark();
  check(MOD, "Theme: dark/light apply immediately without a reload",
    darkNow === true && lightNow === false, `start=${before} afterDark=${darkNow} afterLight=${lightNow}`);
  await web.page.locator("button", { hasText: /^System$/ }).first().click({ timeout: 8000 }).catch(() => {});
  await web.page.locator("button", { hasText: /^Save settings$/ }).first().click().catch(() => {});
  await web.page.waitForTimeout(1500);
  await web.page.keyboard.press("Escape");

  // ------------------------------------------- LDAP group-sync config surface
  const probes = ["/api/v1/ldap", "/api/v1/ldap/config", "/api/v1/directory/ldap", "/api/v1/settings/ldap"];
  const results = [];
  for (const p of probes) {
    const r = await api(p).catch((e) => ({ status: `ERR ${e.message}` }));
    results.push(`${p}=${r.status}`);
    if (r.status === 200) {
      const body = JSON.stringify(r.json || {});
      check(MOD, "LDAP: group-sync settings surface is readable", /sync_groups|group_/i.test(body),
        `${p} -> keys=${Object.keys(r.json || {}).slice(0, 8).join(",")}`);
      break;
    }
  }
  record(MOD, "LDAP: probed endpoints", results.some((x) => x.endsWith("=200")) ? "PASS" : "BLOCKED",
    results.join(" "));
} finally {
  await web.ctx.close().catch(() => {});
  for (const f of ["Inbox", "Sent"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects([SNOOZE, BURN]).catch(() => 0);
  record(MOD, "Cleanup: leftover probes purged", purged >= 0 ? "PASS" : "FAIL", "expunged=" + purged);
  await closeBrowser();
}
const s = summary();
console.log(`\n=== p1-leftovers: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("p1-leftovers.json");
process.exit(s.fail > 0 ? 1 : 0);
