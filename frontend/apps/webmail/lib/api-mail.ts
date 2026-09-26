// Mailbox, message, compose, label and folder ACL API calls.
import type {
  MailAnnouncement,
  MailIdentity,
  MailLabel,
  MailMessage,
  MailPage,
  MailSearchSpec,
  MailThread,
  OutboundAttachment,
  SnoozedMessage,
} from "@mailez/types";

import {
  api,
  apiPost,
  apiPut,
  ApiError,
  LOAD_TIMEOUT_MS,
  HAS_OPTIONAL,
  mailPath,
  mailHeaders,
  handleUnauthorized,
  sessionExpiredText,
  requestWithTimeout,
  API,
} from "./api-client";

export const mailFolders = () =>
  api<string[]>("/mail/folders", { timeoutMs: LOAD_TIMEOUT_MS });

export const mailFolderCreate = (name: string) =>
  apiPost<void>("/mail/folders", { name });

export const mailFolderRename = (name: string, newName: string) =>
  apiPut<void>("/mail/folders", { name, new_name: newName });

export const mailFolderDelete = (name: string) =>
  api<void>(`/mail/folders?name=${encodeURIComponent(name)}`, { method: "DELETE" });

export const mailFolderClear = (name: string) =>
  apiPost<void>("/mail/folders/clear", { name });

export const mailUnseen = () =>
  api<Record<string, number>>("/mail/unseen", { timeoutMs: LOAD_TIMEOUT_MS });

export async function mailMessages(folder: string, page = 0, sort = "date", dir = "", conversation = false): Promise<MailPage> {
  const res = await requestWithTimeout(
    `${API}${mailPath(`/mail/messages?folder=${encodeURIComponent(folder)}&page=${page}&sort=${encodeURIComponent(sort)}&dir=${encodeURIComponent(dir)}${conversation ? "&conversation=1" : ""}`)}`,
    {
      headers: { "Content-Type": "application/json", ...mailHeaders() },
      timeoutMs: LOAD_TIMEOUT_MS,
    },
  );
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    const err = body as { error?: string; code?: string };
    const expired = res.status === 401;
    if (expired) handleUnauthorized("/mail/messages");
    throw new ApiError(expired ? sessionExpiredText() : err.error || res.statusText, res.status, err.code);
  }
  const messages = (await res.json()) ?? [];
  const total = Number(res.headers.get("X-Total-Messages") || 0);
  return { messages, total };
}

export const mailSearch = (folder: string, q: string) =>
  api<MailMessage[]>(`/mail/search?folder=${encodeURIComponent(folder)}&q=${encodeURIComponent(q)}`, {
    timeoutMs: LOAD_TIMEOUT_MS,
  });

// mailSearchSpec runs a structured search (built visually, no syntax parsing).
export const mailSearchSpec = (folder: string, spec: MailSearchSpec) =>
  api<MailMessage[]>("/mail/search", {
    method: "POST",
    body: JSON.stringify({ folder, query: spec }),
    timeoutMs: LOAD_TIMEOUT_MS,
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
  inReplyTo?: string,
  references?: string,
) =>
  api<{ queued?: boolean; outbox_id?: number; undo_seconds?: number; scheduled?: boolean }>(
    "/mail/send",
    {
      method: "POST",
      body: JSON.stringify({ to, cc, bcc, subject, body, html, from, attachments, undo_seconds: undoSeconds, send_at: sendAt, receipt_requested: receiptRequested, burn_after_minutes: burnAfterMinutes, in_reply_to: inReplyTo, references, signature_applied: true }),
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
      signature_applied: true,
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
}) =>
  apiPost<{ sent: number; failed: { email: string; error: string }[] }>("/mail/merge", {
    ...input,
    signature_applied: true,
  });

// mailReadAll marks an entire folder as read.
export const mailReadAll = (folder: string) =>
  apiPost<void>("/mail/read-all", { folder });

// mailAttachmentsZip downloads every attachment of one message as a zip.
export async function mailAttachmentsZip(folder: string, uid: number): Promise<Blob> {
  const res = await fetch(`${API}${mailPath(`/mail/attachments/zip?folder=${encodeURIComponent(folder)}&uid=${uid}`)}`, {
    headers: mailHeaders(),
  });
  if (!res.ok) {
    if (res.status === 401) handleUnauthorized("/mail/attachments/zip");
    throw new Error("download failed");
  }
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
      if (res.status === 401) {
        handleUnauthorized("/uploads");
        throw new Error(sessionExpiredText());
      }
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

// mailUndoSend cancels a parked message within its undo window.
export const mailUndoSend = (outboxId: number) =>
  api<void>(`/mail/outbox/${outboxId}`, { method: "DELETE" });

// mailScheduledToDraft cancels a scheduled send and moves it back to Drafts
// so it can be viewed/edited again (modern-style cancel-to-draft).
export const mailScheduledToDraft = (outboxId: number) =>
  api<{ uid: number }>(`/mail/outbox/${outboxId}/to-draft`, { method: "POST" });

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
  bcc: string[] = [],
  attachments?: OutboundAttachment[],
) =>
  api<{ uid: number }>("/mail/draft", {
    method: "POST",
    body: JSON.stringify({ subject, text, html, replace_uid: replaceUid, to, cc, bcc, attachments }),
  });

export const mailFlag = (folder: string, uid: number, flag: string, value: boolean) =>
  apiPost("/mail/flag", { folder, uid, flag, value });

// mailBurnReveal opens a burn-after-read message. The server starts the reveal
// window and only then returns the body, so the content is never on the wire
// before the user asks for it (and never after the window closes).
export const mailBurnReveal = (folder: string, uid: number) =>
  api<MailMessage>("/mail/burn/reveal", { method: "POST", body: JSON.stringify({ folder, uid }) });

// Snooze: until is a unix-seconds timestamp, or 0/null to wake the message up.
export const mailSnooze = (folder: string, uid: number, until: number | null) =>
  apiPost("/mail/snooze", { folder, uid, until: until ?? 0 });

export const mailSnoozed = () => api<SnoozedMessage[]>("/mail/snoozed");

// Global admin announcement banner (204/undefined when none is active).
// Optional-module surface; without the extended module set this resolves
// "no announcement" locally instead of firing a request that can only 404.
export const mailAnnouncement = (): Promise<MailAnnouncement | undefined> =>
  HAS_OPTIONAL ? api<MailAnnouncement | undefined>("/announcement") : Promise.resolve(undefined);

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

// Deleting deletes: from any folder the message goes to Trash, from Trash
// itself the server purges it.
export const mailDelete = (folder: string, uids: number | number[]) =>
  apiPost("/mail/delete", { folder, uids: Array.isArray(uids) ? uids : [uids] });

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
