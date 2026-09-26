import type { Node as ProseMirrorNode } from "@tiptap/pm/model";
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

import type { MailMessage, MailThread } from "@/lib/api";

// normalizeMessage patches the wire-vs-type gap: the backend can emit null
// for array-typed fields (Go zero values / omitempty), and every unguarded
// .flags.includes / .from[0] in a render path or handler then throws and
// takes the whole route down. Every message entering the store passes
// through here, so components can rely on the arrays existing.
export function normalizeMessage(m: MailMessage): MailMessage {
  return {
    ...m,
    flags: m.flags ?? [],
    from: m.from ?? [],
    to: m.to ?? [],
    cc: m.cc ?? [],
    bcc: m.bcc ?? [],
    attachments: m.attachments ?? [],
  };
}

export function normalizeThread(th: MailThread): MailThread {
  return { ...th, messages: (th?.messages ?? []).map(normalizeMessage) };
}

// selectionKey identifies a list row in the bulk-selection set. IMAP uids are
// unique per mailbox only, and search-all mixes folders, so a bare uid can
// name two different rows.
export function selectionKey(m: MailMessage, fallbackFolder: string): string {
  return `${m.folder || fallbackFolder}/${m.uid}`;
}

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

export const SIGNATURE_ATTR = "data-mailez-signature";

export function buildSignatureHTML(id: number | null, bodyHtml: string): string {
  return `<div ${SIGNATURE_ATTR}="${id && id > 0 ? id : "custom"}"><p>--</p>${bodyHtml}</div>`;
}

export function appendSignatureHTML(html: string, id: number | null, bodyHtml: string): string {
  return `${html}<p><br></p>${buildSignatureHTML(id, bodyHtml)}`;
}

export function stripSignatureHTML(html: string): string {
  if (typeof DOMParser === "undefined") return html;
  const doc = new DOMParser().parseFromString(html, "text/html");
  const block = doc.querySelector(`[${SIGNATURE_ATTR}]`);
  if (!block) return html;
  const spacer = block.previousElementSibling;
  const follower = block.nextElementSibling;
  block.remove();
  const emptyParagraph = (el: Element | null) =>
    !!el && el.tagName === "P" && el.textContent?.trim() === "" && !el.querySelector("img,table,hr");
  if (emptyParagraph(spacer)) {
    spacer?.remove();
  } else if (emptyParagraph(follower)) {
    follower?.remove();
  }
  return doc.body.innerHTML;
}

export function currentSignatureID(html: string): number | null {
  if (typeof DOMParser === "undefined") return null;
  const doc = new DOMParser().parseFromString(html, "text/html");
  const raw = doc.querySelector(`[${SIGNATURE_ATTR}]`)?.getAttribute(SIGNATURE_ATTR);
  const id = Number(raw);
  return Number.isFinite(id) && id > 0 ? id : null;
}

export function swapSignatureHTML(
  html: string,
  id: number | null,
  bodyHtml: string,
  above = false,
): string {
  if (typeof DOMParser === "undefined") return html;
  const doc = new DOMParser().parseFromString(html, "text/html");
  const existing = doc.querySelector(`[${SIGNATURE_ATTR}]`);
  if (!bodyHtml) return existing ? stripSignatureHTML(html) : html;
  if (!existing) return above ? `${buildSignatureHTML(id, bodyHtml)}${html}` : appendSignatureHTML(html, id, bodyHtml);
  const holder = doc.createElement("div");
  holder.innerHTML = buildSignatureHTML(id, bodyHtml);
  existing.replaceWith(...Array.from(holder.childNodes));
  return doc.body.innerHTML;
}

export function appendSignatureText(text: string, bodyText: string): string {
  return text ? `${text}\n\n-- \n${bodyText}` : `-- \n${bodyText}`;
}

export function stripSignatureText(text: string): string {
  return text.replace(/\n\n-- \n[\s\S]*$/, "");
}

export function spliceSignatureText(
  text: string,
  remove: string | null,
  insert: string | null,
  above: boolean,
): string {
  let body = text;
  if (remove && body.includes(remove)) {
    const at = body.indexOf(remove);
    const before = body.slice(0, at).replace(/\n+$/, "");
    const after = body.slice(at + remove.length).replace(/^\n+/, "");
    body = before && after ? `${before}\n\n${after}` : before || after;
  }
  if (!insert) return body;
  if (!body) return insert;
  return above ? `${insert}\n\n${body}` : `${body}\n\n${insert}`;
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
// collapseQuote marks the quote blockquote with data-collapsed="true" so the
// compose editor renders it folded ("…" until clicked), while the
// full quoted text still travels with the sent message.
export function textToHtml(text: string, opts?: { collapseQuote?: boolean }): string {
  const out: string[] = [];
  let run: {quote: boolean; lines: string[]} | null = null;
  const flush = () => {
    if (!run) return;
    const inner = run.lines.map((l) => escHtml(l)).join("<br>") || "<br>";
    const p = `<p>${inner}</p>`;
    out.push(
      run.quote
        ? opts?.collapseQuote
          ? `<blockquote data-collapsed="true">${p}</blockquote>`
          : `<blockquote>${p}</blockquote>`
        : p,
    );
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

// stripCollapseMarkers removes the compose-only data-collapsed attribute
// before a message goes on the wire. The folded state is a UI affordance of
// the composer; recipients must always see the full quote in the HTML body.
export function stripCollapseMarkers(html: string): string {
  return html.replace(/\sdata-collapsed="true"/g, "");
}

// serializeBlockquote is the inverse of textToHtml's quote parsing. TipTap's
// getTextBetween inserts a "\n\n" block separator for EVERY block node —
// including the <blockquote> wrapper and each <p> inside it — so serializing
// a quote as bare text would add blank lines on every reply round trip.
// Rendering quoted content as "> " prefixed lines (the standard email quote
// format) keeps the stored plain-text body stable across reply cycles.
// The caller feeds it the blockquote's text content (with hard breaks already
// expanded to "\n").
export function serializeBlockquote(text: string): string {
  return text
    .split("\n")
    .map((line) => `> ${line}`)
    .join("\n");
}

// normalizeQuoteBody cleans a quoted original before it becomes "> " lines:
// CRLF/CR line endings become LF, and runs of 3+ consecutive blank lines
// collapse to a single blank line (also neutralizing runs accumulated by
// older reply pipelines inside stored messages). Ordinary single (and
// double) blank lines are left untouched.
export function normalizeQuoteBody(text: string): string {
  return text
    .replace(/\r\n/g, "\n")
    .replace(/\r/g, "\n")
    .replace(/\n{3,}/g, "\n\n");
}

// quoteBlockText extracts a blockquote node's plain text for the compose
// editor's text serializer: hard breaks become single newlines, paragraphs
// inside the quote are separated by "\n\n", then every line is prefixed with
// "> " (see serializeBlockquote). Using textBetween instead of textContent
// keeps hard breaks from vanishing.
export function quoteBlockText(node: ProseMirrorNode): string {
  return serializeBlockquote(
    node.textBetween(0, node.content.size, "\n\n", (leaf) =>
      leaf.type.name === "hardBreak" ? "\n" : "",
    ),
  );
}
