import {describe, expect, it} from "vitest";
import {Editor} from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import {normalizeQuoteBody, quoteBlockText, stripCollapseMarkers, textToHtml} from "../../components/mailbox/mail-utils";
import {CollapsibleBlockquote} from "../../components/compose/collapsible-blockquote";

// roundTrip feeds HTML through the real TipTap editor and reads back the
// plain text exactly the way the compose editor produces a sent body:
// blockSeparator "\n\n" plus the blockquote textSerializer.
function roundTrip(html: string): string {
  const editor = new Editor({
    extensions: [StarterKit],
    content: html,
  });
  const text = editor.getText({textSerializers: {blockquote: ({node}) => quoteBlockText(node)}});
  editor.destroy();
  return text;
}

// composeEditor mirrors the compose editor's extension set for the quote
// node, so collapsed markers round-trip through the real parse/render path.
function composeEditor(content: string) {
  return new Editor({
    extensions: [
      StarterKit.configure({blockquote: false}),
      CollapsibleBlockquote,
    ],
    content,
  });
}

describe("textToHtml blank-line round trip", () => {
  it("preserves 4 blank lines inside a plain body", () => {
    const src = "第一行\n\n\n\n\n第五行";
    const html = textToHtml(src);
    expect(html).toBe("<p>第一行<br><br><br><br><br>第五行</p>");
    expect(roundTrip(html)).toBe(src);
  });

  it("preserves blank lines inside a quoted block", () => {
    const src = "> 第一行\n> \n> \n> \n> \n> 第五行";
    const html = textToHtml(src);
    expect(html).toBe("<blockquote><p>第一行<br><br><br><br><br>第五行</p></blockquote>");
    expect(roundTrip(html)).toBe(src);
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
    const plain = roundTrip(textToHtml(quoted));
    // The header stays a plain paragraph; the quoted part follows after a
    // single separator and keeps the exact "> " lines of the original.
    expect(plain).toBe(
      "On 2026-08-26, admin@example.com wrote:\n\n" +
        "> 第一行\n> \n> \n> \n> \n> 第五行",
    );
  });

  it("keeps a single blank line between two paragraphs", () => {
    expect(roundTrip(textToHtml("a\n\nb"))).toBe("a\n\nb");
  });

  it("escapes HTML in quoted content", () => {
    const html = textToHtml("> a < b & c");
    expect(html).toBe("<blockquote><p>a &lt; b &amp; c</p></blockquote>");
  });

  it("does not fold leading blank lines into the first paragraph", () => {
    const html = textToHtml("\n\nOn 2026-08-26, admin@example.com wrote:\n> hi");
    expect(html).toBe("<p>On 2026-08-26, admin@example.com wrote:</p><blockquote><p>hi</p></blockquote>");
  });
});

describe("reply quote blank-line stability", () => {
  it("does not grow blank lines across reply cycles", () => {
    // Simulates the full reply chain: every generation quotes the previous
    // body the way quoteText() does and runs it through the editor pipeline.
    const quote = (text: string) =>
      "\n\nOn 2026-08-26, admin@example.com wrote:\n" +
      text
        .split("\n")
        .map((l) => `> ${l}`)
        .join("\n");

    let body = "第一行\n第二行";
    const maxRuns: number[] = [];
    for (let i = 0; i < 6; i++) {
      const plain = roundTrip(textToHtml(quote(body)));
      body = `回复${i}\n${plain}`;
      const runs = (body.match(/\n{2,}/g) || []).map((s) => s.length);
      maxRuns.push(Math.max(0, ...runs));
      // Nested quotes keep their "> " markers (proper email quote nesting).
      expect(body).toContain("> 第一行");
    }
    // The single separator before the quoted block must never accumulate:
    // every generation stays at exactly one blank line (max newline run 2).
    for (const run of maxRuns) expect(run).toBe(2);
  });
});

describe("normalizeQuoteBody", () => {
  it("collapses pathological blank-line runs stored by older pipelines", () => {
    // uid 7 style pollution: 3 blank lines before the quoted line.
    expect(normalizeQuoteBody("wrote:\r\n\r\n\r\n\r\neeee\r\n")).toBe("wrote:\n\neeee\n");
  });

  it("keeps ordinary single and double blank lines", () => {
    expect(normalizeQuoteBody("a\n\nb")).toBe("a\n\nb");
    expect(normalizeQuoteBody("a\n\n\nb")).toBe("a\n\nb");
  });

  it("normalizes CR line endings too", () => {
    expect(normalizeQuoteBody("a\rb")).toBe("a\nb");
  });
});

describe("Gmail-style collapsed quote", () => {
  it("marks reply quotes with data-collapsed and leaves plain quotes alone", () => {
    const src = "On 2026-08-26, admin@example.com wrote:\n> eeee\n> dddd";
    expect(textToHtml(src, {collapseQuote: true})).toBe(
      "<p>On 2026-08-26, admin@example.com wrote:</p><blockquote data-collapsed=\"true\"><p>eeee<br>dddd</p></blockquote>",
    );
    expect(textToHtml(src)).toBe(
      "<p>On 2026-08-26, admin@example.com wrote:</p><blockquote><p>eeee<br>dddd</p></blockquote>",
    );
  });

  it("parses the marker into the collapsed attribute and keeps the full text", () => {
    const src = "<p>On 2026-08-26, admin@example.com wrote:</p><blockquote data-collapsed=\"true\"><p>eeee</p></blockquote>";
    const editor = composeEditor(src);
    let collapsed = false;
    let found = false;
    editor.state.doc.descendants((node) => {
      if (node.type.name === "blockquote") {
        found = true;
        collapsed = !!node.attrs.collapsed;
      }
      return true;
    });
    expect(found).toBe(true);
    expect(collapsed).toBe(true);
    expect(editor.getHTML()).toContain("data-collapsed=\"true\"");
    expect(
      editor.getText({textSerializers: {blockquote: ({node}) => quoteBlockText(node)}}),
    ).toBe("On 2026-08-26, admin@example.com wrote:\n\n> eeee");
    editor.destroy();
  });

  it("stripCollapseMarkers removes only the compose-only attribute", () => {
    expect(
      stripCollapseMarkers('<p>hi</p><blockquote data-collapsed="true"><p>quote</p></blockquote>'),
    ).toBe("<p>hi</p><blockquote><p>quote</p></blockquote>");
    expect(stripCollapseMarkers("<p>no marker</p>")).toBe("<p>no marker</p>");
  });
});
