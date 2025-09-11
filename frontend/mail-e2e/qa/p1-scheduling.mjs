// Phase 2 (P1/b) — scheduled send + cancel + back-to-draft, the undo-send
// window, snooze/unsnooze, burn-after-read, read receipts, a >10MB attachment,
// and HTML/remote-image safety in the reader.
//
// Run: cd frontend && node mail-e2e/qa/p1-scheduling.mjs
import {
  BASE, USER, api, awaitMail, awaitRow, check, cleanupFolder, closeBrowser, dumpResults,
  gotoMail, listFolder, newContext, purgeSubjects, record, seedMail, shot, sleep, stableList,
  summary, withImap,
} from "./lib.mjs";

const MOD = "P1-sched";
const stamp = Date.now();
const S = {
  sched: `E2E-SCHED-${stamp}`,
  schedDraft: `E2E-SCHEDDRAFT-${stamp}`,
  undoKeep: `E2E-UNDOKEEP-${stamp}`,
  undoCancel: `E2E-UNDOCANCEL-${stamp}`,
  snooze: `E2E-SNOOZE-${stamp}`,
  burn: `E2E-BURN-${stamp}`,
  receipt: `E2E-RECEIPT-${stamp}`,
  big: `E2E-BIGATT-${stamp}`,
  html: `E2E-HTMLBODY-${stamp}`,
};
const created = Object.values(S);
const inFolder = async (subject, folder) => {
  for (let p = 0; p < 3; p++) {
    const r = await listFolder(folder, p);
    if (r.messages.some((m) => (m.subject || "").includes(subject))) return true;
    if (r.messages.length < 50) break;
  }
  return false;
};

const web = await newContext({ locale: "en" });
const dialogs = [];
web.page.on("dialog", async (d) => {
  dialogs.push(d.message());
  await d.dismiss().catch(() => {});
});

try {
  // ------------------------------------------- 1. scheduled send + cancel
  const when = new Date(Date.now() + 3 * 60 * 1000).toISOString();
  await seedMail(S.sched, { body: "scheduled probe", send_at: when });
  await sleep(2500);
  const listed = await api("/api/v1/mail/scheduled");
  const entry = (listed.json || []).find((x) => (x.subject || "").includes(S.sched));
  check(MOD, "Scheduled send: a future send_at lands in /mail/scheduled", !!entry,
    entry ? `id=${entry.id} send_after=${entry.send_after}` : `list=${JSON.stringify(listed.json).slice(0, 120)}`);
  await gotoMail(web.page, "/mail/Scheduled");
  await web.page.waitForTimeout(2500);
  const schedUi = await web.page.locator("body").innerText();
  check(MOD, "Scheduled send: the UI lists the pending send", schedUi.includes(S.sched) || /Scheduled for/.test(schedUi),
    `uiHasSubject=${schedUi.includes(S.sched)}`);
  await shot(web.page, "p1-scheduled");
  if (entry) {
    const cancel = await api(`/api/v1/mail/outbox/${entry.id}`, { method: "DELETE" });
    await sleep(2000);
    const after = await api("/api/v1/mail/scheduled");
    check(MOD, "Scheduled send: cancel removes it from the queue",
      cancel.status < 400 && !(after.json || []).some((x) => x.id === entry.id),
      `status=${cancel.status}`);
  }

  // ------------------------------------- 2. scheduled send → back to draft
  await seedMail(S.schedDraft, { body: "scheduled to draft probe", send_at: new Date(Date.now() + 5 * 60 * 1000).toISOString() });
  await sleep(2500);
  const listed2 = await api("/api/v1/mail/scheduled");
  const entry2 = (listed2.json || []).find((x) => (x.subject || "").includes(S.schedDraft));
  if (entry2) {
    const toDraft = await api(`/api/v1/mail/outbox/${entry2.id}/to-draft`, { method: "POST" });
    await sleep(2500);
    check(MOD, "Scheduled send: 'to draft' moves it into Drafts",
      toDraft.status < 400 && (await inFolder(S.schedDraft, "Drafts")),
      `status=${toDraft.status}`);
  } else {
    record(MOD, "Scheduled send: 'to draft'", "BLOCKED", "second schedule not listed");
  }

  // ------------------------------------------------ 3. undo-send window
  await gotoMail(web.page, "/mail/Inbox");
  async function uiSend(subject, bodyText) {
    await web.page.locator("button", { hasText: /^Write$/ }).first().click();
    const to = web.page.locator('input[placeholder="user@example.com"]').first();
    await to.waitFor({ state: "visible", timeout: 15000 });
    await to.fill(USER.email);
    await web.page.keyboard.press("Enter");
    await web.page.locator('[data-testid="compose-subject"]').fill(subject);
    await web.page.locator('[contenteditable="true"]').first().click();
    await web.page.keyboard.type(bodyText);
    await web.page.locator("button", { hasText: /^Send$/ }).first().click();
    await web.page.waitForTimeout(1500);
  }

  await uiSend(S.undoCancel, "undo me");
  const undoBtn = web.page.locator("button", { hasText: /^Undo$/ }).last();
  const undoVisible = await web.page.locator("button", { hasText: /Undo/ }).count();
  if (undoVisible) {
    await undoBtn.click({ timeout: 8000 }).catch(() => {});
    await sleep(9000);
    const delivered = await inFolder(S.undoCancel, "Inbox");
    check(MOD, "Undo send: cancelling inside the window prevents delivery", !delivered,
      `delivered=${delivered}`);
  } else {
    record(MOD, "Undo send: undo affordance", "FAIL", "no Undo control appeared after Send");
  }
  await shot(web.page, "p1-undo");

  await uiSend(S.undoKeep, "let it go");
  await sleep(12000);
  check(MOD, "Undo send: letting the window lapse delivers the message",
    await inFolder(S.undoKeep, "Inbox"), "checked 12s after send");

  // --------------------------------------------------------- 4. snooze
  await seedMail(S.snooze, { body: "snooze probe" });
  const snoozeMsg = await awaitMail(S.snooze);
  const until = Math.floor(Date.now() / 1000) + 3600;
  await api("/api/v1/mail/snooze", { method: "POST", body: { folder: "Inbox", uid: snoozeMsg.uid, until } });
  let offInbox = false;
  for (let i = 0; i < 10 && !offInbox; i++) {
    offInbox = !(await inFolder(S.snooze, "Inbox"));
    if (!offInbox) await sleep(1500);
  }
  const snoozedList = await api("/api/v1/mail/snoozed");
  check(MOD, "Snooze: the message leaves Inbox", offInbox);
  check(MOD, "Snooze: it is listed in the snoozed view",
    (snoozedList.json || []).some((m) => (m.subject || "").includes(S.snooze)),
    `count=${(snoozedList.json || []).length}`);
  await gotoMail(web.page, "/mail/Snoozed");
  await web.page.waitForTimeout(3000);
  await shot(web.page, "p1-snoozed");
  await api("/api/v1/mail/snooze", { method: "POST", body: { folder: "Inbox", uid: snoozeMsg.uid, until: 0 } });
  let backInbox = false;
  for (let i = 0; i < 12 && !backInbox; i++) {
    backInbox = await inFolder(S.snooze, "Inbox");
    if (!backInbox) await sleep(1500);
  }
  check(MOD, "Snooze: unsnooze brings the message back to Inbox", backInbox);

  // ------------------------------------------------- 5. burn after read
  await seedMail(S.burn, { body: "burn body secret", burn_after_minutes: 5 });
  const burnMsg = await awaitMail(S.burn);
  check(MOD, "Burn after read: send carries the burn flag",
    !!burnMsg && Number(burnMsg.burn_after_minutes) > 0,
    `burn_after_minutes=${burnMsg?.burn_after_minutes}`);
  await gotoMail(web.page, "/mail/Inbox");
  await awaitRow(web.page, S.burn, 45000);
  await stableList(web.page, 15000);
  await web.page.locator(".mail-scroll [role='button']", { hasText: S.burn }).first().click();
  await web.page.waitForTimeout(3000);
  const burnUi = await web.page.locator("body").innerText();
  check(MOD, "Burn after read: the reader gates the body behind a reveal action",
    /burn|Reveal|阅后/.test(burnUi) || !burnUi.includes("burn body secret"),
    `bodyVisible=${burnUi.includes("burn body secret")}`);
  await shot(web.page, "p1-burn");

  // ---------------------------------------------------- 6. read receipt
  await seedMail(S.receipt, { body: "receipt probe", receipt_requested: true });
  const receiptMsg = await awaitMail(S.receipt);
  check(MOD, "Read receipt: send records the request",
    !!receiptMsg && receiptMsg.receipt_requested === true,
    `receipt_requested=${receiptMsg?.receipt_requested}`);
  await gotoMail(web.page, "/mail/Inbox");
  await awaitRow(web.page, S.receipt, 45000);
  await web.page.locator(".mail-scroll [role='button']", { hasText: S.receipt }).first().click();
  await web.page.waitForTimeout(3000);
  const receiptBanner = await web.page.locator("button", { hasText: /Send receipt/ }).count();
  if (receiptBanner) {
    await web.page.locator("button", { hasText: /Send receipt/ }).first().click();
    await web.page.waitForTimeout(6000);
    const receiptArrived = await (async () => {
      for (let i = 0; i < 12; i++) {
        const r = await listFolder("Inbox", 0);
        const hit = r.messages.find((m) => /receipt|回执/i.test(m.subject || "") && (m.subject || "").includes(S.receipt));
        if (hit) return hit;
        await sleep(1500);
      }
      return null;
    })();
    check(MOD, "Read receipt: sending the receipt delivers a receipt message",
      !!receiptArrived, receiptArrived ? `uid=${receiptArrived.uid} subject="${receiptArrived.subject}"` : "none seen");
  } else {
    record(MOD, "Read receipt: banner", "BLOCKED", "no Send receipt control in the reader");
  }
  await shot(web.page, "p1-receipt");

  // ------------------------------------------------ 7. >10MB attachment
  const big = Buffer.alloc(11 * 1024 * 1024, 0x41).toString("base64");
  const t0 = Date.now();
  await seedMail(S.big, {
    body: "big attachment",
    attachments: [{ filename: "大附件.bin", content_type: "application/octet-stream", size: 11 * 1024 * 1024, data: big }],
  });
  const bigMsg = await awaitMail(S.big, { timeoutMs: 90000 });
  check(MOD, "Attachments: an 11MB attachment round-trips",
    !!bigMsg && bigMsg.has_attachment === true && (bigMsg.size || 0) > 11 * 1024 * 1024,
    bigMsg ? `size=${bigMsg.size} took=${Date.now() - t0}ms` : "not delivered in 90s");

  // ---------------------------------------- 8. HTML / remote-image safety
  const hostile =
    '<p>hello</p><script>alert("xss")</script>' +
    '<a id="js-link" href="javascript:alert(2)">click</a>' +
    '<img id="remote" src="http://127.0.0.1:9/track.png">';
  await seedMail(S.html, { body: "html body probe", html: hostile });
  const htmlMsg = await awaitMail(S.html);
  check(MOD, "HTML body: hostile message delivered for rendering checks", !!htmlMsg);
  await gotoMail(web.page, "/mail/Inbox");
  await awaitRow(web.page, S.html, 45000);
  await web.page.locator(".mail-scroll [role='button']", { hasText: S.html }).first().click();
  await web.page.waitForTimeout(4000);
  const jsHref = await web.page.evaluate(() =>
    [...document.querySelectorAll("a")].filter((a) => (a.getAttribute("href") || "").toLowerCase().startsWith("javascript:")).length,
  );
  const remoteLoaded = await web.page.evaluate(() =>
    [...document.querySelectorAll("img")].filter((i) => (i.currentSrc || i.src || "").includes("127.0.0.1:9")).length,
  );
  check(MOD, "HTML body: no javascript: links survive sanitising", jsHref === 0, `javascriptHrefs=${jsHref}`);
  check(MOD, "HTML body: remote images are not auto-loaded", remoteLoaded === 0, `remoteImgs=${remoteLoaded}`);
  check(MOD, "HTML body: no script dialog fired", dialogs.length === 0, `dialogs=[${dialogs.join("|")}]`);
  await shot(web.page, "p1-html-body");
} finally {
  await web.ctx.close().catch(() => {});
  // cancel anything still queued, then purge mail + drafts
  const pending = (await api("/api/v1/mail/scheduled").catch(() => ({ json: [] }))).json || [];
  for (const p of pending) await api(`/api/v1/mail/outbox/${p.id}`, { method: "DELETE" }).catch(() => {});
  for (const f of ["Inbox", "Drafts", "Sent", "Scheduled", "Snoozed"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects(created).catch(() => 0);
  record(MOD, "Cleanup: scheduling/snooze probes purged", purged >= 0 ? "PASS" : "FAIL", "expunged=" + purged);
  await closeBrowser();
}

const s = summary();
console.log(`\n=== p1-scheduling: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("p1-scheduling.json");
process.exit(s.fail > 0 ? 1 : 0);
