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
