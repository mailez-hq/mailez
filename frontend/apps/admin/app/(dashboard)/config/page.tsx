"use client";

import { useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { exportConfig, importConfig, type ConfigBackup, type ConfigStats } from "@/lib/api";

export default function ConfigPage() {
  const t = useTranslations("config");
  const fileRef = useRef<HTMLInputElement>(null);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);

  async function onExport() {
    setError(""); setMessage("");
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
      setMessage(t("exported", {
        domains: s.domains ?? 0,
        users: s.users ?? 0,
        aliases: s.aliases ?? 0,
      }));
    } catch (e) {
      setError(e instanceof Error ? e.message : "export failed");
    }
  }

  async function onImport(file: File) {
    setError(""); setMessage("");
    if (!confirm(t("importConfirm", { name: file.name }))) return;
    try {
      const data = JSON.parse(await file.text()) as ConfigBackup;
      setBusy(true);
      const stats = await importConfig(data);
      setMessage(t("imported", {
        domains: stats.domains,
        users: stats.users,
        aliases: stats.aliases,
        alternatives: stats.alternatives,
        relays: stats.relays,
        fetches: stats.fetches,
        tokens: stats.tokens,
      }));
    } catch (e) {
      setError(e instanceof Error ? e.message : "import failed");
    } finally {
      setBusy(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold">{t("title")}</h1>
      <Card className="max-w-xl">
        <CardHeader>
          <CardTitle className="text-base">{t("backup")}</CardTitle>
          <CardDescription>{t("hint")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Button onClick={onExport} disabled={busy}>{t("export")}</Button>
            <Button variant="outline" disabled={busy} onClick={() => fileRef.current?.click()}>
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
          {error && <p className="text-sm text-red-600">{error}</p>}
          {message && <p className="text-sm text-green-600">{message}</p>}
        </CardContent>
      </Card>
    </div>
  );
}
