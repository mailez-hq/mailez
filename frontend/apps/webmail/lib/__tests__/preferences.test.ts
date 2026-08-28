import { beforeEach, describe, expect, it } from "vitest";

import { DEFAULT_PREFS, PREF_KEY, readPreferences, writePreferences } from "@/lib/preferences";

describe("preferences", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("returns defaults when nothing is stored", () => {
    expect(readPreferences()).toEqual(DEFAULT_PREFS);
  });

  it("round-trips valid values", () => {
    writePreferences({ ...DEFAULT_PREFS, theme: "dark", density: "compact", undoSendSeconds: 20 });
    const p = readPreferences();
    expect(p.theme).toBe("dark");
    expect(p.density).toBe("compact");
    expect(p.undoSendSeconds).toBe(20);
  });

  it("round-trips the landing preference", () => {
    writePreferences({ ...DEFAULT_PREFS, landing: "inbox" });
    expect(readPreferences().landing).toBe("inbox");
    writePreferences({ ...DEFAULT_PREFS, landing: "home" });
    expect(readPreferences().landing).toBe("home");
  });

  it("falls back to defaults on invalid values", () => {
    window.localStorage.setItem(
      "mailez.prefs",
      JSON.stringify({ theme: "neon", density: "huge", undoSendSeconds: 99 }),
    );
    const p = readPreferences();
    expect(p.theme).toBe("system");
    expect(p.density).toBe("cozy");
    expect(p.undoSendSeconds).toBe(5);
  });

  it("defaults undoSendSeconds when the stored prefs lack the field", () => {
    // Prefs written before the undoSendSeconds option existed: the field is
    // absent, so it must fall back to 5 (not undefined, which used to crash
    // the send flow on `res.outbox_id`).
    window.localStorage.setItem(
      "mailez.prefs",
      JSON.stringify({ theme: "light", density: "cozy", accent: "blue" }),
    );
    const p = readPreferences();
    expect(p.undoSendSeconds).toBe(5);
  });
});

describe("collapseReplyQuote preference", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("defaults to Gmail-style collapsed quotes", () => {
    expect(DEFAULT_PREFS.collapseReplyQuote).toBe(true);
    expect(readPreferences().collapseReplyQuote).toBe(true);
  });

  it("reads an explicit false (Fastmail-style expanded quotes)", () => {
    window.localStorage.setItem(PREF_KEY, JSON.stringify({ collapseReplyQuote: false }));
    expect(readPreferences().collapseReplyQuote).toBe(false);
  });

  it("treats malformed stored values as collapsed (safe default)", () => {
    window.localStorage.setItem(PREF_KEY, JSON.stringify({ collapseReplyQuote: "yes" }));
    expect(readPreferences().collapseReplyQuote).toBe(true);
    window.localStorage.setItem(PREF_KEY, "not json");
    expect(readPreferences().collapseReplyQuote).toBe(true);
  });

  it("round-trips through writePreferences", () => {
    writePreferences({ ...DEFAULT_PREFS, collapseReplyQuote: false });
    expect(readPreferences().collapseReplyQuote).toBe(false);
  });
});
