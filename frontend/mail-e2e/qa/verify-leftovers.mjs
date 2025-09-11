// Post-fix regression for D3 (cross-tab flags), D4 (SEARCH latency),
// D8 (snooze hidden from the list API), D5 (empty subject blocked).
// Run: cd frontend && node mail-e2e/qa/verify-leftovers.mjs
import {
  USER, api, awaitMail, check, cleanupFolder, closeBrowser, dumpResults, gotoMail, listFolder,
  newContext, purgeSubjects, record, seedMail, sleep, stableList, summary, withImap,
} from "./lib.mjs";

const MOD = "VERIFY2";
const stamp = Date.now();
const S8 = `E2E-D8-${stamp}`;
const S3 = `E2E-D3-${stamp}`;
try {
  // ------------------------------------------------------------------ D4
  const t0 = Date.now();
  let hits = 0;
  try {
    hits = (await withImap((c) => c.uidSearch(`HEADER Subject "nonexistent-${stamp}"`, "Trash"))).length;
  } catch (e) {
    record(MOD, "D4 SEARCH timing", "FAIL", e.message);
  }
  const ms = Date.now() - t0;
  check(MOD, "D4: SEARCH on Trash is no longer ~45s", ms < 20000, `took=${ms}ms hits=${hits}`);

  // ------------------------------------------------------------------ D8
  await seedMail(S8, { body: "d8 probe" });
  const m8 = await awaitMail(S8);
  await api("/api/v1/mail/snooze", { method: "POST", body: { folder: "Inbox", uid: m8.uid, until: Math.floor(Date.now() / 1000) + 3600 } });
  await sleep(4000);
  const listed = (await listFolder("Inbox", 0)).messages.some((x) => x.uid === m8.uid);
  const snoozed = (await api("/api/v1/mail/snoozed")).json || [];
  check(MOD, "D8: snoozed mail is filtered out of the list API", !listed, `stillListed=${listed}`);
  check(MOD, "D8: it still shows in /mail/snoozed", snoozed.some((x) => (x.subject || "").includes(S8)),
    `count=${snoozed.length}`);
  await api("/api/v1/mail/snooze", { method: "POST", body: { folder: "Inbox", uid: m8.uid, until: 0 } });

  // ------------------------------------------------------------------ D3
  await seedMail(S3, { body: "d3 probe" });
  const m3 = await awaitMail(S3);
  const A = await newContext({ locale: "en" });
  const B = await newContext({ locale: "en" });
  try {
    await gotoMail(A.page, "/mail/Inbox");
    await gotoMail(B.page, "/mail/Inbox");
    for (const p of [A.page, B.page]) {
      for (let i = 0; i < 20; i++) {
        if ((await p.locator("body").innerText()).includes(S3)) break;
        await sleep(1500);
      }
    }
    await stableList(A.page, 15000);
    await A.page.locator(".mail-scroll [role='button']", { hasText: S3 }).first()
      .locator("svg.lucide-star").first().dispatchEvent("click");
    await sleep(3000);
    const flags = (await withImap((c) => c.uidFetch(m3.uid))).flags || [];
    check(MOD, "D3: star applied in tab A reaches the server", flags.includes("\\Flagged"), `flags=[${flags.join(",")}]`);
    // tab B must pick the change up on its own (60s safety-net refresh)
    let converged = false;
    const t3 = Date.now();
    for (let i = 0; i < 12 && !converged; i++) {
      await sleep(8000);
      // Deliberately NO navigation/reload: the point is that the tab picks the
      // change up on its own (60s safety-net refresh).
      const cls = await B.page.locator(".mail-scroll [role='button']", { hasText: S3 }).first()
        .locator("svg.lucide-star").first().getAttribute("class").catch(() => "");
      converged = /fill-|text-amber|star-filled/.test(cls || "");
    }
    check(MOD, "D3: tab B converges without a manual refresh", converged,
      `converged=${converged} after ${Math.round((Date.now() - t3) / 1000)}s`);
  } finally {
    await A.ctx.close();
    await B.ctx.close();
  }

  // ------------------------------------------------------------------ D5
  const C = await newContext({ locale: "en" });
  try {
    await gotoMail(C.page, "/mail/Inbox");
    await C.page.locator("button", { hasText: /^Write$/ }).first().click();
    await C.page.locator('input[placeholder="user@example.com"]').first().fill(USER.email);
    await C.page.keyboard.press("Enter");
    await C.page.locator('[data-testid="compose-subject"]').fill("");
    await C.page.locator('[contenteditable="true"]').first().click();
    await C.page.keyboard.type("empty subject should be blocked");
    await C.page.locator("button", { hasText: /^Send$/ }).first().click();
    await sleep(3000);
    const txt = await C.page.locator("body").innerText();
    const stillOpen = await C.page.locator('[data-testid="compose-subject"]').isVisible().catch(() => false);
    check(MOD, "D5: sending with an empty subject is refused with a message",
      stillOpen && /subject line before sending/i.test(txt), `composerOpen=${stillOpen}`);
  } finally {
    await C.ctx.close();
  }
} finally {
  for (const f of ["Inbox", "Sent", "Drafts"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects([S8, S3]).catch(() => 0);
  record(MOD, "Cleanup: verification mail purged", purged >= 0 ? "PASS" : "FAIL", "expunged=" + purged);
  await closeBrowser();
}
const s = summary();
console.log(`\n=== verify-leftovers: ${s.pass} passed, ${s.fail} failed (${s.total}) ===`);
await dumpResults("verify-leftovers.json");
process.exit(s.fail > 0 ? 1 : 0);
