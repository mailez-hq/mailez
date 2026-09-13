// Why is the GBK body not searchable? This appends one fixture with an ASCII
// marker next to the GBK text, so "indexed at all" and "matched after
// decoding" can be told apart.
import { api, check, USER, withImap } from "./lib.mjs";

const MOD = "gbk-search";
const stamp = Date.now();
const MSGID = `e2e-gbks-${stamp}@126.com`;
const TEXT_B64 =
  "Cgq908rV08q8/sTYo7/T0M7KzOLC8KO/CgrU2iAyMDI2LTA5LTE0IDA1OjA4OjMzo6xhZG1pbkBt" +
  "YWlsZXouY24g0LS1wKO6CgrV4rj2ysfWrsew0/a1vbHIvc+087XEzsrM4qGj";
const ASCII_MARK = `UNIQUEMARK${stamp}`;

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

const raw =
  `From: gushing@126.com\r\nTo: ${USER.email}\r\n` +
  `Subject: E2E-GBKS-${stamp}\r\n` +
  `Message-ID: <${MSGID}>\r\nMIME-Version: 1.0\r\n` +
  `Content-Type: multipart/alternative; boundary="b1"\r\n\r\n` +
  `--b1\r\nContent-Type: text/plain; charset=GBK\r\nContent-Transfer-Encoding: base64\r\n\r\n` +
  TEXT_B64 + "\r\n" +
  // A plain ASCII part: if this is searchable but the GBK part is not, the
  // body text reaches the index and only the charset decoding is at fault.
  `--b1\r\nContent-Type: text/plain; charset=us-ascii\r\n\r\n${ASCII_MARK}\r\n` +
  `--b1--\r\n`;

try {
  await appendRaw("Inbox", raw);
  await new Promise((r) => setTimeout(r, 10000)); // let the change-log tailer index it

  const probe = async (label, q) => {
    const r = await api(`/api/v1/mail/search?folder=Inbox&q=${encodeURIComponent(q)}`);
    const rows = r.json || [];
    console.log(`  ${label.padEnd(28)} q=${JSON.stringify(q).slice(0, 28).padEnd(30)} hits=${rows.length}`);
    return rows;
  };
  const marker = await probe("ASCII in body (raw match)", ASCII_MARK);
  const subj = await probe("subject marker", `E2E-GBKS-${stamp}`);
  const noPunct = await probe("GBK text, no punctuation", "接收邮件呢");
  const punct = await probe("GBK text with punctuation", "接收邮件呢？有问题吗？");
  const body = await probe("GBK word only", "接收邮件");
  await probe("CJK control (known demo mail)", "安全提醒");

  // Re-index the same message via a flag change (changelog update) and ask
  // again: if it becomes searchable, the first indexing pass was the problem.
  {
    const list = await api("/api/v1/mail/messages?folder=Inbox&page=0");
    const row = (list.json || []).find((m) => (m.subject || "").includes(`E2E-GBKS-${stamp}`));
    if (row) {
      const { ImapClient } = await import("./lib.mjs");
      const c = await ImapClient.connect();
      try {
        await c.login();
        await c.select("Inbox", 60000);
        await c.cmd(`UID STORE ${row.uid} +FLAGS (\\Flagged)`, 60000);
      } finally {
        await c.logout();
      }
      await new Promise((r) => setTimeout(r, 8000));
      await probe("after re-index: ASCII marker", ASCII_MARK);
      await probe("after re-index: GBK word", "接收邮件呢");
    }
  }

  // The same questions asked of the engine directly, bypassing the backend:
  // IMAP SEARCH TEXT over the mailbox.
  const imap = await import("./lib.mjs").then((m) => m.ImapClient.connect());
  try {
    await imap.login();
    await imap.select("Inbox", 60000);
    for (const [label, query] of [
      ["imap TEXT ascii marker", ASCII_MARK],
      ["imap TEXT gbk word", "接收邮件呢"],
      ["imap BODY gbk word", "接收邮件呢"],
      ["imap TEXT subject marker", `E2E-GBKS-${stamp}`],
    ]) {
      const cmd = label.startsWith("imap BODY") ? "BODY" : "TEXT";
      const r = await imap.cmd(`UID SEARCH ${cmd} ${JSON.stringify(query)}`, 60000);
      const line = r.lines.find((l) => /^\* SEARCH/.test(l)) || "";
      console.log(`  ${label.padEnd(28)} -> ${line.replace(/^\* SEARCH\s*/, "") || "(none)"}`);
    }
  } finally {
    await imap.logout();
  }

  check(MOD, "indexed at all (subject search)", subj.length > 0, `hits=${subj.length}`);
  check(MOD, "ASCII line inside the decoded part", marker.length > 0, `hits=${marker.length}`);
  check(MOD, "GBK text without punctuation", noPunct.length > 0, `hits=${noPunct.length}`);
  check(MOD, "GBK text with punctuation", punct.length > 0, `hits=${punct.length}`);
  check(MOD, "GBK word", body.length > 0, `hits=${body.length}`);
} finally {
  const n = await withImap(async (c) => {
    let removed = 0;
    for (const f of ["Inbox", "Trash"]) {
      try {
        await c.select(f, 60000);
        const r = await c.cmd(`UID SEARCH HEADER Message-ID "e2e-gbks-"`, 60000);
        const uids = (r.lines.find((l) => /^\* SEARCH/.test(l)) || "").replace(/^\* SEARCH\s*/, "").trim();
        if (!uids) continue;
        await c.cmd(`UID STORE ${uids.split(/\s+/).join(",")} +FLAGS (\\Deleted)`, 60000).catch(() => {});
        await c.cmd("EXPUNGE", 60000).catch(() => {});
        removed += uids.split(/\s+/).length;
      } catch {
        /* missing */
      }
    }
    return removed;
  }).catch(() => -1);
  console.log(`  cleanup: expunged ${n}`);
}
process.exit(0);
