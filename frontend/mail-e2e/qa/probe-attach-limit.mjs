// Finds the real attachment size ceiling: same payload shape, growing sizes,
// sent straight to the backend (no Next proxy) and again through the webmail
// proxy for the sizes that fail.
// Run: cd frontend && node mail-e2e/qa/probe-attach-limit.mjs
import {
  BASE, USER, api, cleanupFolder, closeBrowser, cookie, listFolder, purgeSubjects,
} from "./lib.mjs";

const BACKEND = "http://127.0.0.1:8081";
const stamp = Date.now();
const sizes = [1, 5, 8, 10, 11];
const subjects = [];
const cookieHeader = await cookie();

async function attempt(url, subject, mb) {
  const data = Buffer.alloc(mb * 1024 * 1024, 0x41).toString("base64");
  const payload = {
    to: [USER.email], cc: [], bcc: [], subject, body: "attachment size probe",
    attachments: [{ filename: `probe-${mb}mb.bin`, content_type: "application/octet-stream", size: mb * 1024 * 1024, data }],
  };
  const t0 = Date.now();
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json", cookie: cookieHeader },
      body: JSON.stringify(payload),
    });
    const text = (await res.text()).slice(0, 100);
    return { status: res.status, ms: Date.now() - t0, text: text.replace(/\s+/g, " ") };
  } catch (e) {
    return { status: `ERR`, ms: Date.now() - t0, text: e.message };
  }
}

try {
  for (const mb of sizes) {
    const subject = `E2E-ATT${mb}MB-${stamp}`;
    subjects.push(subject);
    const direct = await attempt(`${BACKEND}/api/v1/mail/send`, subject, mb);
    console.log(`backend-direct ${mb}MB -> ${direct.status} in ${direct.ms}ms :: ${direct.text}`);
  }
  // through the webmail proxy for the two boundary sizes
  for (const mb of [10, 11]) {
    const subject = `E2E-ATTP${mb}MB-${stamp}`;
    subjects.push(subject);
    const proxied = await attempt(`${BASE}/api/v1/mail/send`, subject, mb);
    console.log(`webmail-proxy  ${mb}MB -> ${proxied.status} in ${proxied.ms}ms :: ${proxied.text}`);
  }
  for (const mb of sizes) {
    const subject = `E2E-ATT${mb}MB-${stamp}`;
    const hit = (await listFolder("Inbox", 0)).messages.find((m) => (m.subject || "").includes(subject));
    console.log(`  delivered ${mb}MB: ${hit ? "yes uid=" + hit.uid : "no"}`);
  }
} finally {
  for (const f of ["Inbox", "Sent"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects(subjects).catch(() => 0);
  console.log(`cleanup: expunged=${purged}`);
  await closeBrowser();
}
