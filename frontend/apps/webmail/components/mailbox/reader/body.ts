// Pure body-parsing helpers for the reading pane.

export type Segment = { type: "p" | "quote"; lines: string[] };

// A reply-attribution header line ("On Aug 29, 2026 17:24, x@y.com wrote:").
// It belongs INSIDE the collapsible quote region it introduces - keeping it
// as a plain paragraph leaves a stray "On … wrote:" above every collapsed
// quote and leaks into list snippets.
const ATTRIBUTION_RE = /^\s*on\s.+wrote:\s*$/i;

export function parseBody(text: string): Segment[] {
  const segs: Segment[] = [];
  const lines = text.split(/\r?\n/);
  // Drop trailing empty lines so a body ending in newlines doesn't leave a
  // stray empty line at the bottom.
  while (lines.length > 0 && lines[lines.length - 1].trim() === "") lines.pop();
  let para: string[] | null = null;
  let quote: string[] | null = null;
  const flush = (seg: Segment) => {
    if (seg.lines.length > 0) segs.push(seg);
  };
  for (const raw of lines) {
    const m = raw.match(/^>\s?(.*)$/);
    if (m) {
      if (para) {
        flush({ type: "p", lines: para });
        para = null;
      }
      if (!quote) quote = [];
      quote.push(m[1] || "");
    } else if (ATTRIBUTION_RE.test(raw)) {
      // Attribution header opens the quote region (it folds together
      // with the history it introduces); a regular paragraph after a quote
      // still closes it, so interleaved replies keep their own segments.
      if (para) {
        flush({ type: "p", lines: para });
        para = null;
      }
      if (!quote) quote = [];
      quote.push(raw.trim());
    } else if (raw.trim() === "") {
      // Preserve blank lines inside the open segment: consecutive empty
      // lines become <br>s in the viewer instead of being collapsed away.
      if (para) para.push("");
      else if (quote) quote.push("");
      // Leading blank lines are ignored.
    } else {
      if (quote) {
        flush({ type: "quote", lines: quote });
        quote = null;
      }
      if (!para) para = [];
      para.push(raw);
    }
  }
  if (para) flush({ type: "p", lines: para });
  if (quote) flush({ type: "quote", lines: quote });
  return segs;
}

export function getSnippet(text: string, maxLength = 120): string {
  const segments = parseBody(text);
  const plain = segments
    .filter((s) => s.type === "p")
    // Blank lines are kept for faithful rendering; a snippet flattens them.
    .flatMap((s) => s.lines.filter((l) => l !== ""))
    .join(" ")
    .trim();
  if (plain.length <= maxLength) return plain;
  return plain.slice(0, maxLength).trimEnd() + "…";
}

export function fmtFullDate(d: string) {
  const date = new Date(d);
  // A zero/absent engine date serializes as "0001-01-01T00:00:00Z" (Go's
  // zero time) when a message has no Date header. Treat anything before the
  // common era as "no date" instead of rendering "Jan 1, 0001".
  return Number.isNaN(date.getTime()) || date.getFullYear() < 100
    ? ""
    : date.toLocaleString(undefined, {
        year: "numeric",
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
}

export function fmtShort(d: string) {
  const date = new Date(d);
  if (Number.isNaN(date.getTime()) || date.getFullYear() < 100) return "";
  const now = new Date();
  if (date.toDateString() === now.toDateString())
    return date.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  if (date.getFullYear() === now.getFullYear())
    return date.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  return date.toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}
