import {describe, expect, it} from "vitest";
import {getSnippet, parseBody} from "../../components/mailbox/reader/body";

describe("parseBody", () => {
  it("preserves consecutive blank lines as empty lines", () => {
    const segs = parseBody("第一行\r\n\r\n\r\n\r\n\r\n第五行\r\n");
    expect(segs).toEqual([{type: "p", lines: ["第一行", "", "", "", "", "第五行"]}]);
  });

  it("keeps one blank line between two paragraphs inside the same segment", () => {
    const segs = parseBody("a\n\nb");
    expect(segs).toEqual([{type: "p", lines: ["a", "", "b"]}]);
  });

  it("drops trailing blank lines", () => {
    const segs = parseBody("a\n\n\n");
    expect(segs).toEqual([{type: "p", lines: ["a"]}]);
  });

  it("splits quoted lines into a quote segment without losing blank lines", () => {
    const segs = parseBody("a\n\n> quote\n>\n> next\n\nb");
    expect(segs).toEqual([
      {type: "p", lines: ["a", ""]},
      {type: "quote", lines: ["quote", "", "next", ""]},
      {type: "p", lines: ["b"]},
    ]);
  });

  it("returns an empty array for blank input", () => {
    expect(parseBody("")).toEqual([]);
    expect(parseBody("\n\n")).toEqual([]);
  });

  it("folds the 'On … wrote:' attribution into the quote region it opens", () => {
    // Conversation model: the reply header belongs inside the collapsible quote,
    // not above it as a stray paragraph.
    const segs = parseBody(
      "d\n\nOn 8月29日 17:24, admin@example.com wrote:\n> c\n> more",
    );
    expect(segs).toEqual([
      {type: "p", lines: ["d", ""]},
      {
        type: "quote",
        lines: [
          "On 8月29日 17:24, admin@example.com wrote:",
          "c",
          "more",
        ],
      },
    ]);
  });

  it("lets an interleaved reply close the quote region after an attribution", () => {
    const segs = parseBody("On x wrote:\n> old\nnew reply\n> more old");
    expect(segs).toEqual([
      {type: "quote", lines: ["On x wrote:", "old"]},
      {type: "p", lines: ["new reply"]},
      {type: "quote", lines: ["more old"]},
    ]);
  });
});

describe("getSnippet", () => {
  it("flattens paragraphs for the list preview", () => {
    expect(getSnippet("hello\n\nworld")).toBe("hello world");
  });

  it("excludes quoted history including its attribution header", () => {
    expect(getSnippet("d\n\nOn 8月29日 17:24, admin@example.com wrote:\n> c")).toBe("d");
  });
});
