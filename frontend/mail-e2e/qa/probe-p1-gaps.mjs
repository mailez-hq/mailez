// Probes for three P1 suspects:
//   a) where does the "Scheduled" view live / what does it render?
//   b) are read-receipt and burn-after-read surfaced anywhere (reader + API)?
//   c) does an 11MB attachment (under the product's own 20MB limit) send via
//      the webmail proxy, and does the same payload work against the backend
//      directly?
// Run: cd frontend && node mail-e2e/qa/probe-p1-gaps.mjs
import {
  BASE, USER, api, awaitMail, cleanupFolder, closeBrowser, gotoMail, listFolder, newContext,
  purgeSubjects, seedMail, shot, sleep, stableList,
} from "./lib.mjs";

const stamp = Date.now();
const note = (s) => console.log("   " + s);
const S = {
  sched: `E2E-GAPSCHED-${stamp}`,
  receipt: `E2E-GAPRECEIPT-${stamp}`,
  burn: `E2E-GAPBURN-${stamp}`,
  big: `E2E-GAPBIG-${stamp}`,
};
const BACKEND = "http://127.0.0.1:8081";

// ---------------------------------------------------------------- a) scheduled
await seedMail(S.sched, { body: "scheduled view probe", send_at: new Date(Date.now() + 10 * 60 * 1000).toISOString() });
await sleep(2500);
const { ctx, page } = await newContext({ locale: "en" });
try {
  await gotoMail(page, "/mail/Inbox");
  const schedBtn = page.locator("button", { hasText: /^Scheduled$/ }).first();
  note(`a) sidebar Scheduled button count=${await page.locator("button", { hasText: /^Scheduled$/ }).count()}`);
  await schedBtn.click({ timeout: 8000 }).catch((e) => note(`a) click failed: ${e.message}`));
  await page.waitForTimeout(4000);
  note(`a) url after click = ${page.url().replace(BASE, "")}`);
  const schedText = (await page.locator("body").innerText()).replace(/\s+/g, " ");
  const idx = schedText.indexOf("Scheduled");
  note(`a) panel text around Scheduled: "${schedText.slice(Math.max(0, idx - 40), idx + 200)}"`);
  note(`a) contains seeded subject: ${schedText.includes(S.sched)}`);
  await shot(page, "probe-scheduled-view");

  // ---------------------------------------------------- b) receipt + burn
  await seedMail(S.receipt, { body: "receipt probe", receipt_requested: true });
  await seedMail(S.burn, { body: "burn secret body", burn_after_minutes: 5 });
  const rMsg = await awaitMail(S.receipt);
  const bMsg = await awaitMail(S.burn);
  const rDetail = await api(`/api/v1/mail/message?folder=Inbox&uid=${rMsg.uid}`);
  const bDetail = await api(`/api/v1/mail/message?folder=Inbox&uid=${bMsg.uid}`);
  note(`b) receipt list=${JSON.stringify({ receipt_requested: rMsg.receipt_requested, receipt_to: rMsg.receipt_to })}`);
  note(`b) receipt detail keys=${Object.keys(rDetail.json || {}).join(",")}`);
  note(`b) burn detail=${JSON.stringify({ burn_after_minutes: bDetail.json?.burn_after_minutes })}`);

  await gotoMail(page, "/mail/Inbox");
  await stableList(page, 20000);
  await page.locator(".mail-scroll [role='button']", { hasText: S.receipt }).first().click();
  await page.waitForTimeout(4000);
  const receiptBody = (await page.locator("body").innerText()).replace(/\s+/g, " ");
  note(`b) reader mentions receipt request: ${/read receipt|回执/.test(receiptBody)}`);
  note(`b) "Send receipt" buttons=${await page.locator("button", { hasText: /Send receipt/ }).count()}`);
  await shot(page, "probe-receipt-reader");

  await gotoMail(page, "/mail/Inbox");
  await stableList(page, 20000);
  await page.locator(".mail-scroll [role='button']", { hasText: S.burn }).first().click();
  await page.waitForTimeout(5000);
  const burnBody = (await page.locator("body").innerText()).replace(/\s+/g, " ");
  note(`b) burn reader shows secret text: ${burnBody.includes("burn secret body")}`);
  note(`b) burn reader mentions burn: ${/burn|阅后|Reveal|Reveal message/.test(burnBody)}`);
  await shot(page, "probe-burn-reader");

  // ------------------------------------------------------------ c) 11MB send
  const big = Buffer.alloc(11 * 1024 * 1024, 0x41).toString("base64");
  const payload = {
    to: [USER.email], cc: [], bcc: [], subject: S.big, body: "big attachment",
    attachments: [{ filename: "大附件.bin", content_type: "application/octet-stream", size: 11 * 1024 * 1024, data: big }],
  };
  const cookieHeader = await import("./lib.mjs").then((L) => L.cookie());
  for (const [label, url] of [["webmail-proxy", `${BASE}/api/v1/mail/send`], ["backend-direct", `${BACKEND}/api/v1/mail/send`]]) {
    const t0 = Date.now();
    let status = "?";
    let body = "";
    try {
      const res = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json", cookie: cookieHeader },
        body: JSON.stringify(payload),
      });
      status = res.status;
      body = (await res.text()).slice(0, 120);
    } catch (e) {
      status = `ERR ${e.message}`;
    }
    note(`c) ${label}: status=${status} in ${Date.now() - t0}ms :: ${body.replace(/\s+/g, " ")}`);
  }
} finally {
  await ctx.close().catch(() => {});
  const pending = (await api("/api/v1/mail/scheduled").catch(() => ({ json: [] }))).json || [];
  for (const p of pending) {
    if ((p.subject || "").includes("E2E-")) await api(`/api/v1/mail/outbox/${p.id}`, { method: "DELETE" }).catch(() => {});
  }
  await cleanupFolder("Inbox").catch(() => {});
  const purged = await purgeSubjects(Object.values(S)).catch(() => 0);
  console.log(`cleanup: expunged=${purged}`);
  await closeBrowser();
}
