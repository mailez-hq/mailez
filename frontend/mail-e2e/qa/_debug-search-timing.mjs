// The webmail search goes through the backend's structured query parser, so
// this measures the shapes users actually type: a subject/from filter (header
// conditions, now answered from the cached header block) and a bare keyword
// (TEXT, which still has to read the messages).
import { api, cookie } from "./lib.mjs";

const folder = process.env.SEARCH_FOLDER || "Trash";
await cookie();

const shapes = [
  ["subject: report", "subject:report"],
  ["from: example.com", "from:example.com"],
  ["keyword (headers hit)", "E2E"],
  ["keyword (rare/absent)", "zzz-no-such-token-9f3c"],
  ["keyword (body only)", process.env.SEARCH_BODY_TERM || "message"],
];
for (const [label, q] of shapes) {
  for (let i = 0; i < 2; i++) {
    const t0 = Date.now();
    const r = await api(`/api/v1/mail/search?folder=${folder}&q=${encodeURIComponent(q)}`);
    const rows = Array.isArray(r.json) ? r.json.length : (r.json?.data?.length ?? "?");
    console.log(`${label.padEnd(18)} ${String(Date.now() - t0).padStart(5)}ms  status=${r.status} hits=${rows}`);
  }
}
process.exit(0);
