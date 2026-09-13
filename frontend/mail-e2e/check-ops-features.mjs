import { apiLogin } from "./helper.mjs";

const B = "http://127.0.0.1:8083"; // webmail origin (rewrites well-known to backend)
const A = "http://127.0.0.1:8083"; // admin API via webmail proxy? admin is separate; use webmail origin for /api/v1 too (same backend)
const c = await apiLogin();
const cookie = `${c.name}=${c.value}`;
const j = async (url) => (await fetch(url, { headers: { cookie } })).json();

// P0-1 health
const h = await j(`${A}/api/v1/admin/health`);
const dom = (h.domains || [])[0];
console.log("[health] hostname:", h.hostname, "| domains:", (h.domains || []).length,
  "| first:", dom?.domain, "items:", dom?.items.map((i) => `${i.id}:${i.status}`).join(","));
console.log("[health] system:", (h.system || []).map((i) => `${i.id}:${i.status}`).join(", "));

// P0-2 DNS wizard
const w = await j(`${A}/api/v1/domains/example.com/dns-records`);
console.log("[wizard]", w.domain, "hostname:", w.hostname, "records:", w.records.length,
  "|", w.records.map((r) => `${r.type}/${r.id}:${r.status}`).join(" "));

// P1-4 TOTP status
const t = await j(`${A}/api/v1/me/totp`);
console.log("[totp] enabled:", t.enabled, "| secret len:", (t.secret || "").length, "| otpauth:", (t.otpauth || "").slice(0, 40));

// P2 traffic
const tr = await j(`${A}/api/v1/admin/traffic?days=7`);
console.log("[traffic] sampling:", tr.sampling, "| days:", (tr.days || []).length, "| queue:", tr.latest_queue_depth);

// P0-2 public endpoints through the webmail origin (Next rewrite -> backend)
const moz = await fetch(`${B}/mail/config-v1.1.xml`);
const mozBody = await moz.text();
console.log("[autoconfig]", moz.status, moz.headers.get("content-type"), "| imap host:",
  /<hostname>([^<]+)<\/hostname>/.exec(mozBody)?.[1], "| port:", /<port>(\d+)<\/port>/.exec(mozBody)?.[1]);

const wk = await fetch(`${B}/.well-known/autoconfig/mail/config-v1.1.xml`);
console.log("[well-known autoconfig]", wk.status, (await wk.text()).includes("incomingServer") ? "xml ok" : "BAD");

const sts = await fetch(`${B}/.well-known/mta-sts.txt`);
console.log("[mta-sts]", sts.status, JSON.stringify((await sts.text()).replace(/\n/g, " | ")));

const ms = await fetch(`${B}/stack/autoconfig/mozilla`); // direct stack route (secret-gated in prod, empty locally)
console.log("[stack mozilla]", ms.status);
