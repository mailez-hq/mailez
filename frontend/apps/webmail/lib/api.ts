export type Me = {
  email: string;
  displayed_name: string;
  global_admin: boolean;
  enabled: boolean;
};

export type MailMessage = {
  uid: number;
  seq: number;
  subject: string;
  from: { name: string; email: string }[];
  to: { name: string; email: string }[];
  date: string;
  flags: string[];
  has_attachment: boolean;
  text_body?: string;
  html_body?: string;
};

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

export const apiPost = (path: string, body: unknown) =>
  api(path, { method: "POST", body: JSON.stringify(body) });

export async function login(email: string, pw: string) {
  return api<Me>("/sso/login", { method: "POST", body: JSON.stringify({ email, pw }) });
}

export async function logout() {
  return api("/sso/logout", { method: "POST" });
}

export async function me(): Promise<Me> {
  return api<Me>("/sso/me");
}

export const mailFolders = () => api<string[]>("/mail/folders");

export const mailMessages = (folder: string) =>
  api<MailMessage[]>(`/mail/messages?folder=${encodeURIComponent(folder)}`);

export const mailMessage = (folder: string, uid: number) =>
  api<MailMessage>(`/mail/message?folder=${encodeURIComponent(folder)}&uid=${uid}`);

export const mailSend = (to: string, subject: string, body: string) =>
  apiPost("/mail/send", { to, subject, body });
