import { describe, expect, it } from "vitest";

import { sanitizeMailHTML } from "@/lib/sanitize";

describe("sanitizeMailHTML", () => {
  it("strips scripts, event handlers and embedded objects", () => {
    const out = sanitizeMailHTML(
      '<p>hi</p><script>alert(1)</script><img src="x" onerror="alert(2)"><iframe src="https://evil.test"></iframe><form action="https://evil.test"><input></form>',
    );
    expect(out).toContain("<p>hi</p>");
    expect(out).not.toContain("<script");
    expect(out).not.toContain("onerror");
    expect(out).not.toContain("iframe");
    expect(out).not.toContain("<form");
    expect(out).not.toContain("<input");
  });

  it("keeps formatting, links and the remote-image hook", () => {
    const out = sanitizeMailHTML(
      '<b>bold</b><a href="https://example.com">link</a><table><tr><td>c</td></tr></table><img src="https://x/y.png" data-remote-src="https://x/y.png">',
    );
    expect(out).toContain("<b>bold</b>");
    expect(out).toContain('href="https://example.com"');
    expect(out).toContain("<table>");
    expect(out).toContain('data-remote-src="https://x/y.png"');
  });

  it("is a no-op for empty input", () => {
    expect(sanitizeMailHTML("")).toBe("");
  });
});
