import { describe, expect, it } from "vitest";
import { parseMergeRecipients } from "@/lib/mail-merge";

describe("parseMergeRecipients", () => {
  it("parses email, name and custom vars", () => {
    const out = parseMergeRecipients(
      "bob@example.com, Bob, 工号=1001\ncarol@example.com, Carol, dept=HR\n\ninvalid-line\nalice@example.com",
    );
    expect(out).toHaveLength(3);
    expect(out[0]).toEqual({ email: "bob@example.com", name: "Bob", vars: { 工号: "1001" } });
    expect(out[1]).toEqual({ email: "carol@example.com", name: "Carol", vars: { dept: "HR" } });
    expect(out[2]).toEqual({ email: "alice@example.com", name: "", vars: {} });
  });

  it("handles Chinese commas and tabs", () => {
    const out = parseMergeRecipients("x@example.com，张三\t岗位=经理");
    expect(out[0].name).toBe("张三");
    expect(out[0].vars).toEqual({ 岗位: "经理" });
  });

  it("drops malformed rows", () => {
    const out = parseMergeRecipients("not-an-email\nbob@example.com");
    expect(out).toHaveLength(1);
    expect(out[0].email).toBe("bob@example.com");
  });
});
