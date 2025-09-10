import { apiLogin } from "./helper.mjs";

const B = "http://127.0.0.1:8083";
const c = await apiLogin();
const cookie = `${c.name}=${c.value}`;
const list = async (folder) => {
  const r = await fetch(`${B}/api/v1/mail/messages?folder=${encodeURIComponent(folder)}&page=0&sort=date&dir=desc`, { headers: { cookie } });
  return r.json();
};
const save = (replaceUid, v) =>
  fetch(`${B}/api/v1/mail/draft`, {
    method: "POST",
    headers: { "Content-Type": "application/json", cookie },
    body: JSON.stringify({ subject: "E2E-overwrite", text: `v${v}`, html: `<p>v${v}</p>`, replace_uid: replaceUid, to: [], cc: [], bcc: [] }),
  }).then(async (r) => ({ status: r.status, body: await r.json().catch(() => ({})) }));
const count = () => list("Drafts").then((rows) => rows.filter((m) => m.subject === "E2E-overwrite").length);

// create one draft
const first = await save(0, 1);
const realUid = first.body.uid;
console.log("created uid:", realUid, "| copies:", await count());

// stale-UID save: pretend another tab already replaced it (replace a dead uid)
const dead = await save(999999, 2);
console.log("stale-uid save ->", dead.status, JSON.stringify(dead.body), "| copies:", await count());

// live sequential replace twice
const r1 = await save(realUid, 3);
const r2 = await save(r1.body.uid, 4);
console.log("live replaces ->", r1.status, r2.status, "| copies:", await count());

// cleanup
for (const row of await list("Drafts")) {
  if (row.subject === "E2E-overwrite") {
    await fetch(`${B}/api/v1/mail/delete`, {
      method: "POST", headers: { "Content-Type": "application/json", cookie },
      body: JSON.stringify({ folder: "Drafts", uid: row.uid }),
    });
  }
}
