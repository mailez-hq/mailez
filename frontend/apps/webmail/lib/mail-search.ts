import type { MailSearchSpec } from "@mailez/types";

// applySearchSyntax parses the documented keyword syntax (from:, to:,
// subject:, is:unread, is:flagged, has:attachment, label:, filename:,
// before:, after:) into structured fields on `spec`, mirroring the backend's
// parseSearchQuery; bare words become text. Returns whether any condition
// was recognized, so callers can merge visual-builder state accordingly.
export function applySearchSyntax(keywords: string, spec: MailSearchSpec): boolean {
  let active = false;
  const re = /([a-z]+):"([^"]*)"|([a-z]+):(\S+)|(\S+)/gi;
  let m: RegExpExecArray | null;
  while ((m = re.exec(keywords)) !== null) {
    let key = "";
    let val = "";
    if (m[1]) {
      key = m[1];
      val = m[2];
    } else if (m[3]) {
      key = m[3];
      val = m[4];
    } else {
      spec.text = [...(spec.text ?? []), m[5]];
      active = true;
      continue;
    }
    switch (key.toLowerCase()) {
      case "from":
        spec.from = [...(spec.from ?? []), val];
        active = true;
        break;
      case "to":
        spec.to = [...(spec.to ?? []), val];
        active = true;
        break;
      case "subject":
        spec.subject = [...(spec.subject ?? []), val];
        active = true;
        break;
      case "has":
        if (val.toLowerCase() === "attachment") {
          spec.hasAttachment = true;
          active = true;
        }
        break;
      case "is":
        if (val.toLowerCase() === "unread") {
          spec.unseen = true;
          active = true;
        }
        if (val.toLowerCase() === "flagged" || val.toLowerCase() === "starred") {
          spec.flagged = true;
          active = true;
        }
        break;
      case "label":
        if (val) {
          spec.labels = [...(spec.labels ?? []), val];
          active = true;
        }
        break;
      case "filename":
        if (val) {
          spec.filenames = [...(spec.filenames ?? []), val];
          active = true;
        }
        break;
      case "before": {
        const t = new Date(`${val}T00:00:00Z`);
        if (!Number.isNaN(t.getTime())) {
          spec.before = t.toISOString();
          active = true;
        }
        break;
      }
      case "after": {
        const t = new Date(`${val}T00:00:00Z`);
        if (!Number.isNaN(t.getTime())) {
          spec.after = t.toISOString();
          active = true;
        }
        break;
      }
    }
  }
  return active;
}

// buildSearchSpec merges the free-text keywords with the visual builder
// conditions into a structured spec; null when nothing is active.
export function buildSearchSpec(keywords: string, spec: MailSearchSpec | null): MailSearchSpec | null {
  const merged: MailSearchSpec = spec ? { ...spec } : {};
  const syntaxActive = applySearchSyntax(keywords, merged);
  const active = syntaxActive || Object.entries(merged).some(([, v]) =>
    Array.isArray(v) ? v.length > 0 : Boolean(v),
  );
  return active ? merged : null;
}
