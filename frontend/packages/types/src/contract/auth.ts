export type Me = {
  email: string;
  displayed_name: string;
  global_admin: boolean;
  manager: boolean;
  enabled: boolean;
  quota_bytes?: number;
  quota_bytes_used?: number;
  signature?: string;
};

export type LoginResult = {
  email: string;
  totp_required?: boolean;
  pending_token?: string;
};
