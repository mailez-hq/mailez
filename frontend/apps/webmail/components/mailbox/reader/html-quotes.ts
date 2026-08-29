// Folds quoted reply history in HTML mail bodies into native <details>
// disclosures (Gmail's "show quoted text" model). Runs client-side on the
// already-sanitized HTML string: the engine auto-generates html_body from
// plain text by turning "> " lines into <blockquote>s, so without this pass
// every thread member re-renders the whole nested history that the members
// above it already show.

const ATTRIBUTION_RE = /^on\s.+wrote:\s*$/i;

function isAttribution(el: Element | null): boolean {
  if (!el) return false;
  const tag = el.tagName;
  if (tag !== "DIV" && tag !== "P" && tag !== "SPAN") return false;
  return ATTRIBUTION_RE.test((el.textContent || "").trim());
}

// foldHtmlQuotes wraps every top-level blockquote (blockquotes nested inside
// another blockquote are part of that outer quote and stay as-is) in a closed
// <details>, pulling the "On … wrote:" attribution line in front of it so the
// header collapses together with the history it introduces. Returns the html
// unchanged when there is nothing to fold or when no DOMParser is available
// (the render tree calling this never server-renders, but stay defensive).
export function foldHtmlQuotes(html: string, summaryLabel: string): string {
  if (!html.includes("<blockquote") || typeof DOMParser === "undefined") {
    return html;
  }
  const doc = new DOMParser().parseFromString(html, "text/html");
  // Iterate a static snapshot: moving nodes mid-loop would otherwise skip
  // siblings.
  const quotes = Array.from(doc.body.querySelectorAll("blockquote"));
  let folded = false;
  for (const q of quotes) {
    // Nested quotes ride along with their outermost wrapper.
    if (q.parentElement?.closest("blockquote")) continue;

    const details = doc.createElement("details");
    details.className = "mail-quote-fold";
    const summary = doc.createElement("summary");
    summary.textContent = summaryLabel;
    details.append(summary);

    // Pull the attribution header ("On … wrote:"), plus stray <br>s around
    // it, inside the fold so the collapsed row reads as one unit.
    let prev: Element | null = q.previousElementSibling;
    const moved: Element[] = [];
    while (prev) {
      if (prev.tagName === "BR") {
        moved.unshift(prev);
        prev = prev.previousElementSibling;
        continue;
      }
      if (isAttribution(prev)) {
        moved.unshift(prev);
        prev = prev.previousElementSibling;
        continue;
      }
      break;
    }
    q.replaceWith(details);
    details.append(q);
    for (const m of moved) details.insertBefore(m, q);
    folded = true;
  }
  if (!folded) return html;
  return doc.body.innerHTML;
}
