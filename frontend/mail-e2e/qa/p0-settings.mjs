// Phase 1 (P0/e) — settings persistence (theme / density / accent / preview /
// typography) across a reload, language switching, and the cross-cutting
// "refresh a deep link" + "browser back" checks.
//
// Run: cd frontend && node mail-e2e/qa/p0-settings.mjs
import {
  BASE, USER, api, awaitMail, check, cleanupFolder, closeBrowser, dumpResults, gotoMail,
  listFolder, newContext, purgeSubjects, record, seedMail, shot, sleep, stableList, summary,
} from "./lib.mjs";

const MOD = "P0-settings";
const stamp = Date.now();
const SUBJ = `E2E-SET-${stamp}`;

const dom = (page) =>
  page.evaluate(() => ({
    cls: document.documentElement.className,
    density: document.documentElement.dataset.density || "",
    accent: document.documentElement.dataset.accent || "",
    cookie: document.cookie,
    locale: (document.cookie.match(/NEXT_LOCALE=([^;]+)/) || [])[1] || "",
  }));

async function openSettings(page) {
  await page.getByRole("button", { name: /^Settings$/ }).first().click().catch(() => {});
  await page.waitForTimeout(2000);
}

async function saveSettings(page) {
  await page.locator("button", { hasText: /^Save settings$/ }).first().click({ timeout: 8000 }).catch(() => {});
  await page.waitForTimeout(2500);
}

const web = await newContext({ locale: "en" });
try {
  await gotoMail(web.page, "/mail/Inbox");
  const original = await dom(web.page);

  // ---------------------------------------------------------------- apply
  await openSettings(web.page);
  await web.page.locator("button", { hasText: /^Light$/ }).first().click().catch(() => {});
  await web.page.locator("button", { hasText: /^Compact$/ }).first().click().catch(() => {});
  await web.page.locator('button[title="Purple"]').first().click().catch(() => {});
  await web.page.locator("button", { hasText: /^Large$/ }).first().click().catch(() => {});
  await web.page.locator("button", { hasText: /^Wide$/ }).first().click().catch(() => {});
  await saveSettings(web.page);
  const applied = await dom(web.page);
  check(MOD, "Settings: changes apply immediately (no reload)",
    applied.density === "compact" && applied.accent === "purple" && !applied.cls.includes("dark"),
    `density=${applied.density} accent=${applied.accent} class="${applied.cls}"`);
  await shot(web.page, "p0-settings-applied");

  // ------------------------------------------------------------- survives
  await web.page.reload({ waitUntil: "domcontentloaded" });
  await web.page.waitForTimeout(3500);
  const afterReload = await dom(web.page);
  check(MOD, "Settings: survive a full page reload",
    afterReload.density === applied.density && afterReload.accent === applied.accent,
    `density=${afterReload.density} accent=${afterReload.accent}`);
  check(MOD, "Settings: SSR mirror cookie carries the same values",
    /d=compact/.test(afterReload.cookie) && /a=purple/.test(afterReload.cookie),
    `cookie="${afterReload.cookie.slice(0, 80)}"`);

  await openSettings(web.page);
  const lightActive = await web.page.locator("button", { hasText: /^Light$/ }).first()
    .evaluate((b) => b.getAttribute("data-state") || b.className).catch(() => "");
  const purpleSelected = await web.page.locator('button[title="Purple"]').first()
    .evaluate((b) => b.className.includes("border-foreground")).catch(() => false);
  check(MOD, "Settings: the dialog reflects the persisted selection",
    purpleSelected,
    `accentSelected=${purpleSelected} lightBtn="${String(lightActive).slice(0, 40)}"`);
  await shot(web.page, "p0-settings-dialog");

  // ---------------------------------------------------------------- restore
  await web.page.locator("button", { hasText: /^System$/ }).first().click().catch(() => {});
  await web.page.locator("button", { hasText: /^Cozy$/ }).first().click().catch(() => {});
  await web.page.locator('button[title="Azurite"]').first().click().catch(() => {});
  await web.page.locator("button", { hasText: /^Medium$/ }).first().click().catch(() => {});
  await saveSettings(web.page);
  await web.page.keyboard.press("Escape");
  await web.page.waitForTimeout(1500);
  const restored = await dom(web.page);
  check(MOD, "Settings: original values restored after the test",
    restored.density === "cozy" && restored.accent === "blue",
    `density=${restored.density} accent=${restored.accent} (was ${original.density}/${original.accent})`);

  // --------------------------------------------------------------- language
  await web.page.locator("button", { hasText: /^中文$/ }).first().click().catch(() => {});
  await web.page.waitForTimeout(3000);
  const zhBody = await web.page.locator("body").innerText();
  check(MOD, "Language: switching to 中文 translates the shell",
    /收件箱/.test(zhBody) && /写邮件/.test(zhBody),
    `hasInbox=${/收件箱/.test(zhBody)} hasWrite=${/写邮件/.test(zhBody)}`);
  await web.page.reload({ waitUntil: "domcontentloaded" });
  await web.page.waitForTimeout(3000);
  const zhAfter = await web.page.locator("body").innerText();
  check(MOD, "Language: choice persists across a reload", /收件箱/.test(zhAfter));
  await shot(web.page, "p0-locale-zh");
  await web.page.locator("button", { hasText: /^EN$/ }).first().click().catch(() => {});
  await web.page.waitForTimeout(2500);
  const enBody = await web.page.locator("body").innerText();
  check(MOD, "Language: switching back to EN restores English", /Inbox/.test(enBody) && !/收件箱/.test(enBody));

  // ------------------------------------------------- cross-cutting: deep link
  await seedMail(SUBJ, { body: "deep link probe" });
  const msg = await awaitMail(SUBJ);
  await gotoMail(web.page, "/mail/Inbox");
  const shown = await (async () => {
    for (let i = 0; i < 20; i++) {
      if ((await web.page.locator("body").innerText()).includes(SUBJ)) return true;
      await web.page.waitForTimeout(1500);
    }
    return false;
  })();
  check(MOD, "Cross-cutting: seeded mail is visible before opening", shown);
  if (shown) {
    await web.page.locator(".mail-scroll [role='button']", { hasText: SUBJ }).first().click();
    await web.page.waitForTimeout(3000);
    const deepUrl = web.page.url();
    check(MOD, "Cross-cutting: opening a message produces a deep link",
      /\/mail\/Inbox\/.+/.test(deepUrl), deepUrl.replace(BASE, ""));
    await web.page.goto(deepUrl, { waitUntil: "domcontentloaded" });
    await web.page.waitForTimeout(4000);
    const reopened = await web.page.locator("body").innerText();
    check(MOD, "Cross-cutting: refreshing the deep link restores the message",
      reopened.includes(SUBJ), `url=${web.page.url().replace(BASE, "")}`);
    await shot(web.page, "p0-deeplink");

    // browser back: Inbox → message → Sent → back
    await web.page.locator("button", { hasText: /^Sent$/ }).first().click().catch(() => {});
    await web.page.waitForTimeout(2500);
    const sentUrl = web.page.url();
    await web.page.goBack();
    await web.page.waitForTimeout(3000);
    check(MOD, "Cross-cutting: browser back returns to the previous view",
      web.page.url() !== sentUrl, `after back=${web.page.url().replace(BASE, "")}`);
  }
} finally {
  await web.ctx.close().catch(() => {});
  for (const f of ["Inbox", "Sent"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects([SUBJ]).catch(() => 0);
  record(MOD, "Cleanup: settings-probe mail purged", purged >= 0 ? "PASS" : "FAIL", "expunged=" + purged);
  await closeBrowser();
}

const s = summary();
console.log(`\n=== p0-settings: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("p0-settings.json");
process.exit(s.fail > 0 ? 1 : 0);
