import type { MailSearchSpec, OutboundAttachment } from "@/lib/api";

// System IMAP flags and reserved keywords are excluded from the label list;
// custom keywords (tags) are any other flag names. $Pin and $Snoozed* drive
// the pinned / snoozed features and must never surface as user labels.
export const SYSTEM_FLAGS = new Set([
  "\\Seen", "\\Answered", "\\Flagged", "\\Deleted", "\\Draft", "\\Recent",
  "$Pin", "$Snoozed",
]);

export const PIN_FLAG = "$Pin";
export const MUTE_FLAG = "$Muted";
export const SNOOZE_FLAG = "$Snoozed";
export const SNOOZE_UNTIL_PREFIX = "$SnoozedUntil-";

// Custom keywords are case-insensitive on the wire and go-imap canonicalizes
// unknown flags to lowercase, so match case-insensitively.
const hasFlag = (flags: string[], flag: string) =>
  flags.some((f) => f.toLowerCase() === flag.toLowerCase());

export const isPinned = (m: { flags: string[] }) => hasFlag(m.flags, PIN_FLAG);
export const isMuted = (m: { flags: string[] }) => hasFlag(m.flags, MUTE_FLAG);
  export const isSnoozed = (m: { flags?: string[] } | null | undefined) =>
    m?.flags ? hasFlag(m.flags, SNOOZE_FLAG) : false;

// isUserLabel reports whether a flag is a genuine user-created tag rather
// than a system flag or an internal keyword ($Pin/$Muted/$Snoozed*), so the
// latter never surface as labels in the sidebar or on message rows.
export function isUserLabel(f: string): boolean {
  const low = f.toLowerCase();
  return (
    !f.startsWith("\\") &&
    !SYSTEM_FLAGS.has(f) &&
    !low.startsWith("$snoozed") &&
    low !== "$muted" &&
    !low.startsWith("$pin")
  );
}

// snoozeUntil parses the $SnoozedUntil-<unix> keyword into a Date, or null.
export function snoozeUntil(flags: string[]): Date | null {
  for (const f of flags) {
    if (f.toLowerCase().startsWith(SNOOZE_UNTIL_PREFIX.toLowerCase())) {
      const n = Number(f.slice(SNOOZE_UNTIL_PREFIX.length));
      if (Number.isFinite(n)) return new Date(n * 1000);
    }
  }
  return null;
}

// Label palette used when a label has no explicit color yet; assignment is
// deterministic per name so the same label keeps its color across sessions.
export const LABEL_PALETTE = [
  "#2E6E8E",
  "#2F8E6C",
  "#B4762A",
  "#A04F7A",
  "#6B4FA0",
  "#8E4A2E",
  "#4F7AA0",
  "#5A8E3E",
];

export function labelColor(name: string, assigned?: string) {
  if (assigned) return assigned;
  let h = 0;
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) >>> 0;
  return LABEL_PALETTE[h % LABEL_PALETTE.length];
}

// Saved searches live in localStorage and show up as virtual folders in the
// sidebar. They are either a plain keyword query (bookmark in
// the list toolbar) or a structured spec saved from the search builder.
export type SavedSearch =
  | { id: number; kind: "query"; name: string; query: string }
  | { id: number; kind: "spec"; name: string; spec: MailSearchSpec };

export const SAVED_SEARCH_KEY = "mailez.savedSearches";

// Per-attachment size cap keeps compose messages inside the MTA size limit.
export const MAX_ATTACHMENT_BYTES = 20 * 1024 * 1024;

// escHtml escapes text for safe embedding into HTML bodies (signatures,
// quoted plain-text drafts).
export function escHtml(s: string) {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

// readFileAsBase64 converts a picked/dropped file into the wire attachment
// shape (base64 data) used by the mail send/draft APIs.
export function readFileAsBase64(file: File): Promise<OutboundAttachment> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const data = String(reader.result || "").split(",")[1] || "";
      resolve({
        filename: file.name,
        content_type: file.type || "application/octet-stream",
        size: file.size,
        data,
      });
    };
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

export function fmtBytes(n: number) {
  if (n >= 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${Math.max(1, Math.round(n / 1024))} KB`;
}

// playChime rings a short notification tone without shipping an audio file.
export function playChime() {
  try {
    const Ctx = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
    const ctx = new Ctx();
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.connect(gain);
    gain.connect(ctx.destination);
    osc.frequency.value = 880;
    gain.gain.setValueAtTime(0.05, ctx.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + 0.4);
    osc.start();
    osc.stop(ctx.currentTime + 0.4);
  } catch {
    // audio unavailable; skip silently
  }
}

// textToHtml converts a plain-text draft (reply quoting, AI drafts) into
// sanitized HTML for the rich-text editor: "> " lines become blockquotes,
// everything else becomes paragraphs. Consecutive lines of a run are folded
// into ONE <p> with <br> separators (blank lines become extra <br>s) so that
// TipTap's plain-text readback (block separator "\n\n", hard break "\n")
// round-trips the original newlines exactly — per-line <p>s with empty
// <p><br></p> blocks would double every blank line on readback.
export function textToHtml(text: string): string {
  const out: string[] = [];
  let run: {quote: boolean; lines: string[]} | null = null;
  const flush = () => {
    if (!run) return;
    const inner = run.lines.map((l) => escHtml(l)).join("<br>") || "<br>";
    const p = `<p>${inner}</p>`;
    out.push(run.quote ? `<blockquote>${p}</blockquote>` : p);
    run = null;
  };
  for (const raw of text.replace(/\r\n/g, "\n").split("\n")) {
    const m = raw.match(/^>\s?(.*)$/);
    const quote = !!m;
    // Ignore leading blank lines (and blank lines before the first quoted
    // line) instead of folding them into the first paragraph.
    if (!run && raw.trim() === "") continue;
    if (run && run.quote !== quote) flush();
    if (!run) run = {quote, lines: []};
    run.lines.push(quote ? m![1] : raw);
  }
  flush();
  return out.join("");
}
