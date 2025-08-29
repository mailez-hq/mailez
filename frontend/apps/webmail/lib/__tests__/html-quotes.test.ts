import {describe, expect, it} from "vitest";
import {foldHtmlQuotes} from "../../components/mailbox/reader/html-quotes";

// The engine auto-generates html_body from plain text by turning "> " lines
// into <blockquote>s. foldHtmlQuotes re-collapses that history into native
// <details> disclosures so a thread member doesn't re-render the whole
// conversation that the members above it already show.
describe("foldHtmlQuotes", () => {
  it("wraps a top-level blockquote in a closed details with the summary label", () => {
    const out = foldHtmlQuotes(
      `<p>new reply</p><blockquote><p>older message</p></blockquote>`,
      "Show quoted text",
    );
    const el = document.createElement("div");
    el.innerHTML = out;
    const details = el.querySelector("details.mail-quote-fold");
    expect(details).not.toBeNull();
    expect(details!.hasAttribute("open")).toBe(false);
    expect(details!.querySelector("summary")!.textContent).toBe("Show quoted text");
    expect(details!.querySelector("blockquote p")!.textContent).toBe("older message");
  });

  it("pulls the 'On … wrote:' attribution div inside the fold", () => {
    const out = foldHtmlQuotes(
      `<p>e</p><div>On Aug 29, 2026 17:27, user1@example.com wrote:</div><blockquote><p>d</p></blockquote>`,
      "Show quoted text",
    );
    const el = document.createElement("div");
    el.innerHTML = out;
    const details = el.querySelector("details.mail-quote-fold")!;
    // Attribution moved inside, before the quote; nothing stray left outside.
    expect(details.firstElementChild!.tagName).toBe("SUMMARY");
    expect(details.children[1].textContent).toContain("user1@example.com wrote:");
    expect(details.querySelector("blockquote")).not.toBeNull();
    expect(el.querySelector("details > div")!.textContent).toContain("wrote:");
    expect(el.textContent).not.toMatch(/^On Aug 29/);
  });

  it("leaves blockquotes nested inside another blockquote untouched", () => {
    const html = `<blockquote><p>level1<blockquote><p>level2</p></blockquote></p></blockquote>`;
    const out = foldHtmlQuotes(html, "Show quoted text");
    const el = document.createElement("div");
    el.innerHTML = out;
    const details = el.querySelector("details.mail-quote-fold")!;
    // Exactly one fold: the outer quote; the inner quote stays verbatim.
    expect(el.querySelectorAll("details.mail-quote-fold").length).toBe(1);
    expect(details.querySelector("blockquote blockquote p")!.textContent).toBe("level2");
  });

  it("returns html without blockquotes unchanged", () => {
    const html = `<p>plain</p><p>body</p>`;
    expect(foldHtmlQuotes(html, "Show quoted text")).toBe(html);
  });
});
