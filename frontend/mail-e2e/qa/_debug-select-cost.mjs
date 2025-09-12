// Breaks SELECT's cost into the commands the engine maps it to (LIST,
// STATUS, SELECT), so "the snapshot listing is the expensive part" can be
// confirmed or refuted from outside. Run:
//   node qa/_debug-select-cost.mjs
import { ImapClient } from "./lib.mjs";

const folder = process.env.SEL_FOLDER || "Trash";
const rounds = Number(process.env.SEL_ROUNDS || 4);

const c = await ImapClient.connect();
await c.login();
const time = async (label, fn) => {
  const t0 = Date.now();
  const r = await fn();
  console.log(`${label.padEnd(24)} ${Date.now() - t0}ms ${r ?? ""}`);
};
for (let i = 0; i < rounds; i++) {
  await time(`LIST "" "*"`, async () => (await c.listMailboxes()).length + " names");
  await time(`STATUS ${folder}`, async () => JSON.stringify(await c.status(folder)));
  await time(`SELECT ${folder}`, async () => {
    const r = await c.select(folder);
    return `exists=${r.exists}`;
  });
  await time(`UID FETCH 1:50 (ENVELOPE)`, async () => {
    const r = await c.cmd("UID FETCH 1:50 (ENVELOPE UID)", 60000);
    return `${r.lines.length} lines`;
  });
}
await c.logout();
process.exit(0);
