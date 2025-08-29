// composeSignature fingerprints the compose fields. It is compared against
// the baseline captured when compose opens, so a pristine reply/forward
// (only the auto-generated quote) is never mistaken for user content.
export function composeSignature(vals: {
  to: string[];
  cc: string[];
  bcc: string[];
  subject: string;
  body: string;
  bodyText: string;
  attachments: {filename: string; size: number}[];
}): string {
  return JSON.stringify([
    vals.to,
    vals.cc,
    vals.bcc,
    vals.subject,
    vals.body,
    vals.bodyText,
    vals.attachments.map((a) => `${a.filename}:${a.size}`),
  ]);
}
