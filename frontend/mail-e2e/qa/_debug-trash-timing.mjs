// Times SELECT/SEARCH on each mailbox to separate "slow" from "hung".
import { ImapClient, api } from "./lib.mjs";

const t = () => Date.now();
for (const folder of ["Inbox", "Sent", "Drafts", "Junk", "Trash"]) {
  const c = await ImapClient.connect();
  await c.login();
  const t0 = t();
  try {
    const sel = await c.select(folder, 120000);
    const t1 = t();
    const r = await c.cmd("UID SEARCH HEADER Subject \"E2E-DRAFT-1789137745857\"", 120000);
    const t2 = t();
    console.log(`${folder.padEnd(7)} SELECT=${t1 - t0}ms exists=${sel.exists} SEARCH=${t2 - t1}ms lines=${JSON.stringify(r.lines)}`);
  } catch (e) {
    console.log(`${folder.padEnd(7)} FAILED after ${t() - t0}ms: ${e.message}`);
  }
  await c.logout().catch(() => {});
}
for (const folder of ["Inbox", "Trash"]) {
  const t0 = t();
  const r = await api(`/api/v1/mail/messages?folder=${folder}&page=0`);
  console.log(`API ${folder.padEnd(7)} ${t() - t0}ms status=${r.status} rows=${(r.json || []).length} total=${r.total}`);
}
process.exit(0);
