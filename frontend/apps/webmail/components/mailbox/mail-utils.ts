import type { OutboundAttachment } from "@/lib/api";

// System IMAP flags are excluded from the label list; custom keywords (tags)
// are any other flag names.
export const SYSTEM_FLAGS = new Set(["\\Seen", "\\Answered", "\\Flagged", "\\Deleted", "\\Draft", "\\Recent"]);

// Saved searches live in localStorage and show up as virtual folders in the
// sidebar, FastMail-style.
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
// everything else becomes paragraphs.
export function textToHtml(text: string): string {
  const out: string[] = [];
  let inQuote = false;
  const closeQuote = () => {
    if (inQuote) {
      out.push("</blockquote>");
      inQuote = false;
    }
  };
  for (const raw of text.replace(/\r\n/g, "\n").split("\n")) {
    const m = raw.match(/^>\s?(.*)$/);
    if (m) {
      if (!inQuote) {
        out.push("<blockquote>");
        inQuote = true;
      }
      out.push(`<p>${escHtml(m[1]) || "<br>"}</p>`);
    } else {
      closeQuote();
      out.push(`<p>${escHtml(raw) || "<br>"}</p>`);
    }
  }
  closeQuote();
  return out.join("");
}
