"use client";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import type { Webhook } from "@/lib/api";
import { cn } from "@/lib/utils";

// WebhooksSection manages the push webhook list: create, enable/disable,
// test-fire, delete.
export function WebhooksSection({
  t,
  webhooks,
  whUrl, setWhUrl,
  whSecret, setWhSecret,
  whEvents, setWhEvents,
  whEnabled, setWhEnabled,
  whSaving,
  whTesting,
  whTestMsg,
  onAddWebhook,
  onDeleteWebhook,
  onToggleWebhook,
  onTestWebhook,
}: {
  t: (key: string) => string;
  webhooks: Webhook[] | null;
  whUrl: string; setWhUrl: (v: string) => void;
  whSecret: string; setWhSecret: (v: string) => void;
  whEvents: string; setWhEvents: (v: string) => void;
  whEnabled: boolean; setWhEnabled: (v: boolean) => void;
  whSaving: boolean;
  whTesting: number | null;
  whTestMsg: string;
  onAddWebhook: () => void;
  onDeleteWebhook: (id: number) => void;
  onToggleWebhook: (w: Webhook) => void;
  onTestWebhook: (id: number) => void;
}) {
  return (
    <div className="h-full space-y-3 overflow-y-auto p-5">
      <p className="text-sm font-medium">{t("webhooks")}</p>
      <p className="text-xs text-muted-foreground">{t("webhookIntro")}</p>

      {/* New webhook form */}
      <div className="space-y-1.5 rounded-md border border-border p-3">
        <div className="space-y-1.5">
          <Label>{t("webhookUrl")}</Label>
          <Input
            value={whUrl}
            onChange={(e) => setWhUrl(e.target.value)}
            placeholder="https://example.com/hooks/mail"
            className="text-sm"
          />
        </div>
        <div className="space-y-1.5">
          <Label>{t("webhookSecret")}</Label>
          <Input
            value={whSecret}
            onChange={(e) => setWhSecret(e.target.value)}
            placeholder={t("webhookSecretPlaceholder")}
            className="text-sm font-mono"
          />
        </div>
        <div className="space-y-1.5">
          <Label>{t("webhookEvents")}</Label>
          <Input
            value={whEvents}
            onChange={(e) => setWhEvents(e.target.value)}
            placeholder="mail.received"
            className="text-sm font-mono"
          />
        </div>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Switch checked={whEnabled} onCheckedChange={setWhEnabled} />
            <span className="text-sm text-muted-foreground">{t("webhookEnabled")}</span>
          </div>
          <Button type="button" size="sm" onClick={onAddWebhook} disabled={whSaving}>
            {whSaving ? t("webhookSaving") : t("webhookAdd")}
          </Button>
        </div>
      </div>

      {whTestMsg && <p className="text-xs text-muted-foreground">{whTestMsg}</p>}

      {/* Existing webhooks */}
      {webhooks === null && <p className="text-xs text-muted-foreground">{t("loading")}</p>}
      {webhooks !== null && webhooks.length === 0 && (
        <p className="text-xs text-muted-foreground">{t("webhookEmpty")}</p>
      )}
      <ul className="space-y-1.5">
        {webhooks?.map((w) => (
          <li key={w.id} className="flex items-center gap-2 rounded-md border border-border px-2.5 py-1.5">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <p className="truncate text-sm">{w.url}</p>
                <span
                  className={cn(
                    "rounded px-1.5 py-0.5 text-[10px] font-medium",
                    w.enabled
                      ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
                      : "bg-muted text-muted-foreground",
                  )}
                >
                  {w.enabled ? t("webhookEnabled") : t("webhookDisabled")}
                </span>
              </div>
              <p className="truncate font-mono text-[11px] text-muted-foreground">
                {w.events}
                {w.last_status > 0 && ` · ${t("webhookLast")} ${w.last_status}`}
              </p>
              {w.last_error && (
                <p className="truncate text-[11px] text-destructive">{w.last_error}</p>
              )}
            </div>
            <Switch
              checked={w.enabled}
              onCheckedChange={() => onToggleWebhook(w)}
              aria-label={t("webhookEnabled")}
            />
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => onTestWebhook(w.id)}
              disabled={whTesting === w.id}
            >
              {whTesting === w.id ? t("webhookTesting") : t("webhookTest")}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="text-muted-foreground hover:text-destructive"
              onClick={() => onDeleteWebhook(w.id)}
            >
              {t("webhookDelete")}
            </Button>
          </li>
        ))}
      </ul>
    </div>
  );
}
