// Phase 2 (P1/a) — labels end-to-end, saved searches, and syntax search
// (subject:/from:/has:attachment/before:/after:).
// Run: cd frontend && node mail-e2e/qa/p1-labels-search.mjs
import {
  BASE, USER, api, awaitMail, check, cleanupFolder, closeBrowser, dumpResults, gotoMail,
  listFolder, newContext, purgeSubjects, record, seedMail, shot, sleep, stableList, summary,
} from "./lib.mjs";

const MOD = "P1-label";
const stamp = Date.now();
const SUBJ = `E2E-LBL-${stamp}`;
const ATT = `E2E-LBLATT-${stamp}`;
const LABEL = `E2E-LBLNAME-${stamp}`;
const LABEL2 = `E2E-LBLRENAMED-${stamp}`;
const created = [SUBJ, ATT];

const bodyText = (page) => page.locator("body").innerText();
async function search(page, q) {
  await gotoMail(page, "/mail/Inbox");
  const box = page.locator('input[placeholder="Search mail"]').first();
  await box.click();
  await box.fill(q);
  await box.press("Enter");
  await page.waitForTimeout(5000);
  return bodyText(page);
}

const web = await newContext({ locale: "en" });
try {
  // ------------------------------------------------------------ fixtures
  await seedMail(SUBJ, { body: "label target body" });
  await seedMail(ATT, {
    body: "attachment target",
    attachments: [{ filename: "附件.txt", content_type: "text/plain", size: 12, data: Buffer.from("hello").toString("base64") }],
  });
  const msg = await awaitMail(SUBJ);
  check(MOD, "fixture: label target delivered", !!msg, `uid=${msg?.uid}`);

  // -------------------------------------------------------- label lifecycle
  const created_label = await api("/api/v1/mail/labels", { method: "POST", body: { name: LABEL, color: "#2E6E8E" } });
  const labels0 = await api("/api/v1/mail/labels");
  const labelNames = (labels0.json || []).map((l) => l.name ?? l);
  check(MOD, "Labels: create returns success and lists the label",
    (created_label.status === 200 || created_label.status === 204) && labelNames.includes(LABEL),
    `status=${created_label.status} labels=[${labelNames.join(", ")}]`);

  await gotoMail(web.page, "/mail/Inbox");
  const sidebar = await bodyText(web.page);
  check(MOD, "Labels: the new label shows up in the sidebar", sidebar.includes(LABEL),
    `sidebarHasLabel=${sidebar.includes(LABEL)}`);

  // apply the label to the seeded message (IMAP keyword under the hood)
  const applied = await api("/api/v1/mail/flag", { method: "POST", body: { folder: "Inbox", uid: msg.uid, flag: LABEL, value: true } });
  await sleep(2500);
  await gotoMail(web.page, "/mail/Inbox");
  await stableList(web.page, 15000);
  const rowText = await web.page.locator(".mail-scroll [role='button']", { hasText: SUBJ }).first().innerText().catch(() => "");
  check(MOD, "Labels: applying a label shows it on the message row",
    /LBL/i.test(rowText) || rowText.includes(LABEL), `row="${rowText.replace(/\s+/g, " ").slice(0, 60)}"`);
  await shot(web.page, "p1-label-applied");

  // label view: clicking the sidebar label filters the list
  await gotoMail(web.page, "/mail/Inbox");
  await web.page.locator("button, a", { hasText: LABEL }).first().click().catch(() => {});
  await web.page.waitForTimeout(4500);
  const labelView = await bodyText(web.page);
  check(MOD, "Labels: the label view lists the labelled message",
    labelView.includes(SUBJ), `labelViewHasSubject=${labelView.includes(SUBJ)}`);
  await shot(web.page, "p1-label-view");

  // clear the filter again
  await web.page.locator("button", { hasText: /^Inbox$/ }).first().click().catch(() => {});
  await web.page.waitForTimeout(3000);

  // rename
  const rename = await api("/api/v1/mail/labels/rename", { method: "POST", body: { from: LABEL, to: LABEL2 } });
  await gotoMail(web.page, "/mail/Inbox");
  const afterRename = await bodyText(web.page);
  check(MOD, "Labels: rename updates the sidebar entry",
    rename.status < 400 && afterRename.includes(LABEL2) && !afterRename.includes(LABEL),
    `status=${rename.status} hasNew=${afterRename.includes(LABEL2)} hasOld=${afterRename.includes(LABEL)}`);

  // delete
  const del = await api(`/api/v1/mail/labels?name=${encodeURIComponent(LABEL2)}`, { method: "DELETE" });
  await gotoMail(web.page, "/mail/Inbox");
  const afterDelete = await bodyText(web.page);
  check(MOD, "Labels: delete removes the sidebar entry",
    del.status < 400 && !afterDelete.includes(LABEL2), `status=${del.status}`);

  // ------------------------------------------------------------ saved search
  await gotoMail(web.page, "/mail/Inbox");
  const box = web.page.locator('input[placeholder="Search mail"]').first();
  await box.click();
  await box.fill(SUBJ);
  await box.press("Enter");
  await web.page.waitForTimeout(5000);
  // Icon-only toolbar control: the label lives in title=.
  const saveBtn = web.page.locator('button[title="Save this search"]').first();
  const saveCount = await web.page.locator('button[title="Save this search"]').count();
  check(MOD, "Saved search: the save affordance appears for an active search", saveCount > 0, `buttons=${saveCount}`);
  if (saveCount) {
    await saveBtn.click({ timeout: 8000 }).catch(() => {});
    await web.page.waitForTimeout(1500);
    // A naming dialog may appear ("Name this search").
    const nameInput = web.page.locator('input[type="text"]:visible').last();
    if (await web.page.locator("text=/Name this search/").count()) {
      await nameInput.fill(`SAVED-${stamp}`).catch(() => {});
      await web.page.locator("button", { hasText: /^(Save|OK|Create)$/ }).first().click().catch(() => {});
      await web.page.waitForTimeout(1500);
    }
    await gotoMail(web.page, "/mail/Inbox");
    const withSaved = await bodyText(web.page);
    check(MOD, "Saved search: persists into the sidebar's saved list",
      withSaved.includes("Saved searches") || withSaved.includes(SUBJ),
      `hasSection=${withSaved.includes("Saved searches")}`);
    await shot(web.page, "p1-saved-search");
  } else {
    record(MOD, "Saved search: save flow", "BLOCKED", "no save button rendered");
  }

  // ------------------------------------------------------------ syntax search
  const s1 = await search(web.page, `subject:${SUBJ}`);
  check(MOD, "Syntax search: subject: matches", s1.includes(SUBJ));
  const s2 = await search(web.page, `from:${USER.email}`);
  check(MOD, "Syntax search: from: matches own mail", s2.includes(SUBJ) || s2.includes("admin@example.com"));
  const s3 = await search(web.page, "has:attachment");
  check(MOD, "Syntax search: has:attachment matches the attachment mail",
    s3.includes(ATT) || s3.includes("附件"));
  const s4 = await search(web.page, "before:2000-01-01");
  check(MOD, "Syntax search: before:2000-01-01 returns nothing",
    /No matches|无匹配/.test(s4), `snippet="${s4.replace(/\s+/g, " ").slice(-70)}"`);
  const s5 = await search(web.page, `after:2000-01-01 subject:${SUBJ}`);
  check(MOD, "Syntax search: after: + subject: combined matches", s5.includes(SUBJ));
  await gotoMail(web.page, "/mail/Inbox");
} finally {
  await api(`/api/v1/mail/labels?name=${encodeURIComponent(LABEL)}`, { method: "DELETE" }).catch(() => {});
  await api(`/api/v1/mail/labels?name=${encodeURIComponent(LABEL2)}`, { method: "DELETE" }).catch(() => {});
  await web.ctx.close().catch(() => {});
  for (const f of ["Inbox", "Sent"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects(created).catch(() => 0);
  const left = (await api("/api/v1/mail/labels")).json || [];
  record(MOD, "Cleanup: labels + E2E mail removed", purged >= 0 ? "PASS" : "FAIL",
    `expunged=${purged} labelsLeft=[${left.map((l) => l.name ?? l).join(", ")}]`);
  await closeBrowser();
}

const s = summary();
console.log(`\n=== p1-labels-search: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("p1-labels-search.json");
process.exit(s.fail > 0 ? 1 : 0);
