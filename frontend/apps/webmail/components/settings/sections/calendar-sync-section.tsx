"use client";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { AppToken, AppTokenResult, MeSettings } from "@/lib/api";

// CalendarSyncSection shows the DAV endpoint, the app token used as the DAV
// password, and any additional tokens issued for other devices.
export function CalendarSyncSection({
  t,
  profile,
  davTokens,
  davNewToken,
  davTokenBusy,
  onCreateDavToken,
  onDeleteDavToken,
}: {
  t: (key: string) => string;
  profile: MeSettings | null;
  davTokens: AppToken[];
  davNewToken: AppTokenResult | null;
  davTokenBusy: boolean;
  onCreateDavToken: () => void;
  onDeleteDavToken: (id: number) => void;
}) {
  return (
    <div className="space-y-4">
      <p className="text-sm font-medium">{t("calendarSync")}</p>
      <p className="text-xs text-muted-foreground">{t("calendarSyncIntro")}</p>

      <div className="space-y-2 rounded-lg border border-border p-3">
        <p className="text-sm font-medium">{t("calendarSyncServer")}</p>
        <div className="space-y-1">
          <Label>{t("calendarSyncUrl")}</Label>
          <Input readOnly value={typeof window !== "undefined" ? `${window.location.origin}/dav/` : "/dav/"} />
        </div>
        <div className="space-y-1">
          <Label>{t("calendarSyncUsername")}</Label>
          <Input readOnly value={profile?.email || ""} />
        </div>
        <div className="space-y-1">
          <Label>{t("calendarSyncPassword")}</Label>
          <div className="flex items-center gap-2">
            <Input readOnly value={davNewToken?.token || t("calendarSyncPasswordHint")} className="font-mono" />
            <Button type="button" size="sm" onClick={onCreateDavToken} disabled={davTokenBusy}>
              {davTokenBusy ? t("calendarSyncGenerating") : t("calendarSyncGenerate")}
            </Button>
          </div>
          {davNewToken?.token && (
            <Button
              type="button"
              size="xs"
              variant="outline"
              onClick={() => navigator.clipboard?.writeText(davNewToken.token || "")}
            >
              {t("copy")}
            </Button>
          )}
        </div>
        {davTokens.length > 0 && (
          <div className="space-y-1.5 border-t border-border pt-2">
            <p className="text-xs font-medium text-muted-foreground">{t("calendarSyncTokens")}</p>
            {davTokens.map((tok) => (
              <div key={tok.id} className="flex items-center gap-2 rounded-md border border-border px-2.5 py-1.5">
                <div className="min-w-0 flex-1">
                  <p className="truncate font-mono text-[11px] text-muted-foreground">
                    {tok.ip || "app token"}
                  </p>
                  <p className="text-[10px] text-muted-foreground">
                    {tok.created_at ? new Date(tok.created_at).toLocaleString() : "—"}
                  </p>
                </div>
                <Button type="button" size="sm" variant="ghost" className="text-destructive" onClick={() => onDeleteDavToken(tok.id)}>
                  {t("revoke")}
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="space-y-1 rounded-lg border border-border p-3">
        <p className="text-sm font-medium">{t("calendarSyncClients")}</p>
        <p className="text-xs text-muted-foreground">{t("calendarSyncClientsHint")}</p>
      </div>
    </div>
  );
}
