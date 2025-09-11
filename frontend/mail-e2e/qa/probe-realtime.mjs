// How long does an open tab take to notice a change made by ANOTHER client?
// Measures: new mail arrival, and removal (move to Trash), with the SSE
// connection and any poll traffic visible in the request log.
// Run: cd frontend && node mail-e2e/qa/probe-realtime.mjs
import {
  BASE, api, awaitMail, cleanupFolder, closeBrowser, gotoMail, listFolder, newContext,
  purgeSubjects, seedMail, shot, sleep, stableList,
} from "./lib.mjs";

const stamp = Date.now();
const SUBJ = `E2E-RT-${stamp}`;
const note = (s) => console.log("   " + s);

const { ctx, page } = await newContext({ locale: "en" });
let lastEvent = null;
page.on("request", (r) => {
  const u = r.url().replace(BASE, "");
  if (/\/events|\/mail\/messages/.test(u)) lastEvent = `${new Date().toISOString().slice(11, 19)} ${u.slice(0, 60)}`;
});
try {
  await gotoMail(page, "/mail/Inbox");
  await stableList(page, 15000);
  note(`opened Inbox; last event/poll: ${lastEvent || "none"}`);

  // 1) new mail via API (another client)
  const t0 = Date.now();
  await seedMail(SUBJ, { body: "realtime probe" });
  const msg = await awaitMail(SUBJ);
  let appeared = 0;
  for (let i = 0; i < 20; i++) {
    if ((await page.locator("body").innerText()).includes(SUBJ)) {
      appeared = Date.now() - t0;
      break;
    }
    await sleep(1000);
  }
  note(`new mail uid=${msg?.uid}: appeared in the open tab after ${appeared ? appeared + "ms" : ">20s"}`);
  note(`last request while waiting: ${lastEvent}`);
  await shot(page, "probe-realtime-newmail");

  // 2) move it away via API (another client)
  const t1 = Date.now();
  await api("/api/v1/mail/move", { method: "POST", body: { folder: "Inbox", uids: [msg.uid], destination: "Trash" } });
  let gone = 0;
  for (let i = 0; i < 25; i++) {
    if (!(await page.locator("body").innerText()).includes(SUBJ)) {
      gone = Date.now() - t1;
      break;
    }
    await sleep(1000);
  }
  note(`moved to Trash: row left the open tab after ${gone ? gone + "ms" : ">25s"}`);
  note(`last request while waiting: ${lastEvent}`);
  await shot(page, "probe-realtime-moved");

  const apiList = await listFolder("Inbox", 0);
  note(`server says Inbox has ${apiList.messages.length} rows (message present: ${apiList.messages.some((m) => (m.subject || "").includes(SUBJ))})`);
} finally {
  await ctx.close().catch(() => {});
  await cleanupFolder("Inbox").catch(() => {});
  const purged = await purgeSubjects([SUBJ]).catch(() => 0);
  console.log(`cleanup: expunged=${purged}`);
  await closeBrowser();
}
