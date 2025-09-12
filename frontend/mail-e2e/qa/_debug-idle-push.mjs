// Regression probe for the engine's snapshot memo: a client sitting in IDLE
// must still be told (EXISTS) when mail arrives. Selects INBOX, enters IDLE,
// sends itself a message through the API and waits for the untagged EXISTS.
import { ImapClient, api, check, cookie, purgeSubjects, seedMail } from "./lib.mjs";

const MOD = "idle-push";
const SUBJ = `E2E-IDLE-${Date.now()}`;

const c = await ImapClient.connect();
try {
  await c.login();
  const sel = await c.select("INBOX");
  const existsBefore = c.transcript.filter((l) => /EXISTS/.test(l)).length;

  // Enter IDLE without the helper's command/response pairing: the server
  // answers "+ idling" and then pushes untagged updates.
  const tag = `q${++c.tag}`;
  c.sock.write(`${tag} IDLE\r\n`);

  await new Promise((r) => setTimeout(r, 500));
  await seedMail(SUBJ, { body: "idle push body" });

  const deadline = Date.now() + 30000;
  let pushed = false;
  while (Date.now() < deadline && !pushed) {
    await new Promise((r) => setTimeout(r, 500));
    pushed = c.transcript.slice(-40).some((l) => /\* \d+ EXISTS/.test(l));
  }
  check(MOD, "IDLE receives EXISTS for mail delivered after SELECT", pushed, `before=${existsBefore}`);
  c.sock.write("DONE\r\n");
  await new Promise((r) => setTimeout(r, 500));
} finally {
  await c.logout();
  await api("/api/v1/mail/messages?folder=Inbox&page=0").catch(() => {});
  await purgeSubjects([SUBJ]);
}
process.exit(0);
