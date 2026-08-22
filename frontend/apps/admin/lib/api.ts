export type Me = {
  email: string;
  displayed_name: string;
  global_admin: boolean;
  manager: boolean;
  enabled: boolean;
};

import type { Alias, AuditLog, DkimInfo, SignupDomain } from "@/lib/types";

const API = "/api/v1";

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error((body as { error?: string }).error || res.statusText);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export const apiPost = <T,>(path: string, body: unknown) =>
  api<T>(path, { method: "POST", body: JSON.stringify(body) });

export const apiPut = <T,>(path: string, body: unknown) =>
  api<T>(path, { method: "PUT", body: JSON.stringify(body) });

export const apiDelete = (path: string) => api(path, { method: "DELETE" });

export async function login(email: string, pw: string) {
  return api<Me>("/sso/login", {
    method: "POST",
    body: JSON.stringify({ email, pw }),
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
export const deleteAnonAlias = (email: string) =>
  apiDelete(`/anon-aliases/${encodeURIComponent(email)}`);

// DKIM signing keys (global admin)
export const domainDkim = (name: string) =>
  api<DkimInfo>(`/domains/${encodeURIComponent(name)}/dkim`);
export const generateDomainDkim = (name: string) =>
  api<DkimInfo>(`/domains/${encodeURIComponent(name)}/dkim`, { method: "POST" });

// audit trail
export const auditLogs = () => api<AuditLog[]>("/audit");

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
