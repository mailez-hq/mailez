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
});

describe("getSnippet", () => {
  it("flattens paragraphs for the list preview", () => {
    expect(getSnippet("hello\n\nworld")).toBe("hello world");
  });
});
