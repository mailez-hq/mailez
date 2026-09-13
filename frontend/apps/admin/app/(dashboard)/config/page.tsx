"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Download, Upload } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import { EXTRA_TABS, ExtraConfigPanel } from "@/modules/config-extra";
import {
  exportConfig, importConfig, totpDisable, totpEnable, totpStatus,
  type ConfigBackup, type ConfigStats, type TotpStatus,
} from "@/lib/api";

type ConfigTab = "backup" | "security" | (string & {});

export default function ConfigPage() {
  const t = useTranslations("config");
  const fileRef = useRef<HTMLInputElement>(null);
  const [tab, setTab] = useState<ConfigTab>("backup");

  // Two-factor (TOTP) self-service for the signed-in admin.
  const [totp, setTotp] = useState<TotpStatus | null>(null);
  const [totpCode, setTotpCode] = useState("");
  const [totpBusy, setTotpBusy] = useState(false);
  const [totpError, setTotpError] = useState("");
  const [totpMessage, setTotpMessage] = useState("");

  const loadTotp = useCallback(async () => {
    try {
      setTotp(await totpStatus());
    } catch {
      setTotp(null);
    }
  }, []);

  useEffect(() => { loadTotp(); }, [loadTotp]);

  async function submitTotp(enable: boolean) {
    setTotpError(""); setTotpMessage(""); setTotpBusy(true);
    try {
      if (enable) await totpEnable(totpCode);
      else await totpDisable(totpCode);
      setTotpCode("");
      setTotpMessage(enable ? t("totpEnabledOk") : t("totpDisabledOk"));
      await loadTotp();
    } catch (e) {
      setTotpError(e instanceof Error ? e.message : "failed");
    } finally {
      setTotpBusy(false);
    }
  }

  const [backupBusy, setBackupBusy] = useState(false);
  const [backupError, setBackupError] = useState("");
  const [backupMessage, setBackupMessage] = useState("");

  async function onExport() {
    setBackupError(""); setBackupMessage("");
    try {
      const data = await exportConfig();
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `mailez-backup-${new Date().toISOString().slice(0, 10)}.json`;
      a.click();
      URL.revokeObjectURL(url);
      const s = data as unknown as ConfigStats;
      setBackupMessage(t("exported", {
        domains: s.domains ?? 0,
        users: s.users ?? 0,
        aliases: s.aliases ?? 0,
      }));
    } catch (e) {
      setBackupError(e instanceof Error ? e.message : "export failed");
    }
  }

  async function onImport(file: File) {
    setBackupError(""); setBackupMessage("");
    if (!confirm(t("importConfirm", { name: file.name }))) return;
    try {
      const data = JSON.parse(await file.text()) as ConfigBackup;
      setBackupBusy(true);
      const stats = await importConfig(data);
      setBackupMessage(t("imported", {
        domains: stats.domains,
        users: stats.users,
        aliases: stats.aliases,
        alternatives: stats.alternatives,
        relays: stats.relays,
        fetches: stats.fetches,
        tokens: stats.tokens,
      }));
    } catch (e) {
      setBackupError(e instanceof Error ? e.message : "import failed");
    } finally {
      setBackupBusy(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  // An optional config module appends extra tabs (branding, AI, LDAP); the
  // default module set exports no extra tabs, so only backup remains.
  const tabs: { key: ConfigTab; label: string }[] = [
    { key: "backup", label: t("backup") },
    { key: "security", label: t("security") },
    ...EXTRA_TABS.map((key) => ({ key: key as ConfigTab, label: t(key) })),
  ];

  return (
    <div className="space-y-4">
      <PageHeader title={t("title")} description={t("desc")} />

      <div className="flex gap-1 border-b border-border" role="tablist">
        {tabs.map(({ key, label }) => (
          <button
            key={key}
            role="tab"
            aria-selected={tab === key}
            onClick={() => setTab(key)}
            className={cn(
              "-mb-px border-b-2 px-3 py-2 text-sm transition-colors",
              tab === key
                ? "border-primary font-medium text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground",
            )}
          >
            {label}
          </button>
        ))}
      </div>

      {tab === "backup" && (
        <Card className="max-w-xl">
          <CardHeader>
            <CardTitle className="text-base">{t("backup")}</CardTitle>
            <CardDescription>{t("hint")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center gap-2">
              <Button onClick={onExport} disabled={backupBusy}>
                <Download />
                {t("export")}
              </Button>
              <Button variant="outline" disabled={backupBusy} onClick={() => fileRef.current?.click()}>
                <Upload />
                {t("import")}
              </Button>
              <Input
                ref={fileRef}
                type="file"
                accept="application/json,.json"
                className="hidden"
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  if (f) onImport(f);
                }}
              />
            </div>
            {backupError && <p className="text-sm text-red-600">{backupError}</p>}
            {backupMessage && <p className="text-sm text-green-600">{backupMessage}</p>}
          </CardContent>
        </Card>
      )}

      {tab === "security" && (
        <Card className="max-w-xl">
          <CardHeader>
            <CardTitle className="text-base">{t("totp")}</CardTitle>
            <CardDescription>{t("totpDesc")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center justify-between">
              <Label>{t("totpState")}</Label>
              <span className="text-sm font-medium">
                {totp?.enabled ? t("totpOn") : t("totpOff")}
              </span>
            </div>

            {totp && !totp.enabled && (
              <div className="space-y-2">
                <Label>{t("totpSecret")}</Label>
                <code className="block break-all rounded-md bg-muted px-3 py-2 text-xs">
                  {totp.secret}
                </code>
                <p className="text-xs text-muted-foreground">
                  {t("totpSecretHint")}
                </p>
                <code className="block break-all rounded-md bg-muted px-3 py-2 text-[11px]">
                  {totp.otpauth}
                </code>
              </div>
            )}

            <div className="flex items-end gap-2">
              <div className="flex-1 space-y-2">
                <Label>{totp?.enabled ? t("totpDisableCode") : t("totpVerifyCode")}</Label>
                <Input
                  value={totpCode}
                  onChange={(e) => setTotpCode(e.target.value)}
                  placeholder="123456"
                  inputMode="numeric"
                  maxLength={6}
                />
              </div>
              <Button
                disabled={totpBusy || totpCode.length < 6 || !totp}
                onClick={() => submitTotp(!totp?.enabled)}
                variant={totp?.enabled ? "outline" : "default"}
              >
                {totp?.enabled ? t("totpDisableAction") : t("totpEnableAction")}
              </Button>
            </div>

            {totpError && <p className="text-sm text-red-600">{totpError}</p>}
            {totpMessage && <p className="text-sm text-green-600">{totpMessage}</p>}
          </CardContent>
        </Card>
      )}

      {tab !== "backup" && tab !== "security" && <ExtraConfigPanel tab={tab} />}
    </div>
  );
}
