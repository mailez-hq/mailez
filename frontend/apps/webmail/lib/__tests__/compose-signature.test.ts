import { describe, expect, it } from "vitest";

import { composeSignature } from "@/lib/compose-signature";

const BASE = {
  to: ["a@x.com"],
  cc: [] as string[],
  bcc: [] as string[],
  subject: "Hi",
  body: "<p>yo</p>",
  bodyText: "yo",
  attachments: [{ filename: "a.pdf", size: 10 }],
};

describe("composeSignature", () => {
  it("is stable for identical fields", () => {
    expect(composeSignature(BASE)).toBe(composeSignature({ ...BASE }));
  });

  it("changes when any field changes", () => {
    const baseline = composeSignature(BASE);
    expect(composeSignature({ ...BASE, to: [] })).not.toBe(baseline);
    expect(composeSignature({ ...BASE, cc: ["c@x.com"] })).not.toBe(baseline);
    expect(composeSignature({ ...BASE, bcc: ["b@x.com"] })).not.toBe(baseline);
    expect(composeSignature({ ...BASE, subject: "Hi!" })).not.toBe(baseline);
    expect(composeSignature({ ...BASE, body: "<p>yo!</p>" })).not.toBe(baseline);
    expect(composeSignature({ ...BASE, bodyText: "yo!" })).not.toBe(baseline);
    expect(composeSignature({ ...BASE, attachments: [] })).not.toBe(baseline);
  });

  it("distinguishes attachments by name and size", () => {
    const a = composeSignature({ ...BASE, attachments: [{ filename: "a.pdf", size: 10 }] });
    const b = composeSignature({ ...BASE, attachments: [{ filename: "b.pdf", size: 10 }] });
    const c = composeSignature({ ...BASE, attachments: [{ filename: "a.pdf", size: 11 }] });
    expect(a).not.toBe(b);
    expect(a).not.toBe(c);
  });

  it("is order-sensitive for recipients and attachments", () => {
    expect(composeSignature({ ...BASE, to: ["a@x.com", "b@x.com"] }))
      .not.toBe(composeSignature({ ...BASE, to: ["b@x.com", "a@x.com"] }));
  });
});
