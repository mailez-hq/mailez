// Times the list API so a caching change shows up as a dropped per-request
// cost. Pass several bases to interleave them, which cancels machine-load
// drift between the runs:
//   LIST_BASES=http://127.0.0.1:8081,http://127.0.0.1:8091 node qa/_debug-list-timing.mjs
import { api, cookie } from "./lib.mjs";

const bases = (process.env.LIST_BASES || process.env.LIST_BASE || "http://127.0.0.1:8083").split(",");
const folder = process.env.LIST_FOLDER || "Trash";
const rounds = Number(process.env.LIST_ROUNDS || 5);

const median = (xs) => {
  const s = [...xs].sort((a, b) => a - b);
  return s.length % 2 ? s[(s.length - 1) / 2] : Math.round((s[s.length / 2 - 1] + s[s.length / 2]) / 2);
};

await cookie();
const samples = new Map(bases.map((b) => [b, []]));
for (let i = 0; i < rounds; i++) {
  for (const base of bases) {
    const t0 = Date.now();
    const r = await api(`/api/v1/mail/messages?folder=${folder}&page=0`, { base });
    const ms = Date.now() - t0;
    samples.get(base).push(ms);
    console.log(`#${i + 1} ${base} ${ms}ms status=${r.status} rows=${(r.json || []).length} total=${r.total}`);
  }
}
for (const base of bases) {
  const xs = samples.get(base);
  console.log(`MEDIAN ${base} ${median(xs)}ms over ${xs.length} rounds (${xs.join(", ")})`);
}
process.exit(0);
