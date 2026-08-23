import DOMPurify, { type Config } from "dompurify";

// Sanitizes untrusted email HTML before it is rendered in the webmail.
// Scripts, event handlers, forms and other executable content are stripped;
// links, cid:/data:image sources and relative URLs stay usable, and the
// reader's own remote-image policy keeps working via data-remote-src.
const SANITIZE_CONFIG: Config = {
  USE_PROFILES: { html: true },
  FORBID_TAGS: [
    "script",
    "style",
    "iframe",
    "object",
    "embed",
    "form",
    "input",
    "button",
    "select",
    "textarea",
    "meta",
    "link",
    "base",
  ],
  FORBID_ATTR: ["style", "formaction", "background", "xlink:href"],
  ALLOW_DATA_ATTR: false,
  ADD_ATTR: ["data-remote-src"],
  ALLOWED_URI_REGEXP:
    /^(?:(?:https?|mailto|tel|cid):|data:image\/(?:png|jpe?g|gif|webp|svg\+xml);|#|\/|about:blank)/i,
};

export function sanitizeMailHTML(html: string): string {
  // Never run DOMPurify (or emit raw HTML) during server rendering: the
  // message body is rendered client-side only after hydration.
  if (!html || typeof window === "undefined") return "";
  return DOMPurify.sanitize(html, SANITIZE_CONFIG);
}
