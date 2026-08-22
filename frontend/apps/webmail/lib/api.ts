export type Me = {
  email: string;
  displayed_name: string;
  global_admin: boolean;
  enabled: boolean;
};

export type MailAttachment = {
  filename: string;
  content_type: string;
  size: number;
  data?: string;
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
  attachments?: MailAttachment[];
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

export const apiPost = <T,>(path: string, body: unknown) =>
  api<T>(path, { method: "POST", body: JSON.stringify(body) });

export const apiPut = <T,>(path: string, body: unknown) =>
  api<T>(path, { method: "PUT", body: JSON.stringify(body) });

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

export type MailPage = { messages: MailMessage[]; total: number };

export async function mailMessages(folder: string, page = 0): Promise<MailPage> {
  const res = await fetch(`${API}/mail/messages?folder=${encodeURIComponent(folder)}&page=${page}`, {
    headers: { "Content-Type": "application/json" },
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error((body as { error?: string }).error || res.statusText);
  }
  const messages = await res.json();
  const total = Number(res.headers.get("X-Total-Messages") || 0);
  return { messages, total };
}

export const mailSearch = (folder: string, q: string) =>
  api<MailMessage[]>(`/mail/search?folder=${encodeURIComponent(folder)}&q=${encodeURIComponent(q)}`);

export const mailMessage = (folder: string, uid: number) =>
  api<MailMessage>(`/mail/message?folder=${encodeURIComponent(folder)}&uid=${uid}`);

export const mailSend = (to: string, subject: string, body: string) =>
  apiPost("/mail/send", { to, subject, body });

export const mailFlag = (folder: string, uid: number, flag: string, value: boolean) =>
  apiPost("/mail/flag", { folder, uid, flag, value });

export const mailDelete = (folder: string, uid: number) =>
  apiPost("/mail/delete", { folder, uid });

export type AIStatus = { enabled: boolean; provider: string };

export const aiStatus = () => api<AIStatus>("/ai/status");

export const aiSummarize = (text: string) =>
  api<{ summary: string }>("/ai/summarize", { method: "POST", body: JSON.stringify({ text }) });

export const aiDraft = (context: string) =>
  api<{ draft: string }>("/ai/draft", { method: "POST", body: JSON.stringify({ context }) });

// Self-service settings (GET /me / PUT /me/settings / PUT /me/password)
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
};

export const meProfile = () => api<MeSettings>("/me");

export const updateMeSettings = (body: Partial<MeSettings>) =>
  apiPut("/me/settings", body);

export const changePassword = (oldPw: string, newPw: string) =>
  api("/me/password", { method: "PUT", body: JSON.stringify({ old_pw: oldPw, new_pw: newPw }) });

// address book
export type Contact = {
  id: number;
  user_email: string;
  name: string;
  email: string;
  comment: string;
};

export const contacts = () => api<Contact[]>("/contacts");

export const createContact = (name: string, email: string, comment = "") =>
  apiPost<Contact>("/contacts", { name, email, comment });

export const deleteContact = (id: number) =>
  api(`/contacts/${id}`, { method: "DELETE" });
