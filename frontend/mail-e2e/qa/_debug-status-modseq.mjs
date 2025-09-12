// STATUS HIGHESTMODSEQ must report the mailbox version and advance on any
// change, including a flag-only edit — that is what the push watcher now
// baselines on.
import { ImapClient, check } from "./lib.mjs";

const c = await ImapClient.connect();
const modseqOf = async (folder) => {
  const r = await c.cmd(`STATUS "${folder}" (HIGHESTMODSEQ)`);
  const m = /HIGHESTMODSEQ (\d+)/i.exec(r.lines.find((l) => /HIGHESTMODSEQ/i.test(l)) || "");
  return m ? Number(m[1]) : null;
};
try {
  await c.login();
  // Cost comparison: the watcher's version-only STATUS against the counter
  // shape it replaced (the counters force a whole-mailbox walk).
  for (let i = 0; i < 3; i++) {
    let t0 = Date.now();
    await c.cmd('STATUS "Trash" (HIGHESTMODSEQ)');
    const cheap = Date.now() - t0;
    t0 = Date.now();
    await c.cmd('STATUS "Trash" (MESSAGES UNSEEN UIDNEXT)');
    console.log(`  version-only STATUS ${cheap}ms | counters STATUS ${Date.now() - t0}ms`);
  }
  const before = await modseqOf("Inbox");
  check("status-modseq", "STATUS reports HIGHESTMODSEQ", before !== null, `before=${before}`);

  // A flag-only change on some message must advance the version.
  const sel = await c.select("Inbox");
  const uid = Number((/^\* SEARCH (.+)$/m.exec((await c.cmd("UID SEARCH ALL")).lines.join("\n")) || [])[1]?.split(" ").pop());
  check("status-modseq", "found a message to flag", Number.isFinite(uid), `uid=${uid} exists=${sel.exists}`);
  const flagged = await c.cmd(`UID STORE ${uid} +FLAGS (\\Flagged)`);
  check("status-modseq", "STORE +FLAGS succeeded", flagged.ok, flagged.status);

  const after = await modseqOf("Inbox");
  check("status-modseq", "a flag-only change advances the version", after > before, `${before} -> ${after}`);

  await c.cmd(`UID STORE ${uid} -FLAGS (\\Flagged)`);
} finally {
  await c.logout();
}
process.exit(0);
