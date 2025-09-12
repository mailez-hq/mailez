// Verifies the branch Select now only takes on a miss: a brand-new account
// with no mailbox documents must still get INBOX (plus the defaults) on its
// first SELECT. Creates the account through the admin API and deletes it
// again in a finally block.
import { ADMIN_BASE, api, check, ImapClient, USER } from "./lib.mjs";

const email = `e2e-fresh-${Date.now()}@example.com`;
const password = "E2eDemo2026!";
const admin = { base: ADMIN_BASE };

const created = await api("/api/v1/users", {
  ...admin,
  method: "POST",
  body: { email, password, quota_bytes: 1048576, enabled: true, displayed_name: "E2E Fresh" },
});
console.log(`create ${email} -> ${created.status}`);

try {
  const c = await ImapClient.connect();
  try {
    await c.login(email, password);
    const sel = await c.select("INBOX");
    check("fresh SELECT INBOX", sel.ok && sel.exists === 0, `ok=${sel.ok} exists=${sel.exists}`);
    const names = await c.listMailboxes();
    const want = ["INBOX", "Trash", "Drafts", "Sent", "Junk"];
    check("fresh defaults provisioned", want.every((n) => names.includes(n)), names.join(","));
    const missing = await c.select("NoSuchFolder");
    check("unknown folder is NO", !missing.ok, missing.lines.at(-1));
  } finally {
    await c.logout();
  }
  const login = await api("/api/v1/sso/login", {
    method: "POST",
    body: { email, pw: password },
    headers: {},
  });
  console.log(`webmail login for the fresh account -> ${login.status}`);
} finally {
  const del = await api(`/api/v1/users/${encodeURIComponent(email)}`, { ...admin, method: "DELETE" });
  console.log(`delete -> ${del.status}`);
}
process.exit(0);
