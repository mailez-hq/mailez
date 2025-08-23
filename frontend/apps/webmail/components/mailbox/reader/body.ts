// Pure body-parsing helpers for the reading pane.

export type Segment = { type: "p" | "quote"; lines: string[] };

export function parseBody(text: string): Segment[] {
  const segs: Segment[] = [];
  let para: string[] | null = null;
  let quote: string[] | null = null;
  const flushPara = () => {
    if (para) {
      segs.push({ type: "p", lines: para });
      para = null;
    }
  };
  const flushQuote = () => {
    if (quote) {
      segs.push({ type: "quote", lines: quote });
      quote = null;
    }
  };
  for (const raw of text.split(/\r?\n/)) {
    const m = raw.match(/^>\s?(.*)$/);
    if (m) {
      flushPara();
      if (!quote) quote = [];
      quote.push(m[1] || "");
    } else if (raw.trim() === "") {
      flushPara();
      flushQuote();
    } else {
      flushQuote();
      if (!para) para = [];
      para.push(raw);
    }
  }
  flushPara();
  flushQuote();
  return segs;
}

export function getSnippet(text: string, maxLength = 120): string {
  const segments = parseBody(text);
  const plain = segments
    .filter((s) => s.type === "p")
    .flatMap((s) => s.lines)
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
