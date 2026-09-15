"use client";

import { useEffect, useState } from "react";
import QRCode from "qrcode";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { AppTokenResult, MeSettings } from "@/lib/api";
import { buildDcLoginUri, fetchDcServerSettings } from "@/lib/deltachat";

// DeltaChatSection lets the signed-in user mint an app token and render it as
// a Delta Chat DCLOGIN QR: scan it in the Delta Chat app and the account
// (address, password, IMAP/SMTP settings) configures itself. The token is the
// same app-password mechanism as the DAV section — revocable, never the main
// password — and the QR only exists until the dialog closes.
export function DeltaChatSection({
  t,
  profile,
  newToken,
  busy,
  onCreate,
}: {
  t: (key: string) => string;
  profile: MeSettings | null;
  newToken: AppTokenResult | null;
  busy: boolean;
  onCreate: () => void;
}) {
  const [qr, setQr] = useState("");

  useEffect(() => {
    let cancelled = false;
    if (!newToken?.token || !profile?.email) {
      setQr("");
      return;
    }
    (async () => {
      const settings = await fetchDcServerSettings();
      const uri = buildDcLoginUri(profile.email, newToken.token || "", settings);
      const dataUrl = await QRCode.toDataURL(uri, { margin: 1, width: 220 });
      if (!cancelled) setQr(dataUrl);
    })().catch(() => {
      if (!cancelled) setQr("");
    });
    return () => {
      cancelled = true;
    };
  }, [newToken, profile]);

  return (
    <div className="space-y-4">
      <p className="text-sm font-medium">{t("deltachat")}</p>
      <p className="text-xs text-muted-foreground">{t("deltachatIntro")}</p>

      <div className="space-y-3 rounded-lg border border-border p-3">
        <div className="space-y-1">
          <Label>{t("deltachatAccount")}</Label>
          <Input readOnly value={profile?.email || ""} />
        </div>

        {!newToken?.token ? (
          <Button type="button" size="sm" onClick={onCreate} disabled={busy}>
            {busy ? t("deltachatGenerating") : t("deltachatGenerate")}
          </Button>
        ) : (
          <>
            <div className="flex flex-col items-center gap-2 rounded-md border border-border p-3">
              {qr ? (
                <img src={qr} alt="Delta Chat" className="h-[220px] w-[220px]" />
              ) : (
                <div className="flex h-[220px] w-[220px] items-center justify-center text-xs text-muted-foreground">
                  {t("deltachatRendering")}
                </div>
              )}
              <p className="text-center text-xs text-muted-foreground">{t("deltachatScanHint")}</p>
            </div>
            <div className="space-y-1">
              <Label>{t("deltachatPassword")}</Label>
              <div className="flex items-center gap-2">
                <Input readOnly value={newToken.token} className="font-mono" />
                <Button
                  type="button"
                  size="xs"
                  variant="outline"
                  onClick={() => navigator.clipboard?.writeText(newToken.token || "")}
                >
                  {t("copy")}
                </Button>
              </div>
              <p className="text-[10px] text-muted-foreground">{t("deltachatPasswordHint")}</p>
            </div>
          </>
        )}
      </div>

      <div className="space-y-1 rounded-lg border border-border p-3">
        <p className="text-sm font-medium">{t("deltachatClients")}</p>
        <p className="text-xs text-muted-foreground">{t("deltachatClientsHint")}</p>
      </div>
    </div>
  );
}
