// Regression: edit a draft -> save -> close -> reopen must show the edited
// content in exactly ONE draft copy. Guards two real bugs we shipped:
//  1) the close-save once fired with a stale render closure and overwrote the
//     just-saved body with the pre-edit text ("draft lost my typing")
//  2) draft saves once appended duplicates instead of replacing
import { chromium } from "playwright";
import { apiLogin } from "./helper.mjs";

const B = "http://127.0.0.1:8083";
const c = await apiLogin();
const cookie = `${c.name}=${c.value}`;
const list = async () => {
  const r = await fetch(`${B}/api/v1/mail/messages?folder=Drafts&page=0&sort=date&dir=desc`, { headers: { cookie } });
  return r.json();
};
const save = (replaceUid, v) =>
  fetch(`${B}/api/v1/mail/draft`, {
    method: "POST",
    headers: { "Content-Type": "application/json", cookie },
    body: JSON.stringify({ subject: "E2E-edit-repro", text: `v${v}`, html: `<p>v${v}</p>`, replace_uid: replaceUid, to: [], cc: [], bcc: [] }),
  }).then((r) => r.json());
const cleanup = async () => {
  for (const row of await list()) {
    if (row.subject === "E2E-edit-repro") {
      await fetch(`${B}/api/v1/mail/delete`, {
        method: "POST", headers: { "Content-Type": "application/json", cookie },
        body: JSON.stringify({ folder: "Drafts", uid: row.uid }),
      });
    }
  }
};

let failed = false;
try {
  await cleanup();
  const seeded = await save(0, 1);
  console.log("seeded draft uid:", seeded.uid);

  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, locale: "zh-CN" });
  await page.context().addCookies([{ name: c.name, value: c.value, domain: "127.0.0.1", path: "/" }]);
  await page.goto(`${B}/mail/Drafts`, { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(6000);

  await page.locator(".mail-scroll [role='button']", { hasText: "E2E-edit-repro" }).first().click();
  await page.waitForTimeout(3000);
  await page.getByRole("button", { name: /^edit|编辑$/i }).first().click({ timeout: 15000 });
  await page.waitForTimeout(2000);
  const editor = page.locator("[contenteditable='true']").first();
  await editor.click();
  await page.keyboard.press("Control+End");
  await editor.type("a");
  await page.waitForTimeout(600);
  await page.getByRole("button", { name: /save draft|保存草稿/i }).first().click();
  await page.waitForTimeout(3000);
  await page.keyboard.press("Escape");
  await page.waitForTimeout(1200);
  const stillOpen = await page.locator("[contenteditable='true']").count();
  if (stillOpen > 0) {
    const xBtn = page.locator("[role='dialog'] button:has(.lucide-x), button[aria-label*='lose' i]").first();
    await xBtn.click({ timeout: 5000 }).catch(() => {});
  }
  await page.waitForTimeout(3500);

  const copies = (await list()).filter((m) => m.subject === "E2E-edit-repro");
  const oneCopy = copies.length === 1;
  console.log(`${oneCopy ? "PASS" : "FAIL"}  draft-edit: exactly one draft copy after edit+save+close — ${copies.map((m) => m.uid).join(",")}`);
  failed = failed || !oneCopy;

  const detailResp = page.waitForResponse((res) => res.url().includes("/mail/message?") && res.ok());
  await page.locator(".mail-scroll [role='button']", { hasText: "E2E-edit-repro" }).first().click();
  const detail = await (await detailResp).json();
  const kept = (detail.text_body || "").includes("a");
  console.log(`${kept ? "PASS" : "FAIL"}  draft-edit: reopened draft keeps the typed content — ${JSON.stringify((detail.text_body || "").slice(0, 20))}`);
  failed = failed || !kept;

  await browser.close();
} finally {
  await cleanup();
}
process.exit(failed ? 1 : 0);
