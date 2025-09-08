import { apiLogin } from "./helper.mjs";
const BASE = "http://127.0.0.1:8083";

// One-time cleanup: move every E2E-* test mail out of Inbox so the matrix
// runs against a quiet mailbox (SSE churn from stale test data was making
// every interaction racy).
const cookie = await apiLogin();
const c = `${cookie.name}=${cookie.value}`;
let moved = 0;
for (let page = 0; page < 10; page++) {
  const list = await fetch(`${BASE}/api/v1/mail/messages?folder=Inbox&page=${page}&sort=date&dir=desc`, {
    headers: { cookie: c },
  }).then((r) => r.json());
  const msgs = Array.isArray(list) ? list : [];
  const targets = msgs.filter((m) => /^E2E-/.test(m.subject || ""));
  if (!targets.length) break;
  const uids = targets.map((m) => m.uid);
  const res = await fetch(`${BASE}/api/v1/mail/move`, {
    method: "POST",
    headers: { "Content-Type": "application/json", cookie: c },
    body: JSON.stringify({ folder: "Inbox", uids, destination: "Trash" }),
  });
  console.log(`page ${page}: moved ${uids.length} -> ${res.status}`);
  moved += uids.length;
  if (msgs.length <= targets.length) break;
  await new Promise((r) => setTimeout(r, 800));
}
console.log("total moved:", moved);
