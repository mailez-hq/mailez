import { apiLogin } from "./helper.mjs";

const BASE = "http://127.0.0.1:8083";
const cookie = await apiLogin();
const c = `${cookie.name}=${cookie.value}`;

// Replicate matrix scenario 2 exactly: send a mail, wait for it to appear,
// then move it to Trash. 5 rounds, log every move/list status.
for (let round = 1; round <= 5; round++) {
  const subj = `E2E-DEL-${Date.now()}-${round}`;
  const send = await fetch(`${BASE}/api/v1/mail/send`, {
    method: "POST",
    headers: { "Content-Type": "application/json", cookie: c },
    body: JSON.stringify({ to: ["admin@example.com"], subject: subj, text_body: "matrix seed body" }),
  });
  // scenario 2 waits for the row to appear (variable latency), then stableList
  await new Promise((r) => setTimeout(r, 7000));
  const list = await fetch(`${BASE}/api/v1/mail/messages?folder=Inbox&page=0&sort=date&dir=desc`, { headers: { cookie: c } });
  const rows = await list.json().catch(() => []);
  const row = (Array.isArray(rows) ? rows : []).find((m) => m.subject === subj);
  if (!row) {
    console.log(`round ${round}: send=${send.status} row NOT FOUND in list (list=${list.status}, rows=${Array.isArray(rows) ? rows.length : "?"})`);
    continue;
  }
  const preview = row.preview ? "yes" : "no";
  const res = await fetch(`${BASE}/api/v1/mail/move`, {
    method: "POST",
    headers: { "Content-Type": "application/json", cookie: c },
    body: JSON.stringify({ folder: "Inbox", uids: [row.uid], destination: "Trash" }),
  });
  console.log(`round ${round}: send=${send.status} move=${res.status} preview=${preview} subj=${subj}`);
  res.body?.cancel?.();
  await new Promise((r) => setTimeout(r, 2500));
}
