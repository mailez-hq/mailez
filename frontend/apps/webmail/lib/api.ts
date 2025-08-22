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
  thread_id?: string;
  thread_count?: number;
  thread_latest?: boolean;
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
  // Operation endpoints may answer 200 with an empty/plain body (e.g. "OK");
  // tolerate that instead of failing JSON parsing.
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

export type MailThread = {
  thread_id: string;
  subject: string;
  messages: MailMessage[];
};

export const mailThread = (folder: string, threadId: string) =>
  api<MailThread>(
    `/mail/thread?folder=${encodeURIComponent(folder)}&thread_id=${encodeURIComponent(threadId)}`,
  );

export const mailSend = (to: string, subject: string, body: string, html?: string, from?: string) =>
  apiPost("/mail/send", { to, subject, body, html, from });

export const mailFlag = (folder: string, uid: number, flag: string, value: boolean) =>
  apiPost("/mail/flag", { folder, uid, flag, value });

export const mailDelete = (folder: string, uid: number) =>
  apiPost("/mail/delete", { folder, uid });

export const mailMove = (folder: string, uids: number[], destination: string) =>
  apiPost("/mail/move", { folder, uids, destination });

export type MailIdentity = {
  email: string;
  name: string;
  dkim_enabled: boolean;
};

export const mailIdentities = () => api<MailIdentity[]>("/mail/identities");

// Sieve filter rules (ManageSieve)
export type SieveScript = {
  name: string;
  active: boolean;
};

export const sieveList = () => api<SieveScript[]>("/sieve");

export const sieveGet = (name: string) =>
  api<{ name: string; content: string }>(`/sieve/${encodeURIComponent(name)}`);

export const sievePut = (name: string, content: string, activate: boolean) =>
  api(`/sieve/${encodeURIComponent(name)}`, {
    method: "PUT",
    body: JSON.stringify({ content, activate }),
  });

export const sieveDelete = (name: string) =>
  api(`/sieve/${encodeURIComponent(name)}`, { method: "DELETE" });

export const sieveActivate = (name: string) =>
  apiPost(`/sieve/${encodeURIComponent(name)}/activate`, {});

export type AIStatus = { enabled: boolean; provider: string };

export const aiStatus = () => api<AIStatus>("/ai/status");

export const aiSummarize = (text: string) =>
  api<{ summary: string }>("/ai/summarize", { method: "POST", body: JSON.stringify({ text }) });

export const aiDraft = (context: string) =>
  api<{ draft: string }>("/ai/draft", { method: "POST", body: JSON.stringify({ context }) });

export const aiPrioritize = (messages: { uid: number; subject: string; from: string }[]) =>
  api<{ scores: Record<string, number> }>("/ai/prioritize", {
    method: "POST",
    body: JSON.stringify({ messages }),
  });

export const aiSearch = (query: string) =>
  api<{ query: string; messages: MailMessage[] }>("/ai/search", {
    method: "POST",
    body: JSON.stringify({ query }),
  });

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
