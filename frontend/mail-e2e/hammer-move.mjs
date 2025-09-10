import { apiLogin } from "./helper.mjs";

const BASE = "http://127.0.0.1:8083";
const cookie = await apiLogin();
const c = `${cookie.name}=${cookie.value}`;

// Seed one canary mail, then move it Inbox->Trash->Inbox repeatedly and
// count how many moves fail while list calls keep succeeding.
const send = await fetch(`${BASE}/api/v1/mail/send`, {
  method: "POST",
  headers: { "Content-Type": "application/json", cookie: c },
  body: JSON.stringify({ to: ["admin@example.com"], subject: `E2E-MOVECANARY-${Date.now()}`, text_body: "canary" }),
});
console.log("seed send:", send.status);
await new Promise((r) => setTimeout(r, 6000));

let ok = 0, err = 0, listOk = 0, listErr = 0;
const statuses = [];
const t0 = Date.now();
let toTrash = true;
while (Date.now() - t0 < 60_000) {
  const list = await fetch(`${BASE}/api/v1/mail/messages?folder=Inbox&page=0&sort=date&dir=desc`, { headers: { cookie: c } });
  if (list.status === 200) { listOk++; } else { listErr++; }
  const rows = await list.json().catch(() => []);
  const row = (Array.isArray(rows) ? rows : []).find((m) => m.subject?.startsWith("E2E-MOVECANARY-"));
  if (!row) { await new Promise((r) => setTimeout(r, 1500)); continue; }
  const res = await fetch(`${BASE}/api/v1/mail/move`, {
    method: "POST",
    headers: { "Content-Type": "application/json", cookie: c },
    body: JSON.stringify({ folder: "Inbox", uids: [row.uid], destination: toTrash ? "Trash" : "Inbox" }),
  });
  if (res.status === 204) { ok++; toTrash = !toTrash; } else { err++; statuses.push(res.status); res.body?.cancel?.(); }
  await new Promise((r) => setTimeout(r, 1500));
}
console.log(`moves ok=${ok} err=${err} statuses=${JSON.stringify([...new Set(statuses)])} | list ok=${listOk} err=${listErr}`);
