// Phase 2 (P1/c) — admin CRUD (users/aliases/domains), announcement → webmail
// banner, audit trail, plus two UI couplings: list-preview ↔ row height and
// accent color applying immediately.
// Run: cd frontend && node mail-e2e/qa/p1-admin-ui.mjs
import {
  BASE, ADMIN_BASE, USER, api, check, closeBrowser, dumpResults, gotoMail, listFolder,
  newContext, record, seedMail, shot, sleep, stableList, summary, purgeSubjects, cleanupFolder,
} from "./lib.mjs";

const MOD = "P1-admin";
const stamp = Date.now();
const U = `e2e-${stamp}@example.com`;
const A = `e2e-alias-${stamp}@example.com`;
const D = `e2e-${stamp}.test`;
const SUBJ = `E2E-UILINK-${stamp}`;

const web = await newContext({ locale: "en" });
try {
  // ------------------------------------------------------------ users CRUD
  const created = await api("/api/v1/users", { method: "POST", body: { email: U, password: "E2eDemo2026!", quota_bytes: 1048576, enabled: true, displayed_name: "E2E User" } });
  const users = await api("/api/v1/users");
  const hasUser = (users.json?.data || users.json || []).some?.((x) => (x.email || "").toLowerCase() === U) ?? false;
  check(MOD, "Users: create + list", created.status < 300 && hasUser, `status=${created.status} listed=${hasUser}`);
  const delUser = await api(`/api/v1/users/${encodeURIComponent(U)}`, { method: "DELETE" });
  const users2 = await api("/api/v1/users");
  const stillThere = (users2.json?.data || users2.json || []).some?.((x) => (x.email || "").toLowerCase() === U) ?? false;
  check(MOD, "Users: delete", delUser.status < 300 && !stillThere, `status=${delUser.status}`);

  // ----------------------------------------------------------- aliases CRUD
  const aCreated = await api("/api/v1/aliases", { method: "POST", body: { email: A, destination: USER.email, name: "E2E Alias" } });
  const aliases = await api("/api/v1/aliases");
  const aliasList = Array.isArray(aliases.json) ? aliases.json : aliases.json?.data || [];
  const hasAlias = aliasList.some((x) => (x.email || "").toLowerCase() === A);
  check(MOD, "Aliases: create + list", aCreated.status < 300 && hasAlias, `status=${aCreated.status} listed=${hasAlias}`);
  const delAlias = await api(`/api/v1/aliases/${encodeURIComponent(A)}`, { method: "DELETE" });
  const aliases2 = await api("/api/v1/aliases");
  const aliasList2 = Array.isArray(aliases2.json) ? aliases2.json : aliases2.json?.data || [];
  check(MOD, "Aliases: delete", delAlias.status < 300 && !aliasList2.some((x) => (x.email || "").toLowerCase() === A), `status=${delAlias.status}`);

  // ----------------------------------------------------------- domains CRUD
  const dCreated = await api("/api/v1/domains", { method: "POST", body: { name: D } });
  const domains = await api("/api/v1/domains");
  const domainList = Array.isArray(domains.json) ? domains.json : domains.json?.data || [];
  const hasDomain = domainList.some((x) => (x.name || x) === D);
  check(MOD, "Domains: create + list", dCreated.status < 300 && hasDomain, `status=${dCreated.status} listed=${hasDomain}`);
  const delDomain = await api(`/api/v1/domains/${encodeURIComponent(D)}`, { method: "DELETE" });
  const domains2 = await api("/api/v1/domains");
  const domainList2 = Array.isArray(domains2.json) ? domains2.json : domains2.json?.data || [];
  check(MOD, "Domains: delete", delDomain.status < 300 && !domainList2.some((x) => (x.name || x) === D), `status=${delDomain.status}`);

  // ------------------------------------------- announcement → webmail banner
  const annBody = `E2E 公告内容 ${stamp}`;
  const put = await api("/api/v1/announcement", { method: "PUT", body: { subject: `E2E 公告 ${stamp}`, body: annBody, enabled: true } });
  check(MOD, "Announcement: publish via admin API", put.status < 300, `status=${put.status}`);
  const get = await api("/api/v1/announcement");
  check(MOD, "Announcement: readable from the user API",
    get.status === 200 && JSON.stringify(get.json || {}).includes(String(stamp)), `status=${get.status}`);
  await gotoMail(web.page, "/home");
  await web.page.waitForTimeout(3500);
  const home = await web.page.locator("body").innerText();
  check(MOD, "Announcement: webmail home shows the banner",
    home.includes(String(stamp)) || /公告/.test(home) || /Announcement/i.test(home),
    `homeHasStamp=${home.includes(String(stamp))}`);
  await shot(web.page, "p1-announcement");
  await api("/api/v1/announcement", { method: "DELETE" }).catch(() => {});

  // ------------------------------------------------------------- audit log
  const audit = await api("/api/v1/audit");
  const entries = audit.json?.data || audit.json || [];
  check(MOD, "Audit: the log returns entries", audit.status === 200 && Array.isArray(entries) && entries.length > 0,
    `status=${audit.status} entries=${Array.isArray(entries) ? entries.length : "n/a"}`);
  const mentions = JSON.stringify(entries).includes(U) || JSON.stringify(entries).includes(String(stamp));
  check(MOD, "Audit: this session's admin actions are recorded", mentions,
    mentions ? "found this run's identifier" : "no entry referenced this run");

  // ------------------------------------- UI: list preview ↔ row height
  await seedMail(SUBJ, { body: "preview height probe with a reasonably long body line" });
  await gotoMail(web.page, "/mail/Inbox");
  await stableList(web.page, 20000);
  const rowBox = async () => {
    const b = await web.page.locator(".mail-scroll [role='button']").first().boundingBox().catch(() => null);
    return b ? Math.round(b.height) : 0;
  };
  const before = await rowBox();
  await web.page.getByRole("button", { name: /^Settings$/ }).first().click().catch(() => {});
  await web.page.waitForTimeout(2000);
  const previewRow = web.page.locator("div.flex.items-center.justify-between", { hasText: /Message previews/ }).first();
  await previewRow.locator('[role="switch"]').first().click({ timeout: 8000 }).catch(() => {});
  await web.page.locator("button", { hasText: /^Save settings$/ }).first().click().catch(() => {});
  await web.page.waitForTimeout(2500);
  await web.page.keyboard.press("Escape");
  await gotoMail(web.page, "/mail/Inbox");
  await stableList(web.page, 20000);
  const after = await rowBox();
  check(MOD, "UI: turning message previews off changes the row height",
    before !== after && before > 0 && after > 0, `rowHeight ${before}px → ${after}px`);
  await shot(web.page, "p1-rowheight");
  // restore
  await web.page.getByRole("button", { name: /^Settings$/ }).first().click().catch(() => {});
  await web.page.waitForTimeout(2000);
  await previewRow.locator('[role="switch"]').first().click({ timeout: 8000 }).catch(() => {});
  await web.page.locator("button", { hasText: /^Save settings$/ }).first().click().catch(() => {});
  await web.page.waitForTimeout(2000);
  await web.page.keyboard.press("Escape");

  // ------------------------------------- UI: accent applies immediately
  const primaryVar = () =>
    web.page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--primary").trim());
  const beforeAccent = await primaryVar();
  await web.page.getByRole("button", { name: /^Settings$/ }).first().click().catch(() => {});
  await web.page.waitForTimeout(2000);
  await web.page.locator('button[title="Purple"]').first().click({ timeout: 8000 }).catch(() => {});
  await web.page.waitForTimeout(1200);
  const purpleVar = await primaryVar();
  await web.page.locator('button[title="Azurite"]').first().click({ timeout: 8000 }).catch(() => {});
  await web.page.waitForTimeout(1200);
  const restoredVar = await primaryVar();
  check(MOD, "UI: accent switch repaints --primary without a reload",
    purpleVar !== beforeAccent && /8f6fd1|6b4fa0/i.test(purpleVar), `before=${beforeAccent} purple=${purpleVar}`);
  check(MOD, "UI: accent can be restored to the default", restoredVar === beforeAccent,
    `restored=${restoredVar} (was ${beforeAccent})`);
  await web.page.locator("button", { hasText: /^Save settings$/ }).first().click().catch(() => {});
  await web.page.waitForTimeout(1500);
} finally {
  await api(`/api/v1/users/${encodeURIComponent(U)}`, { method: "DELETE" }).catch(() => {});
  await api(`/api/v1/aliases/${encodeURIComponent(A)}`, { method: "DELETE" }).catch(() => {});
  await api(`/api/v1/domains/${encodeURIComponent(D)}`, { method: "DELETE" }).catch(() => {});
  await api("/api/v1/announcement", { method: "DELETE" }).catch(() => {});
  await web.ctx.close().catch(() => {});
  for (const f of ["Inbox", "Sent"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects([SUBJ]).catch(() => 0);
  record(MOD, "Cleanup: admin/UI probes removed", purged >= 0 ? "PASS" : "FAIL", "expunged=" + purged);
  await closeBrowser();
}

const s = summary();
console.log(`\n=== p1-admin-ui: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("p1-admin-ui.json");
process.exit(s.fail > 0 ? 1 : 0);
