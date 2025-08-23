import type { Me } from "./auth";

export type MailAttachment = {
  filename: string;
  content_type: string;
  size: number;
  data?: string;
};

// OutboundAttachment is a file being attached to a message being written:
// data is base64-encoded on the wire.
export type OutboundAttachment = {
  filename: string;
  content_type: string;
  size: number;
  data: string;
};

export type MailMessage = {
  uid: number;
  id: string;
  seq: number;
  subject: string;
  from: { name: string; email: string }[];
  to: { name: string; email: string }[];
  cc?: { name: string; email: string }[];
  date: string;
  flags: string[];
  has_attachment: boolean;
  thread_id?: string;
  thread_count?: number;
  thread_latest?: boolean;
  folder?: string;
  text_body?: string;
  html_body?: string;
  attachments?: MailAttachment[];
  // List-Unsubscribe (RFC 2369): an https URL (backend proxies the request)
  // or a mailto: (client opens a pre-filled compose). unsubscribe_post marks
  // an RFC 8058 one-click entry.
  unsubscribe_url?: string;
  unsubscribe_post?: boolean;
};

export type MailPage = { messages: MailMessage[]; total: number };

// MailLabel is a user-defined tag: messages carry the name as an IMAP
// keyword while the definition row only stores presentation data (color).
export type MailLabel = {
  id: number;
  name: string;
  color: string;
};

export type MailThread = {
  thread_id: string;
  subject: string;
  messages: MailMessage[];
};

export type MailIdentity = {
  email: string;
  name: string;
  dkim_enabled: boolean;
  signature?: string;
};

export type PushSubscriptionInput = {
  endpoint: string;
  keys: { p256dh: string; auth: string };
};

export type SieveScript = {
  name: string;
  active: boolean;
};

export type AIStatus = { enabled: boolean; provider: string };

export type DraftTone = "formal" | "concise" | "friendly";

export type PgpStatus = {
  has_key: boolean;
  public_key?: string;
  fingerprint?: string;
};

export type TotpStatus = {
  enabled: boolean;
  secret?: string;
  otpauth?: string;
};

export type MeSettings = Me & {
  forward_enabled: boolean;
  forward_destination: string;
  forward_keep: boolean;
  reply_enabled: boolean;
  reply_subject: string;
  reply_body: string;
  reply_startdate: string;
  reply_enddate: string;
  spam_enabled: boolean;
  spam_mark_as_read: boolean;
  spam_threshold: number;
  signature: string;
  whitelist: string;
  blacklist: string;
};

export type Contact = {
  id: number;
  user_email: string;
  name: string;
  email: string;
  comment: string;
};
