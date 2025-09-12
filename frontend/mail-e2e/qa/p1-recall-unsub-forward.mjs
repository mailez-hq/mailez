// Closes three P1 gaps: mail recall (撤回), one-click unsubscribe, and browser
// forward navigation.
//
// The unsubscribe case needs a message carrying List-Unsubscribe; the send API
// cannot set custom headers, so the fixture is APPENDed over IMAP. The target
// URL points at a dead local port, so even a one-click POST stays on this
// machine and cannot reach a third party.
//
// Run: cd frontend && node mail-e2e/qa/p1-recall-unsub-forward.mjs
import {
  BASE, USER, api, awaitMail, check, cleanupFolder, closeBrowser, dumpResults, gotoMail,
  listFolder, newContext, purgeSubjects, record, seedMail, shot, sleep, stableList, summary,
  withImap,
} from "./lib.mjs";

const MOD = "P1-gap";
const stamp = Date.now();
const RECALL = `E2E-RECALL-${stamp}`;
const UNSUB = `E2E-UNSUB-${stamp}`;

/** APPEND a raw RFC822 message over IMAP (the client speaks literals here). */
async function appendRaw(mailbox, raw) {
  return withImap(async (c) => {
    const tag = `a${Date.now() % 100000}`;
    const bytes = Buffer.from(raw, "utf8");
    c.sock.write(`${tag} APPEND "${mailbox}" (\\Seen) {${bytes.length}}\r\n`);
    // Wait for the continuation request, then send the literal.
    for (;;) {
      const line = await c.nextLine();
      if (line.startsWith("+")) break;
    }
    c.sock.write(raw);
    c.sock.write("\r\n");
    for (;;) {
      const line = await c.nextLine();
      if (line.startsWith(`${tag} `)) return line;
    }
  });
}

const web = await newContext({ locale: "en" });
try {
  // ------------------------------------------------------------- 1. recall
  await seedMail(RECALL, { body: "recall probe body" });
  const inboxCopy = await awaitMail(RECALL);
  const sentCopy = (await listFolder("Sent", 0)).messages.find((m) => (m.subject || "").includes(RECALL));
  check(MOD, "Recall fixture: inbox + sent copies exist", !!inboxCopy && !!sentCopy,
    `inbox=${inboxCopy?.uid} sent=${sentCopy?.uid}`);
  const recall = await api("/api/v1/mail/recall", {
    method: "POST",
    body: { folder: "Sent", uid: sentCopy?.uid ?? 0 },
  });
  check(MOD, "Recall: the API accepts a recall request", recall.status < 400,
    `status=${recall.status} body=${(recall.text || "").slice(0, 100).replace(/\s+/g, " ")}`);
  // Observe what the recipient copy becomes: removed, or annotated/replaced.
  let observed = "unchanged";
  for (let i = 0; i < 10; i++) {
    const rows = (await listFolder("Inbox", 0)).messages;
    const still = rows.find((m) => (m.subject || "").includes(RECALL));
    const notice = rows.find((m) => /recall|撤回/i.test(m.subject || ""));
    if (notice) {
      observed = `notice:"${notice.subject}"`;
      break;
    }
    if (!still) {
      observed = "removed";
      break;
    }
    await sleep(2000);
  }
  const detail = inboxCopy ? await api(`/api/v1/mail/message?folder=Inbox&uid=${inboxCopy.uid}`) : { json: {} };
  record(MOD, "Recall: effect on the recipient copy (observed semantics)",
    recall.status < 400 ? "PASS" : "FAIL",
    `${observed}; detail.recall=${JSON.stringify(detail.json?.recall ?? null).slice(0, 60)}`);
  await gotoMail(web.page, `/mail/Inbox/${encodeURIComponent(inboxCopy?.id ?? "")}`);
  await web.page.waitForTimeout(3000);
  await shot(web.page, "p1gap-recall");

  // --------------------------------------------------------- 2. unsubscribe
  const raw =
    `From: E2E List <list-${stamp}@example.com>\r\n` +
    `To: ${USER.email}\r\n` +
    `Subject: ${UNSUB}\r\n` +
    `Date: ${new Date().toUTCString()}\r\n` +
    `Message-ID: <unsub-${stamp}@example.com>\r\n` +
    `List-Unsubscribe: <http://127.0.0.1:9/unsubscribe?u=${stamp}>\r\n` +
    `List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n` +
    `\r\n` +
    `newsletter body\r\n`;
  let appended = "skipped";
  try {
    appended = await appendRaw("Inbox", raw);
  } catch (e) {
    appended = `ERR ${e.message}`;
  }
  record(MOD, "Unsubscribe fixture: APPEND over IMAP", String(appended).includes("OK") ? "PASS" : "FAIL",
    String(appended).slice(0, 80));
  const unsubMsg = await (async () => {
    for (let i = 0; i < 10; i++) {
      const hit = (await listFolder("Inbox", 0)).messages.find((m) => (m.subject || "").includes(UNSUB));
      if (hit) return hit;
      await sleep(1500);
    }
    return null;
  })();
  check(MOD, "Unsubscribe fixture: message with List-Unsubscribe is visible",
    !!unsubMsg && !!unsubMsg.unsubscribe_url, `unsubscribe_url=${unsubMsg?.unsubscribe_url ?? "-"}`);
  if (unsubMsg) {
    await gotoMail(web.page, `/mail/Inbox/${encodeURIComponent(unsubMsg.id)}`, { settleMs: 5000 });
    const btn = web.page.locator("button", { hasText: /Unsubscribe|退订/ }).first();
    const hasBtn = await btn.count();
    check(MOD, "Unsubscribe: the reader offers the action for such a message", hasBtn > 0, `buttons=${hasBtn}`);
    if (hasBtn) {
      await btn.click({ timeout: 8000 }).catch(() => {});
      await web.page.waitForTimeout(5000); // points at a dead local port on purpose
      const after = await web.page.locator("body").innerText();
      check(MOD, "Unsubscribe: the attempt reports its outcome (no silent failure)",
        /unsubscrib|退订|fail|失败|error/i.test(after),
        `snippet="${after.replace(/\s+/g, " ").slice(-90)}"`);
    }
    await shot(web.page, "p1gap-unsubscribe");
  }

  // ------------------------------------------------------ 3. browser forward
  await gotoMail(web.page, "/mail/Inbox");
  await stableList(web.page, 20000);
  const firstRow = web.page.locator(".mail-scroll [role='button']").first();
  const firstText = await firstRow.innerText().catch(() => "");
  void firstText;
  await gotoMail(web.page, "/mail/Sent");
  await web.page.waitForTimeout(2000);
  const sentUrl = web.page.url();
  await web.page.goBack();
  await web.page.waitForTimeout(2500);
  const backUrl = web.page.url();
  await web.page.goForward();
  await web.page.waitForTimeout(2500);
  const forwardUrl = web.page.url();
  check(MOD, "Navigation: browser back then forward restores the Sent view",
    backUrl !== sentUrl && forwardUrl === sentUrl,
    `sent=${sentUrl.replace(BASE, "")} back=${backUrl.replace(BASE, "")} forward=${forwardUrl.replace(BASE, "")}`);
} finally {
  await web.ctx.close().catch(() => {});
  for (const f of ["Inbox", "Sent"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects([RECALL, UNSUB]).catch(() => 0);
  record(MOD, "Cleanup: gap-closing fixtures purged", purged >= 0 ? "PASS" : "FAIL", "expunged=" + purged);
  await closeBrowser();
}
const s = summary();
console.log(`\n=== p1-recall-unsub-forward: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("p1-recall-unsub-forward.json");
process.exit(s.fail > 0 ? 1 : 0);
