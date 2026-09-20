// Generic request layer for the webmail API: base path, mailbox scoping,
// errors, timeouts and the shared wire types. Domain api modules build on it.
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

export const API = "/api/v1";

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
export function mailPath(path: string): string {
  if (activeAccountId == null || !path.startsWith("/mail/")) return path;
  const sep = path.includes("?") ? "&" : "?";
  return `${path}${sep}account_id=${activeAccountId}`;
}

export function mailHeaders(): Record<string, string> {
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

// Build-time marker (injected by next.config.ts from MAILEZ_MODULES):
// "true" when an extended module set is baked in, empty for the default
// set. Module-aware API helpers consult it and resolve locally when the
// extended modules are absent, instead of firing requests that can only
// 404. Shared with the domain modules.
export const HAS_OPTIONAL = (process.env.NEXT_PUBLIC_MAILEZ_MODULES_ACTIVE ?? "") === "true";

// Marker for components that hide optional-module UI wholesale (e.g. the
// calendar share button). Keep in sync with HAS_OPTIONAL.
export const HAS_OPTIONAL_MODULES = HAS_OPTIONAL;

// A 401 on an authed surface means the session cookie expired or was
// revoked server-side. Instead of letting every poll toast errors forever,
// bounce to the login screen once; the guard flag stops in-flight request
// storms from chaining redirects. Auth endpoints (/auth, /sso) display
// their own 401s.
let bouncedToLogin = false;
export function sessionBounced(): boolean {
  return bouncedToLogin;
}

// Human copy for the brief window between the first 401 and the login-page
// navigation: the raw server text ("authentication required") must never
// surface in the UI. Locale follows the NEXT_LOCALE cookie the switcher
// persists (see app/layout.tsx).
export function sessionExpiredText(): string {
  const zh =
    typeof document !== "undefined" &&
    /(?:^|;\s*)NEXT_LOCALE=zh/.test(document.cookie);
  return zh ? "登录已过期，请重新登录" : "Your session has expired. Please sign in again.";
}

function authSurface(path: string): boolean {
  return path.startsWith("/auth") || path.startsWith("/sso");
}

export function handleUnauthorized(path: string) {
  if (bouncedToLogin || typeof window === "undefined") return;
  if (authSurface(path)) return;
  bouncedToLogin = true;
  if (window.location.pathname === "/") return; // already on the login page
  // Carry the reason and, on mail surfaces, a return link so the sign-in
  // page can explain the bounce and restore where the user was after
  // logging back in (standard session-expiry flow).
  const here = window.location.pathname + window.location.search;
  const target =
    here === "/home" || here.startsWith("/mail")
      ? `/?expired=1&next=${encodeURIComponent(here)}`
      : "/?expired=1";
  window.location.href = target;
}

// fetch has no deadline of its own. A stalled request (backend restarting, a
// dropped IMAP connection) used to leave the mailbox pinned to its loading
// skeleton indefinitely, because the promise simply never settled. Every
// request now aborts on a deadline and reports it through the ordinary error
// path; LOAD_TIMEOUT_MS is the tighter budget for the reads that gate the
// first paint of the list (folder counts, message page, search), so a stuck
// load tells the user instead of spinning forever.
const DEFAULT_TIMEOUT_MS = 60_000;
export const LOAD_TIMEOUT_MS = 30_000;

export type ApiInit = RequestInit & { timeoutMs?: number };

function requestTimeoutText(): string {
  const zh =
    typeof document !== "undefined" &&
    /(?:^|;\s*)NEXT_LOCALE=zh/.test(document.cookie);
  return zh ? "请求超时，请稍后重试" : "The request timed out. Please try again.";
}

export async function requestWithTimeout(url: string, init: ApiInit = {}): Promise<Response> {
  // An explicit signal still wins: callers that manage their own cancellation
  // keep full control of the request's lifetime.
  const { timeoutMs, ...rest } = init;
  const signal = rest.signal ?? AbortSignal.timeout(timeoutMs ?? DEFAULT_TIMEOUT_MS);
  try {
    return await fetch(url, { ...rest, signal });
  } catch (err) {
    if (err instanceof DOMException && err.name === "TimeoutError") {
      throw new ApiError(requestTimeoutText(), 0);
    }
    throw err;
  }
}

export async function api<T>(path: string, init?: ApiInit): Promise<T> {
  const res = await requestWithTimeout(`${API}${mailPath(path)}`, {
    headers: { "Content-Type": "application/json", ...mailHeaders() },
    ...init,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    const err = body as { error?: string; code?: string };
    const expired = res.status === 401 && !authSurface(path);
    if (expired) handleUnauthorized(path);
    throw new ApiError(
      expired ? sessionExpiredText() : err.error || res.statusText,
      res.status,
      err.code,
    );
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
