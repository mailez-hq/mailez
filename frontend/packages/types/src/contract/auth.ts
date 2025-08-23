export type Me = {
  email: string;
  displayed_name: string;
  global_admin: boolean;
  manager: boolean;
  enabled: boolean;
};

export type LoginResult = {
  email: string;
  totp_required?: boolean;
  pending_token?: string;
};
