// Auth, session, account, delegation and self-service API calls.
import type {
  LoginResult,
  MailAccount,
  MailDelegation,
  DelegationListing,
  Me,
  MeSettings,
  Signature,
  TotpStatus,
} from "@mailez/types";

import {
  api,
  apiPost,
  apiPut,
  HAS_OPTIONAL,
} from "./api-client";

export async function login(email: string, pw: string) {
  return api<LoginResult>("/sso/login", { method: "POST", body: JSON.stringify({ email, pw }) });
}

export async function loginTotp(pendingToken: string, code: string) {
  return api<{ email: string }>("/sso/login/totp", {
    method: "POST",
    body: JSON.stringify({ pending_token: pendingToken, code }),
  });
}

// ---- passkeys (WebAuthn): passwordless sign-in + credential management ----

export async function passkeyLoginBegin(email: string) {
  return api<{ options: unknown }>("/sso/passkey/login/begin", {
    method: "POST",
    body: JSON.stringify({ email }),
  });
}

export async function passkeyLoginFinish(email: string, assertion: unknown) {
  return api<{ email: string }>("/sso/passkey/login/finish", {
    method: "POST",
    body: JSON.stringify({ email, ...(assertion as Record<string, unknown>) }),
  });
}

export interface WebauthnCredentialInfo {
  id: number;
  name: string;
  credential_id: string;
  transports: string;
  last_used_at?: string;
  created_at: string;
}

export const webauthnList = () =>
  api<{ credentials: WebauthnCredentialInfo[] }>("/me/webauthn");

export const webauthnRegisterBegin = () =>
  apiPost<{ options: unknown }>("/me/webauthn/register/begin", {});

export const webauthnRegisterFinish = (name: string, credential: unknown) =>
  apiPost<{ id: number; name: string }>("/me/webauthn/register/finish", {
    name,
    ...(credential as Record<string, unknown>),
  });

export const webauthnDelete = (id: number) =>
  api<void>(`/me/webauthn/${id}`, { method: "DELETE" });

export async function logout() {
  return api("/sso/logout", { method: "POST" });
}

export async function me(): Promise<Me> {
  return api<Me>("/sso/me");
}

// Aggregated external accounts (full aggregation client).
export const accounts = () => api<MailAccount[]>("/accounts");

export const accountCreate = (input: {
  name: string;
  email: string;
  imap_host: string;
  imap_port: number;
  imap_security: string;
  smtp_host?: string;
  smtp_port?: number;
  smtp_security?: string;
  username: string;
  password: string;
}) => apiPost<MailAccount>("/accounts", input);

export const accountUpdate = (
  id: number,
  input: Partial<{
    name: string;
    email: string;
    imap_host: string;
    imap_port: number;
    imap_security: string;
    smtp_host: string;
    smtp_port: number;
    smtp_security: string;
    username: string;
    password: string;
    enabled: boolean;
  }>,
) => apiPut<MailAccount>(`/accounts/${id}`, input);

export const accountDelete = (id: number) =>
  api<void>(`/accounts/${id}`, { method: "DELETE" });

export const accountTest = (id: number) =>
  apiPost<{ ok: boolean }>(`/accounts/${id}/test`, {});

// Mailbox delegation / shared mailboxes. Optional module: without the
// extended module set the listing resolves locally instead of issuing a
// request that can only 404.
export const delegations = (): Promise<DelegationListing> =>
  HAS_OPTIONAL
    ? api<DelegationListing>("/delegations")
    : Promise.resolve({ granted: [], received: [] });

export const delegationCreate = (input: {
  delegate_email: string;
  can_send: boolean;
  full_access: boolean;
}) => apiPost<MailDelegation>("/delegations", input);

export const delegationUpdate = (
  id: number,
  input: { can_send: boolean; full_access: boolean },
) => apiPut<MailDelegation>(`/delegations/${id}`, input);

export const delegationDelete = (id: number) =>
  api<void>(`/delegations/${id}`, { method: "DELETE" });

// App passwords (used by IMAP/SMTP and the DAV servers).
export type AppToken = {
  id: number;
  user_email: string;
  ip: string;
  created_at?: string;
};

export type AppTokenResult = AppToken & { token?: string };

export const appTokens = () => api<{ data: AppToken[]; total: number }>("/tokens");

export const appTokenCreate = () =>
  apiPost<AppTokenResult>("/tokens", {});

export const appTokenDelete = (id: number) =>
  api<void>(`/tokens/${id}`, { method: "DELETE" });

// Two-factor authentication (TOTP)
export const totpStatus = () => api<TotpStatus>("/me/totp");

export const totpEnable = (code: string) =>
  api<void>("/me/totp/enable", { method: "POST", body: JSON.stringify({ code }) });

export const totpDisable = (code: string) =>
  api<void>("/me/totp", { method: "DELETE", body: JSON.stringify({ code }) });

// Self-service settings (GET /me / PUT /me/settings / PUT /me/password)
export const meProfile = () => api<MeSettings>("/me");

// Recent successful password sign-ins (latest first), from the login audit trail.
export type RecentLogin = { time: string; ip: string };
export const meLogins = () => api<RecentLogin[]>("/me/logins");

export const updateMeSettings = (body: Partial<MeSettings>) =>
  apiPut("/me/settings", body);

export const signatureList = () => api<Signature[]>("/me/signatures");

export type SignatureInput = {
  name: string;
  identity_email?: string;
  body_html: string;
  default_for_new?: boolean;
  default_for_reply?: boolean;
};

export const signatureCreate = (body: SignatureInput) =>
  apiPost<Signature>("/me/signatures", body);

export const signatureUpdate = (id: number, body: SignatureInput) =>
  apiPut<Signature>(`/me/signatures/${id}`, body);

export const signatureDelete = (id: number) =>
  api<void>(`/me/signatures/${id}`, { method: "DELETE" });

export const signatureSetDefault = (id: number, kind: "new" | "reply", enabled: boolean) =>
  apiPut<Signature>(`/me/signatures/${id}/default`, { kind, enabled });

export const changePassword = (oldPw: string, newPw: string) =>
  api("/me/password", { method: "PUT", body: JSON.stringify({ old_pw: oldPw, new_pw: newPw }) });
