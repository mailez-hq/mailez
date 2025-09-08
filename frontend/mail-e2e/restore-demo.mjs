import { apiLogin } from "./helper.mjs";
const BASE = "http://127.0.0.1:8083";

// Restore non-test mails from Trash back to Inbox (matrix scenarios deleted
// demo-seed rows alongside E2E junk).
const cookie = await apiLogin();
const c = `${cookie.name}=${cookie.value}`;
let restored = 0;
for (let page = 0; page < 10; page++) {
  const list = await fetch(`${BASE}/api/v1/mail/messages?folder=Trash&page=${page}&sort=date&dir=desc`, {
    headers: { cookie: c },
  }).then((r) => r.json());
  const targets = (Array.isArray(list) ? list : []).filter((m) => !/^E2E-/.test(m.subject || ""));
  if (!targets.length) break;
  const res = await fetch(`${BASE}/api/v1/mail/move`, {
    method: "POST",
    headers: { "Content-Type": "application/json", cookie: c },
    body: JSON.stringify({ folder: "Trash", uids: targets.map((m) => m.uid), destination: "Inbox" }),
  });
  console.log(`page ${page}: restore ${targets.length} -> ${res.status}`, targets.map((m) => m.subject).slice(0, 3));
  restored += targets.length;
  await new Promise((r) => setTimeout(r, 800));
}
console.log("total restored:", restored);
