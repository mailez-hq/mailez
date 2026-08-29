// Sieve rules model: the visual rule builder compiles into one ManageSieve
// script ("mailez-rules") and parses it back. Round-trip fidelity lives in a
// single header comment (# mailez-rules <base64 JSON>), so hand-written
// scripts stay first-class citizens — when the header is absent the script
// is treated as custom code the builder will not touch.

export type SieveConditionField = "from" | "to" | "subject";
export type SieveConditionOp = "contains" | "is" | "starts";

export interface SieveCondition {
  field: SieveConditionField;
  op: SieveConditionOp;
  value: string;
}

export type SieveAction =
  | { type: "moveTo"; folder: string }
  | { type: "markRead" }
  | { type: "star" }
  | { type: "forward"; address: string }
  | { type: "discard" };

export interface SieveRule {
  id: string;
  name: string;
  enabled: boolean;
  /** true: every condition must match (allof); false: any (anyof). */
  matchAll: boolean;
  conditions: SieveCondition[];
  actions: SieveAction[];
}

/** The canonical script name the visual builder owns. */
export const RULES_SCRIPT_NAME = "mailez-rules";

const HEADER_PREFIX = "# mailez-rules ";

export function newRuleId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `r${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`;
}

// encodeRules/decodeRules are unicode-safe base64 of the JSON payload.
function encodeRules(rules: SieveRule[]): string {
  const json = JSON.stringify(rules);
  const bytes = new TextEncoder().encode(json);
  let bin = "";
  bytes.forEach((b) => {
    bin += String.fromCharCode(b);
  });
  return btoa(bin);
}

function decodeRules(b64: string): SieveRule[] | null {
  try {
    const bin = atob(b64.trim());
    const bytes = Uint8Array.from(bin, (ch) => ch.charCodeAt(0));
    const json = new TextDecoder().decode(bytes);
    const parsed = JSON.parse(json);
    if (!Array.isArray(parsed)) return null;
    return parsed as SieveRule[];
  } catch {
    return null;
  }
}

/** escapeSieveString quotes a value for a Sieve string literal. */
export function escapeSieveString(v: string): string {
  return `"${v.replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"`;
}

const opMap: Record<SieveConditionOp, string> = {
  contains: ":contains",
  is: ":is",
  // No :startswith in RFC 5228 — glob-match with a trailing *.
  starts: ":matches",
};

function conditionTest(c: SieveCondition): string {
  const op = opMap[c.op];
  const value = c.op === "starts" ? escapeSieveString(`${c.value}*`) : escapeSieveString(c.value);
  switch (c.field) {
    case "from":
      return `address ${op} "from" ${value}`;
    case "to":
      // arriving copies addressed via To or Cc both count as "to"
      return `address ${op} ["to", "cc"] ${value}`;
    case "subject":
      return `header ${op} "subject" ${value}`;
  }
}

function ruleBody(a: SieveAction): string | null {
  switch (a.type) {
    case "moveTo":
      return a.folder.trim() ? `fileinto ${escapeSieveString(a.folder.trim())};` : null;
    case "markRead":
      return 'addflag "\\\\Seen";';
    case "star":
      return 'addflag "\\\\Flagged";';
    case "forward":
      return a.address.trim() ? `redirect ${escapeSieveString(a.address.trim())};` : null;
    case "discard":
      return "discard;";
  }
}

/** compileRules renders the rule set as one Sieve script. */
export function compileRules(rules: SieveRule[]): string {
  const needs = new Set<string>(["fileinto"]);
  for (const r of rules) {
    for (const a of r.actions) {
      if (a.type === "markRead" || a.type === "star") needs.add("imap4flags");
    }
  }
  const out: string[] = [];
  out.push(HEADER_PREFIX + encodeRules(rules));
  out.push(`require [${[...needs].map((n) => escapeSieveString(n)).join(", ")}];`);
  for (const r of rules) {
    out.push("");
    out.push(`# ${r.name || "rule"}`);
    if (!r.enabled || r.conditions.length === 0 || !r.actions.length) {
      // Disabled or incomplete rules are kept as comments so the builder
      // round-trip preserves them without the engine evaluating anything.
      out.push(`# (disabled) ${JSON.stringify({ id: r.id })}`);
      continue;
    }
    const tests = r.conditions.filter((c) => c.value.trim()).map(conditionTest);
    if (tests.length === 0) {
      out.push("# (disabled)");
      continue;
    }
    const test =
      tests.length === 1 ? tests[0] : `${r.matchAll ? "allof" : "anyof"} (${tests.join(", ")})`;
    const bodies = r.actions.map(ruleBody).filter((b): b is string => b !== null);
    const keepsCopy = r.actions.some((a) => a.type === "moveTo" || a.type === "discard");
    if (!keepsCopy) bodies.push("keep;");
    out.push(`if ${test} {`);
    for (const b of bodies) out.push(`  ${b}`);
    out.push("}");
  }
  out.push("");
  return out.join("\n");
}

/** parseRules recovers the rule set from a script; null when the script is
 * not builder-owned (custom code) or the payload is unreadable. */
export function parseRules(content: string): SieveRule[] | null {
  for (const line of content.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (trimmed.startsWith(HEADER_PREFIX)) {
      return decodeRules(trimmed.slice(HEADER_PREFIX.length));
    }
    if (trimmed !== "" && !trimmed.startsWith("#")) break;
  }
  return null;
}

/** hasRulesScript reports whether the builder's script exists in the list. */
export function hasRulesScript(scripts: { name: string }[]): boolean {
  return scripts.some((s) => s.name === RULES_SCRIPT_NAME);
}
