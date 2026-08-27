import type {
  Alias, Announcement, ArchiveSettings, ArchivedMessage, AuditLog, DkimInfo, LoginResult, Me, Page, SignupDomain,
  DlpRule, PendingApproval,
} from "@mailez/types";

// Re-export the shared auth types for existing importers of @/lib/api.
export type { Me, LoginResult };

const API = "/api/v1";

// ApiError carries the HTTP status and the backend's machine-readable code.
export class ApiError extends Error {
  readonly status: number;
  readonly code?: string;

  constructor(message: string, status: number, code?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    const err = body as { error?: string; code?: string };
    throw new ApiError(err.error || res.statusText, res.status, err.code);
  }
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  if (!text) return undefined as T;
  try {
    return JSON.parse(text) as T;
  } catch {
    return undefined as T;
  }
}

export const apiPost = <T,>(path: string, body: unknown) =>
  api<T>(path, { method: "POST", body: JSON.stringify(body) });

export const apiPut = <T,>(path: string, body: unknown) =>
  api<T>(path, { method: "PUT", body: JSON.stringify(body) });

export const apiDelete = (path: string) => api(path, { method: "DELETE" });

export async function login(email: string, pw: string) {
  return api<LoginResult>("/sso/login", {
    method: "POST",
    body: JSON.stringify({ email, pw }),
  });
}

export async function loginTotp(pendingToken: string, code: string) {
  return api<{ email: string }>("/sso/login/totp", {
    method: "POST",
    body: JSON.stringify({ pending_token: pendingToken, code }),
  });
}

export async function logout() {
  return api("/sso/logout", { method: "POST" });
}

export async function me(): Promise<Me> {
  return api<Me>("/sso/me");
}

// self-registration (public)
export const signupDomains = () => api<SignupDomain[]>("/signup/domains");
export const signup = (email: string, pw: string) =>
  api("/signup", { method: "POST", body: JSON.stringify({ email, pw }) });

// anonymous aliases
export const anonAliases = () => api<Alias[]>("/anon-aliases");
export const anonmailDomains = () => api<string[]>("/anon-aliases/domains");
export const createAnonAlias = (domain: string, displayName: string) =>
  apiPost("/anon-aliases", { domain, display_name: displayName });

// global announcement banner
export const announcement = () => api<Announcement | undefined>("/announcement");
export const saveAnnouncement = (subject: string, body: string, enabled: boolean) =>
  apiPut<Announcement>("/announcement", { subject, body, enabled });
export const clearAnnouncement = () => apiDelete("/announcement");
export const deleteAnonAlias = (email: string) =>
  apiDelete(`/anon-aliases/${encodeURIComponent(email)}`);

// DKIM signing keys (global admin)
export const domainDkim = (name: string) =>
  api<DkimInfo>(`/domains/${encodeURIComponent(name)}/dkim`);
export const generateDomainDkim = (name: string) =>
  api<DkimInfo>(`/domains/${encodeURIComponent(name)}/dkim`, { method: "POST" });

// audit trail
export const auditLogs = (page = 1, limit = 50) =>
  api<Page<AuditLog>>(`/audit?page=${page}&limit=${limit}`);

// compliance email archive
export const archiveSettings = () =>
  api<{ global: ArchiveSettings; domains: ArchiveSettings[] }>("/archive/settings");
export const saveArchiveSettings = (body: Partial<ArchiveSettings>) =>
  apiPut<ArchiveSettings>("/archive/settings", body);
export const archiveMessages = (
  params: Record<string, string>,
  page = 1,
  limit = 50,
) => {
  const qs = new URLSearchParams({ page: String(page), limit: String(limit), ...params });
  return api<Page<ArchivedMessage>>(`/archive/messages?${qs}`);
};
export const archiveMessage = (id: number) =>
  api<{ message: ArchivedMessage; preview: string }>(`/archive/messages/${id}`);
export const reviewArchiveMessage = (id: number, reviewed: boolean, note: string) =>
  apiPost<ArchivedMessage>(`/archive/messages/${id}/review`, { reviewed, note });
export const deleteArchiveMessage = (id: number) =>
  apiDelete(`/archive/messages/${id}`);
export const archiveExportUrl = (params: Record<string, string>) => {
  const qs = new URLSearchParams(params).toString();
  return `/api/v1/archive/export${qs ? `?${qs}` : ""}`;
};

// DLP rules + approval workflow
export const dlpRules = () => api<{ data: DlpRule[] }>("/dlp/rules");
export const createDlpRule = (body: Partial<DlpRule>) =>
  apiPost<DlpRule>("/dlp/rules", body);
export const updateDlpRule = (id: number, body: Partial<DlpRule>) =>
  apiPut<DlpRule>(`/dlp/rules/${id}`, body);
export const deleteDlpRule = (id: number) =>
  apiDelete(`/dlp/rules/${id}`);
export const dlpApprovals = (status: string, page = 1, limit = 50) =>
  api<Page<PendingApproval>>(`/dlp/approvals?status=${status}&page=${page}&limit=${limit}`);
export const dlpApproval = (id: number) =>
  api<{ approval: PendingApproval; preview: string }>(`/dlp/approvals/${id}`);
export const decideApproval = (id: number, decision: "approve" | "reject", reason: string) =>
  apiPost<PendingApproval>(`/dlp/approvals/${id}/decision`, { decision, reason });

// config backup
export type ConfigBackup = {
  domains: unknown[];
  users: unknown[];
  aliases: unknown[];
  alternatives: unknown[];
  relays: unknown[];
  fetches: unknown[];
  tokens: unknown[];
};
export type ConfigStats = {
  domains: number;
  users: number;
  aliases: number;
  alternatives: number;
  relays: number;
  fetches: number;
  tokens: number;
};
export const exportConfig = () => api<ConfigBackup>("/config/export");
export const importConfig = (data: ConfigBackup) =>
  apiPost<ConfigStats>("/config/import", data);

// AI provider settings (admin console)
export interface AiConfigView {
  enabled: boolean;
  provider: string;
  base_url: string;
  model: string;
  has_api_key?: boolean;
}

export const getAIConfig = () => api<AiConfigView>("/config/ai");

export const putAIConfig = (input: {
  enabled?: boolean;
  provider?: string;
  base_url?: string;
  api_key?: string;
  model?: string;
}) => apiPut<AiConfigView>("/config/ai", input);

// AD/LDAP directory integration.
export type LdapConfigView = {
  enabled: boolean;
  host: string;
  port: number;
  security: string; // none | starttls | tls
  base_dn: string;
  bind_dn: string;
  has_bind_pw?: boolean;
  user_filter: string;
  mail_attr: string;
  uid_attr: string;
  upn_attr: string;
  email_domain: string;
  name_attr: string;
  dept_attr: string;
  title_attr: string;
  phone_attr: string;
  auto_create: boolean;
  sync_groups: boolean;
  group_filter: string;
  group_name_attr: string;
  group_mail_attr: string;
  group_member_attr: string;
  sync_minutes: number;
  updated_at?: string;
};

export const getLDAPConfig = () => api<LdapConfigView>("/ldap");

export const putLDAPConfig = (input: Partial<LdapConfigView> & { bind_password?: string }) =>
  apiPut<LdapConfigView>("/ldap", input);

export const testLDAP = (input: {
  host: string;
  port: number;
  security: string;
  base_dn: string;
  bind_dn: string;
  bind_password?: string;
  user_filter: string;
}) => apiPost<{ ok: boolean }>("/ldap/test", input);

export const syncLDAP = () =>
  apiPost<{ created: number; disabled: number; added: number; updated: number }>("/ldap/sync", {});
