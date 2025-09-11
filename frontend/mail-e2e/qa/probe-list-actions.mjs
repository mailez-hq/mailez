// Focused probes for the four P0/list-action failures, each run twice with
// request logging and screenshots so the report can separate "test missed the
// control" from "the product misbehaves".
//   a) "Mark all as read"
//   b) row Delete → where does the message end up?
//   c) "New folder" dialog
//   d) list sort menu
//
// Run: cd frontend && node mail-e2e/qa/probe-list-actions.mjs
import {
  BASE, USER, api, awaitMail, cleanupFolder, closeBrowser, gotoMail, listFolder, newContext,
  purgeSubjects, seedMail, shot, sleep, stableList, withImap,
} from "./lib.mjs";

const stamp = Date.now();
const log = [];
const note = (s) => {
  log.push(s);
  console.log("   " + s);
};
const S = {
  mark: `E2E-MARK-${stamp}`,
  del: `E2E-DEL3-${stamp}`,
  sort: `E2E-SORT-${stamp}`,
};
const FOLDER = `E2E-FOLDPROBE-${stamp}`;

async function instrument(page, tag) {
  page.on("request", (r) => {
    const u = r.url().replace(BASE, "");
    if (/\/mail\/(read-all|delete|move|folders)/.test(u)) note(`${tag} REQ ${r.method()} ${u.slice(0, 80)}`);
  });
  page.on("response", async (r) => {
    const u = r.url().replace(BASE, "");
    if (/\/mail\/(read-all|delete|move|folders)/.test(u)) {
      let b = "";
      try {
        b = (await r.text()).slice(0, 120);
      } catch {
        /* ignore */
      }
      note(`${tag} RESP ${r.status()} ${u.slice(0, 60)} :: ${b.replace(/\s+/g, " ")}`);
    }
  });
}

const { ctx, page } = await newContext({ locale: "en" });
await instrument(page, "web");

async function flagsOf(uid, folder = "Inbox") {
  return (await withImap((c) => c.uidFetch(uid, folder))).flags || [];
}

try {
  // ---------------------------------------------------------- a) mark-all-read
  await seedMail(S.mark, { body: "mark all probe" });
  const markMsg = await awaitMail(S.mark);
  note(`a) seeded uid=${markMsg?.uid} flags=[${(await flagsOf(markMsg.uid)).join(",")}]`);
  for (let round = 1; round <= 2; round++) {
    await gotoMail(page, "/mail/Inbox");
    await stableList(page, 20000);
    const btn = page.locator("button", { hasText: /Mark all as read/ }).first();
    const visible = await btn.isVisible().catch(() => false);
    note(`a${round}) button visible=${visible} count=${await page.locator("button", { hasText: /Mark all as read/ }).count()}`);
    if (visible) {
      await btn.click({ timeout: 8000 }).catch((e) => note(`a${round}) click failed: ${e.message}`));
    } else {
      // maybe it lives behind the overflow menu
      const more = page.locator("button[aria-haspopup]").last();
      await more.click().catch(() => {});
      await page.waitForTimeout(800);
      const items = await page.locator("[role='menuitem'], [role='option']").allInnerTexts().catch(() => []);
      note(`a${round}) overflow menu items=${JSON.stringify(items.slice(0, 12))}`);
      await page.keyboard.press("Escape");
    }
    await sleep(4000);
    const f = await flagsOf(markMsg.uid);
    note(`a${round}) after click flags=[${f.join(",")}] url=${page.url().replace(BASE, "")}`);
    await shot(page, `probe-markall-r${round}`);
  }

  // ------------------------------------------------------- b) delete target
  await seedMail(S.del, { body: "delete target probe" });
  const delMsg = await awaitMail(S.del);
  note(`b) seeded uid=${delMsg?.uid}`);
  for (let round = 1; round <= 2; round++) {
    if (round === 2) {
      await seedMail(S.del + "-r2", { body: "delete target probe r2" });
    }
    await gotoMail(page, "/mail/Inbox");
    await stableList(page, 20000);
    const subj = round === 1 ? S.del : S.del + "-r2";
    const row = page.locator(".mail-scroll [role='button']", { hasText: subj }).first();
    const uid = round === 1 ? delMsg.uid : (await awaitMail(subj))?.uid;
    await row.locator("button[title='Delete']").first().dispatchEvent("click");
    await sleep(4000);
    // Where is it now? scan Trash pages and IMAP.
    let foundInTrash = null;
    for (let p = 0; p < 3; p++) {
      const pageN = await listFolder("Trash", p);
      const hit = pageN.messages.find((m) => m.uid === uid || (m.subject || "").includes(subj));
      if (hit) {
        foundInTrash = `page${p} uid=${hit.uid}`;
        break;
      }
    }
    const imapTrash = await withImap((c) => c.uidSearch(`HEADER Subject ${JSON.stringify(subj)}`, "Trash"));
    const imapInbox = await withImap((c) => c.uidSearch(`HEADER Subject ${JSON.stringify(subj)}`, "Inbox"));
    note(`b${round}) uid=${uid} inTrash(API)=${foundInTrash || "no"} imapTrash=[${imapTrash.join(",")}] imapInbox=[${imapInbox.join(",")}]`);
    await shot(page, `probe-delete-r${round}`);
  }

  // --------------------------------------------------------- c) new folder
  for (let round = 1; round <= 2; round++) {
    await gotoMail(page, "/mail/Inbox");
    const newFolderBtn = page.locator("button", { hasText: /New folder/ }).first();
    const cnt = await page.locator("button", { hasText: /New folder/ }).count();
    note(`c${round}) "New folder" buttons=${cnt} visible=${await newFolderBtn.isVisible().catch(() => false)}`);
    if (cnt) {
      await newFolderBtn.click({ timeout: 8000 }).catch((e) => note(`c${round}) click failed: ${e.message}`));
      await page.waitForTimeout(1500);
      const inputCount = await page.locator("#folder-name").count();
      note(`c${round}) dialog input #folder-name count=${inputCount}`);
      await shot(page, `probe-newfolder-r${round}`);
      if (inputCount) {
        const name = round === 1 ? FOLDER : FOLDER + "-r2";
        await page.locator("#folder-name").fill(name);
        await page.locator("button", { hasText: /^Save$/ }).first().click().catch((e) => note(`c${round}) save failed: ${e.message}`));
        await sleep(3500);
        const folders = (await api("/api/v1/mail/folders")).json || [];
        note(`c${round}) folders now=[${folders.join(", ")}] created=${folders.includes(name)}`);
        await api(`/api/v1/mail/folders?name=${encodeURIComponent(name)}`, { method: "DELETE" }).catch(() => {});
      }
    } else {
      const labels = await page.locator("aside button, nav button").allInnerTexts().catch(() => []);
      note(`c${round}) sidebar buttons=${JSON.stringify(labels.slice(0, 18))}`);
    }
  }

  // -------------------------------------------------------------- d) sort
  await seedMail(S.sort, { body: "sort probe" });
  await sleep(2500);
  for (let round = 1; round <= 2; round++) {
    await gotoMail(page, "/mail/Inbox");
    await stableList(page, 20000);
    const trigger = page.locator("button[aria-haspopup]", { hasText: /Newest first|Oldest first/ }).first();
    const tCount = await trigger.count();
    note(`d${round}) sort triggers=${tCount}`);
    if (tCount) {
      await trigger.click({ timeout: 8000 }).catch((e) => note(`d${round}) trigger click failed: ${e.message}`));
      await page.waitForTimeout(1200);
      const items = await page.locator("[role='menuitem'], [role='option']").allInnerTexts().catch(() => []);
      note(`d${round}) menu items=${JSON.stringify(items.slice(0, 8))}`);
      await shot(page, `probe-sort-menu-r${round}`);
      const want = round === 1 ? /Oldest first/ : /Newest first/;
      await page.locator("[role='menuitem'], [role='option']", { hasText: want }).first().click().catch((e) => note(`d${round}) option click failed: ${e.message}`));
      await sleep(4000);
      const first = await page.locator(".mail-scroll [role='button']").first().innerText().catch(() => "");
      const apiAsc = (await listFolder("Inbox", 0, { dir: "asc" })).messages[0]?.subject || "";
      const apiDesc = (await listFolder("Inbox", 0, { dir: "desc" })).messages[0]?.subject || "";
      note(`d${round}) firstRow="${first.replace(/\s+/g, " ").slice(0, 70)}"`);
      note(`d${round}) apiAsc0="${apiAsc.slice(0, 40)}" apiDesc0="${apiDesc.slice(0, 40)}"`);
      await shot(page, `probe-sort-after-r${round}`);
    }
  }
} finally {
  await shot(page, "probe-list-final").catch(() => {});
  await ctx.close().catch(() => {});
  for (const f of ["Inbox", "Trash"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects([...Object.values(S), S.del + "-r2", FOLDER, FOLDER + "-r2"]).catch(() => 0);
  await api(`/api/v1/mail/folders?name=${encodeURIComponent(FOLDER)}`, { method: "DELETE" }).catch(() => {});
  await api(`/api/v1/mail/folders?name=${encodeURIComponent(FOLDER + "-r2")}`, { method: "DELETE" }).catch(() => {});
  console.log(`cleanup: expunged=${purged}`);
  await closeBrowser();
}

console.log("\n===== probe log =====");
console.log(log.join("\n"));
