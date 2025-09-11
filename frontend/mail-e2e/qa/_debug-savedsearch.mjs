// Dumps the toolbar controls while a search is active, to find the
// "Save this search" affordance (label vs title vs menu).
import { gotoMail, newContext, closeBrowser, seedMail, awaitMail, purgeSubjects, cleanupFolder } from "./lib.mjs";

const stamp = Date.now();
const SUBJ = `E2E-SS-${stamp}`;
await seedMail(SUBJ, { body: "saved search probe" });
await awaitMail(SUBJ);

const { ctx, page } = await newContext({ locale: "en" });
await gotoMail(page, "/mail/Inbox");
const box = page.locator('input[placeholder="Search mail"]').first();
await box.click();
await box.fill(SUBJ);
await box.press("Enter");
await page.waitForTimeout(5000);

const buttons = await page.evaluate(() =>
  [...document.querySelectorAll("button")].filter((b) => b.offsetWidth).map((b) => ({
    text: (b.innerText || "").replace(/\s+/g, " ").trim().slice(0, 26),
    title: b.getAttribute("title") || "",
    aria: b.getAttribute("aria-label") || "",
  })),
);
console.log("VISIBLE BUTTONS WHILE SEARCHING:");
for (const b of buttons) console.log(`  text="${b.text}" title="${b.title}" aria="${b.aria}"`);
const panel = await page.locator("body").innerText();
console.log("panel mentions 'Save this search':", /Save this search/.test(panel));
console.log("panel mentions 'Saved searches':", /Saved searches/.test(panel));

await ctx.close();
await cleanupFolder("Inbox").catch(() => {});
await purgeSubjects([SUBJ]).catch(() => {});
await closeBrowser();
process.exit(0);
