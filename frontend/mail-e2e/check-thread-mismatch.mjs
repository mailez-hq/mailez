import { apiLogin } from "./helper.mjs";

const B = "http://127.0.0.1:8083";
const c = await apiLogin();
const cookie = `${c.name}=${c.value}`;
const list = async (folder, conversation) => {
  const r = await fetch(`${B}/api/v1/mail/messages?folder=${encodeURIComponent(folder)}&page=0&sort=date&dir=desc${conversation ? "&conversation=1" : ""}`, { headers: { cookie } });
  return r.json();
};

const target = "PGZhNjczZThiY2EyMTliNjg5YmRlMmIwNEBleGFtcGxlLmNvbT4"; // routable id as it appears in list rows
for (const folder of ["Inbox", "Sent", "Drafts", "Trash", "Archive"]) {
  const rows = await list(folder);
  if (!Array.isArray(rows)) {
    console.log(`${folder}: non-array response`, JSON.stringify(rows).slice(0, 80));
    continue;
  }
  const hits = rows.filter((m) => (m.id || "") === target);
  for (const m of hits) {
    console.log(`${folder}: uid=${m.uid} id=${m.id} tid=${JSON.stringify(m.thread_id)} count=${m.thread_count} subj=${JSON.stringify(m.subject)} from=${JSON.stringify(m.from?.[0]?.email)}`);
  }
}

// find the exact row by id across inbox list, then pull its thread
const inbox = await list("Inbox");
const row = (inbox || []).find((m) => m.id?.includes(target));
if (!row) {
  console.log("target row not found in Inbox list");
} else {
  console.log("list row: uid=", row.uid, "tid=", JSON.stringify(row.thread_id), "count=", row.thread_count, "subj=", JSON.stringify(row.subject));
  const tr = await fetch(`${B}/api/v1/mail/thread?folder=Inbox&thread_id=${encodeURIComponent(row.thread_id)}`, { headers: { cookie } });
  const th = await tr.json();
  const msgs = th?.messages || th || [];
  console.log("thread endpoint returned:", Array.isArray(msgs) ? msgs.length : JSON.stringify(th).slice(0, 120), "messages");
  for (const m of Array.isArray(msgs) ? msgs : []) {
    console.log(`  thread member: uid=${m.uid} subj=${JSON.stringify(m.subject)} from=${JSON.stringify(m.from?.[0]?.email)} date=${m.date}`);
  }
}
