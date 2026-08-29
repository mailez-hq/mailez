import { describe, expect, it } from "vitest";

import type { MailSearchSpec } from "@mailez/types";
import { applySearchSyntax, buildSearchSpec } from "@/lib/mail-search";

describe("applySearchSyntax", () => {
  it("collects bare words into text", () => {
    const spec: MailSearchSpec = {};
    expect(applySearchSyntax("hello   world", spec)).toBe(true);
    expect(spec.text).toEqual(["hello", "world"]);
  });

  it("parses quoted and unquoted from:/to:/subject:", () => {
    const spec: MailSearchSpec = {};
    applySearchSyntax('from:"Alice A" to:bob@x.com subject:invoice', spec);
    expect(spec.from).toEqual(["Alice A"]);
    expect(spec.to).toEqual(["bob@x.com"]);
    expect(spec.subject).toEqual(["invoice"]);
  });

  it("is case-insensitive on keys", () => {
    const spec: MailSearchSpec = {};
    applySearchSyntax("FROM:alice", spec);
    expect(spec.from).toEqual(["alice"]);
  });

  it("recognizes has:attachment", () => {
    const spec: MailSearchSpec = {};
    expect(applySearchSyntax("has:attachment", spec)).toBe(true);
    expect(spec.hasAttachment).toBe(true);
  });

  it("recognizes is:unread / is:flagged / is:starred", () => {
    const spec: MailSearchSpec = {};
    applySearchSyntax("is:unread is:flagged is:starred", spec);
    expect(spec.unseen).toBe(true);
    expect(spec.flagged).toBe(true);
  });

  it("ignores unknown is: values without activating", () => {
    const spec: MailSearchSpec = {};
    expect(applySearchSyntax("is:draft", spec)).toBe(false);
    expect(spec.unseen).toBeUndefined();
    expect(spec.flagged).toBeUndefined();
  });

  it("collects label: and filename: values", () => {
    const spec: MailSearchSpec = {};
    applySearchSyntax("label:work filename:report.pdf", spec);
    expect(spec.labels).toEqual(["work"]);
    expect(spec.filenames).toEqual(["report.pdf"]);
  });

  it("parses before:/after: into ISO timestamps and ignores bad dates", () => {
    const spec: MailSearchSpec = {};
    applySearchSyntax("before:2024-06-01 after:2024-01-15", spec);
    expect(spec.before).toBe("2024-06-01T00:00:00.000Z");
    expect(spec.after).toBe("2024-01-15T00:00:00.000Z");

    const bad: MailSearchSpec = {};
    expect(applySearchSyntax("before:notadate", bad)).toBe(false);
    expect(bad.before).toBeUndefined();
  });

  it("ignores unknown keys entirely", () => {
    const spec: MailSearchSpec = {};
    expect(applySearchSyntax("bogus:value", spec)).toBe(false);
    expect(Object.keys(spec)).toHaveLength(0);
  });
});

describe("buildSearchSpec", () => {
  it("returns null when neither keywords nor builder spec are active", () => {
    expect(buildSearchSpec("", null)).toBeNull();
  });

  it("passes through an active visual-builder spec even with empty keywords", () => {
    expect(buildSearchSpec("", { unseen: true })).toEqual({ unseen: true });
  });

  it("merges keywords into a copy of the visual spec without mutating it", () => {
    const visual = { from: ["carol@x.com"], unseen: true };
    const merged = buildSearchSpec("has:attachment holiday", visual);
    expect(merged).toEqual({
      from: ["carol@x.com"],
      unseen: true,
      hasAttachment: true,
      text: ["holiday"],
    });
    expect(visual).toEqual({ from: ["carol@x.com"], unseen: true });
  });

  it("returns null when only an empty builder spec exists", () => {
    expect(buildSearchSpec("", {})).toBeNull();
  });
});
