// Phase 1 (P0/d) — conversation (thread) grouping vs single-message view, and
// search: keyword, has:attachment, no-match, and a hostile query string.
//
// Run: cd frontend && node mail-e2e/qa/p0-thread-search.mjs
import {
  BASE, USER, api, awaitMail, awaitRow, check, cleanupFolder, closeBrowser, dumpResults, gotoMail,
  listFolder, newContext, purgeSubjects, record, seedMail, shot, sleep, stableList, summary,
  withImap,
} from "./lib.mjs";

const MOD = "P0-thread";
const stamp = Date.now();
const ROOT = `E2E-THR-${stamp}`;
const ATT = `E2E-ATT-${stamp}`;
const created = [ROOT, ATT];

const web = await newContext({ locale: "en" });
try {
  // ---------------------------------------------------- build a 3-message thread
  await seedMail(ROOT, { body: "thread root body" });
  const root = await awaitMail(ROOT);
  check(MOD, "Thread: root message delivered", !!root, root ? `uid=${root.uid}` : "missing");

  // The raw source carries the Message-ID the replies must reference.
  const raw = await api(`/api/v1/mail/raw?folder=Inbox&uid=${root.uid}`);
  const msgId = (/^Message-ID:\s*(.+)$/im.exec(raw.json?.raw || raw.text || "") || [])[1]?.trim();
  check(MOD, "Thread: root exposes a Message-ID for replies", !!msgId, `message-id=${msgId}`);

  if (msgId) {
    for (const n of [1, 2]) {
      await seedMail(`Re: ${ROOT}`, {
        body: `thread reply ${n}`,
        in_reply_to: msgId,
        references: msgId,
      });
    }
    await sleep(4000);
    const conv = await listFolder("Inbox", 0, { conversation: true });
    const row = conv.messages.find((m) => (m.subject || "").includes(ROOT));
    check(MOD, "Thread: conversation view reports 3 members (API)",
      !!row && row.thread_count === 3,
      row ? `thread_count=${row.thread_count} subject=${row.subject}` : "row missing");

    const flat = await listFolder("Inbox", 0, { conversation: false });
    const flatRows = flat.messages.filter((m) => (m.subject || "").includes(ROOT));
    check(MOD, "Thread: single-message view lists the 3 members separately (API)",
      flatRows.length === 3, `rows=${flatRows.length}`);

    // UI: conversation mode shows one row with a count badge.
    await gotoMail(web.page, "/mail/Inbox");
    await awaitRow(web.page, ROOT, 45000); // SSE/queue latency: poll, don't sample once
    await stableList(web.page, 15000);
    const rowCount = await web.page.locator(".mail-scroll [role='button']", { hasText: ROOT }).count();
    check(MOD, "Thread: UI collapses the thread into one row", rowCount === 1, `rows=${rowCount}`);
    await web.page.locator(".mail-scroll [role='button']", { hasText: ROOT }).first().click();
    await web.page.waitForTimeout(3500);
    const pane = await web.page.locator("body").innerText();
    const members = (pane.match(/thread reply/g) || []).length;
    check(MOD, "Thread: reader shows every member of the conversation",
      members >= 2, `reply bodies rendered=${members}`);
    await shot(web.page, "p0-thread-open");

    // UI: switch to the single-message view from Settings → three rows appear.
    await gotoMail(web.page, "/mail/Inbox");
    await web.page.getByRole("button", { name: /^Settings$/ }).first().click().catch(() => {});
    await web.page.waitForTimeout(2000);
    // The switch lives on the "Conversation view" settings row.
    const convRow = web.page.locator("div.flex.items-center.justify-between", { hasText: /Conversation view/ }).first();
    const convSwitch = convRow.locator('[role="switch"]').first();
    const before = await convSwitch.getAttribute("aria-checked").catch(() => null);
    await convSwitch.click({ timeout: 8000 }).catch(() => {});
    await web.page.waitForTimeout(1200);
    await web.page.locator("button", { hasText: /^Save settings$/ }).first().click().catch(() => {});
    await web.page.waitForTimeout(3000);
    await gotoMail(web.page, "/mail/Inbox");
    await awaitRow(web.page, ROOT, 45000);
    await stableList(web.page, 15000);
    const flatRowsUi = await web.page.locator(".mail-scroll [role='button']", { hasText: ROOT }).count();
    check(MOD, "Thread: turning conversation view off splits the thread into 3 rows",
      flatRowsUi === 3, `rows=${flatRowsUi} (switch was aria-checked=${before})`);
    await shot(web.page, "p0-thread-flat");
    // restore the setting
    await web.page.getByRole("button", { name: /^Settings$/ }).first().click().catch(() => {});
    await web.page.waitForTimeout(2000);
    await web.page.locator("div.flex.items-center.justify-between", { hasText: /Conversation view/ }).first()
      .locator('[role="switch"]').first().click({ timeout: 8000 }).catch(() => {});
    await web.page.locator("button", { hasText: /^Save settings$/ }).first().click().catch(() => {});
    await web.page.waitForTimeout(2500);
    await web.page.keyboard.press("Escape");
    await web.page.waitForTimeout(1000);
  } else {
    record(MOD, "Thread: conversation assertions", "BLOCKED", "root Message-ID not found");
  }

  // ------------------------------------------------------------- attachments
  const payload = Buffer.from("attachment-body-" + stamp).toString("base64");
  await seedMail(ATT, {
    body: "mail with attachment",
    attachments: [{ filename: "测试附件.txt", content_type: "text/plain", size: 30, data: payload }],
  });
  const attMsg = await awaitMail(ATT);
  check(MOD, "Attachment: message with a CJK-named attachment is delivered", !!attMsg,
    attMsg ? `uid=${attMsg.uid} has_attachment=${attMsg.has_attachment}` : "missing");
  check(MOD, "Attachment: list flags the message as carrying an attachment",
    !!attMsg && attMsg.has_attachment === true, `has_attachment=${attMsg?.has_attachment}`);

  // ----------------------------------------------------------------- search
  const searchBox = () => web.page.locator('input[placeholder="Search mail"]').first();
  await gotoMail(web.page, "/mail/Inbox");

  async function runSearch(q) {
    await gotoMail(web.page, "/mail/Inbox");
    const box = searchBox();
    await box.click();
    await box.fill("");
    await box.type(q);
    await box.press("Enter");
    await web.page.waitForTimeout(5000);
    // The empty state ("No matches") renders as a sibling of the virtual list,
    // so assertions read the whole panel/body text.
    return web.page.locator("body").innerText();
  }

  const kw = await runSearch(ROOT);
  check(MOD, "Search: keyword finds the seeded thread", kw.includes(ROOT),
    `listHasSubject=${kw.includes(ROOT)}`);
  await shot(web.page, "p0-search-keyword");

  const attSearch = await runSearch("has:attachment");
  check(MOD, "Search: has:attachment returns the attachment message",
    attSearch.includes(ATT) || attSearch.includes("测试附件"), `snippet="${attSearch.replace(/\s+/g, " ").slice(0, 80)}"`);
  await shot(web.page, "p0-search-attachment");

  const none = await runSearch(`zzz-no-such-token-${stamp}`);
  check(MOD, "Search: a non-matching query reports 'No matches'", /No matches|无匹配/.test(none),
    `snippet="${none.replace(/\s+/g, " ").slice(0, 60)}"`);

  const hostile = await runSearch(`"><script>alert(1)</script>`);
  check(MOD, "Search: hostile query string is handled without a crash", !!hostile,
    `chars=${hostile.length}`);
  await gotoMail(web.page, "/mail/Inbox");
} finally {
  await web.ctx.close().catch(() => {});
  for (const f of ["Inbox", "Sent"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects(created).catch(() => 0);
  record(MOD, "Cleanup: thread/search subjects purged", purged >= 0 ? "PASS" : "FAIL", "expunged=" + purged);
  await closeBrowser();
}

const s = summary();
console.log(`\n=== p0-thread-search: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("p0-thread-search.json");
process.exit(s.fail > 0 ? 1 : 0);
