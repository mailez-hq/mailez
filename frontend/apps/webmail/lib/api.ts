import type {
  AIStatus,
  Contact,
  DraftTone,
  LoginResult,
  MailAnnouncement,
  MailAccount,
  MailDelegation,
  DelegationListing,
  CalendarEvent,
  CalendarEventInput,
  CalendarShare,
  CalendarShareListing,
  OrgContact,
  MailAttachment,
  MailIdentity,
  MailLabel,
  MailMessage,
  MailInvitation,
  MailPage,
  MailSearchSpec,
  MailThread,
  Me,
  MeSettings,
  OutboundAttachment,
  PgpStatus,
  PushSubscriptionInput,
  SieveScript,
  SnoozedMessage,
  SmimeCert,
  SmimeStatus,
  TotpStatus,
  Webhook,
} from "@mailez/types";

// The wire types live in the shared @mailez/types package; re-export them so
// existing components keep importing from "@/lib/api".
export type {
  AIStatus,
  Contact,
  DraftTone,
  LoginResult,
  MailAnnouncement,
  MailAccount,
  MailDelegation,
  DelegationListing,
  CalendarEvent,
  CalendarEventInput,
  CalendarShare,
  CalendarShareListing,
  OrgContact,
  MailAttachment,
  MailIdentity,
  MailLabel,
  MailMessage,
  MailInvitation,
  MailPage,
  MailSearchSpec,
  MailThread,
  Me,
  MeSettings,
  OutboundAttachment,
  PgpStatus,
  PushSubscriptionInput,
  SieveScript,
  SnoozedMessage,
  SmimeCert,
  SmimeStatus,
  TotpStatus,
  Webhook,
};

const API = "/api/v1";

// Active aggregated account: when set, every /mail/* request is scoped to that
// external mailbox so the whole UI (folders, list, reading pane) follows the
// account switch. The internal gateway account is the null default.
let activeAccountId: number | null = null;
export const setActiveAccountId = (id: number | null) => {
  activeAccountId = id;
};
export const getActiveAccountId = () => activeAccountId;

// Active delegated mailbox: when set, every /mail/* request runs in that
// owner's mailbox context (X-Delegate-Email). Only full-access grants are
// listed by the backend, so the header is safe to send for the whole mail
// surface; the backend re-validates the grant on every request.
let activeDelegateEmail: string | null = null;
export const setActiveDelegateEmail = (email: string | null) => {
  activeDelegateEmail = email;
};
export const getActiveDelegateEmail = () => activeDelegateEmail;

// mailPath appends the active account selector to mailbox-scoped requests;
// mailHeaders adds the delegated-mailbox context header.
function mailPath(path: string): string {
  if (activeAccountId == null || !path.startsWith("/mail/")) return path;
  const sep = path.includes("?") ? "&" : "?";
  return `${path}${sep}account_id=${activeAccountId}`;
}

function mailHeaders(): Record<string, string> {
  if (activeDelegateEmail != null) return { "X-Delegate-Email": activeDelegateEmail };
  return {};
}

// ApiError carries the HTTP status and the backend's machine-readable code
// (e.g. "rate_limited") so the UI can react programmatically.
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
  const res = await fetch(`${API}${mailPath(path)}`, {
    headers: { "Content-Type": "application/json", ...mailHeaders() },
    ...init,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    const err = body as { error?: string; code?: string };
    throw new ApiError(err.error || res.statusText, res.status, err.code);
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
  return api<LoginResult>("/sso/login", { method: "POST", body: JSON.stringify({ email, pw }) });
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

export const mailFolders = () => api<string[]>("/mail/folders");

export const mailFolderCreate = (name: string) =>
  apiPost<void>("/mail/folders", { name });

export const mailFolderRename = (name: string, newName: string) =>
  apiPut<void>("/mail/folders", { name, new_name: newName });

export const mailFolderDelete = (name: string) =>
  api<void>(`/mail/folders?name=${encodeURIComponent(name)}`, { method: "DELETE" });

export const mailFolderClear = (name: string) =>
  apiPost<void>("/mail/folders/clear", { name });

export const mailUnseen = () => api<Record<string, number>>("/mail/unseen");

export async function mailMessages(folder: string, page = 0, sort = "date", dir = ""): Promise<MailPage> {
  const res = await fetch(
    `${API}${mailPath(`/mail/messages?folder=${encodeURIComponent(folder)}&page=${page}&sort=${encodeURIComponent(sort)}&dir=${encodeURIComponent(dir)}`)}`,
    { headers: { "Content-Type": "application/json", ...mailHeaders() } },
  );
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

// mailSearchSpec runs a structured search (built visually, no syntax parsing).
export const mailSearchSpec = (folder: string, spec: MailSearchSpec) =>
  api<MailMessage[]>("/mail/search", {
    method: "POST",
    body: JSON.stringify({ folder, query: spec }),
  });

// mailMessage fetches a single message. It is addressed by its stable routable
// id (preferred, survives mailbox moves) or, as a fallback, by its IMAP uid.
export const mailMessage = (folder: string, ref: { id: string } | { uid: number }) => {
  const q =
    "id" in ref
      ? `id=${encodeURIComponent(ref.id)}`
      : `uid=${ref.uid}`;
  return api<MailMessage>(`/mail/message?folder=${encodeURIComponent(folder)}&${q}`);
};

export const mailRaw = (folder: string, uid: number) =>
  api<{ raw: string }>(`/mail/raw?folder=${encodeURIComponent(folder)}&uid=${uid}`);

export const mailThread = (folder: string, threadId: string) =>
  api<MailThread>(
    `/mail/thread?folder=${encodeURIComponent(folder)}&thread_id=${encodeURIComponent(threadId)}`,
  );

// mailSend submits a message. With undoSeconds > 0 the backend parks it in the
// outbox for that window and returns its id, so the caller can offer undo. A
// sendAt (RFC3339) future value schedules the send instead.
export const mailSend = (
  to: string[],
  cc: string[],
  bcc: string[],
  subject: string,
  body: string,
  html?: string,
  from?: string,
  attachments?: OutboundAttachment[],
  undoSeconds = 0,
  sendAt?: string,
  receiptRequested = false,
  burnAfterMinutes = 0,
) =>
  api<{ queued?: boolean; outbox_id?: number; undo_seconds?: number; scheduled?: boolean }>(
    "/mail/send",
    {
      method: "POST",
      body: JSON.stringify({ to, cc, bcc, subject, body, html, from, attachments, undo_seconds: undoSeconds, send_at: sendAt, receipt_requested: receiptRequested, burn_after_minutes: burnAfterMinutes }),
    },
  );

// mailSendReply sends an inline quick reply with proper threading headers.
export const mailSendReply = (
  to: string[],
  cc: string[],
  subject: string,
  body: string,
  inReplyTo: string,
  references: string,
) =>
  api("/mail/send", {
    method: "POST",
    body: JSON.stringify({
      to, cc, bcc: [], subject, body, html: "",
      undo_seconds: 0, in_reply_to: inReplyTo, references,
    }),
  });

// mailReceipt answers a read-receipt request (RFC 3798).
export const mailReceipt = (folder: string, uid: number) =>
  apiPost<void>("/mail/receipt", { folder, uid });

// mailRecall recalls a sent message (sends X-MS-Recall notices to the
// recipients and flags the sent copy).
export const mailRecall = (folder: string, uid: number) =>
  apiPost<{ notified: number }>("/mail/recall", { folder, uid });

// mailRecallApply deletes the original message targeted by a recall notice.
export const mailRecallApply = (messageId: string) =>
  apiPost<{ removed: number }>("/mail/recall/apply", { message_id: messageId });

// MailMergeRecipient is one personalized recipient of a 逐封群发.
export type MailMergeRecipient = {
  email: string;
  name?: string;
  vars?: Record<string, string>;
};

// mailMerge sends one personalized copy per recipient ({{name}}, {{email}}
// and custom {{var}} placeholders are substituted per row).
export const mailMerge = (input: {
  subject: string;
  body: string;
  html?: string;
  from?: string;
  recipients: MailMergeRecipient[];
}) => apiPost<{ sent: number; failed: { email: string; error: string }[] }>("/mail/merge", input);

// mailReadAll marks an entire folder as read.
export const mailReadAll = (folder: string) =>
  apiPost<void>("/mail/read-all", { folder });

// mailAttachmentsZip downloads every attachment of one message as a zip.
export async function mailAttachmentsZip(folder: string, uid: number): Promise<Blob> {
  const res = await fetch(`${API}${mailPath(`/mail/attachments/zip?folder=${encodeURIComponent(folder)}&uid=${uid}`)}`, {
    headers: mailHeaders(),
  });
  if (!res.ok) throw new Error("download failed");
  return res.blob();
}

// uploadLargeAttachment stores a file in the large-attachment relay (超大附件)
// and returns the token-protected download URL for embedding in the body.
export const uploadLargeAttachment = (file: File) => {
  const fd = new FormData();
  fd.append("file", file);
  return fetch(`${API}${mailPath("/uploads")}`, {
    method: "POST",
    headers: mailHeaders(),
    body: fd,
  }).then(async (res) => {
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error((body as { error?: string }).error || "upload failed");
    }
    return (await res.json()) as {
      id: number;
      filename: string;
      size: number;
      expires: string;
      url: string;
    };
  });
};

// aiReplies returns short ready-to-send reply suggestions (Smart Reply).
export const aiReplies = (text: string) =>
  apiPost<{ replies: string[] }>("/ai/replies", { text });

// mailUndoSend cancels a parked message within its undo window.
export const mailUndoSend = (outboxId: number) =>
  api<void>(`/mail/outbox/${outboxId}`, { method: "DELETE" });

// ScheduledSend is one queued message waiting for its send_at moment.
export interface ScheduledSend {
  id: number;
  from: string;
  subject: string;
  send_at: string;
  recipients: string[];
}

// mailScheduled lists the current user's upcoming scheduled sends.
export const mailScheduled = () => api<ScheduledSend[]>("/mail/scheduled");

// mailUnsubscribe triggers a sender's List-Unsubscribe URL through the
// backend (avoids CORS and hides the user's IP from the sender).
export const mailUnsubscribe = (url: string, post: boolean) =>
  apiPost("/mail/unsubscribe", { url, post });

export const mailSaveDraft = (
  subject: string,
  text: string,
  html: string,
  replaceUid = 0,
  to: string[] = [],
  cc: string[] = [],
  attachments?: OutboundAttachment[],
) =>
  api<{ uid: number }>("/mail/draft", {
    method: "POST",
    body: JSON.stringify({ subject, text, html, replace_uid: replaceUid, to, cc, attachments }),
  });

export const mailFlag = (folder: string, uid: number, flag: string, value: boolean) =>
  apiPost("/mail/flag", { folder, uid, flag, value });

// Snooze: until is a unix-seconds timestamp, or 0/null to wake the message up.
export const mailSnooze = (folder: string, uid: number, until: number | null) =>
  apiPost("/mail/snooze", { folder, uid, until: until ?? 0 });

export const mailSnoozed = () => api<SnoozedMessage[]>("/mail/snoozed");

export const aiTranslate = (text: string, target: string) =>
  apiPost<{translation: string}>("/ai/translate", {text, target});

// Global admin announcement banner (204/undefined when none is active).
export const mailAnnouncement = () => api<MailAnnouncement | undefined>("/announcement");

// Label definitions (name + color) persisted per account.
export const mailLabels = () => api<MailLabel[]>("/mail/labels");

export const mailLabelSave = (name: string, color: string) =>
  apiPost<MailLabel>("/mail/labels", { name, color });

export const mailLabelRename = (from: string, to: string) =>
  apiPost("/mail/labels/rename", { from, to });

export const mailLabelDelete = (name: string) =>
  api(`/mail/labels?name=${encodeURIComponent(name)}`, { method: "DELETE" });

// Compose templates (canned responses / 常用语).
export interface MailTemplate {
  id: number;
  name: string;
  subject: string;
  html: string;
  text: string;
}

export const mailTemplates = () => api<MailTemplate[]>("/mail/templates");

export const mailTemplateSave = (tpl: {id?: number; name: string; subject?: string; html: string; text: string}) =>
  apiPost<MailTemplate>("/mail/templates", tpl);

export const mailTemplateDelete = (id: number) =>
  api(`/mail/templates/${id}`, { method: "DELETE" });

export const mailDelete = (folder: string, uid: number) =>
  apiPost("/mail/delete", { folder, uid });

// Contacts.
export const contactsDedupe = () => apiPost<{merged: number; removed: number}>("/contacts/dedupe", {});

export const carddavGet = () => api<{url: string; username: string; has_auth: boolean}>("/contacts/carddav");

export const carddavSet = (body: {url: string; username?: string; password?: string}) =>
  apiPut<void>("/contacts/carddav", body);

export const carddavSync = () => apiPost<{added: number; updated: number; total: number}>("/contacts/carddav/sync", {});

export const mailMove = (folder: string, uids: number[], destination: string) =>
  apiPost("/mail/move", { folder, uids, destination });

export interface FolderACLEntry {
  identifier: string;
  rights: string;
}

export const mailACL = (folder: string) =>
  api<{ folder: string; entries: FolderACLEntry[]; my_rights: string }>(
    `/mail/acl?folder=${encodeURIComponent(folder)}`,
  );

export const mailACLSet = (folder: string, identifier: string, rights: string) =>
  api<void>("/mail/acl", {
    method: "PUT",
    body: JSON.stringify({ folder, identifier, rights }),
  });

export const mailACLDelete = (folder: string, identifier: string) =>
  api<void>(
    `/mail/acl?folder=${encodeURIComponent(folder)}&identifier=${encodeURIComponent(identifier)}`,
    { method: "DELETE" },
  );

export const mailIdentities = () => api<MailIdentity[]>("/mail/identities");

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

// Mailbox delegation / shared mailboxes.
export const delegations = () => api<DelegationListing>("/delegations");

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

// Calendar (webmail JSON API over the same store as CalDAV).
export const calendarEvents = (from?: string, to?: string) => {
  const q = new URLSearchParams();
  if (from) q.set("from", from);
  if (to) q.set("to", to);
  const qs = q.toString();
  return api<CalendarEvent[]>(`/calendar/events${qs ? `?${qs}` : ""}`);
};

export const calendarEventCreate = (input: CalendarEventInput) =>
  apiPost<CalendarEvent>("/calendar/events", input);

export const calendarEventUpdate = (id: number, input: CalendarEventInput) =>
  apiPut<CalendarEvent>(`/calendar/events/${id}`, input);

export const calendarEventDelete = (id: number) =>
  api<void>(`/calendar/events/${id}`, { method: "DELETE" });

// Calendar sharing and ICS subscription.
export const calendarShares = () => api<CalendarShareListing>("/calendar/shares");

export const calendarShareCreate = (shareeEmail: string, readOnly: boolean) =>
  apiPost<CalendarShare>("/calendar/shares", { sharee_email: shareeEmail, read_only: readOnly });

export const calendarShareDelete = (id: number) =>
  api<void>(`/calendar/shares/${id}`, { method: "DELETE" });

export const calendarFeed = () => api<{ url: string }>("/calendar/feed");

// Meeting invitations (iTIP): respond to a received REQUEST or send a new
// one to attendees.
export const inviteRespond = (ics: string, action: "accept" | "decline" | "tentative") =>
  apiPost<{ ok: boolean }>("/invites/respond", { ics, action });

export const inviteSend = (input: {
  to: string[];
  summary: string;
  location: string;
  description: string;
  start: string;
  end?: string;
}) => apiPost<{ ok: boolean }>("/invites/send", input);

// Organization address book (read-only, synced from AD/LDAP).
export const orgContacts = () => api<OrgContact[]>("/contacts/org");

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

// Web Push subscriptions (new-mail notifications via service worker).
export const pushVapid = () => api<{ public_key: string }>("/push/vapid");

export const pushSubscribe = (sub: PushSubscriptionInput) =>
  apiPost("/push/subscribe", sub);

export const pushUnsubscribe = (sub: { endpoint: string }) =>
  api("/push/subscribe", { method: "DELETE", body: JSON.stringify(sub) });

// Sieve filter rules (ManageSieve)
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

export const aiStatus = () => api<AIStatus>("/ai/status");

export const aiSummarize = (text: string) =>
  api<{ summary: string }>("/ai/summarize", { method: "POST", body: JSON.stringify({ text }) });

export const aiDraft = (context: string, tone: DraftTone = "formal") =>
  api<{ draft: string }>("/ai/draft", {
    method: "POST",
    body: JSON.stringify({ context, tone }),
  });

export const aiPrioritize = (messages: { uid: number; subject: string; from: string }[]) =>
  api<{ scores: Record<string, number>; categories?: Record<string, string> }>("/ai/prioritize", {
    method: "POST",
    body: JSON.stringify({ messages }),
  });

export const aiSearch = (query: string) =>
  api<{ query: string; messages: MailMessage[] }>("/ai/search", {
    method: "POST",
    body: JSON.stringify({ query }),
  });

// Built-in OpenPGP (key management + encrypt/decrypt/sign/verify)
export const pgpStatus = () => api<PgpStatus>("/me/pgp");

export const pgpGenerate = (): Promise<PgpStatus> =>
  api<{ public_key: string; fingerprint: string }>("/me/pgp/generate", {
    method: "POST",
    body: JSON.stringify({}),
  }).then((k) => ({ has_key: true, ...k }));

export const pgpDelete = () => api<void>("/me/pgp", { method: "DELETE" });

export interface PgpKey {
  id: number;
  email: string;
  public_key: string;
  fingerprint: string;
  created_at: string;
}

export const pgpListKeys = () => api<PgpKey[]>("/me/pgp/keys");

export const pgpImportKey = (email: string, publicKey: string) =>
  api<PgpKey>("/me/pgp/keys", {
    method: "POST",
    body: JSON.stringify({ email, public_key: publicKey }),
  });

export const pgpDeleteKey = (id: number) =>
  api<void>(`/me/pgp/keys/${id}`, { method: "DELETE" });

export const pgpLookup = (email: string) =>
  api<{ public_key: string }>(`/pgp/key?email=${encodeURIComponent(email)}`);

export const pgpEncrypt = (text: string, publicKey: string) =>
  api<{ encrypted: string }>("/mail/pgp/encrypt", {
    method: "POST",
    body: JSON.stringify({ text, public_key: publicKey }),
  });

export const pgpDecrypt = (text: string) =>
  api<{ plaintext: string }>("/mail/pgp/decrypt", {
    method: "POST",
    body: JSON.stringify({ text }),
  });

export const pgpSign = (text: string) =>
  api<{ signature: string }>("/mail/pgp/sign", {
    method: "POST",
    body: JSON.stringify({ text }),
  });

export const pgpVerify = (text: string, signature: string, publicKey: string) =>
  api<{ valid: boolean }>("/mail/pgp/verify", {
    method: "POST",
    body: JSON.stringify({ text, signature, public_key: publicKey }),
  });

// Webhook event callbacks
export const webhookList = () => api<Webhook[]>("/webhooks");

export const webhookCreate = (w: { url: string; secret: string; events: string; enabled?: boolean }) =>
  api<Webhook>("/webhooks", { method: "POST", body: JSON.stringify(w) });

export const webhookUpdate = (id: number, w: { url?: string; secret?: string; events?: string; enabled?: boolean }) =>
  api<Webhook>(`/webhooks/${id}`, { method: "PUT", body: JSON.stringify(w) });

export const webhookDelete = (id: number) =>
  api<void>(`/webhooks/${id}`, { method: "DELETE" });

export const webhookTest = (id: number) =>
  api<{ ok: boolean; status: number; error?: string }>(`/webhooks/${id}/test`, { method: "POST" });

// S/MIME (certificate management + CMS encrypt/decrypt/sign/verify)
export const smimeStatus = () => api<SmimeStatus>("/me/smime");

export const smimeImport = (inp: {
  cert_pem?: string;
  private_key?: string;
  p12_b64?: string;
  p12_password?: string;
}) =>
  api<{ email: string; fingerprint: string; subject: string; issuer: string; not_after: string }>(
    "/me/smime",
    { method: "POST", body: JSON.stringify(inp) },
  ).then((k) => ({ has_cert: true, ...k }));

export const smimeDelete = () => api<void>("/me/smime", { method: "DELETE" });

export const smimeListCerts = () => api<SmimeCert[]>("/me/smime/certs");

export const smimeImportCert = (email: string, certPem: string) =>
  api<SmimeCert>("/me/smime/certs", {
    method: "POST",
    body: JSON.stringify({ email, cert_pem: certPem }),
  });

export const smimeDeleteCert = (id: number) =>
  api<void>(`/me/smime/certs/${id}`, { method: "DELETE" });

export const smimeLookup = (email: string) =>
  api<{ cert_pem: string; fingerprint: string }>(`/smime/cert?email=${encodeURIComponent(email)}`);

export const smimeEncrypt = (text: string, certPem: string) =>
  api<{ encrypted: string }>("/mail/smime/encrypt", {
    method: "POST",
    body: JSON.stringify({ text, cert_pem: certPem }),
  });

export const smimeDecrypt = (text: string) =>
  api<{ plaintext: string }>("/mail/smime/decrypt", {
    method: "POST",
    body: JSON.stringify({ text }),
  });

export const smimeSign = (text: string) =>
  api<{ signature: string }>("/mail/smime/sign", {
    method: "POST",
    body: JSON.stringify({ text }),
  });

export const smimeVerify = (signature: string, certPem: string) =>
  api<{ valid: boolean; content: string }>("/mail/smime/verify", {
    method: "POST",
    body: JSON.stringify({ signature, cert_pem: certPem }),
  });

// Two-factor authentication (TOTP)
export const totpStatus = () => api<TotpStatus>("/me/totp");

export const totpEnable = (code: string) =>
  api<void>("/me/totp/enable", { method: "POST", body: JSON.stringify({ code }) });

export const totpDisable = (code: string) =>
  api<void>("/me/totp", { method: "DELETE", body: JSON.stringify({ code }) });

// Self-service settings (GET /me / PUT /me/settings / PUT /me/password)
export const meProfile = () => api<MeSettings>("/me");

export const updateMeSettings = (body: Partial<MeSettings>) =>
  apiPut("/me/settings", body);

export const changePassword = (oldPw: string, newPw: string) =>
  api("/me/password", { method: "PUT", body: JSON.stringify({ old_pw: oldPw, new_pw: newPw }) });

// address book
export const contacts = () => api<Contact[]>("/contacts");

export const createContact = (name: string, email: string, comment = "", groups = "", avatar = "") =>
  apiPost<Contact>("/contacts", { name, email, comment, groups, avatar });

export const updateContact = (
  id: number,
  body: Partial<Pick<Contact, "name" | "email" | "comment" | "groups" | "avatar">>,
) => apiPut<Contact>(`/contacts/${id}`, body);

export const deleteContact = (id: number) =>
  api(`/contacts/${id}`, { method: "DELETE" });

export const exportContacts = async (): Promise<string> => {
  const res = await fetch(`${API}/contacts/export`);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new ApiError((body as { error?: string }).error || res.statusText, res.status);
  }
  return res.text();
};

export const importContacts = (data: string) =>
  apiPost<{ added: number; total: number }>("/contacts/import", { data });
