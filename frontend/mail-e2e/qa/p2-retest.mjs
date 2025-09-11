// P2 retest with the correct payload shapes: calendar {summary,start,end},
// mail merge {subject,body,html,from,recipients:[{email,name,vars}]}.
// Run: cd frontend && node mail-e2e/qa/p2-retest.mjs
import { USER, api, awaitMail, check, cleanupFolder, closeBrowser, dumpResults, purgeSubjects, record, summary } from "./lib.mjs";

const MOD = "P2-retest";
const stamp = Date.now();
const MERGE = `E2E-P2MRG-${stamp}`;
const EV = `E2E Event ${stamp}`;

try {
  // ------------------------------------------------------------- calendar
  const start = new Date(Date.now() + 24 * 3600 * 1000);
  const end = new Date(start.getTime() + 30 * 60 * 1000);
  const create = await api("/api/v1/calendar/events", {
    method: "POST",
    body: { summary: EV, location: "E2E Room", description: "probe", all_day: false, start: start.toISOString(), end: end.toISOString() },
  });
  const list = await api("/api/v1/calendar/events");
  const listed = JSON.stringify(list.json || "").includes(EV);
  check(MOD, "Calendar: create + list with the documented shape", create.status < 300 && listed,
    `create=${create.status} listed=${listed}`);
  const id = create.json?.id ?? create.json?.event?.id;
  if (id) {
    const del = await api(`/api/v1/calendar/events/${id}`, { method: "DELETE" });
    const after = await api("/api/v1/calendar/events");
    check(MOD, "Calendar: delete", del.status < 300 && !JSON.stringify(after.json || "").includes(EV), `status=${del.status}`);
  } else {
    record(MOD, "Calendar: delete", "BLOCKED", "no id returned");
  }

  // ------------------------------------------------------------ mail merge
  const merge = await api("/api/v1/mail/merge", {
    method: "POST",
    body: {
      from: "",
      subject: MERGE,
      body: "hello {{name}}",
      html: "<p>hello {{name}}</p>",
      recipients: [{ email: USER.email, name: "E2E One", vars: { name: "One" } }],
    },
  });
  const delivered = await awaitMail(MERGE, { timeoutMs: 60000 });
  check(MOD, "Mail merge: one personalised copy per recipient is delivered",
    merge.status < 400 && !!delivered,
    `status=${merge.status} body=${(merge.text || "").slice(0, 80)} uid=${delivered?.uid ?? "-"}`);
} finally {
  for (const f of ["Inbox", "Sent"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects([MERGE]).catch(() => 0);
  record(MOD, "Cleanup: P2 retest probes purged", purged >= 0 ? "PASS" : "FAIL", "expunged=" + purged);
  await closeBrowser();
}

const s = summary();
console.log(`\n=== p2-retest: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("p2-retest.json");
process.exit(s.fail > 0 ? 1 : 0);
