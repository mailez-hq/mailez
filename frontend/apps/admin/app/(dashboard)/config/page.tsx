"use client";

import { useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Download, Upload } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { EXTRA_TABS, ExtraConfigPanel } from "@/modules/config-extra";
import {
  exportConfig, importConfig,
  type ConfigBackup, type ConfigStats,
} from "@/lib/api";

type ConfigTab = "backup" | (string & {});

export default function ConfigPage() {
  const t = useTranslations("config");
  const fileRef = useRef<HTMLInputElement>(null);
  const [tab, setTab] = useState<ConfigTab>("backup");

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

      {tab !== "backup" && <ExtraConfigPanel tab={tab} />}
    </div>
  );
}
