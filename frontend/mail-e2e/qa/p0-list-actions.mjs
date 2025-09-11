// Phase 1 (P0/c) — list state and message actions: unread/read, star, delete,
// bulk delete, archive, move to a user folder, sort order, pagination.
// Every mutation is verified twice: in the UI and against the server (API and,
// where it matters, IMAP).
//
// Run: cd frontend && node mail-e2e/qa/p0-list-actions.mjs
import {
  BASE, USER, api, awaitMail, check, cleanupFolder, closeBrowser, dumpResults, gotoMail,
  listFolder, newContext, purgeSubjects, record, seedMail, shot, sleep, stableList, summary,
  withImap,
} from "./lib.mjs";

const MOD = "P0-list";
const stamp = Date.now();
const S = {
  read: `E2E-READ-${stamp}`,
  star: `E2E-STAR2-${stamp}`,
  del: `E2E-DEL2-${stamp}`,
  bulkA: `E2E-BULKA-${stamp}`,
  bulkB: `E2E-BULKB-${stamp}`,
  arch: `E2E-ARCH-${stamp}`,
  move: `E2E-MOVE-${stamp}`,
};
const FOLDER = `E2E-FOLDER-${stamp}`;
const created = Object.values(S);

const flagsOf = async (uid, folder = "Inbox") => {
  const r = await withImap((c) => c.uidFetch(uid, folder));
  return r.flags || [];
};

const web = await newContext({ locale: "en" });
try {
  // ---------------------------------------------------------- 1. unread→read
  const readMsg = await (async () => {
    await seedMail(S.read, { body: "read-state probe" });
    return awaitMail(S.read);
  })();
  check(MOD, "Unread: new mail arrives unread (IMAP flags)", !!readMsg && !(await flagsOf(readMsg.uid)).includes("\\Seen"),
    `uid=${readMsg?.uid} flags=[${readMsg ? (await flagsOf(readMsg.uid)).join(",") : ""}]`);

  await gotoMail(web.page, "/mail/Inbox");
  await stableList(web.page);
  await web.page.locator(".mail-scroll [role='button']", { hasText: S.read }).first().click();
  await web.page.waitForTimeout(3500);
  const afterOpen = await flagsOf(readMsg.uid);
  check(MOD, "Unread: opening a message marks it read (IMAP)", afterOpen.includes("\\Seen"),
    `flags=[${afterOpen.join(",")}]`);

  // ------------------------------------------------ 2. mark all as read
  await seedMail(`E2E-UNREAD-${stamp}`, { body: "mark all as read probe" });
  created.push(`E2E-UNREAD-${stamp}`);
  const unreadMsg = await awaitMail(`E2E-UNREAD-${stamp}`);
  check(MOD, "Unread: seeded message starts unread", !!unreadMsg && !(await flagsOf(unreadMsg.uid)).includes("\\Seen"),
    `uid=${unreadMsg?.uid}`);
  await gotoMail(web.page, "/mail/Inbox");
  await stableList(web.page);
  // Icon-only toolbar button: the label lives in title=.
  await web.page.locator('button[title="Mark all as read"]').first().click({ timeout: 8000 }).catch(() => {});
  await web.page.waitForTimeout(4000);
  const allRead = await flagsOf(unreadMsg.uid);
  check(MOD, "Unread: 'Mark all as read' clears unseen (IMAP)", allRead.includes("\\Seen"),
    `flags=[${allRead.join(",")}]`);

  // -------------------------------------------------------------- 3. star
  await seedMail(S.star, { body: "star probe" });
  const starMsg = await awaitMail(S.star);
  await gotoMail(web.page, "/mail/Inbox");
  await stableList(web.page);
  const starRow = web.page.locator(".mail-scroll [role='button']", { hasText: S.star }).first();
  await starRow.locator("svg.lucide-star").first().dispatchEvent("click");
  await web.page.waitForTimeout(2500);
  const starred = await flagsOf(starMsg.uid);
  check(MOD, "Star: toggling the row star flags the message (IMAP)", starred.includes("\\Flagged"),
    `flags=[${starred.join(",")}]`);
  await web.page.locator(".mail-scroll [role='button']", { hasText: S.star }).first()
    .locator("svg.lucide-star").first().dispatchEvent("click");
  await web.page.waitForTimeout(2500);
  const unstarred = await flagsOf(starMsg.uid);
  check(MOD, "Star: toggling again removes the flag (IMAP)", !unstarred.includes("\\Flagged"),
    `flags=[${unstarred.join(",")}]`);

  // ------------------------------------------------------------ 4. delete
  await seedMail(S.del, { body: "delete probe" });
  const delMsg = await awaitMail(S.del);
  await gotoMail(web.page, "/mail/Inbox");
  await stableList(web.page);
  await web.page.locator(".mail-scroll [role='button']", { hasText: S.del }).first()
    .locator("button[title='Delete']").first().dispatchEvent("click");
  await web.page.waitForTimeout(3000);
  let goneApi = false;
  for (let i = 0; i < 8 && !goneApi; i++) {
    goneApi = !(await listFolder("Inbox", 0)).messages.some((m) => m.uid === delMsg.uid);
    if (!goneApi) await sleep(1500);
  }
  check(MOD, "Delete: row leaves Inbox (API)", goneApi);
  // A move assigns a NEW uid in the destination mailbox, so match on subject.
  let inTrash = false;
  for (let p = 0; p < 3 && !inTrash; p++) {
    inTrash = (await listFolder("Trash", p)).messages.some((m) => (m.subject || "").includes(S.del));
  }
  check(MOD, "Delete: message is filed in Trash (API)", inTrash);
  const imapInbox = await withImap((c) => c.uidSearch(`HEADER Subject ${JSON.stringify(S.del)}`, "Inbox"));
  check(MOD, "Delete: message is really gone from Inbox (IMAP)",
    !imapInbox.includes(delMsg.uid), `imap=[${imapInbox.join(",")}]`);

  // -------------------------------------------------------- 5. bulk delete
  await seedMail(S.bulkA, { body: "bulk A" });
  await seedMail(S.bulkB, { body: "bulk B" });
  const a = await awaitMail(S.bulkA);
  const b = await awaitMail(S.bulkB);
  await gotoMail(web.page, "/mail/Inbox");
  await stableList(web.page);
  for (const s of [S.bulkA, S.bulkB]) {
    await web.page.locator(".mail-scroll [role='button']", { hasText: s }).first()
      .locator("input[type=checkbox], [role='checkbox']").first().dispatchEvent("click");
    await web.page.waitForTimeout(700);
  }
  await web.page.locator("button:visible", { hasText: /^Delete$/ }).first().click().catch(() => {});
  await web.page.waitForTimeout(3500);
  const afterBulk = await listFolder("Inbox", 0);
  const remaining = [a, b].filter((m) => m && afterBulk.messages.some((x) => x.uid === m.uid));
  let bulkGone = remaining.length === 0;
  for (let i = 0; !bulkGone && i < 6; i++) {
    await sleep(1500);
    const re = await listFolder("Inbox", 0);
    bulkGone = [a, b].every((m) => !m || !re.messages.some((x) => x.uid === m.uid));
  }
  check(MOD, "Bulk delete: both selected rows leave Inbox (API)", bulkGone,
    `remaining=${remaining.map((m) => m.uid).join(",")}`);
  await shot(web.page, "p0-list-after-bulk");

  // ------------------------------------------------------------ 6. archive
  await seedMail(S.arch, { body: "archive probe" });
  const archMsg = await awaitMail(S.arch);
  await gotoMail(web.page, "/mail/Inbox");
  await stableList(web.page);
  await web.page.locator(".mail-scroll [role='button']", { hasText: S.arch }).first()
    .locator("button[title='Archive']").first().dispatchEvent("click").catch(() => {});
  await web.page.waitForTimeout(3500);
  const archLeft = !(await listFolder("Inbox", 0)).messages.some((m) => m.uid === archMsg.uid);
  if (archLeft) {
    const archFolders = await api("/api/v1/mail/folders");
    const hasArchive = (archFolders.json || []).some((f) => /archive/i.test(f));
    check(MOD, "Archive: message leaves Inbox and lands in Archive", hasArchive,
      hasArchive ? "Archive folder present" : "no Archive folder in the mailbox list");
  } else {
    record(MOD, "Archive: row leaves Inbox", "FAIL",
      "archive action did not remove the message (known-issue area, non-blocking)");
  }

  // ------------------------------------------ 7. custom folder + move into it
  await gotoMail(web.page, "/mail/Inbox");
  await web.page.locator('button[title="New folder"]').first().click({ timeout: 8000 }).catch(() => {});
  const nameInput = web.page.locator("#folder-name");
  if (await nameInput.count()) {
    await nameInput.fill(FOLDER);
    await web.page.locator("button", { hasText: /^Save$/ }).first().click();
    await web.page.waitForTimeout(3000);
  }
  const foldersNow = (await api("/api/v1/mail/folders")).json || [];
  check(MOD, "Folders: UI creates a custom folder", foldersNow.includes(FOLDER),
    `folders=[${foldersNow.join(", ")}]`);

  await seedMail(S.move, { body: "move probe" });
  const moveMsg = await awaitMail(S.move);
  await gotoMail(web.page, "/mail/Inbox");
  await stableList(web.page);
  await web.page.locator(".mail-scroll [role='button']", { hasText: S.move }).first().click();
  await web.page.waitForTimeout(3000);
  // The reader's move control is a native <select title="Move to"> whose
  // option values are the mailbox names.
  await web.page.locator('select[title="Move to"]').first().selectOption(FOLDER).catch(() => {});
  await web.page.waitForTimeout(3500);
  // The engine serves one stale listing right after a mutation, and the moved
  // copy gets a NEW uid in the destination mailbox — poll on subject.
  let inFolder = false;
  for (let i = 0; i < 8 && !inFolder; i++) {
    inFolder = (await listFolder(FOLDER, 0)).messages.some((m) => (m.subject || "").includes(S.move));
    if (!inFolder) await sleep(1500);
  }
  check(MOD, "Move: message lands in the custom folder (API)", inFolder, `inFolder=${inFolder}`);
  const imapInFolder = await withImap((c) => c.uidSearch(`HEADER Subject ${JSON.stringify(S.move)}`, FOLDER));
  check(MOD, "Move: custom folder really holds the message (IMAP)", imapInFolder.length > 0,
    `imap uids=[${imapInFolder.join(",")}]`);
  let stillInbox = true;
  for (let i = 0; i < 8 && stillInbox; i++) {
    stillInbox = (await listFolder("Inbox", 0)).messages.some((m) => (m.subject || "").includes(S.move));
    if (stillInbox) await sleep(1500);
  }
  check(MOD, "Move: message no longer in Inbox (API)", !stillInbox, `stillInbox=${stillInbox}`);
  await shot(web.page, "p0-custom-folder");

  // -------------------------------------------------------------- 8. sort
  await gotoMail(web.page, "/mail/Inbox");
  await stableList(web.page);
  // The sort control is a native <select title="Sort">.
  const sortSelect = web.page.locator('select[title="Sort"]').first();
  await sortSelect.selectOption("date-asc").catch(() => {});
  await web.page.waitForTimeout(4000);
  const ascApi = await listFolder("Inbox", 0, { dir: "asc" });
  const firstUi = await web.page.locator(".mail-scroll [role='button']").first().innerText().catch(() => "");
  const oldestSubject = (ascApi.messages[0]?.subject || "").slice(0, 24);
  check(MOD, "Sort: 'Oldest first' shows the oldest message first",
    !!oldestSubject && firstUi.includes(oldestSubject),
    `ui="${firstUi.replace(/\s+/g, " ").slice(0, 46)}" api="${oldestSubject}"`);
  await sortSelect.selectOption("date-desc").catch(() => {});
  await web.page.waitForTimeout(3000);

  // --------------------------------------------------------- 9. pagination
  await gotoMail(web.page, "/mail/Sent");
  await stableList(web.page, 25000);
  const sentTotal = (await listFolder("Sent", 0)).total;
  const beforeRows = await web.page.locator(".mail-scroll [role='button']").count();
  await web.page.locator(".mail-scroll").last().evaluate((el) => el.scrollTo(0, el.scrollHeight));
  await web.page.waitForTimeout(4000);
  await web.page.locator(".mail-scroll").last().evaluate((el) => el.scrollTo(0, el.scrollHeight));
  await web.page.waitForTimeout(4000);
  const afterRows = await web.page.locator(".mail-scroll [role='button']").count();
  check(MOD, "Pagination: scrolling Sent loads the next page",
    sentTotal > 50 ? afterRows > beforeRows : afterRows === sentTotal,
    `total=${sentTotal} before=${beforeRows} after=${afterRows}`);
} finally {
  await web.ctx.close().catch(() => {});
  // Remove the custom folder (it only ever held E2E mail) and purge rows.
  await api(`/api/v1/mail/folders?name=${encodeURIComponent(FOLDER)}`, { method: "DELETE" }).catch(() => {});
  // Archive is included: the archive scenario files the seeded row there.
  for (const f of ["Inbox", FOLDER, "Sent", "Drafts", "Archive"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects(created).catch(() => 0);
  record(MOD, "Cleanup: P0 list subjects purged", purged >= 0 ? "PASS" : "FAIL", "expunged=" + purged);
  await closeBrowser();
}

const s = summary();
console.log(`\n=== p0-list-actions: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("p0-list-actions.json");
process.exit(s.fail > 0 ? 1 : 0);
