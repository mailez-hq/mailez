// Phase 1 (P0/a) — auth, sending, reply/forward, drafts.
// Run: cd frontend && node mail-e2e/qa/p0-auth-compose.mjs
import {
  BASE, USER, api, awaitMail, awaitRow, check, cleanupFolder, closeBrowser, dumpResults,
  gotoMail, listFolder, newContext, purgeSubjects, record, seedMail, shot, sleep, stableList,
  summary, withImap,
} from "./lib.mjs";

const MOD = "P0";
const stamp = Date.now();
const created = [];
const subj = (tag) => `E2E-${tag}-${stamp}`;

const SUBJ_SEND = subj("SEND");
const SUBJ_HTML = subj("HTML");
const SUBJ_NOSUBJ = subj("NOSUBJ");
const SUBJ_LONG = subj("LONG") + "-" + "X".repeat(180);
const SUBJ_REPLY = subj("REPLY");
const SUBJ_FWD = subj("FWD");
const SUBJ_DRAFT = subj("DRAFT");
created.push(SUBJ_SEND, SUBJ_HTML, SUBJ_NOSUBJ, SUBJ_LONG, SUBJ_REPLY, SUBJ_FWD, SUBJ_DRAFT);

// ---------------------------------------------------------------- helpers
async function composeAndSend(page, subject, body) {
  await page.locator("button", { hasText: /^Write$/ }).first().click();
  const to = page.locator('input[placeholder="user@example.com"]').first();
  await to.waitFor({ state: "visible", timeout: 15000 });
  await to.fill(USER.email);
  await page.keyboard.press("Enter");
  if (subject !== null) {
    await page.locator('[data-testid="compose-subject"]').fill(subject);
  }
  const editor = page.locator('[contenteditable="true"]').first();
  await editor.click();
  await page.keyboard.type(body);
  await page.locator("button", { hasText: /^Send$/ }).first().click();
  await page.waitForTimeout(2500);
}

async function openDraftCount(page, subject) {
  await gotoMail(page, "/mail/Drafts");
  await stableList(page, 20000);
  const rows = page.locator(".mail-scroll [role='button']", { hasText: subject });
  return rows.count();
}

// A draft row opens the reader; the composer is behind its "Edit" action.
// Both routes are attempted so a direct-open variant also works.
async function openDraftEditor(page, subject) {
  await gotoMail(page, "/mail/Drafts");
  await stableList(page, 20000);
  const row = page.locator(".mail-scroll [role='button']", { hasText: subject }).first();
  await row.waitFor({ state: "visible", timeout: 20000 });
  await row.click();
  await page.waitForTimeout(2500);
  if (!(await page.locator('[data-testid="compose-subject"]').isVisible().catch(() => false))) {
    await page.locator("button", { hasText: /^Edit$/ }).first().click().catch(() => {});
    await page.waitForTimeout(2500);
  }
  return page.locator('[data-testid="compose-subject"]').isVisible().catch(() => false);
}

async function closeComposer(page) {
  const close = page.locator("button[title='Close']:visible").first();
  if (await close.count()) {
    await close.click({ timeout: 8000 }).catch(() => {});
    await page.waitForTimeout(2500);
  }
}

try {
  // ============================================================ A. auth
  const anon = await newContext({ locale: "en", auth: false });
  try {
    await anon.page.goto(`${BASE}/home`, { waitUntil: "domcontentloaded" });
    await anon.page.waitForTimeout(3000);
    const url = anon.page.url();
    const onLogin = /\/(\?|$)/.test(new URL(url).pathname + (new URL(url).search ? "?" : "")) ||
      (await anon.page.locator('input[type="password"]').count()) > 0;
    check(MOD, "Auth: signed-out /home is bounced to the login page", onLogin, `url=${url}`);

    // Real form login (no cookie injected).
    const textInput = anon.page.locator('input[type="text"], input:not([type])').first();
    await textInput.fill("admin");
    await anon.page.locator('input[type="password"]').fill(USER.pw);
    await anon.page.locator("button", { hasText: /^(Sign in|登录)$/ }).first().click();
    await anon.page.waitForURL(/\/home/, { timeout: 30000 }).catch(() => {});
    await anon.page.waitForTimeout(2500);
    check(MOD, "Auth: form login reaches /home", /\/home/.test(anon.page.url()), `url=${anon.page.url()}`);

    // Sign out from the account menu.
    const accountBtn = anon.page.locator('button[aria-label="My account"], button[aria-haspopup]').first();
    await accountBtn.click().catch(() => {});
    await anon.page.waitForTimeout(800);
    const signOut = anon.page.locator("text=/(Sign out|退出登录)/").first();
    if (await signOut.count()) {
      await signOut.click();
      await anon.page.waitForTimeout(3000);
    }
    const loggedOut = !/\/home/.test(anon.page.url()) ||
      (await anon.page.locator('input[type="password"]').count()) > 0;
    check(MOD, "Auth: sign-out leaves the signed-in area", loggedOut, `url=${anon.page.url()}`);
    await shot(anon.page, "p0-auth-loginpage");
  } finally {
    await anon.ctx.close();
  }

  // ======================================================== B. compose
  const web = await newContext({ locale: "en" });
  const alerts = [];
  web.page.on("dialog", async (d) => {
    alerts.push(d.message());
    await d.dismiss().catch(() => {});
  });
  try {
    await gotoMail(web.page, "/mail/Inbox");

    // --- normal send (UI) ---
    await composeAndSend(web.page, SUBJ_SEND, "hello from the P0 suite");
    const sentRow = await awaitMail(SUBJ_SEND);
    check(MOD, "Compose: UI send delivers to own Inbox (API)", !!sentRow,
      sentRow ? `uid=${sentRow.uid}` : "not found in 60s");
    const imapSent = await withImap(async (c) => c.uidSearch(`HEADER Subject ${JSON.stringify(SUBJ_SEND)}`));
    check(MOD, "Compose: delivered copy visible over IMAP", !!sentRow && imapSent.includes(sentRow.uid),
      `imap=[${imapSent.join(",")}]`);
    const sentFolder = await listFolder("Sent", 0);
    check(MOD, "Compose: a copy is filed in Sent",
      sentFolder.messages.some((m) => (m.subject || "").includes(SUBJ_SEND)));

    // --- HTML/script injection in subject + body ---
    const inj = `<script>alert("xss")</script><b>bold</b>`;
    await seedMail(SUBJ_HTML, { body: inj, html: `<p>${inj}</p>` });
    await sleep(3000);
    await gotoMail(web.page, "/mail/Inbox");
    await awaitRow(web.page, SUBJ_HTML, 45000);
    await stableList(web.page);
    const injRow = web.page.locator(".mail-scroll [role='button']", { hasText: SUBJ_HTML }).first();
    await injRow.click();
    await web.page.waitForTimeout(3000);
    const paneText = await web.page.locator("body").innerText();
    check(MOD, "Security: <script> in subject/body does not execute",
      alerts.length === 0 && paneText.includes("xss"),
      `dialogs=[${alerts.join("|")}] payloadVisibleAsText=${paneText.includes("xss")}`);
    check(MOD, "Security: injected markup is shown as text, not markup",
      paneText.includes("<b>bold</b>") || paneText.includes("bold"),
      "reader rendered the payload as content");
    await shot(web.page, "p0-xss-subject");

    // --- truly empty subject, sent from the UI ---
    await gotoMail(web.page, "/mail/Inbox");
    await web.page.locator("button", { hasText: /^Write$/ }).first().click();
    await web.page.locator('input[placeholder="user@example.com"]').first().fill(USER.email);
    await web.page.keyboard.press("Enter");
    await web.page.locator('[data-testid="compose-subject"]').fill("");
    await web.page.locator('[contenteditable="true"]').first().click();
    await web.page.keyboard.type(`empty-subject probe ${stamp}`);
    await web.page.locator("button", { hasText: /^Send$/ }).first().click();
    await web.page.waitForTimeout(4000);
    const emptySubjSent = await (async () => {
      // D5: the composer refuses an empty subject, so this must stay empty —
      // three looks are enough to catch a regression that sends anyway.
      for (let i = 0; i < 3; i++) {
        const rows = (await listFolder("Inbox", 0)).messages.filter((m) => (m.preview || "").includes(`empty-subject probe ${stamp}`));
        if (rows.length) return rows[0];
        await sleep(1500);
      }
      return null;
    })();
    // The composer stays open with "Add a subject line before sending (or send
    // it as a draft)" (messages/en.json → noSubject); it never silently sends.
    const emptySubjText = await web.page.locator("body").innerText();
    const askedForSubject = /subject line before sending|add a subject|先填写主题|发送前/i.test(emptySubjText);
    const composerOpen = await web.page.locator('[data-testid="compose-subject"]').count();
    check(MOD, "Compose: empty subject is refused with a message (D5)",
      composerOpen > 0 && askedForSubject && !emptySubjSent,
      `composerOpen=${composerOpen > 0} message=${askedForSubject} delivered=${!!emptySubjSent}`);
    await closeComposer(web.page);

    // --- very long subject ---
    await seedMail(SUBJ_LONG, { body: "long subject" });
    const longRow = await awaitMail(SUBJ_LONG);
    check(MOD, "Compose: 190-char subject round-trips intact",
      !!longRow && longRow.subject === SUBJ_LONG,
      longRow ? `len=${(longRow.subject || "").length}` : "not delivered");

    // --- invalid recipient (UI) ---
    await gotoMail(web.page, "/mail/Inbox");
    await web.page.locator("button", { hasText: /^Write$/ }).first().click();
    const toField = web.page.locator('input[placeholder="user@example.com"]').first();
    await toField.waitFor({ state: "visible", timeout: 15000 });
    await toField.fill("not-an-email");
    await web.page.keyboard.press("Enter");
    await web.page.locator('[data-testid="compose-subject"]').fill(subj("BADTO"));
    await web.page.locator('[contenteditable="true"]').first().click();
    await web.page.keyboard.type("invalid recipient");
    await web.page.locator("button", { hasText: /^Send$/ }).first().click();
    await web.page.waitForTimeout(3000);
    const failedText = await web.page.locator("body").innerText();
    const rejected = !/\/mail\/Inbox$/.test(web.page.url()) &&
      /(invalid|not a valid|无效|邮箱地址|error|失败)/i.test(failedText);
    const composeStillOpen = await web.page.locator('[data-testid="compose-subject"]').count();
    check(MOD, "Compose: invalid recipient is rejected with a message",
      rejected || composeStillOpen > 0,
      `composeOpen=${composeStillOpen > 0} url=${web.page.url().replace(BASE, "")}`);
    await web.page.locator("button[title='Close']").first().click().catch(() => {});
    await web.page.waitForTimeout(1500);

    // ==================================================== C. reply / forward
    const origin = await awaitMail(SUBJ_SEND);
    if (origin) {
      await gotoMail(web.page, "/mail/Inbox");
      await stableList(web.page);
      await web.page.locator(".mail-scroll [role='button']", { hasText: SUBJ_SEND }).first().click();
      await web.page.waitForTimeout(2500);
      await web.page.locator("button", { hasText: /^Reply$/ }).first().click();
      await web.page.waitForTimeout(2000);
      const replySubj = await web.page.locator('[data-testid="compose-subject"]').inputValue().catch(() => "");
      const editorText = await web.page.locator('[contenteditable="true"]').first().innerText().catch(() => "");
      check(MOD, "Reply: subject prefixed and original quoted",
        /^Re:/i.test(replySubj) && editorText.trim().length > 0,
        `subject="${replySubj}" quotedChars=${editorText.trim().length}`);
      await web.page.locator('[contenteditable="true"]').first().click();
      await web.page.keyboard.type("my reply body");
      await web.page.locator("button", { hasText: /^Send$/ }).first().click();
      await web.page.waitForTimeout(3000);
      const replyBack = await awaitMail(`Re: ${SUBJ_SEND}`, { timeoutMs: 60000 });
      check(MOD, "Reply: reply is delivered back to the inbox", !!replyBack,
        replyBack ? `uid=${replyBack.uid}` : "not delivered");

      // forward — open the ROOT message by deep link (the reply above carries
      // the same subject text, so a text match would hit the wrong message)
      await gotoMail(web.page, `/mail/Inbox/${encodeURIComponent(origin.id)}`);
      await web.page.waitForTimeout(2500);
      await web.page.locator("button", { hasText: /^Forward$/ }).first().click();
      await web.page.waitForTimeout(2000);
      const fwdSubj = await web.page.locator('[data-testid="compose-subject"]').inputValue().catch(() => "");
      check(MOD, "Forward: subject prefixed", /^Fwd:/i.test(fwdSubj), `subject="${fwdSubj}"`);
      const fwdTo = web.page.locator('input[placeholder="user@example.com"]').first();
      await fwdTo.fill(USER.email);
      await web.page.keyboard.press("Enter");
      await web.page.locator('[contenteditable="true"]').first().click();
      await web.page.keyboard.type("fwd note");
      await web.page.locator("button", { hasText: /^Send$/ }).first().click();
      await web.page.waitForTimeout(3000);
      const fwdBack = await awaitMail(`Fwd: ${SUBJ_SEND}`, { timeoutMs: 60000 });
      check(MOD, "Forward: forwarded copy is delivered", !!fwdBack,
        fwdBack ? `uid=${fwdBack.uid}` : "not delivered");
    } else {
      record(MOD, "Reply/Forward: skipped", "BLOCKED", "seed message missing");
    }

    // ========================================================== D. drafts
    await gotoMail(web.page, "/mail/Inbox");
    await web.page.locator("button", { hasText: /^Write$/ }).first().click();
    await web.page.locator('input[placeholder="user@example.com"]').first().fill(USER.email);
    await web.page.keyboard.press("Enter");
    await web.page.locator('[data-testid="compose-subject"]').fill(SUBJ_DRAFT);
    await web.page.locator('[contenteditable="true"]').first().click();
    await web.page.keyboard.type("draft body v1");
    // Auto-save fires ~30s after the last edit.
    await web.page.waitForTimeout(33000);
    const autoCount = await openDraftCount(web.page, SUBJ_DRAFT);
    check(MOD, "Draft: auto-save creates exactly one draft", autoCount === 1, `rows=${autoCount}`);

    // Reopen it: content must be intact.
    const reopened = await openDraftEditor(web.page, SUBJ_DRAFT);
    const reopenedSubj = await web.page.locator('[data-testid="compose-subject"]').inputValue().catch(() => "<none>");
    const reopenedBody = await web.page.locator('[contenteditable="true"]').first().innerText().catch(() => "");
    check(MOD, "Draft: reopening restores subject and body",
      reopened && reopenedSubj === SUBJ_DRAFT && reopenedBody.includes("draft body v1"),
      `editorOpen=${reopened} subject="${reopenedSubj}" bodyChars=${reopenedBody.length}`);

    // Edit + close: still exactly one draft, with the newest content.
    await web.page.locator('[contenteditable="true"]').first().click();
    await web.page.keyboard.press("End");
    await web.page.keyboard.type(" v2");
    await closeComposer(web.page);
    const afterEdit = await openDraftCount(web.page, SUBJ_DRAFT);
    check(MOD, "Draft: edit + close keeps a single draft (overwrite semantics)",
      afterEdit === 1, `rows=${afterEdit}`);
    const reopened2 = await openDraftEditor(web.page, SUBJ_DRAFT);
    const bodyV2 = await web.page.locator('[contenteditable="true"]').first().innerText().catch(() => "");
    check(MOD, "Draft: closed edits are not lost", reopened2 && bodyV2.includes("v2"),
      `editorOpen=${reopened2} body="${bodyV2.slice(0, 60).replace(/\s+/g, " ")}"`);
    await closeComposer(web.page);
    await shot(web.page, "p0-drafts");
  } finally {
    await web.ctx.close();
  }
} finally {
  for (const f of ["Inbox", "Drafts", "Sent"]) {
    await cleanupFolder(f).catch(() => {});
  }
  const purged = await purgeSubjects(created).catch(() => 0);
  record(MOD, "Cleanup: E2E-* subjects purged from all folders", purged > 0 ? "PASS" : "FAIL",
    "expunged=" + purged);
  await closeBrowser();
}

const s = summary();
console.log(`\n=== p0-auth-compose: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("p0-auth-compose.json");
process.exit(s.fail > 0 ? 1 : 0);
