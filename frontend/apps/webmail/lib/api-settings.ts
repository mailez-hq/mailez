// Settings-surface API calls: server settings, assist, push, filters,
// encryption and webhooks.
import type {
  AIStatus,
  DraftTone,
  MailMessage,
  PgpStatus,
  PushSubscriptionInput,
  SieveScript,
  SmimeCert,
  SmimeStatus,
  Webhook,
} from "@mailez/types";

import {
  api,
  apiPost,
  ApiError,
  HAS_OPTIONAL,
  API,
} from "./api-client";

// aiReplies returns short ready-to-send reply suggestions (Smart Reply).
export const aiReplies = (text: string) =>
  apiPost<{ replies: string[] }>("/ai/replies", { text });

export const aiTranslate = (text: string, target: string) =>
  apiPost<{translation: string}>("/ai/translate", {text, target});

export type ServerSettings = {
  hostname: string;
  domain: string;
  domains: string[];
  default_domain: string;
  smtp: { plain: number; submission: number; ssl: number };
  pop3: { plain: number; ssl: number };
  imap: { plain: number; ssl: number };
  branding?: BrandingConfig;
  // Federated sign-in advertisement (optional module): the login
  // page renders the SSO button only when the backend mounts the routes.
  oidc?: { enabled: boolean };
};

// BrandingConfig is the customizable login-page brand. Empty
// fields fall back to the built-in Mailez brand. website_url is the
// footer brand-link target: community builds serve the product site,
// white-label deployments serve their own (or empty to drop the link).
export type BrandingConfig = {
  title?: string;
  subtitle?: string;
  tagline?: string;
  feature1?: string;
  feature2?: string;
  feature3?: string;
  logo_url?: string;
  hero_url?: string;
  copyright?: string;
  contact?: string;
  website_url?: string;
};

export const serverSettings = () => api<ServerSettings>("/server/settings");

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

// Optional-module capability probe. Without the extended module set this
// throws the same 404-shaped ApiError the real probe would return, so the
// locked hint still renders — without a network round-trip that can only
// fail.
export const aiStatus = (): Promise<AIStatus> =>
  HAS_OPTIONAL
    ? api<AIStatus>("/ai/status")
    : Promise.reject(new ApiError("ai/status: unavailable without the extended module set", 404));

export const aiSummarize = (text: string) =>
  api<{ summary: string }>("/ai/summarize", { method: "POST", body: JSON.stringify({ text }) });

export const aiDraft = (context: string, tone: DraftTone = "formal") =>
  api<{ draft: string }>("/ai/draft", {
    method: "POST",
    body: JSON.stringify({ context, tone }),
  });

export const aiDraftNew = (subject: string, hint: string, tone: DraftTone = "formal") =>
  api<{ draft: string }>("/ai/draft", {
    method: "POST",
    body: JSON.stringify({ subject, hint, tone }),
  });

export const aiComposeDraft = (instruction: string) =>
  api<{ to: string[]; subject: string; body: string }>("/ai/compose", {
    method: "POST",
    body: JSON.stringify({ instruction }),
  });

export type AiComposeEvent = { type: "to" | "subject" | "body" | "error" | "done"; text?: string };

// aiComposeStream opens the streaming compose endpoint and invokes onEvent
// for every SSE payload as the model writes.
export async function aiComposeStream(
  instruction: string,
  onEvent: (ev: AiComposeEvent) => void,
): Promise<void> {
  const res = await fetch(`${API}/ai/compose/stream`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ instruction }),
  });
  if (!res.ok || !res.body) {
    let msg = res.statusText;
    try {
      const body = await res.text();
      if (body) msg = body;
    } catch {
      // keep statusText
    }
    throw new Error(msg);
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    let idx: number;
    while ((idx = buf.indexOf("\n\n")) >= 0) {
      const chunk = buf.slice(0, idx);
      buf = buf.slice(idx + 2);
      for (const line of chunk.split("\n")) {
        if (!line.startsWith("data:")) continue;
        const payload = line.slice(5).trim();
        if (!payload) continue;
        try {
          const ev = JSON.parse(payload) as AiComposeEvent;
          onEvent(ev);
        } catch (err) {
          // Tolerate malformed JSON frames, but never swallow errors thrown
          // by the consumer (they must surface to the UI).
          if (err instanceof SyntaxError) continue;
          throw err;
        }
      }
    }
  }
}

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

// S/MIME (certificate management + CMS encrypt/decrypt/sign/verify).
// Optional module: without the extended module set this short-circuits the
// two reads the settings shell probes unconditionally; the write/import
// helpers are only reachable from the settings section of that module.
export const smimeStatus = (): Promise<SmimeStatus> =>
  HAS_OPTIONAL ? api<SmimeStatus>("/me/smime") : Promise.resolve({ has_cert: false });

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

export const smimeListCerts = (): Promise<SmimeCert[]> =>
  HAS_OPTIONAL ? api<SmimeCert[]>("/me/smime/certs") : Promise.resolve([]);

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
