"use client";

import { useCallback, useEffect, useState } from "react";

import { Fingerprint, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  totpDisable, totpEnable, webauthnDelete, webauthnList,
  webauthnRegisterBegin, webauthnRegisterFinish,
  type TotpStatus, type WebauthnCredentialInfo,
} from "@/lib/api";
import { createCredential, passkeySupported } from "@/lib/webauthn";

// TwoFactorSection enrolls or disenrolls TOTP second factor and manages
// registered passkeys (WebAuthn credentials).
export function TwoFactorSection({
  t,
  totp, setTotp,
  totpCode, setTotpCode,
  totpBusy, setTotpBusy,
  setError,
}: {
  // values carries ICU interpolation values (next-intl t satisfies this).
  t: (key: string, values?: Record<string, string | number>) => string;
  totp: TotpStatus | null; setTotp: (v: TotpStatus | null) => void;
  totpCode: string; setTotpCode: (v: string) => void;
  totpBusy: boolean; setTotpBusy: (v: boolean) => void;
  setError: (v: string) => void;
}) {
  // Passkey state is self-contained: the parent passes only the shared
  // error sink, the way the TOTP block did before passkeys landed.
  const [creds, setCreds] = useState<WebauthnCredentialInfo[] | null>(null);
  const [credName, setCredName] = useState("");
  const [credBusy, setCredBusy] = useState(false);

  const loadCreds = useCallback(async () => {
    try {
      const res = await webauthnList();
      setCreds(res.credentials ?? []);
    } catch {
      setCreds([]);
    }
  }, []);

  useEffect(() => {
    if (passkeySupported()) void loadCreds();
    else setCreds([]);
  }, [loadCreds]);

  async function registerPasskey() {
    setCredBusy(true);
    setError("");
    try {
      const { options } = await webauthnRegisterBegin();
      const credential = await createCredential(options as Parameters<typeof createCredential>[0]);
      await webauthnRegisterFinish(credName.trim(), credential);
      setCredName("");
      await loadCreds();
    } catch (e) {
      setError(e instanceof Error ? e.message : "passkey registration failed");
    } finally {
      setCredBusy(false);
    }
  }

  async function deletePasskey(id: number) {
    setCredBusy(true);
    setError("");
    try {
      await webauthnDelete(id);
      await loadCreds();
    } catch (e) {
      setError(e instanceof Error ? e.message : "passkey delete failed");
    } finally {
      setCredBusy(false);
    }
  }

  return (
    <div className="h-full space-y-3 overflow-y-auto p-5">
      <p className="text-sm font-medium">{t("twoFactor")}</p>
      {totp === null && <p className="text-sm text-muted-foreground">{t("loading")}</p>}
      {totp && totp.enabled && (
        <div className="space-y-2">
          <p className="text-xs text-muted-foreground">{t("twoFactorEnabled")}</p>
          <div className="flex gap-1.5">
            <Input
              value={totpCode}
              onChange={(e) => setTotpCode(e.target.value)}
              placeholder="123456"
              inputMode="numeric"
              className="h-8 w-28"
            />
            <Button
              size="sm"
              variant="outline"
              disabled={totpBusy}
              onClick={async () => {
                setTotpBusy(true);
                setError("");
                try {
                  await totpDisable(totpCode.trim());
                  setTotp({ enabled: false });
                  setTotpCode("");
                } catch (e) {
                  setError(e instanceof Error ? e.message : "2fa disable failed");
                } finally {
                  setTotpBusy(false);
                }
              }}
            >
              {t("disable")}
            </Button>
          </div>
        </div>
      )}
      {totp && !totp.enabled && (
        <div className="space-y-2">
          <p className="text-xs text-muted-foreground">{t("twoFactorHint")}</p>
          {totp.otpauth && (
            <p className="rounded-md border border-border bg-muted/40 p-2 font-mono text-[10px] break-all text-muted-foreground">
              {totp.otpauth}
            </p>
          )}
          <div className="flex gap-1.5">
            <Input
              value={totpCode}
              onChange={(e) => setTotpCode(e.target.value)}
              placeholder="123456"
              inputMode="numeric"
              className="h-8 w-28"
            />
            <Button
              size="sm"
              disabled={totpBusy}
              onClick={async () => {
                setTotpBusy(true);
                setError("");
                try {
                  await totpEnable(totpCode.trim());
                  setTotp({ enabled: true });
                  setTotpCode("");
                } catch (e) {
                  setError(e instanceof Error ? e.message : "2fa enable failed");
                } finally {
                  setTotpBusy(false);
                }
              }}
            >
              {t("enable")}
            </Button>
          </div>
        </div>
      )}

      {passkeySupported() && (
        <div className="space-y-2 border-t border-border pt-3">
          <p className="flex items-center gap-1.5 text-sm font-medium">
            <Fingerprint className="size-4" />
            {t("passkeys")}
          </p>
          <p className="text-xs text-muted-foreground">{t("passkeysHint")}</p>
          {creds === null && <p className="text-sm text-muted-foreground">{t("loading")}</p>}
          {creds !== null && creds.length === 0 && (
            <p className="text-xs text-muted-foreground">{t("passkeysEmpty")}</p>
          )}
          {creds !== null && creds.length > 0 && (
            <ul className="space-y-1">
              {creds.map((c) => (
                <li
                  key={c.id}
                  className="flex items-center justify-between gap-2 rounded-md border border-border px-2 py-1.5 text-xs"
                >
                  <span className="min-w-0 flex-1 truncate">
                    {c.name || c.credential_id.slice(0, 16)}
                    {c.last_used_at && (
                      <span className="ml-1 text-muted-foreground">
                        · {t("passkeyLastUsed", { time: new Date(c.last_used_at).toLocaleString() })}
                      </span>
                    )}
                  </span>
                  <Button
                    size="xs"
                    variant="outline"
                    disabled={credBusy}
                    onClick={() => void deletePasskey(c.id)}
                  >
                    <Trash2 className="size-3" />
                    {t("delete")}
                  </Button>
                </li>
              ))}
            </ul>
          )}
          <div className="flex gap-1.5">
            <Input
              value={credName}
              onChange={(e) => setCredName(e.target.value)}
              placeholder={t("passkeyNamePlaceholder")}
              className="h-8 w-44"
            />
            <Button size="sm" disabled={credBusy || creds === null} onClick={() => void registerPasskey()}>
              {t("passkeyAdd")}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
