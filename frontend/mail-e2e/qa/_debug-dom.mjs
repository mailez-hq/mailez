// Dumps the controls of the inbox screen so selectors match the real DOM.
import { gotoMail, newContext, closeBrowser, stableList } from "./lib.mjs";

const { ctx, page } = await newContext({ locale: "en" });
await gotoMail(page, "/mail/Inbox");
await stableList(page, 20000);

const buttons = await page.evaluate(() =>
  [...document.querySelectorAll("button")].map((b) => ({
    text: (b.innerText || "").replace(/\s+/g, " ").trim().slice(0, 30),
    title: b.getAttribute("title") || "",
    aria: b.getAttribute("aria-label") || "",
    popup: b.getAttribute("aria-haspopup") || "",
    vis: !!(b.offsetWidth || b.offsetHeight),
  })),
);
console.log("BUTTONS:");
for (const b of buttons) {
  console.log(`  vis=${b.vis ? "y" : "n"} popup=${b.popup.padEnd(6)} title="${b.title}" aria="${b.aria}" text="${b.text}"`);
}

const popups = await page.evaluate(() =>
  [...document.querySelectorAll("[aria-haspopup]")].map((e) => ({
    tag: e.tagName,
    text: (e.innerText || "").replace(/\s+/g, " ").trim().slice(0, 30),
    aria: e.getAttribute("aria-label") || "",
    popup: e.getAttribute("aria-haspopup"),
  })),
);
console.log("POPUP TRIGGERS:", JSON.stringify(popups, null, 1));

const toolbarText = await page.locator(".mail-scroll").first().evaluate((el) => el.parentElement?.innerText?.slice(0, 300));
console.log("PANEL HEADER TEXT:", JSON.stringify(toolbarText));
await ctx.close();
await closeBrowser();
process.exit(0);
