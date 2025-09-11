// Phase 0 — environment smoke. Confirms the three surfaces (REST API, webmail
// UI, admin UI) plus the mail engine (IMAP + delivery) are usable before the
// functional suites run. Seeded mail is E2E-prefixed and moved to Trash at the
// end.
//
// Run:  cd frontend && node mail-e2e/qa/phase0.mjs
import {
  ADMIN_BASE, USER, api, awaitMail, awaitRow, check, cleanupFolder, closeBrowser,
  dumpResults, gotoMail, listFolder, newContext, record, seedMail, shot, sleep, stableList,
  summary, withImap, ImapClient, purgeSubjects,
} from "./lib.mjs";

const MOD = "P0-smoke";
const subject = `E2E-SMOKE-${Date.now()}`;
let seeded = null;

try {
  // ---------------------------------------------------------------- 1. API
  const me = await api("/api/v1/sso/me");
  check(MOD, "REST: session valid after /sso/login", me.status === 200 && me.json?.email === USER.email,
    `status=${me.status} email=${me.json?.email}`);

  const folders = await api("/api/v1/mail/folders");
  const folderNames = Array.isArray(folders.json) ? folders.json : [];
  check(MOD, "REST: /mail/folders returns the mailbox list",
    folders.status === 200 && folderNames.some((f) => /^inbox$/i.test(f)),
    `status=${folders.status} folders=[${folderNames.join(", ")}]`);

  const unseen = await api("/api/v1/mail/unseen");
  check(MOD, "REST: /mail/unseen returns per-folder counters",
    unseen.status === 200 && !!unseen.json && typeof unseen.json === "object",
    `status=${unseen.status} body=${JSON.stringify(unseen.json)?.slice(0, 120)}`);

  const inbox = await listFolder("Inbox", 0);
  check(MOD, "REST: /mail/messages returns a page + total",
    inbox.status === 200 && inbox.messages.length > 0 && inbox.total > 0,
    `status=${inbox.status} rows=${inbox.messages.length} total=${inbox.total}`);

  // --------------------------------------------------------------- 2. IMAP
  const imapFacts = await withImap(async (c) => {
    const status = await c.status("Inbox");
    const mailboxes = await c.listMailboxes();
    const sel = await c.select("Inbox");
    return { status, mailboxes, sel };
  });
  check(MOD, "IMAP: LOGIN + STATUS Inbox works", imapFacts.status.ok && imapFacts.status.messages !== null,
    `messages=${imapFacts.status.messages} unseen=${imapFacts.status.unseen}`);
  check(MOD, "IMAP: mailbox count matches REST total",
    imapFacts.status.messages === inbox.total,
    `imap=${imapFacts.status.messages} api=${inbox.total}`);
  check(MOD, "IMAP: LIST exposes the standard folders",
    ["INBOX", "SENT", "DRAFTS", "TRASH", "JUNK"].every((n) => imapFacts.mailboxes.some((m) => m.toUpperCase() === n)),
    `mailboxes=[${imapFacts.mailboxes.join(", ")}]`);

  // ------------------------------------------------------------- 3. webmail
  const web = await newContext({ locale: "en" });
  try {
    await gotoMail(web.page, "/home");
    const accountVisible = await web.page
      .getByText(USER.email, { exact: false })
      .first()
      .waitFor({ state: "visible", timeout: 20000 })
      .then(() => true)
      .catch(() => false);
    check(MOD, "Webmail: /home renders the signed-in shell", accountVisible,
      `account chip visible=${accountVisible} consoleErrors=${web.logs.length}`);

    await gotoMail(web.page, "/mail/Inbox");
    const listVisible = await web.page.locator(".mail-scroll").first().isVisible().catch(() => false);
    const rows = await web.page.locator(".mail-scroll [role='button']").count();
    check(MOD, "Webmail: inbox list renders rows", listVisible && rows > 0, `rows=${rows}`);
    await shot(web.page, "phase0-inbox");

    // ------------------------------------------------- 4. delivery round-trip
    await seedMail(subject);
    const found = await awaitMail(subject);
    seeded = found;
    check(MOD, "Mail: self-addressed send lands in Inbox (API)", !!found,
      found ? `uid=${found.uid} subject=${found.subject}` : "not found within 60s");

    if (found) {
      const imapHit = await withImap(async (c) => c.uidSearch(`HEADER Subject ${JSON.stringify(subject)}`));
      check(MOD, "Mail: delivered message is visible over IMAP",
        imapHit.includes(found.uid),
        `imap uids=[${imapHit.join(",")}] api uid=${found.uid}`);

      await stableList(web.page);
      const inUi = await awaitRow(web.page, subject, 45000);
      check(MOD, "Mail: delivered message appears in the webmail list", inUi);
      await shot(web.page, "phase0-delivered");

      // ------------------------------------------------ 5. cleanup round-trip
      await api("/api/v1/mail/move", {
        method: "POST",
        body: { folder: "Inbox", uids: [found.uid], destination: "Trash" },
      });
      let gone = false;
      for (let i = 0; i < 8 && !gone; i++) {
        const page0 = await listFolder("Inbox", 0);
        gone = !page0.messages.some((m) => m.uid === found.uid);
        if (!gone) await sleep(1500);
      }
      check(MOD, "Mail: moved message leaves Inbox (API)", gone);
      const imapGone = await withImap(async (c) => c.uidSearch(`HEADER Subject ${JSON.stringify(subject)}`));
      check(MOD, "Mail: moved message leaves Inbox (IMAP)", !imapGone.includes(found.uid),
        `imap uids=[${imapGone.join(",")}]`);
    }
  } finally {
    await web.ctx.close();
  }

  // --------------------------------------------------------------- 6. admin
  const admin = await newContext({ locale: "en" });
  try {
    let status = "?";
    try {
      const resp = await admin.page.goto(`${ADMIN_BASE}/`, { waitUntil: "domcontentloaded" });
      status = resp ? resp.status() : "?";
    } catch (e) {
      status = `ERR ${e.message}`;
    }
    await admin.page.waitForTimeout(3000);
    const body = (await admin.page.locator("body").innerText().catch(() => "")) || "";
    const broken = /Application error|Internal Server Error|502 Bad Gateway/i.test(body);
    check(MOD, "Admin: console boots without an error page",
      !broken && body.trim().length > 40,
      `http=${status} bodyChars=${body.trim().length} :: ${body.trim().slice(0, 60).replace(/\s+/g, " ")}`);
    await shot(admin.page, "phase0-admin");
  } finally {
    await admin.ctx.close();
  }

  // --------------------------------------------------------------- 7. engines
  const im2 = await ImapClient.connect().catch((e) => e);
  check(MOD, "IMAP: a second connection is still accepted (engine-lb)",
    im2 instanceof ImapClient, im2 instanceof ImapClient ? "connected" : String(im2.message));
  if (im2 instanceof ImapClient) await im2.logout();
} finally {
  // Cleanup: only E2E-* rows leave Inbox, and only into Trash.
  await cleanupFolder("Inbox").catch((e) => console.log("cleanup failed:", e.message));
  const purged = await purgeSubjects([subject]).catch(() => 0);
  const left = await listFolder("Inbox", 0).then((r) => r.messages.filter((m) => /^E2E-/.test(m.subject || "")));
  record("P0-smoke", "Cleanup: no E2E-* rows left in Inbox", left.length === 0 ? "PASS" : "FAIL",
    "remaining=" + left.length);
  record("P0-smoke", "Cleanup: seeded subject purged from all folders (IMAP)",
    purged > 0 ? "PASS" : "FAIL", `messages expunged=${purged}`);
  await closeBrowser();
}

const s = summary();
console.log(`\n=== phase0: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("phase0.json");
process.exit(s.fail > 0 ? 1 : 0);
