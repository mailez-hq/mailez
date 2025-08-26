// Pure body-parsing helpers for the reading pane.

export type Segment = { type: "p" | "quote"; lines: string[] };

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
  return Number.isNaN(date.getTime())
    ? d
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
  if (Number.isNaN(date.getTime())) return "";
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
