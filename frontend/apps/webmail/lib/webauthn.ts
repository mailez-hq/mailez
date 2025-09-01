"use client";

// Browser-side WebAuthn (passkey) helpers: turn the server's creation /
// request options JSON into live credential calls and serialize the results
// back into the exact JSON shape the server (go-webauthn) parses.

function b64uToBuf(s: string): ArrayBuffer {
  const norm = s.replace(/-/g, "+").replace(/_/g, "/");
  const pad = norm.length % 4 === 0 ? "" : "=".repeat(4 - (norm.length % 4));
  const bin = atob(norm + pad);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out.buffer;
}

function bufToB64u(b: ArrayBuffer): string {
  const bytes = new Uint8Array(b);
  let bin = "";
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

// Minimal shapes of the server options we consume; extra fields are passed
// through untouched.
interface CreationOptionsJSON {
  publicKey?: Record<string, unknown>;
  [key: string]: unknown;
}
interface RequestOptionsJSON {
  publicKey?: Record<string, unknown>;
  [key: string]: unknown;
}

export interface WebauthnResponseJSON {
  [field: string]: string | string[] | undefined;
}
export interface WebauthnCredentialJSON {
  id: string;
  rawId: string;
  type: "public-key";
  response: WebauthnResponseJSON;
}

function baseCreationPublicKey(o: CreationOptionsJSON): PublicKeyCredentialCreationOptions {
  const p = (o.publicKey ?? o) as unknown as {
    challenge: string;
    rp: PublicKeyCredentialRpEntity;
    user: Record<string, unknown> & { id: string };
    pubKeyCredParams: PublicKeyCredentialParameters[];
    timeout?: number;
    excludeCredentials?: PublicKeyCredentialDescriptorJSON[];
    authenticatorSelection?: AuthenticatorSelectionCriteria;
    attestation?: AttestationConveyancePreference;
  };
  const user = p.user;
  return {
    challenge: b64uToBuf(p.challenge),
    rp: p.rp,
    user: {
      ...(user as unknown as PublicKeyCredentialUserEntity),
      id: b64uToBuf(String(user.id)),
    },
    pubKeyCredParams: p.pubKeyCredParams,
    timeout: p.timeout,
    excludeCredentials: (p.excludeCredentials ?? []).map(
      (c) => ({ ...c, id: b64uToBuf(String(c.id)) }) as PublicKeyCredentialDescriptor,
    ),
    authenticatorSelection: p.authenticatorSelection,
    attestation: p.attestation,
  };
}

interface PublicKeyCredentialDescriptorJSON {
  id: string;
  type?: string;
  transports?: string[];
}

interface AssertionOptionsJSONShape {
  challenge: string;
  timeout?: number;
  rpId?: string;
  allowCredentials?: PublicKeyCredentialDescriptorJSON[];
  userVerification?: UserVerificationRequirement;
}

function baseRequestPublicKey(o: RequestOptionsJSON): PublicKeyCredentialRequestOptions {
  const p = (o.publicKey ?? o) as unknown as AssertionOptionsJSONShape;
  return {
    challenge: b64uToBuf(String(p.challenge)),
    timeout: p.timeout,
    rpId: p.rpId,
    allowCredentials: (p.allowCredentials ?? []).map(
      (c) => ({ ...c, id: b64uToBuf(String(c.id)) }) as PublicKeyCredentialDescriptor,
    ),
    userVerification: p.userVerification,
  };
}

// createCredential runs navigator.credentials.create() for a registration
// ceremony and returns the attestation for the server to verify.
export async function createCredential(optionsJSON: CreationOptionsJSON): Promise<WebauthnCredentialJSON> {
  const cred = (await navigator.credentials.create({
    publicKey: baseCreationPublicKey(optionsJSON),
  })) as PublicKeyCredential | null;
  if (!cred) throw new Error("webauthn: no credential returned");
  const r = cred.response as AuthenticatorAttestationResponse;
  const transports = (() => {
    try {
      return r.getTransports?.() ?? [];
    } catch {
      return [];
    }
  })();
  return {
    id: cred.id,
    rawId: bufToB64u(cred.rawId),
    type: "public-key",
    response: {
      clientDataJSON: bufToB64u(r.clientDataJSON),
      attestationObject: bufToB64u(r.attestationObject),
      transports,
    },
  };
}

// getAssertion runs navigator.credentials.get() for an authentication
// ceremony and returns the assertion for the server to verify.
export async function getAssertion(optionsJSON: RequestOptionsJSON): Promise<WebauthnCredentialJSON> {
  const cred = (await navigator.credentials.get({
    publicKey: baseRequestPublicKey(optionsJSON),
  })) as PublicKeyCredential | null;
  if (!cred) throw new Error("webauthn: no assertion returned");
  const r = cred.response as AuthenticatorAssertionResponse;
  return {
    id: cred.id,
    rawId: bufToB64u(cred.rawId),
    type: "public-key",
    response: {
      clientDataJSON: bufToB64u(r.clientDataJSON),
      authenticatorData: bufToB64u(r.authenticatorData),
      signature: bufToB64u(r.signature),
      userHandle: r.userHandle ? bufToB64u(r.userHandle) : "",
    },
  };
}

// passkeySupported reports whether this browser can drive a passkey flow at
// all (secure context + API present).
export function passkeySupported(): boolean {
  try {
    return typeof window !== "undefined" &&
      window.isSecureContext &&
      typeof navigator.credentials?.create === "function" &&
      typeof navigator.credentials?.get === "function";
  } catch {
    return false;
  }
}
