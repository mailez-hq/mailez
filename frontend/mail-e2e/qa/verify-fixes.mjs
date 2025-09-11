// Post-fix verification for the two P0 defects (D1 invalid recipient, D2 stale
// row after another client's move/delete). Re-runnable regression evidence.
// Run: cd frontend && node mail-e2e/qa/verify-fixes.mjs
import {
  USER, api, awaitMail, check, cleanupFolder, closeBrowser, dumpResults, gotoMail, listFolder,
  newContext, purgeSubjects, record, seedMail, shot, sleep, stableList, summary,
} from "./lib.mjs";

const MOD = "VERIFY";
const stamp = Date.now();
const BAD = `E2E-VERIFY-BAD-${stamp}`;
const OK = `E2E-VERIFY-OK-${stamp}`;
const RT = `E2E-VERIFY-RT-${stamp}`;

const web = await newContext({ locale: "en" });
try {
  // ------------------------------------------- D1a: a valid send still works
  await gotoMail(web.page, "/mail/Inbox");
  await web.page.locator("button", { hasText: /^Write$/ }).first().click();
  const okTo = web.page.locator('input[placeholder="user@example.com"]').first();
  await okTo.waitFor({ state: "visible", timeout: 15000 });
  await okTo.fill(USER.email);
  await web.page.keyboard.press("Enter");
  await web.page.locator('[data-testid="compose-subject"]').fill(OK);
  await web.page.locator('[contenteditable="true"]').first().click();
  await web.page.keyboard.type("valid recipient control");
  await web.page.locator("button", { hasText: /^Send$/ }).first().click();
  const okSent = await (async () => {
    for (let i = 0; i < 20; i++) {
      const hit = (await listFolder("Inbox", 0)).messages.find((m) => (m.subject || "").includes(OK));
      if (hit) return hit;
      await sleep(1500);
    }
    return null;
  })();
  check(MOD, "D1: a valid recipient still sends end-to-end", !!okSent,
    okSent ? `uid=${okSent.uid}` : "not delivered");

  // ------------------------------------------------- D1: invalid recipient
  await gotoMail(web.page, "/mail/Inbox");
  await web.page.locator("button", { hasText: /^Write$/ }).first().click();
  const to = web.page.locator('input[placeholder="user@example.com"]').first();
  await to.waitFor({ state: "visible", timeout: 15000 });
  await to.fill("not-an-email");
  await web.page.keyboard.press("Enter");
  await web.page.locator('[data-testid="compose-subject"]').fill(BAD);
  await web.page.locator('[contenteditable="true"]').first().click();
  await web.page.keyboard.type("should never be sent");
  await web.page.locator("button", { hasText: /^Send$/ }).first().click();
  await web.page.waitForTimeout(3000);
  const stillOpen = await web.page.locator('[data-testid="compose-subject"]').isVisible().catch(() => false);
  const bodyText = await web.page.locator("body").innerText();
  const errorShown = /not a valid email address/i.test(bodyText);
  check(MOD, "D1: invalid recipient keeps the composer open", stillOpen, `composerOpen=${stillOpen}`);
  check(MOD, "D1: invalid recipient shows an inline error", errorShown,
    `errorText=${JSON.stringify(bodyText.split("\n").find((l) => /valid email/i.test(l)) || "")}`);
  await shot(web.page, "verify-d1-invalid-recipient");

  const queued = (await listFolder("Sent", 0)).messages.some((m) => (m.subject || "").includes(BAD));
  check(MOD, "D1: nothing was queued for the invalid address", !queued);

  await web.page.locator("button[title='Close']:visible").first().click().catch(() => {});
  await web.page.waitForTimeout(2000);

  // ------------------------------------------------- D2: stale row after move
  await seedMail(RT, { body: "stale row probe" });
  const rt = await awaitMail(RT);
  await gotoMail(web.page, "/mail/Inbox");
  await stableList(web.page, 15000);
  const seen = await (async () => {
    for (let i = 0; i < 20; i++) {
      if ((await web.page.locator("body").innerText()).includes(RT)) return true;
      await web.page.waitForTimeout(1500);
    }
    return false;
  })();
  check(MOD, "D2: seeded row visible before the external move", seen);
  const t0 = Date.now();
  await api("/api/v1/mail/move", { method: "POST", body: { folder: "Inbox", uids: [rt.uid], destination: "Trash" } });
  let goneAfter = 0;
  for (let i = 0; i < 30; i++) {
    if (!(await web.page.locator("body").innerText()).includes(RT)) {
      goneAfter = Date.now() - t0;
      break;
    }
    await sleep(1000);
  }
  check(MOD, "D2: open tab drops the row without a manual refresh",
    goneAfter > 0 && goneAfter < 20000, `converged in ${goneAfter ? goneAfter + "ms" : ">30s"}`);
  await shot(web.page, "verify-d2-realtime");
} finally {
  await web.ctx.close().catch(() => {});
  for (const f of ["Inbox", "Sent"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects([BAD, OK, RT]).catch(() => 0);
  record(MOD, "Cleanup: verification mail purged", purged >= 0 ? "PASS" : "FAIL", "expunged=" + purged);
  await closeBrowser();
}

const s = summary();
console.log(`\n=== verify-fixes: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("verify-fixes.json");
process.exit(s.fail > 0 ? 1 : 0);
