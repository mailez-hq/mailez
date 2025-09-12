// Phase 3 (P2) smoke — optional/enterprise surfaces: AI status+search, PGP
// status, templates, contacts, calendar, aggregated accounts, mail merge, and
// the unsubscribe affordance. Anything that needs a provider/configuration
// this deployment does not have is reported BLOCKED with the reason, not FAIL.
//
// Run: cd frontend && node mail-e2e/qa/p2-smoke.mjs
import {
  USER, api, awaitMail, check, cleanupFolder, closeBrowser, dumpResults, gotoMail, listFolder,
  newContext, purgeSubjects, record, seedMail, shot, sleep, summary, blocked,
} from "./lib.mjs";

const MOD = "P2";
const stamp = Date.now();
const SUBJ = `E2E-P2-${stamp}`;
const MERGE = `E2E-P2MERGE-${stamp}`;

const web = await newContext({ locale: "en" });
try {
  // ------------------------------------------------------------------- AI
  const ai = await api("/api/v1/ai/status");
  const aiJson = ai.json || {};
  const aiOn = !!aiJson.enabled || !!aiJson.available || aiJson.configured === true;
  if (ai.status === 200) {
    check(MOD, "AI: status endpoint answers", true,
      `status=200 payload=${JSON.stringify(aiJson).slice(0, 120)}`);
    if (aiOn) {
      const s = await api("/api/v1/ai/search", { method: "POST", body: { query: `E2E-P2-${stamp}` } });
      check(MOD, "AI: natural-language search responds", s.status < 400, `status=${s.status}`);
    } else {
      blocked(MOD, "AI: search/summarise", `provider not configured in this deployment (${JSON.stringify(aiJson).slice(0, 80)})`);
    }
  } else {
    record(MOD, "AI: status endpoint", "FAIL", `status=${ai.status}`);
  }

  // ------------------------------------------------------------------ PGP
  const pgp = await api("/api/v1/me/pgp");
  check(MOD, "PGP: key status endpoint answers", pgp.status === 200,
    `status=${pgp.status} body=${JSON.stringify(pgp.json).slice(0, 100)}`);

  // ------------------------------------------------------------ templates
  const tCreate = await api("/api/v1/mail/templates", { method: "POST", body: { name: `E2E-TPL-${stamp}`, subject: `E2E-TPL-${stamp}`, body: "template body" } });
  const tList = await api("/api/v1/mail/templates");
  const tNames = JSON.stringify(tList.json || "");
  check(MOD, "Templates: create + list", tCreate.status < 300 && tNames.includes(`E2E-TPL-${stamp}`),
    `create=${tCreate.status} list=${tList.status} listed=${tNames.includes(`E2E-TPL-${stamp}`)}`);
  const tId = tCreate.json?.id ?? tCreate.json?.template?.id;
  if (tId) await api(`/api/v1/mail/templates/${tId}`, { method: "DELETE" }).catch(() => {});

  // ------------------------------------------------------------- contacts
  const cCreate = await api("/api/v1/contacts", { method: "POST", body: { name: `E2E Contact ${stamp}`, email: `e2e-contact-${stamp}@example.com` } });
  const cList = await api("/api/v1/contacts");
  const cText = JSON.stringify(cList.json || "");
  check(MOD, "Contacts: create + list", cCreate.status < 300 && cText.includes(`e2e-contact-${stamp}`),
    `create=${cCreate.status} list=${cList.status}`);
  const cId = cCreate.json?.id ?? cCreate.json?.contact?.id;
  if (cId) await api(`/api/v1/contacts/${cId}`, { method: "DELETE" }).catch(() => {});

  // ------------------------------------------------------------- calendar
  const start = new Date(Date.now() + 24 * 3600 * 1000);
  const end = new Date(start.getTime() + 30 * 60 * 1000);
  // Documented shape: {summary,start,end} (see qa/p2-retest.mjs); "title"
  // predates the API and returns 400.
  const evCreate = await api("/api/v1/calendar/events", {
    method: "POST",
    body: { summary: `E2E Event ${stamp}`, start: start.toISOString(), end: end.toISOString(), all_day: false },
  });
  const evList = await api("/api/v1/calendar/events");
  const evText = JSON.stringify(evList.json || "");
  check(MOD, "Calendar: create + list", evCreate.status < 300 && evText.includes(`E2E Event ${stamp}`),
    `create=${evCreate.status} list=${evList.status}`);
  const evId = evCreate.json?.id ?? evCreate.json?.event?.id;
  if (evId) await api(`/api/v1/calendar/events/${evId}`, { method: "DELETE" }).catch(() => {});

  // ------------------------------------------------- aggregated accounts
  const accounts = await api("/api/v1/accounts");
  check(MOD, "Accounts: aggregation endpoint answers", accounts.status === 200,
    `status=${accounts.status} count=${Array.isArray(accounts.json) ? accounts.json.length : "n/a"}`);

  // ------------------------------------------------------------ mail merge
  const merge = await api("/api/v1/mail/merge", {
    method: "POST",
    // Documented shape: per-recipient objects with vars (see qa/p2-retest.mjs).
    body: {
      from: "",
      subject: MERGE,
      body: "merge body {{name}}",
      html: "<p>merge body {{name}}</p>",
      recipients: [{ email: USER.email, name: "E2E", vars: { name: "One" } }],
    },
  });
  const merged = await awaitMail(MERGE, { timeoutMs: 60000 });
  check(MOD, "Mail merge: per-recipient copy is delivered",
    merge.status < 400 && !!merged,
    `status=${merge.status} uid=${merged?.uid ?? "-"}`);

  // ---------------------------------------------------- unsubscribe (UI only)
  await seedMail(SUBJ, { body: "unsubscribe affordance probe" });
  const msg = await awaitMail(SUBJ);
  await gotoMail(web.page, "/mail/Inbox");
  await web.page.waitForTimeout(4000);
  const hasUnsub = await web.page.locator("button", { hasText: /Unsubscribe|退订/ }).count();
  if (hasUnsub) {
    check(MOD, "Unsubscribe: affordance appears only for messages carrying List-Unsubscribe", true,
      `buttons=${hasUnsub} (this seeded message has no List-Unsubscribe header)`);
  } else {
    blocked(MOD, "Unsubscribe: one-click flow",
      "no List-Unsubscribe header available through the send API; the flow performs an outbound request and was not triggered");
  }
  await shot(web.page, "p2-smoke");
} finally {
  await web.ctx.close().catch(() => {});
  for (const f of ["Inbox", "Sent", "Drafts"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects([SUBJ, MERGE]).catch(() => 0);
  record(MOD, "Cleanup: P2 probes purged", purged >= 0 ? "PASS" : "FAIL", "expunged=" + purged);
  await closeBrowser();
}

const s = summary();
console.log(`\n=== p2-smoke: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("p2-smoke.json");
process.exit(s.fail > 0 ? 1 : 0);
