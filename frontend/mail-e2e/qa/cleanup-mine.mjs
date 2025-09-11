// Purges the exact E2E-* subjects this audit created. Subjects can be passed
// as arguments; with none, it cleans the known probe leftovers.
// Run: cd frontend && node mail-e2e/qa/cleanup-mine.mjs [subject ...]
import { purgeSubjects } from "./lib.mjs";

const defaults = [
  "E2E-DRAFTPROBE-1789137499139",
  "E2E-DRAFT-1789137745857",
  "E2E-DRAFTLOSS-1789137745857",
];
const subjects = process.argv.slice(2).filter(Boolean);
const list = subjects.length ? subjects : defaults;
const purged = await purgeSubjects(list);
console.log(`purged=${purged} for ${list.length} subject(s)`);
process.exit(0);
