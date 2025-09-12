// End-to-end check for report D3 after the watcher moved to the mailbox
// version: starring a message in another client must push a "mail" event to
// an open SSE stream (the counter-based watcher could never see it).
import { BASE, ImapClient, api, check, cleanupFolder, cookie, purgeSubjects, seedMail } from "./lib.mjs";

const MOD = "sse-flags";
const SUBJ = `E2E-SSEFLAG-${Date.now()}`;

// Collect SSE blocks in the background for the duration of the probe.
const blocks = [];
const c = await cookie();
const ac = new AbortController();
const stream = (async () => {
  const res = await fetch(`${BASE}/api/v1/events`, {
    headers: { cookie: c, accept: "text/event-stream" },
    signal: ac.signal,
  });
  const reader = res.body.getReader();
  const dec = new TextDecoder();
  let buf = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buf += dec.decode(value, { stream: true });
    let i;
    while ((i = buf.indexOf("\n\n")) >= 0) {
      const block = buf.slice(0, i);
      buf = buf.slice(i + 2);
      if (block.includes("event: mail")) blocks.push(block);
    }
  }
})().catch(() => {});

const mailBlocks = () => blocks.filter((b) => /"folders":\[[^\]]*Inbox/i.test(b));

try {
  await seedMail(SUBJ, { body: "sse flag probe" });
  let uid = null;
  for (let i = 0; i < 20 && !uid; i++) {
    await new Promise((r) => setTimeout(r, 1500));
    const r = await api("/api/v1/mail/messages?folder=Inbox&page=0");
    uid = (r.json || []).find((x) => x.subject === SUBJ)?.uid ?? null;
  }
  check(MOD, "seeded message is in Inbox", uid > 0, `uid=${uid}`);

  // The watcher's day job first: a delivery must push.
  const delivered = Date.now() + 40000;
  while (Date.now() < delivered && mailBlocks().length === 0) {
    await new Promise((r) => setTimeout(r, 1000));
  }
  check(MOD, "a delivery still pushes", mailBlocks().length > 0, `events=${mailBlocks().length}`);

  // Let the delivery-triggered pushes settle so the next event we see can only
  // be the star below.
  await new Promise((r) => setTimeout(r, 3000));
  const before = mailBlocks().length;

  const imap = await ImapClient.connect();
  try {
    await imap.login();
    await imap.select("Inbox");
    const r = await imap.cmd(`UID STORE ${uid} +FLAGS (\\Flagged)`);
    check(MOD, "star applied over IMAP", r.ok, r.status);
  } finally {
    await imap.logout();
  }

  const deadline = Date.now() + 40000;
  let seen = false;
  while (Date.now() < deadline && !seen) {
    await new Promise((r) => setTimeout(r, 1000));
    seen = mailBlocks().length > before;
  }
  check(MOD, "an open SSE stream is told about a flag-only change", seen,
    `events=${mailBlocks().length - before} (waited <=40s for the 20s watcher tick)`);
} finally {
  ac.abort();
  await stream;
  // Unflag through the API path used by the UI, then clean the probe message.
  const rows = await api("/api/v1/mail/messages?folder=Inbox&page=0").catch(() => ({ json: [] }));
  const row = (rows.json || []).find((x) => x.subject === SUBJ);
  if (row) {
    await api("/api/v1/mail/flag", {
      method: "POST",
      body: { folder: "Inbox", uid: row.uid, flag: "\\Flagged", value: false },
    }).catch(() => {});
  }
  await cleanupFolder("Inbox", [SUBJ]);
  await purgeSubjects([SUBJ]);
}
process.exit(0);
