// Where does a "Move to <custom folder>" land? Polls every mailbox (API) and
// every mailbox (IMAP) for the moved subject.
// Run: cd frontend && node mail-e2e/qa/probe-move2.mjs
import {
  BASE, api, awaitMail, cleanupFolder, closeBrowser, gotoMail, listFolder, newContext,
  purgeSubjects, seedMail, sleep, stableList, withImap,
} from "./lib.mjs";

const stamp = Date.now();
const FOLDER = `E2E-MV2-${stamp}`;
const SUBJ = `E2E-MV2-${stamp}`;
const note = (s) => console.log("   " + s);

await api("/api/v1/mail/folders", { method: "POST", body: { name: FOLDER } });
await seedMail(SUBJ, { body: "move2 probe" });
const msg = await awaitMail(SUBJ);
note(`folder=${FOLDER} seeded uid=${msg?.uid}`);

const { ctx, page } = await newContext({ locale: "en" });
page.on("response", (r) => {
  if (r.url().includes("/mail/move")) note(`RESP ${r.status()} ${r.url().replace(BASE, "").slice(0, 70)}`);
});
try {
  await gotoMail(page, "/mail/Inbox");
  await stableList(page, 20000);
  await page.locator(".mail-scroll [role='button']", { hasText: SUBJ }).first().click();
  await page.waitForTimeout(3000);
  const opts = await page.locator('select[title="Move to"] option').allInnerTexts().catch(() => []);
  note(`move select options=${JSON.stringify(opts)}`);
  await page.locator('select[title="Move to"]').first().selectOption(FOLDER).catch((e) => note(`select failed: ${e.message}`));
  await sleep(6000);

  const folders = (await api("/api/v1/mail/folders")).json || [];
  note(`folders=[${folders.join(", ")}]`);
  for (const f of folders) {
    for (let attempt = 0; attempt < 3; attempt++) {
      const r = await listFolder(f, 0);
      const hit = r.messages.find((m) => (m.subject || "").includes(SUBJ));
      if (hit) {
        note(`API: found in "${f}" page0 uid=${hit.uid} (attempt ${attempt + 1})`);
        break;
      }
      await sleep(2000);
    }
  }
  for (const f of folders) {
    const uids = await withImap((c) => c.uidSearch(`HEADER Subject ${JSON.stringify(SUBJ)}`, f)).catch(() => []);
    if (uids.length) note(`IMAP: found in "${f}" uids=[${uids.join(",")}]`);
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
