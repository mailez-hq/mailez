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

export type User = {
  email: string;
  displayed_name: string;
  quota_bytes: number;
  global_admin: boolean;
  enabled: boolean;
  created_at: string;
};

export type Alias = {
  email: string;
  destination: string;
  wildcard: boolean;
  disabled: boolean;
};
