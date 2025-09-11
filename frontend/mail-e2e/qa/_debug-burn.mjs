// Which DOM container actually holds the burn body? Decides whether the list
// preview, the reader, or my text-subtraction heuristic is at fault.
// Run: cd frontend && node mail-e2e/qa/_debug-burn.mjs
import { api, awaitMail, cleanupFolder, closeBrowser, gotoMail, listFolder, newContext, purgeSubjects, seedMail, shot, stableList } from "./lib.mjs";

const stamp = Date.now();
const SUBJ = `E2E-BURNDBG-${stamp}`;
const SECRET = `SECRET${stamp}`;
await seedMail(SUBJ, { body: `body ${SECRET} text`, burn_after_minutes: 5 });
const m = await awaitMail(SUBJ);

const holders = async (page, label) => {
  const found = await page.evaluate((s) => {
    const out = [];
    for (const el of document.querySelectorAll("div, article, section, main, p, td")) {
      const t = el.innerText || "";
      if (t.includes(s)) {
        out.push(`${el.tagName}.${(el.className || "").toString().slice(0, 34)} len=${t.length}`);
      }
    }
    return out.slice(0, 6);
  }, SECRET);
  console.log(`  [${label}] containers holding the secret: ${found.length ? "\n    " + found.join("\n    ") : "NONE"}`);
};

const listRowHasSecret = async (page) =>
  (await page.locator(".mail-scroll [role='button']", { hasText: SUBJ }).first().innerText()).includes(SECRET);

const { ctx, page } = await newContext({ locale: "en" });
try {
  await gotoMail(page, "/mail/Inbox");
  await stableList(page, 20000);
  console.log(`API row: burn_after_minutes=${(await listFolder("Inbox", 0)).messages.find((x) => (x.subject || "").includes(SUBJ))?.burn_after_minutes}`);
  console.log(`list row contains secret: ${await listRowHasSecret(page)}`);
  await holders(page, "list only");

  await page.locator(".mail-scroll [role='button']", { hasText: SUBJ }).first().click();
  await page.waitForTimeout(5000);
  console.log("--- after opening (before reveal) ---");
  console.log(`list row contains secret: ${await listRowHasSecret(page)}`);
  await holders(page, "reader open, unrevealed");
  await shot(page, "debug-burn-unrevealed");

  const reveal = page.locator("button", { hasText: /Reveal/ }).first();
  if (await reveal.count()) {
    await reveal.click();
    await page.waitForTimeout(5000);
    console.log("--- after clicking Reveal ---");
    console.log(`list row contains secret: ${await listRowHasSecret(page)}`);
    await holders(page, "after reveal");
    await shot(page, "debug-burn-revealed");
  } else {
    console.log("no Reveal button found");
  }
} finally {
  await ctx.close().catch(() => {});
  await cleanupFolder("Inbox").catch(() => {});
  console.log("cleanup expunged=", await purgeSubjects([SUBJ]).catch(() => 0));
  await closeBrowser();
}
