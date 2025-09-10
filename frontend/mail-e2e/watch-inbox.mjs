import { apiLogin } from "./helper.mjs";

// Watch Inbox count every 30s for 20 minutes; log any drop to zero with a
// timestamp so the engine's state-rollback trigger can be correlated with
// mailezine logs.
const BASE = "http://127.0.0.1:8083";
const cookie = await apiLogin();
const c = `${cookie.name}=${cookie.value}`;
let prev = -1;
const t0 = Date.now();
while (Date.now() - t0 < 20 * 60_000) {
  try {
    const l = await fetch(`${BASE}/api/v1/mail/messages?folder=Inbox&page=0&sort=date&dir=desc`, { headers: { cookie: c } });
    const rows = await l.json().catch(() => null);
    const n = Array.isArray(rows) ? rows.length : `status ${l.status}`;
    if (n !== prev) {
      console.log(`[${new Date().toISOString()}] inbox=${n}`);
      prev = n;
    }
  } catch (e) {
    console.log(`[${new Date().toISOString()}] probe failed: ${e.message}`);
  }
  await new Promise((r) => setTimeout(r, 30_000));
}
console.log("watch done");
