import { apiLogin } from "./helper.mjs";

const BASE = "http://127.0.0.1:8083";
const cookie = await apiLogin();
const c = `${cookie.name}=${cookie.value}`;
let folders = 0, foldersErr = 0, unseen = 0, unseenErr = 0, listErr = 0;
const t0 = Date.now();
// 90 seconds of the same polling profile the app produces (folders + unseen
// every ~1.2s), deliberately WITHOUT the messages/preview endpoint.
while (Date.now() - t0 < 90_000) {
  try {
    const a = await fetch(`${BASE}/api/v1/mail/folders`, { headers: { cookie: c } });
    if (a.status === 200) folders++; else { foldersErr++; a.body?.cancel?.(); }
  } catch { foldersErr++; }
  try {
    const b = await fetch(`${BASE}/api/v1/mail/unseen`, { headers: { cookie: c } });
    if (b.status === 200) unseen++; else { unseenErr++; b.body?.cancel?.(); }
  } catch { unseenErr++; }
  await new Promise((r) => setTimeout(r, 600));
}
console.log(`folders ok=${folders} err=${foldersErr} | unseen ok=${unseen} err=${unseenErr}`);
