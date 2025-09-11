// Which attribute makes the submission policy reject a message: attachment
// type or size? Four fixed combinations, straight to the backend.
// Run: cd frontend && node mail-e2e/qa/probe-attach-policy.mjs
import { USER, api, cleanupFolder, closeBrowser, cookie, listFolder, purgeSubjects } from "./lib.mjs";

const BACKEND = "http://127.0.0.1:8081";
const stamp = Date.now();
const c = await cookie();
const cases = [
  { tag: "1KB-octet", type: "application/octet-stream", name: "blob.bin", kb: 1 },
  { tag: "1KB-text", type: "text/plain", name: "note.txt", kb: 1 },
  { tag: "1MB-text", type: "text/plain", name: "big.txt", kb: 1024 },
  { tag: "1MB-octet", type: "application/octet-stream", name: "big.bin", kb: 1024 },
  { tag: "1MB-pdf", type: "application/pdf", name: "doc.pdf", kb: 1024 },
];
const subjects = cases.map((x) => `E2E-POLICY-${x.tag}-${stamp}`);

try {
  for (const [i, cs] of cases.entries()) {
    const bytes = Buffer.alloc(cs.kb * 1024, 0x41);
    const res = await fetch(`${BACKEND}/api/v1/mail/send`, {
      method: "POST",
      headers: { "Content-Type": "application/json", cookie: c },
      body: JSON.stringify({
        to: [USER.email], cc: [], bcc: [], subject: subjects[i], body: "policy probe",
        attachments: [{ filename: cs.name, content_type: cs.type, size: bytes.length, data: bytes.toString("base64") }],
      }),
    });
    const text = (await res.text()).slice(0, 90);
    console.log(`${cs.tag.padEnd(10)} ${cs.type.padEnd(26)} -> ${res.status} ${text.replace(/\s+/g, " ")}`);
  }
  await new Promise((r) => setTimeout(r, 3000));
  const inbox = await listFolder("Inbox", 0);
  for (const [i, cs] of cases.entries()) {
    const hit = inbox.messages.find((m) => (m.subject || "").includes(subjects[i]));
    console.log(`  delivered ${cs.tag}: ${hit ? "yes uid=" + hit.uid : "no"}`);
  }
} finally {
  for (const f of ["Inbox", "Sent"]) await cleanupFolder(f).catch(() => {});
  const purged = await purgeSubjects(subjects).catch(() => 0);
  console.log(`cleanup: expunged=${purged}`);
  await closeBrowser();
}
