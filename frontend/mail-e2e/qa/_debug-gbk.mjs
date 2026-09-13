// Reproduces the reported 126.com message: an RFC 2047 GBK subject and a
// multipart/alternative body whose parts are base64 over GBK. Appended over
// IMAP (verbatim payloads from the report), then checked through the API the
// webmail list and reader use.
import { api, check, USER, withImap } from "./lib.mjs";

// The send API cannot carry arbitrary headers, so the fixture is APPENDed over
// IMAP (same helper as qa/p1-recall-unsub-forward.mjs).
async function appendRaw(mailbox, raw) {
  return withImap(async (c) => {
    const tag = `a${Date.now() % 100000}`;
    const bytes = Buffer.from(raw, "utf8");
    c.sock.write(`${tag} APPEND "${mailbox}" (\\Seen) {${bytes.length}}\r\n`);
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

const MOD = "gbk";
const stamp = Date.now();
const SUBJ_WORD = "=?GBK?B?UmU6suLK1NK7z8K21M3it6LLzdPKvP4=?=";
const SUBJ_TEXT = "Re:测试一下对外发送邮件";
// The report's text part, base64 over GBK, joined back into one line.
const TEXT_B64 =
  "Cgq908rV08q8/sTYo7/T0M7KzOLC8KO/CgrU2iAyMDI2LTA5LTE0IDA1OjA4OjMzo6xhZG1pbkBt" +
  "YWlsZXouY24g0LS1wKO6CgrV4rj2ysfWrsew0/a1vbHIvc+087XEzsrM4qGj";
// base64 over GBK → these exact bytes.
const TEXT_TEXT = "接收邮件呢？有问题吗？";
const MSGID = `e2e-gbk-${stamp}@126.com`;

// The fixture's subject is the encoded word itself (that is the point), so the
// E2E--prefix cleanup helpers do not apply: purge it by Message-ID instead.
async function purgeFixture(folders = ["Inbox", "Trash"]) {
  return withImap(async (c) => {
    let removed = 0;
    for (const folder of folders) {
      try {
        await c.select(folder, 60000);
        const r = await c.cmd('UID SEARCH HEADER Message-ID "e2e-gbk-"', 60000);
        const line = r.lines.find((l) => /^\* SEARCH/.test(l)) || "";
        const uids = line.replace(/^\* SEARCH\s*/, "").trim().split(/\s+/).filter(Boolean);
        if (!uids.length) continue;
        await c.cmd(`UID STORE ${uids.join(",")} +FLAGS (\\Deleted)`, 60000).catch(() => {});
        await c.cmd("EXPUNGE", 60000).catch(() => {});
        removed += uids.length;
      } catch {
        /* folder missing or nothing to do */
      }
    }
    return removed;
  });
}

const raw =
  `Received: from 117.135.210.8 by mail.mailez.cn (mailezine) with SMTP id e2e-gbk-${stamp};\r\n` +
  `\tMon, 14 Sep 2026 05:09:32 +0800\r\n` +
  `Date: Mon, 14 Sep 2026 05:09:29 +0800 (CST)\r\n` +
  `From: =?GBK?B?pe2/qKXtsuwg?= <gushing@126.com>\r\n` +
  `To: ${USER.email}\r\n` +
  `Subject: ${SUBJ_WORD}\r\n` +
  `Message-ID: <${MSGID}>\r\n` +
  `Mime-Version: 1.0\r\n` +
  `Content-Type: multipart/alternative; boundary="----=_Part_e2e_gbk_${stamp}"\r\n` +
  `\r\n` +
  `------=_Part_e2e_gbk_${stamp}\r\n` +
  `Content-Type: text/plain; charset=GBK\r\n` +
  `Content-Transfer-Encoding: base64\r\n\r\n` +
  TEXT_B64 + "\r\n" +
  `------=_Part_e2e_gbk_${stamp}\r\n` +
  `Content-Type: text/html; charset=GBK\r\n` +
  `Content-Transfer-Encoding: base64\r\n\r\n` +
  Buffer.from(`<p>${TEXT_TEXT}</p>`).toString("base64") + "\r\n" +
  `------=_Part_e2e_gbk_${stamp}--\r\n`;

try {
  try {
    const r = await appendRaw("Inbox", raw);
    check(MOD, "fixture appended over IMAP", String(r).includes("OK"), String(r).slice(0, 60));
  } catch (e) {
    check(MOD, "fixture appended over IMAP", false, e.message);
  }

  let row = null;
  for (let i = 0; i < 12 && !row; i++) {
    await new Promise((r) => setTimeout(r, 1500));
    const list = await api("/api/v1/mail/messages?folder=Inbox&page=0");
    row = (list.json || []).find((m) => (m.subject || "") === SUBJ_TEXT || (m.subject || "").includes("GBK?B?")) ?? null;
  }
  check(MOD, "list row subject is decoded", row?.subject === SUBJ_TEXT, `subject=${JSON.stringify(row?.subject)}`);

  if (row) {
    const detail = await api(`/api/v1/mail/message?folder=Inbox&uid=${row.uid}`);
    const body = detail.json?.text_body ?? "";
    check(MOD, "reader body is decoded, not replacement characters",
      body.includes(TEXT_TEXT) && !body.includes("\uFFFD"),
      `len=${body.length} snippet=${JSON.stringify(body.slice(0, 40))}`);
    check(MOD, "reader subject is decoded", detail.json?.subject === SUBJ_TEXT,
      `subject=${JSON.stringify(detail.json?.subject)}`);
    // The same bytes are unsearchable as raw data: the body is GBK behind
    // base64, so a TEXT search has to decode before matching.
    // Search is served from the per-node full-text index, which converges from
    // the change log (one sync interval), so poll rather than sampling once.
    let hit = false;
    let hits = 0;
    for (let i = 0; i < 12 && !hit; i++) {
      const found = await api(`/api/v1/mail/search?folder=Inbox&q=${encodeURIComponent(TEXT_TEXT)}`);
      hits = (found.json || []).length;
      hit = (found.json || []).some((m) => m.uid === row.uid);
      if (!hit) await new Promise((r) => setTimeout(r, 2000));
    }
    check(MOD, "search finds the text inside the GBK base64 body", hit, `hits=${hits} wantUid=${row.uid}`);
  } else {
    check(MOD, "reader body is decoded, not replacement characters", false, "no row to open");
    check(MOD, "reader subject is decoded", false, "no row to open");
    check(MOD, "search finds the text inside the GBK base64 body", false, "no row to open");
  }
} finally {
  const removed = await purgeFixture().catch(() => -1);
  console.log(`  cleanup: expunged ${removed} fixture message(s)`);
}
process.exit(0);
