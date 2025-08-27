export type Domain = {
  name: string;
  max_users: number;
  max_aliases: number;
  max_quota_bytes: number;
  signup_enabled: boolean;
  anonmail_enabled: boolean;
  comment: string;
  created_at: string;
};

export type DkimInfo = {
  domain: string;
  selector: string;
  enabled: boolean;
  record: string;
  public_key: string;
};

export type User = {
  email: string;
  displayed_name: string;
  quota_bytes: number;
  global_admin: boolean;
  enabled: boolean;
  enable_imap: boolean;
  enable_pop: boolean;
  allow_spoofing: boolean;
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
  created_at: string;
};

export type Alias = {
  email: string;
  destination: string;
  wildcard: boolean;
  disabled: boolean;
  hostname?: string;
  owner_email?: string;
};

export type AuditLog = {
  id: number;
  created_at: string;
  user: string;
  ip: string;
  method: string;
  path: string;
  status: number;
};

export type ArchiveDirection = "inbound" | "outbound";

export type ArchiveSettings = {
  id: number;
  domain: string;
  enabled: boolean;
  capture_inbound: boolean;
  capture_outbound: boolean;
  retention_days: number;
  updated_at: string;
};

export type ArchivedMessage = {
  id: number;
  direction: ArchiveDirection;
  domain: string;
  envelope_from: string;
  envelope_to: string;
  message_id: string;
  from: string;
  to: string;
  cc: string;
  subject: string;
  date: string;
  size: number;
  archived_at: string;
  expires_at: string | null;
  reviewed: boolean;
  reviewed_by: string;
  reviewed_at: string | null;
  review_note: string;
};

export type DlpRule = {
  id: number;
  name: string;
  enabled: boolean;
  pattern: string;
  is_regex: boolean;
  scope: string;
  action: "block" | "hold";
  severity: string;
  approvers: string;
  hold_hours: number;
  note: string;
  created_at: string;
  updated_at: string;
};

export type PendingApproval = {
  id: number;
  rule_name: string;
  severity: string;
  sender_email: string;
  from: string;
  recipients: string;
  subject: string;
  status: string;
  approver: string;
  decision_at: string | null;
  reason: string;
  created_at: string;
  expires_at: string | null;
};

export type SignupDomain = {
  name: string;
  max_users: number;
  user_count: number;
  signup_full: boolean;
};

export type Token = {
  id: number;
  user_email: string;
  ip: string;
};

export type Fetch = {
  id: number;
  user_email: string;
  protocol: string;
  host: string;
  port: number;
  tls: boolean;
  username: string;
  keep: boolean;
  scan: boolean;
  invisible: boolean;
  folders: string;
  last_check: string | null;
  error: string;
};

export type Relay = {
  name: string;
  smtp: string;
  comment: string;
};

export type Alternative = {
  name: string;
  domain_name: string;
};

// Uniform pagination envelope returned by list endpoints.
export type Page<T> = {
  data: T[];
  total: number;
  page: number;
  limit: number;
};

// Global announcement banner shown to every user (admin-authored).
export type Announcement = {
  id: number;
  subject: string;
  body: string;
  enabled: boolean;
};
