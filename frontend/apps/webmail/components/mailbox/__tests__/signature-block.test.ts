import { describe, expect, it } from "vitest";

import {
  appendSignatureHTML,
  appendSignatureText,
  buildSignatureHTML,
  currentSignatureID,
  spliceSignatureText,
  stripSignatureHTML,
  stripSignatureText,
  swapSignatureHTML,
} from "@/components/mailbox/mail-utils";

describe("compose signature block", () => {
  it("appends a signature the composer can find again", () => {
    const html = appendSignatureHTML("<p>Hi</p>", 7, "<p>Ada</p>");
    expect(html).toContain('data-mailez-signature="7"');
    expect(html).toContain("<p>--</p>");
    expect(currentSignatureID(html)).toBe(7);
  });

  it("replaces the existing signature instead of stacking another one", () => {
    const first = appendSignatureHTML("<p>Hi</p>", 1, "<p>Ada</p>");
    const second = appendSignatureHTML(stripSignatureHTML(first), 2, "<p>Grace</p>");
    expect(second.match(/data-mailez-signature/g)).toHaveLength(1);
    expect(currentSignatureID(second)).toBe(2);
    expect(second).not.toContain("Ada");
  });

  it("strips back to the user's own content", () => {
    const html = appendSignatureHTML("<p>Hi</p>", 3, "<p>Ada</p>");
    expect(stripSignatureHTML(html)).toBe("<p>Hi</p>");
  });

  it("leaves a body without a signature untouched", () => {
    expect(stripSignatureHTML("<p>Hi</p>")).toBe("<p>Hi</p>");
    expect(currentSignatureID("<p>Hi</p>")).toBeNull();
    expect(currentSignatureID(buildSignatureHTML(null, "<p>x</p>"))).toBeNull();
  });

  it("keeps the plain-text twin in step", () => {
    expect(appendSignatureText("Hi", "Ada")).toBe("Hi\n\n-- \nAda");
    expect(appendSignatureText("", "Ada")).toBe("-- \nAda");
    expect(stripSignatureText(appendSignatureText("Hi", "Ada"))).toBe("Hi");
    expect(stripSignatureText("Hi")).toBe("Hi");
  });

  it("swaps the signature where it already sits", () => {
    const reply = swapSignatureHTML("<p>Note</p><p><br></p><blockquote>old</blockquote>", 4, "<p>SIG-ONE</p>", true);
    const swapped = swapSignatureHTML(reply, 5, "<p>SIG-TWO</p>");
    expect(currentSignatureID(swapped)).toBe(5);
    expect(swapped).toContain("SIG-TWO");
    expect(swapped).not.toContain("SIG-ONE");
    expect(swapped).toContain("<p>Note</p>");
    expect(swapped.indexOf("data-mailez-signature")).toBeLessThan(swapped.indexOf("blockquote"));
  });

  it("inserts a signature above a reply and at the end of a new message", () => {
    expect(
      swapSignatureHTML("<blockquote>old</blockquote>", 1, "<p>Ada</p>", true),
    ).toMatch(/^<div data-mailez-signature="1">/);
    expect(swapSignatureHTML("<p>Hi</p>", 1, "<p>Ada</p>")).toMatch(/<\/div>$/);
  });

  it("removes a signature on request", () => {
    const html = swapSignatureHTML(appendSignatureHTML("<p>Hi</p>", 2, "<p>Ada</p>"), 2, "<p>Ada</p>");
    expect(stripSignatureHTML(html)).toBe("<p>Hi</p>");
    expect(swapSignatureHTML(html, null, "")).toBe("<p>Hi</p>");
  });

  it("splices the plain-text signature above or below the message", () => {
    const block = "-- \nAda";
    const above = spliceSignatureText("> quoted", null, block, true);
    expect(above).toBe("-- \nAda\n\n> quoted");
    const multi = "-- \nAda\n\nCTO";
    const swapped = spliceSignatureText(above, block, multi, true);
    expect(swapped).toBe("-- \nAda\n\nCTO\n\n> quoted");
    expect(spliceSignatureText(swapped, multi, null, true)).toBe("> quoted");
    expect(spliceSignatureText("Hi", null, block, false)).toBe("Hi\n\n-- \nAda");
  });
});
