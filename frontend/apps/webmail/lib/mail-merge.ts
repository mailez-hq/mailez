import type { MailMergeRecipient } from "@/lib/api";

// parseMergeRecipients converts pasted "email, 姓名[, 变量=值...]" lines into
// merge recipients. The first column is the address, the second the display
// name, and any further columns become {{var}} placeholders.
export function parseMergeRecipients(text: string): MailMergeRecipient[] {
  const out: MailMergeRecipient[] = [];
  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line) continue;
    const cols = line.split(/[,，\t]/).map((c) => c.trim()).filter(Boolean);
    const email = cols.shift() || "";
    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email)) continue;
    const name = cols.shift() || "";
    const vars: Record<string, string> = {};
    for (const col of cols) {
      const eq = col.indexOf("=");
      if (eq > 0) vars[col.slice(0, eq).trim()] = col.slice(eq + 1).trim();
    }
    out.push({ email, name, vars });
  }
  return out;
}
