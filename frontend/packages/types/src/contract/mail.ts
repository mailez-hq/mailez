import type { Me } from "./auth";

// Announcement banner shown in the webmail (admin-authored, global).
export type MailAnnouncement = {
  id: number;
  subject: string;
  body: string;
};

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
  // Deterministic auto-classification: work | social | newsletter | shopping |
  // finance | other. Derived server-side from sender domain and headers.
  category?: string;
  // Parsed iTIP meeting invitation (REQUEST/REPLY/CANCEL) when present.
  invitation?: MailInvitation;
  // Read-receipt request (RFC 3798 Disposition-Notification-To). The reader
  // offers a "send receipt" action unless the $MDNSent keyword is set.
  receipt_requested?: boolean;
  receipt_to?: string;
  // Recall notice (Outlook-style X-MS-Recall) targeting an original message.
  recall?: { message_id: string; subject: string };
};

export type MailInvitation = {
  method: string;
  uid: string;
  summary: string;
  location: string;
  description: string;
  start?: string;
  end?: string;
  all_day: boolean;
  organizer: string;
  attendees: string[];
  ics: string;
};

export type MailPage = { messages: MailMessage[]; total: number };

// MailSearchSpec is the structured form of an advanced search: the visual
// search builder produces this directly, and the client POSTs it to
// /mail/search without any syntax-string round-trip. Dates are RFC 3339.
export type MailSearchSpec = {
  text?: string[];
  from?: string[];
  to?: string[];
  subject?: string[];
  hasAttachment?: boolean;
  unseen?: boolean;
  flagged?: boolean;
  labels?: string[];
  filenames?: string[];
  before?: string | null;
  after?: string | null;
};

export type SnoozedMessage = MailMessage & { until: string };

// MailLabel is a user-defined tag: messages carry the ASCII wire keyword
// (possibly =XX-encoded for non-ASCII display names like Chinese) while the
// API exposes the human-readable name. The keyword field is the IMAP atom;
// clients that speak display names can ignore it.
export type MailLabel = {
  id: number;
  name: string;
  keyword?: string;
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
  delegated?: boolean;
};

// MailDelegation is a mailbox-sharing grant: the owner lets a delegate send
// as them (can_send) and optionally operate the full mailbox (full_access).
export type MailDelegation = {
  id: number;
  owner_email: string;
  delegate_email: string;
  can_send: boolean;
  full_access: boolean;
  delegate_name?: string;
  owner_name?: string;
  created_at: string;
  updated_at: string;
};

export type DelegationListing = {
  granted: MailDelegation[];
  received: MailDelegation[];
};

// CalendarEvent is one entry in the built-in calendar (shared with CalDAV).
export type CalendarEvent = {
  id: number;
  uid: string;
  summary: string;
  location: string;
  description: string;
  all_day: boolean;
  start: string; // RFC3339
  end?: string; // RFC3339
  rrule: string;
  // Reminder delivered this many minutes before start (0 = none).
  reminder_minutes?: number;
  // Calendar sharing context: the owning account and whether the viewer may
  // only read (shared calendars are read-only unless granted read-write).
  owner_email?: string;
  read_only?: boolean;
  updated_at: string;
};

export type CalendarEventInput = {
  summary: string;
  location?: string;
  description?: string;
  all_day?: boolean;
  start: string;
  end?: string;
  rrule?: string;
  reminder_minutes?: number;
};

// CalendarShare is a calendar grant between two accounts.
export type CalendarShare = {
  id: number;
  owner_email: string;
  sharee_email: string;
  read_only: boolean;
};

export type CalendarShareListing = {
  owned: CalendarShare[];
  granted: CalendarShare[];
};

// OrgContact is a read-only organization directory entry synced from AD/LDAP.
export type OrgContact = {
  id: number;
  email: string;
  name: string;
  department: string;
  title: string;
  phone: string;
  updated_at: string;
};

// MailAccount is an external IMAP/SMTP mailbox aggregated into the inbox
// (full aggregation client). The password never leaves the backend; these
// rows only expose configuration and health.
export type MailAccount = {
  id: number;
  user_email: string;
  name: string;
  email: string;
  imap_host: string;
  imap_port: number;
  imap_security: string; // none | starttls | tls
  smtp_host: string;
  smtp_port: number;
  smtp_security: string;
  username: string;
  enabled: boolean;
  last_error: string;
  created_at: string;
  updated_at: string;
};

export type PushSubscriptionInput = {
  endpoint: string;
  keys: { p256dh: string; auth: string };
};

export type SieveScript = {
  name: string;
  active: boolean;
};

// A user-configured HTTP callback that receives signed event deliveries.
export type Webhook = {
  id: number;
  user_email: string;
  url: string;
  secret: string;
  events: string; // comma-separated, e.g. "mail.received"
  enabled: boolean;
  last_status: number;
  last_error: string;
  last_sent_at: string | null;
  created_at: string;
};

export type AIStatus = { enabled: boolean; provider: string };

export type DraftTone = "formal" | "concise" | "friendly";

export type PgpStatus = {
  has_key: boolean;
  public_key?: string;
  fingerprint?: string;
};

// The user's installed S/MIME identity (own certificate + private key).
export type SmimeStatus = {
  has_cert: boolean;
  email?: string;
  fingerprint?: string;
  subject?: string;
  issuer?: string;
  not_after?: string;
};

// An imported S/MIME certificate used to encrypt to an external recipient.
export type SmimeCert = {
  id: number;
  user_email: string;
  email: string;
  cert_pem: string;
  fingerprint: string;
  subject: string;
  issuer: string;
  not_after: string;
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
  groups: string;
  avatar: string;
};
