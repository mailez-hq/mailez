import { apiLogin } from "./helper.mjs";

const BASE = "http://127.0.0.1:8083";
const cookie = await apiLogin();
const c = `${cookie.name}=${cookie.value}`;
let ok = 0, err = 0, lat = [];
const t0 = Date.now();
while (Date.now() - t0 < 90_000) {
  const t = Date.now();
  try {
    const r = await fetch(`${BASE}/api/v1/mail/messages?folder=Inbox&page=0&sort=date&dir=desc&conversation=1`, { headers: { cookie: c } });
    if (r.status === 200) { ok++; lat.push(Date.now() - t); await r.json(); } else { err++; r.body?.cancel?.(); }
  } catch { err++; }
  await new Promise((r) => setTimeout(r, 600));
}
lat.sort((a, b) => a - b);
console.log(`list(with preview) ok=${ok} err=${err} | p50=${lat[Math.floor(lat.length / 2)] || "-"}ms max=${lat[lat.length - 1] || "-"}ms`);
