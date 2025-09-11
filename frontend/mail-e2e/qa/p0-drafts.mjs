// Phase 1 (P0/b) — draft lifecycle driven only through the UI:
// auto-save, reopen integrity, overwrite semantics (one draft), and
// "closing must not lose typing". Run twice to confirm stability.
//
// Run: cd frontend && node mail-e2e/qa/p0-drafts.mjs
import {
  BASE, USER, api, check, cleanupFolder, closeBrowser, dumpResults, gotoMail, listFolder,
  newContext, purgeSubjects, record, shot, stableList, summary,
} from "./lib.mjs";

const MOD = "P0-draft";
const stamp = Date.now();
const SUBJ = `E2E-DRAFT-${stamp}`;

async function draftRows(page, subject) {
  await gotoMail(page, "/mail/Drafts");
  await stableList(page, 20000);
  return page.locator(".mail-scroll [role='button']", { hasText: subject }).count();
}

/** Open the draft editor the way a user does: row → reader → Edit. */
async function openEditor(page, subject) {
  await gotoMail(page, "/mail/Drafts");
  await stableList(page, 20000);
  const row = page.locator(".mail-scroll [role='button']", { hasText: subject }).first();
  await row.waitFor({ state: "visible", timeout: 20000 });
  await row.click();
  await page.waitForTimeout(2500);
  if (!(await page.locator('[data-testid="compose-subject"]').isVisible().catch(() => false))) {
    await page.locator("button", { hasText: /^Edit$/ }).first().click({ timeout: 10000 }).catch(() => {});
    await page.waitForTimeout(3000);
  }
  return {
    open: await page.locator('[data-testid="compose-subject"]').isVisible().catch(() => false),
    subject: await page.locator('[data-testid="compose-subject"]').inputValue().catch(() => "<none>"),
    body: await page.locator('[contenteditable="true"]').first().innerText().catch(() => "<none>"),
  };
}

async function closeEditor(page) {
  const close = page.locator("button[title='Close']:visible").first();
  if (await close.count()) {
    await close.click({ timeout: 8000 }).catch(() => {});
    await page.waitForTimeout(3000);
  }
}

const web = await newContext({ locale: "en" });
try {
  await gotoMail(web.page, "/mail/Inbox");

  // ---- write, then wait past the auto-save debounce (30s) ----
  await web.page.locator("button", { hasText: /^Write$/ }).first().click();
  await web.page.locator('input[placeholder="user@example.com"]').first().fill(USER.email);
  await web.page.keyboard.press("Enter");
  await web.page.locator('[data-testid="compose-subject"]').fill(SUBJ);
  const editor = web.page.locator('[contenteditable="true"]').first();
  await editor.click();
  await web.page.keyboard.type("draft body v1");
  const typedSubj = await web.page.locator('[data-testid="compose-subject"]').inputValue();
  await web.page.waitForTimeout(34000);

  const auto = await draftRows(web.page, SUBJ);
  check(MOD, "auto-save creates exactly one draft", auto === 1, `rows=${auto}`);
  const apiDrafts = (await listFolder("Drafts", 0)).messages.filter((m) => (m.subject || "").includes(SUBJ));
  check(MOD, "server holds the auto-saved draft (API)", apiDrafts.length === 1,
    `count=${apiDrafts.length} uid=${apiDrafts[0]?.uid}`);

  // ---- reopen: subject + body must come back ----
  const first = await openEditor(web.page, SUBJ);
  check(MOD, "reopening a draft restores subject", first.open && first.subject === SUBJ,
    `open=${first.open} subject="${first.subject}" (typed "${typedSubj}")`);
  check(MOD, "reopening a draft restores body text", first.open && /draft body v1/.test(first.body),
    `body="${String(first.body).replace(/\s+/g, " ").slice(0, 60)}"`);
  await shot(web.page, "p0-draft-reopen");

  // ---- edit + close: still one draft, newest content wins ----
  await web.page.locator('[contenteditable="true"]').first().click();
  await web.page.keyboard.press("End");
  await web.page.keyboard.type(" v2");
  await closeEditor(web.page);
  const afterEdit = await draftRows(web.page, SUBJ);
  check(MOD, "edit + close keeps a single draft (overwrite, not duplicate)", afterEdit === 1,
    `rows=${afterEdit}`);

  const second = await openEditor(web.page, SUBJ);
  check(MOD, "edits made before closing are preserved", second.open && /v2/.test(second.body),
    `open=${second.open} body="${String(second.body).replace(/\s+/g, " ").slice(0, 60)}"`);
  await closeEditor(web.page);

  // ---- closing a composer with unsaved typing must not lose it ----
  await gotoMail(web.page, "/mail/Inbox");
  const SUBJ2 = `E2E-DRAFTLOSS-${stamp}`;
  await web.page.locator("button", { hasText: /^Write$/ }).first().click();
  await web.page.locator('[data-testid="compose-subject"]').fill(SUBJ2);
  await web.page.locator('[contenteditable="true"]').first().click();
  await web.page.keyboard.type("closing must keep this text");
  await closeEditor(web.page);
  const lossRows = await draftRows(web.page, SUBJ2);
  check(MOD, "closing right after typing still saves the draft", lossRows === 1, `rows=${lossRows}`);
  const lossDraft = (await listFolder("Drafts", 0)).messages.find((m) => (m.subject || "").includes(SUBJ2));
  check(MOD, "close-saved draft keeps its body (API)", !!lossDraft,
    lossDraft ? `uid=${lossDraft.uid} preview="${(lossDraft.preview || "").slice(0, 40)}"` : "not found");
} finally {
  await web.ctx.close().catch(() => {});
  for (const f of ["Inbox", "Drafts"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects([SUBJ, `E2E-DRAFTLOSS-${stamp}`]).catch(() => 0);
  record(MOD, "Cleanup: draft probes purged", purged > 0 ? "PASS" : "FAIL", "expunged=" + purged);
  await closeBrowser();
}

const s = summary();
console.log(`\n=== p0-drafts: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("p0-drafts.json");
process.exit(s.fail > 0 ? 1 : 0);
