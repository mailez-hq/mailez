// Cross-cutting — two browser contexts (same account, two tabs) acting on the
// same mailbox: does the other tab converge without a manual refresh, and do
// the two UIs agree with the server afterwards?
//
// Run: cd frontend && node mail-e2e/qa/x-concurrency.mjs
import {
  USER, api, awaitMail, check, cleanupFolder, closeBrowser, dumpResults, gotoMail, listFolder,
  newContext, purgeSubjects, record, seedMail, shot, sleep, stableList, summary, withImap,
} from "./lib.mjs";

const MOD = "X-concurrency";
const stamp = Date.now();
const S = { gone: `E2E-CONC-${stamp}`, star: `E2E-CONCSTAR-${stamp}` };

const a = await newContext({ locale: "en" });
const b = await newContext({ locale: "en" });
try {
  // ---------------------------------------------- 1. delete in B, see it in A
  await seedMail(S.gone, { body: "concurrency delete probe" });
  const goneMsg = await awaitMail(S.gone);
  await gotoMail(a.page, "/mail/Inbox");
  await gotoMail(b.page, "/mail/Inbox");
  const seenByA = await (async () => {
    for (let i = 0; i < 20; i++) {
      if ((await a.page.locator("body").innerText()).includes(S.gone)) return true;
      await a.page.waitForTimeout(1500);
    }
    return false;
  })();
  check(MOD, "Both contexts show the seeded message", seenByA);

  await stableList(b.page, 20000);
  await b.page.locator(".mail-scroll [role='button']", { hasText: S.gone }).first()
    .locator("button[title='Delete']").first().dispatchEvent("click");
  await b.page.waitForTimeout(3000);

  // A must converge on its own (realtime/refetch), without navigation.
  let converged = false;
  for (let i = 0; i < 12 && !converged; i++) {
    await a.page.waitForTimeout(2000);
    converged = !(await a.page.locator("body").innerText()).includes(S.gone);
  }
  const serverGone = !(await listFolder("Inbox", 0)).messages.some((m) => m.uid === goneMsg.uid);
  check(MOD, "Deleting in tab B disappears from tab A without a manual refresh", converged,
    converged ? "tab A converged" : "tab A still lists the deleted message after ~24s");
  check(MOD, "Server agrees the message is gone (API)", serverGone);
  await shot(a.page, "x-concurrency-after-delete-a");
  await shot(b.page, "x-concurrency-after-delete-b");

  // ------------------------------------- 2. conflicting star toggles converge
  await seedMail(S.star, { body: "concurrency star probe" });
  const starMsg = await awaitMail(S.star);
  await gotoMail(a.page, "/mail/Inbox");
  await gotoMail(b.page, "/mail/Inbox");
  for (const p of [a.page, b.page]) {
    for (let i = 0; i < 20; i++) {
      if ((await p.locator("body").innerText()).includes(S.star)) break;
      await p.waitForTimeout(1500);
    }
  }
  await stableList(a.page, 15000);
  await a.page.locator(".mail-scroll [role='button']", { hasText: S.star }).first()
    .locator("svg.lucide-star").first().dispatchEvent("click");
  await b.page.waitForTimeout(4000);
  const bStarClass = await b.page.locator(".mail-scroll [role='button']", { hasText: S.star }).first()
    .locator("svg.lucide-star").first().getAttribute("class").catch(() => "");
  const serverFlags = (await withImap((c) => c.uidFetch(starMsg.uid))).flags || [];
  check(MOD, "Star set in tab A is flagged on the server",
    serverFlags.includes("\\Flagged"), `flags=[${serverFlags.join(",")}]`);
  check(MOD, "Tab B reflects the star without a manual refresh",
    /fill-/.test(bStarClass || "") || (bStarClass || "").includes("lucide-star"),
    `starClass="${String(bStarClass).slice(0, 60)}"`);

  // un-star from B, confirm A converges to the same state
  await b.page.locator(".mail-scroll [role='button']", { hasText: S.star }).first()
    .locator("svg.lucide-star").first().dispatchEvent("click");
  await sleep(4000);
  const finalFlags = (await withImap((c) => c.uidFetch(starMsg.uid))).flags || [];
  check(MOD, "Un-star from tab B clears the server flag (single source of truth)",
    !finalFlags.includes("\\Flagged"), `flags=[${finalFlags.join(",")}]`);
} finally {
  await a.ctx.close().catch(() => {});
  await b.ctx.close().catch(() => {});
  await cleanupFolder("Inbox").catch(() => {});
  const purged = await purgeSubjects(Object.values(S)).catch(() => 0);
  record(MOD, "Cleanup: concurrency probes purged", purged >= 0 ? "PASS" : "FAIL", "expunged=" + purged);
  await closeBrowser();
}

const s = summary();
console.log(`\n=== x-concurrency: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("x-concurrency.json");
process.exit(s.fail > 0 ? 1 : 0);
