// Smoke for the two list-row annotations that need the row's header section
// (newsletter category, burn-after marker) after the engine started memoising
// body sections. Sends one burn-after mail to itself, checks the list row,
// then cleans the message up by subject.
import { api, check, cleanupFolder, purgeSubjects, seedMail } from "./lib.mjs";

const MOD = "row-annotations";
const SUBJ = `E2E-ANNOT-${Date.now()}`;
const folder = "Inbox";

try {
  await seedMail(SUBJ, { body: "ANNOTBODY marker text", burn_after_minutes: 5 });
  let row = null;
  for (let i = 0; i < 20 && !row; i++) {
    await new Promise((r) => setTimeout(r, 1500));
    const r = await api(`/api/v1/mail/messages?folder=${folder}&page=0`);
    row = (r.json || []).find((x) => x.subject === SUBJ) || null;
  }
  check(MOD, "burn row is annotated from its header", row?.burn_after_minutes === 5, `burn=${row?.burn_after_minutes}`);
  // The no-leak contract is on the rendered row (the UI gates the excerpt on
  // burn_after_minutes), so it lives in qa/verify-pending.mjs, not here; the
  // API has always carried the preview field for these rows.
  check(MOD, "burn row keeps its identity fields", row?.subject === SUBJ && row?.uid > 0, `uid=${row?.uid}`);

  // A plain message in the same page must still carry a preview: that is the
  // 4KB partial section the memo also serves.
  const other = await api(`/api/v1/mail/messages?folder=${folder}&page=0`);
  const withPreview = (other.json || []).filter((x) => x.preview && x.preview.length > 0).length;
  check(MOD, "previews still hydrate", withPreview > 0, `rows with preview=${withPreview}`);
} finally {
  await cleanupFolder(folder, [SUBJ]);
  await purgeSubjects([SUBJ]);
}
process.exit(0);
