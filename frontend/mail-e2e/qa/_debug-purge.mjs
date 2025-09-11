// Temporary debug helper for purgeSubjects: prints raw IMAP SEARCH responses
// per folder for a subject given on the command line.
import { ImapClient, listFolder } from "./lib.mjs";

const subject = process.argv[2] || "E2E-DRAFT-1789137745857";
const c = await ImapClient.connect();
await c.login();
for (const folder of ["Inbox", "Drafts", "Sent", "Junk", "Trash"]) {
  try {
    const sel = await c.select(folder);
    const r = await c.cmd(`UID SEARCH HEADER Subject ${JSON.stringify(subject)}`);
    console.log(folder.padEnd(7), "selectOk=" + sel.ok, "exists=" + sel.exists, "raw=" + JSON.stringify(r.lines));
  } catch (e) {
    console.log(folder.padEnd(7), "ERROR", e.message);
  }
}
await c.logout();
const api = await listFolder("Trash", 0);
console.log("api Trash subjects with E2E-DRAFT:", api.messages.filter((m) => /E2E-DRAFT/.test(m.subject || "")).map((m) => m.uid + ":" + m.subject).join(" | "));
process.exit(0);
