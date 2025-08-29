import { describe, expect, test } from "vitest";

import {
  compileRules,
  escapeSieveString,
  parseRules,
  RULES_SCRIPT_NAME,
  type SieveRule,
} from "@/lib/sieve-rules";

function sampleRules(): SieveRule[] {
  return [
    {
      id: "r1",
      name: "Newsletters to Archive",
      enabled: true,
      matchAll: false,
      conditions: [
        { field: "from", op: "contains", value: "newsletter" },
        { field: "subject", op: "starts", value: "Weekly" },
      ],
      actions: [{ type: "moveTo", folder: "Archive/News" }],
    },
    {
      id: "r2",
      name: "Mark boss read+star",
      enabled: true,
      matchAll: true,
      conditions: [
        { field: "from", op: "is", value: "boss@corp.example" },
        { field: "to", op: "contains", value: "admin" },
      ],
      actions: [{ type: "markRead" }, { type: "star" }],
    },
    {
      id: "r3",
      name: "Disabled draft",
      enabled: false,
      matchAll: true,
      conditions: [{ field: "subject", op: "contains", value: "x" }],
      actions: [{ type: "discard" }],
    },
  ];
}

describe("sieve-rules", () => {
  test("round-trips the rule set through the header comment", () => {
    const rules = sampleRules();
    const script = compileRules(rules);
    expect(script).toContain(`# mailez-rules `);
    expect(parseRules(script)).toEqual(rules);
  });

  test("compiles conditions with the right Sieve tests", () => {
    const script = compileRules(sampleRules());
    expect(script).toContain('require ["fileinto", "imap4flags"];');
    expect(script).toContain('address :contains "from" "newsletter"');
    expect(script).toContain('header :matches "subject" "Weekly*"');
    expect(script).toContain('anyof (address :contains "from" "newsletter", header :matches "subject" "Weekly*")');
    expect(script).toContain('allof (address :is "from" "boss@corp.example", address :contains ["to", "cc"] "admin")');
    expect(script).toContain('fileinto "Archive/News";');
    expect(script).toContain('addflag "\\\\Seen";');
  });

  test("moveTo ends the rule with stop-less keep semantics and markRead keeps a copy", () => {
    const script = compileRules(sampleRules());
    // moveTo/discard rules must not append keep (fileinto already replaces
    // the implicit INBOX copy); flag-only rules must keep.
    const moveToBlock = script.split("# Mark boss")[0];
    expect(moveToBlock).not.toContain("keep;");
    const flagsBlock = script.slice(script.indexOf("# Mark boss"));
    expect(flagsBlock).toContain("keep;");
  });

  test("disabled rules are preserved but never compiled", () => {
    const script = compileRules(sampleRules());
    expect(script).toContain("# (disabled)");
    expect(script).not.toContain("discard;");
  });

  test("escapes quotes and backslashes in values", () => {
    expect(escapeSieveString('a"b\\c')).toBe('"a\\"b\\\\c"');
    const script = compileRules([
      {
        id: "x",
        name: "q",
        enabled: true,
        matchAll: true,
        conditions: [{ field: "subject", op: "contains", value: 'say "hi"' }],
        actions: [{ type: "moveTo", folder: "Weird\\Name" }],
      },
    ]);
    expect(script).toContain('header :contains "subject" "say \\"hi\\""');
    expect(script).toContain('fileinto "Weird\\\\Name";');
  });

  test("unicode folders survive the round trip", () => {
    const rules: SieveRule[] = [
      {
        id: "u1",
        name: "中文规则",
        enabled: true,
        matchAll: true,
        conditions: [{ field: "subject", op: "contains", value: "发票" }],
        actions: [{ type: "moveTo", folder: "归档/票据" }],
      },
    ];
    const script = compileRules(rules);
    expect(script).toContain('header :contains "subject" "发票"');
    expect(script).toContain('fileinto "归档/票据";');
    expect(parseRules(script)).toEqual(rules);
  });

  test("custom scripts parse to null", () => {
    expect(parseRules('require ["fileinto"];\nif header :contains "subject" "x" {\n  fileinto "J";\n}\n')).toBeNull();
    expect(parseRules("# mailez-rules !!!not-base64!!!\nkeep;\n")).toBeNull();
    expect(parseRules("")).toBeNull();
  });

  test("empty rule set compiles to a minimal script", () => {
    const script = compileRules([]);
    expect(script).toContain("# mailez-rules ");
    expect(script).toContain('require ["fileinto"];');
    expect(parseRules(script)).toEqual([]);
  });

  test("RULES_SCRIPT_NAME is the canonical script name", () => {
    expect(RULES_SCRIPT_NAME).toBe("mailez-rules");
  });
});
