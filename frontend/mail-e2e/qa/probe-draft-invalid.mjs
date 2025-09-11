// Focused probes for two suspects found by p0-auth-compose:
//   (1) opening a draft for editing may not restore its content, and closing
//       that editor may drop the draft entirely;
//   (2) sending to an obviously invalid recipient may close the composer with
//       no visible error.
// Both are re-run twice and instrumented with request/console logs + shots.
//
// Run: cd frontend && node mail-e2e/qa/probe-draft-invalid.mjs
import {
  BASE, USER, api, gotoMail, listFolder, newContext, purgeSubjects, shot, sleep, stableList,
  closeBrowser, withImap, cleanupFolder,
} from "./lib.mjs";

const stamp = Date.now();
const DRAFT_SUBJ = `E2E-DRAFTPROBE-${stamp}`;
const BAD_SUBJ = `E2E-BADTO-${stamp}`;
const log = [];
const note = (s) => {
  log.push(s);
  console.log("   " + s);
};

async function apiDraftUids(subject) {
  const { messages } = await listFolder("Drafts", 0);
  return messages.filter((m) => (m.subject || "").includes(subject)).map((m) => m.uid);
}

async function instrument(page) {
  page.on("request", (r) => {
    const u = r.url().replace(BASE, "");
    if (/\/mail\/(draft|send|move|message)/.test(u)) note(`REQ  ${r.method()} ${u.slice(0, 90)}`);
  });
  page.on("response", async (r) => {
    const u = r.url().replace(BASE, "");
    if (/\/mail\/(draft|send)/.test(u)) {
      let body = "";
      try {
        body = (await r.text()).slice(0, 160);
      } catch {
        /* stream */
      }
      note(`RESP ${r.status()} ${u.slice(0, 70)} :: ${body.replace(/\s+/g, " ")}`);
    }
  });
  page.on("console", (m) => {
    if (m.type() === "error") note(`CONSOLE ${m.text().slice(0, 160)}`);
  });
  page.on("dialog", (d) => note(`DIALOG ${d.message()}`));
}

// ---------------------------------------------------------- create a draft
const created = await api("/api/v1/mail/draft", {
  method: "POST",
  body: { subject: DRAFT_SUBJ, text: "probe body line 1", html: "<p>probe body line 1</p>", replace_uid: 0, to: [USER.email], cc: [], bcc: [] },
});
console.log(`draft create -> ${created.status} ${JSON.stringify(created.json)}`);
await sleep(2000);
console.log("draft uids before UI:", await apiDraftUids(DRAFT_SUBJ));

const { ctx, page } = await newContext({ locale: "en" });
await instrument(page);
try {
  // ==================================================== PROBE 1: draft open
  for (let round = 1; round <= 2; round++) {
    note(`--- draft probe round ${round} ---`);
    await gotoMail(page, "/mail/Drafts");
    await stableList(page, 20000);
    const row = page.locator(".mail-scroll [role='button']", { hasText: DRAFT_SUBJ }).first();
    const rowVisible = await row.isVisible().catch(() => false);
    note(`draft row visible in list: ${rowVisible}`);
    if (!rowVisible) break;

    await row.click();
    await page.waitForTimeout(2500);
    const paneSubject = (await page.locator("main, body").first().innerText()).slice(0, 300).replace(/\s+/g, " ");
    note(`after row click, pane text: ${paneSubject.slice(0, 120)}`);

    let editorOpen = await page.locator('[data-testid="compose-subject"]').isVisible().catch(() => false);
    if (!editorOpen) {
      const editBtn = page.locator("button", { hasText: /^Edit$/ }).first();
      note(`compose not open; Edit button present=${await editBtn.count()}`);
      await editBtn.click().catch((e) => note(`Edit click failed: ${e.message}`));
      await page.waitForTimeout(2500);
      editorOpen = await page.locator('[data-testid="compose-subject"]').isVisible().catch(() => false);
    }
    const subjVal = editorOpen
      ? await page.locator('[data-testid="compose-subject"]').inputValue().catch(() => "<err>")
      : "<composer closed>";
    const bodyVal = editorOpen
      ? await page.locator('[contenteditable="true"]').first().innerText().catch(() => "<err>")
      : "<composer closed>";
    const toVal = editorOpen
      ? await page.locator('input[placeholder="user@example.com"]').first().inputValue().catch(() => "<err>")
      : "<composer closed>";
    note(`editorOpen=${editorOpen} subject="${subjVal}" to="${toVal}" body="${String(bodyVal).slice(0, 60).replace(/\s+/g, " ")}"`);
    note(`draft uids while editor open: [${(await apiDraftUids(DRAFT_SUBJ)).join(",")}]`);
    await shot(page, `probe-draft-open-r${round}`);

    // close without touching anything, then see whether the draft survived
    const close = page.locator("button[title='Close']:visible").first();
    if (await close.count()) {
      await close.click({ timeout: 8000 }).catch(() => {});
      await page.waitForTimeout(3000);
    }
    const after = await apiDraftUids(DRAFT_SUBJ);
    note(`draft uids after closing editor: [${after.join(",")}]`);
    if (!after.length && round === 1) {
      note("draft disappeared after close — re-seeding for round 2");
      await api("/api/v1/mail/draft", {
        method: "POST",
        body: { subject: DRAFT_SUBJ, text: "probe body line 1", html: "<p>probe body line 1</p>", replace_uid: 0, to: [USER.email], cc: [], bcc: [] },
      });
      await sleep(2500);
      note(`draft uids after re-seed: [${(await apiDraftUids(DRAFT_SUBJ)).join(",")}]`);
    }
  }

  // ============================================== PROBE 2: invalid recipient
  note("--- invalid recipient probe ---");
  await gotoMail(page, "/mail/Inbox");
  await page.locator("button", { hasText: /^Write$/ }).first().click();
  const to = page.locator('input[placeholder="user@example.com"]').first();
  await to.waitFor({ state: "visible", timeout: 15000 });
  await to.fill("not-an-email");
  await page.keyboard.press("Enter");
  await page.locator('[data-testid="compose-subject"]').fill(BAD_SUBJ);
  await page.locator('[contenteditable="true"]').first().click();
  await page.keyboard.type("body for invalid recipient probe");
  await page.locator("button", { hasText: /^Send$/ }).first().click();
  await page.waitForTimeout(4000);
  const composerOpen = await page.locator('[data-testid="compose-subject"]').isVisible().catch(() => false);
  const bodyText = await page.locator("body").innerText();
  const toastish = bodyText.split("\n").filter((l) => /(invalid|not a valid|无效|失败|error|不正确)/i.test(l)).slice(0, 4);
  note(`composerOpen=${composerOpen} url=${page.url().replace(BASE, "")}`);
  note(`visible error-ish lines: ${JSON.stringify(toastish)}`);
  const sentCopy = (await listFolder("Sent", 0)).messages.filter((m) => (m.subject || "").includes(BAD_SUBJ));
  note(`copies in Sent with that subject: ${sentCopy.length}`);
  await shot(page, "probe-invalid-recipient");

  // ============================================== protocol truth for the draft
  const imapDrafts = await withImap(async (c) => {
    await c.select("Drafts");
    const r = await c.cmd(`UID SEARCH HEADER Subject ${JSON.stringify(DRAFT_SUBJ)}`);
    const line = r.lines.find((l) => /^\* SEARCH/.test(l)) || "";
    return line.replace(/^\* SEARCH\s*/, "").trim();
  });
  note(`IMAP Drafts search "${DRAFT_SUBJ}" -> [${imapDrafts}]`);
} finally {
  await shot(page, "probe-final").catch(() => {});
  await ctx.close();
  await cleanupFolder("Inbox").catch(() => {});
  await cleanupFolder("Drafts").catch(() => {});
  await cleanupFolder("Sent").catch(() => {});
  const purged = await purgeSubjects([DRAFT_SUBJ, BAD_SUBJ]).catch(() => 0);
  console.log(`cleanup: expunged=${purged}`);
  await closeBrowser();
}

console.log("\n===== probe log =====");
console.log(log.join("\n"));
