"use client";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { totpDisable, totpEnable, type TotpStatus } from "@/lib/api";

// TwoFactorSection enrolls or disenrolls TOTP second factor.
export function TwoFactorSection({
  t,
  totp, setTotp,
  totpCode, setTotpCode,
  totpBusy, setTotpBusy,
  setError,
}: {
  t: (key: string) => string;
  totp: TotpStatus | null; setTotp: (v: TotpStatus | null) => void;
  totpCode: string; setTotpCode: (v: string) => void;
  totpBusy: boolean; setTotpBusy: (v: boolean) => void;
  setError: (v: string) => void;
}) {
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
    </div>
  );
}
