// Diagnosis for the unsubscribe gap: is the header present in the stored raw
// message, does the list row carry it, does the detail carry it?
// Run: cd frontend && node mail-e2e/qa/_debug-unsub.mjs
import { USER, api, cleanupFolder, closeBrowser, listFolder, purgeSubjects, sleep, withImap } from "./lib.mjs";

const stamp = Date.now();
const SUBJ = `E2E-UNSDBG-${stamp}`;

const raw =
  `From: E2E List <list-${stamp}@example.com>\r\n` +
  `To: ${USER.email}\r\n` +
  `Subject: ${SUBJ}\r\n` +
  `Date: ${new Date().toUTCString()}\r\n` +
  `Message-ID: <uns-${stamp}@example.com>\r\n` +
  `List-Unsubscribe: <http://127.0.0.1:9/unsub?u=${stamp}>\r\n` +
  `List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n` +
  `\r\n` +
  `body\r\n`;

await withImap(async (c) => {
  const tag = `u${stamp % 100000}`;
  const bytes = Buffer.from(raw, "utf8");
  c.sock.write(`${tag} APPEND "Inbox" (\\Seen) {${bytes.length}}\r\n`);
  for (;;) if ((await c.nextLine()).startsWith("+")) break;
  c.sock.write(raw);
  c.sock.write("\r\n");
  for (;;) if ((await c.nextLine()).startsWith(`${tag} `)) break;
});
await sleep(3000);

const row = (await listFolder("Inbox", 0)).messages.find((m) => (m.subject || "").includes(SUBJ));
console.log("list row:", row ? `uid=${row.uid} category=${row.category} unsubscribe_url=${row.unsubscribe_url ?? "-"}` : "NOT FOUND");
if (row) {
  const detail = await api(`/api/v1/mail/message?folder=Inbox&uid=${row.uid}`);
  console.log("detail:", `status=${detail.status} unsubscribe_url=${detail.json?.unsubscribe_url ?? "-"} post=${detail.json?.unsubscribe_post}`);
  const rawMsg = await api(`/api/v1/mail/raw?folder=Inbox&uid=${row.uid}`);
  const body = rawMsg.json?.raw || rawMsg.text || "";
  console.log("raw has List-Unsubscribe:", /list-unsubscribe:/i.test(body), "| raw has -Post:", /list-unsubscribe-post:/i.test(body));
}
await cleanupFolder("Inbox");
console.log("cleanup expunged=", await purgeSubjects([SUBJ]));
await closeBrowser();
