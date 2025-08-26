import {describe, expect, it} from "vitest";
import {textToHtml} from "../../components/mailbox/mail-utils";

// Simulates TipTap's getText over the HTML the compose editor receives:
// each <p> block contributes its text, blocks are joined with "\n\n",
// and a <br> inside a block counts as a single "\n". This mirrors the
// editor->plain-text round trip that produces the sent body.
function tipTapRoundTrip(html: string): string {
  const blocks: string[] = [];
  for (const m of html.matchAll(/<p>([\s\S]*?)<\/p>/g)) {
    blocks.push(m[1].replace(/<br\s*\/?>/gi, "\n").replace(/<[^>]+>/g, ""));
  }
  return blocks.join("\n\n");
}

describe("textToHtml blank-line round trip", () => {
  it("preserves 4 blank lines inside a plain body", () => {
    const src = "第一行\n\n\n\n\n第五行";
    const html = textToHtml(src);
    expect(html).toBe("<p>第一行<br><br><br><br><br>第五行</p>");
    expect(tipTapRoundTrip(html)).toBe(src);
  });

  it("preserves blank lines inside a quoted block", () => {
    const src = "> 第一行\n> \n> \n> \n> \n> 第五行";
    const html = textToHtml(src);
    expect(html).toBe("<blockquote><p>第一行<br><br><br><br><br>第五行</p></blockquote>");
    expect(tipTapRoundTrip(html)).toBe("第一行\n\n\n\n\n第五行");
  });

  it("round-trips the reply quote pipeline (header + quoted body)", () => {
    const textBody = "第一行\r\n\r\n\r\n\r\n\r\n第五行\r\n";
    const quoted =
      "\n\nOn 2026-08-26, admin@example.com wrote:\n" +
      textBody
        .trim()
        .split("\n")
        .map((l) => `> ${l}`)
        .join("\n");
    const html = textToHtml(quoted);
    const plain = tipTapRoundTrip(html);
    // The quoted part must keep exactly 4 blank lines (5 newlines).
    const quotedPart = plain.slice(plain.indexOf("第一行"), plain.indexOf("第五行") + "第五行".length);
    expect(quotedPart).toBe("第一行\n\n\n\n\n第五行");
  });

  it("keeps a single blank line between two paragraphs", () => {
    expect(tipTapRoundTrip(textToHtml("a\n\nb"))).toBe("a\n\nb");
  });

  it("escapes HTML in quoted content", () => {
    const html = textToHtml("> a < b & c");
    expect(html).toBe("<blockquote><p>a &lt; b &amp; c</p></blockquote>");
  });
});
