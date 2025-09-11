// Focused probe: "Move to <user folder>" — reader toolbar path and row
// context-menu path, both instrumented (DOM dumps + request log + shots).
// Run: cd frontend && node mail-e2e/qa/probe-move.mjs
import {
  BASE, api, awaitMail, cleanupFolder, closeBrowser, gotoMail, listFolder, newContext,
  purgeSubjects, seedMail, shot, sleep, stableList,
} from "./lib.mjs";

const stamp = Date.now();
const FOLDER = `E2E-MOVEPROBE-${stamp}`;
const SUBJ = `E2E-MOVEPROBE-${stamp}`;
const log = [];
const note = (s) => {
  log.push(s);
  console.log("   " + s);
};

// folder via API so the probe isolates the UI move, not folder creation
const created = await api("/api/v1/mail/folders", { method: "POST", body: { name: FOLDER } });
note(`folder create -> ${created.status} ${created.text.slice(0, 80)}`);

const { ctx, page } = await newContext({ locale: "en" });
page.on("request", (r) => {
  const u = r.url().replace(BASE, "");
  if (/\/mail\/(move|message)/.test(u)) note(`REQ ${r.method()} ${u.slice(0, 80)}`);
});
page.on("response", (r) => {
  const u = r.url().replace(BASE, "");
  if (/\/mail\/move/.test(u)) note(`RESP ${r.status()} ${u.slice(0, 60)}`);
});

try {
  await seedMail(SUBJ, { body: "move probe body" });
  const msg = await awaitMail(SUBJ);
  note(`seeded uid=${msg?.uid}`);

  for (let round = 1; round <= 2; round++) {
    await gotoMail(page, "/mail/Inbox");
    await stableList(page, 20000);
    await page.locator(".mail-scroll [role='button']", { hasText: SUBJ }).first().click();
    await page.waitForTimeout(3000);

    const toolbar = await page.evaluate(() =>
      [...document.querySelectorAll("button")].filter((b) => b.offsetWidth).map((b) => `${b.getAttribute("title") || ""}|${(b.innerText || "").replace(/\s+/g, " ").trim().slice(0, 18)}|popup=${b.getAttribute("aria-haspopup") || ""}`),
    );
    note(`round ${round} visible buttons: ${JSON.stringify(toolbar.slice(0, 30))}`);

    // reader "Move to" control
    const moveBtn = page.locator("button", { hasText: /Move to/ }).first();
    const mbCount = await page.locator("button", { hasText: /Move to/ }).count();
    note(`round ${round} Move-to buttons=${mbCount}`);
    if (mbCount) {
      await moveBtn.click({ timeout: 8000 }).catch((e) => note(`click failed: ${e.message}`));
      await page.waitForTimeout(1200);
      const items = await page.locator("[role='menuitem'], [role='option']").allInnerTexts().catch(() => []);
      note(`round ${round} menu items=${JSON.stringify(items.slice(0, 12))}`);
      await shot(page, `probe-move-menu-r${round}`);
      const item = page.locator("[role='menuitem'], [role='option']", { hasText: FOLDER }).first();
      const ic = await item.count();
      note(`round ${round} target item count=${ic}`);
      if (ic) await item.click({ timeout: 8000 }).catch((e) => note(`item click failed: ${e.message}`));
    }
    await sleep(4000);
    const inFolder = (await listFolder(FOLDER, 0)).messages.some((m) => (m.subject || "").includes(SUBJ));
    const inInbox = (await listFolder("Inbox", 0)).messages.some((m) => (m.subject || "").includes(SUBJ));
    note(`round ${round} after click: inFolder=${inFolder} inInbox=${inInbox} url=${page.url().replace(BASE, "")}`);
    await shot(page, `probe-move-after-r${round}`);
  }
} finally {
  await ctx.close().catch(() => {});
  await cleanupFolder("Inbox").catch(() => {});
  await cleanupFolder(FOLDER).catch(() => {});
  const purged = await purgeSubjects([SUBJ]).catch(() => 0);
  await api(`/api/v1/mail/folders?name=${encodeURIComponent(FOLDER)}`, { method: "DELETE" }).catch(() => {});
  console.log(`cleanup: expunged=${purged}`);
  await closeBrowser();
}
console.log("\n===== probe log =====");
console.log(log.join("\n"));
