"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Download, Upload } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import {
  exportConfig, getAIConfig, importConfig, putAIConfig,
  type ConfigBackup, type ConfigStats,
} from "@/lib/api";

export default function ConfigPage() {
  const t = useTranslations("config");
  const fileRef = useRef<HTMLInputElement>(null);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [aiEnabled, setAiEnabled] = useState(false);
  const [aiBaseUrl, setAiBaseUrl] = useState("");
  const [aiApiKey, setAiApiKey] = useState("");
  const [aiModel, setAiModel] = useState("");
  const [aiHasKey, setAiHasKey] = useState(false);
  const [aiMessage, setAiMessage] = useState("");

  useEffect(() => {
    getAIConfig()
      .then((c) => {
        setAiEnabled(c.enabled);
        setAiBaseUrl(c.base_url);
        setAiModel(c.model);
        setAiHasKey(!!c.has_api_key);
      })
      .catch(() => {});
  }, []);

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

  async function onSaveAI() {
    setError(""); setAiMessage("");
    try {
      const saved = await putAIConfig({
        enabled: aiEnabled,
        provider: "openai",
        base_url: aiBaseUrl,
        api_key: aiApiKey,
        model: aiModel,
      });
      setAiHasKey(!!saved.has_api_key);
      setAiApiKey("");
      setAiMessage(t("aiSaved"));
    } catch (e) {
      setError(e instanceof Error ? e.message : t("aiFailed"));
    }
  }

  return (
    <div className="space-y-4">
      <PageHeader title={t("title")} description={t("desc")} />
      <Card className="max-w-xl">
        <CardHeader>
          <CardTitle className="text-base">{t("backup")}</CardTitle>
          <CardDescription>{t("hint")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Button onClick={onExport} disabled={busy}>
              <Download />
              {t("export")}
            </Button>
            <Button variant="outline" disabled={busy} onClick={() => fileRef.current?.click()}>
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
          {error && <p className="text-sm text-red-600">{error}</p>}
          {message && <p className="text-sm text-green-600">{message}</p>}
        </CardContent>
      </Card>
      <Card className="max-w-xl">
        <CardHeader>
          <CardTitle className="text-base">{t("ai")}</CardTitle>
          <CardDescription>{t("aiHint")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <Label htmlFor="ai-enabled">{t("aiEnabled")}</Label>
            <Switch
              id="ai-enabled"
              checked={aiEnabled}
              onCheckedChange={setAiEnabled}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="ai-base-url">{t("aiBaseUrl")}</Label>
            <Input
              id="ai-base-url"
              value={aiBaseUrl}
              onChange={(e) => setAiBaseUrl(e.target.value)}
              placeholder={t("aiBaseUrlPlaceholder")}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="ai-api-key">
              {t("aiApiKey")}
              {aiHasKey && <span className="ml-2 text-xs text-muted-foreground">({t("aiApiKeySet")})</span>}
            </Label>
            <Input
              id="ai-api-key"
              type="password"
              value={aiApiKey}
              onChange={(e) => setAiApiKey(e.target.value)}
              placeholder={t("aiApiKeyPlaceholder")}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="ai-model">{t("aiModel")}</Label>
            <Input
              id="ai-model"
              value={aiModel}
              onChange={(e) => setAiModel(e.target.value)}
              placeholder={t("aiModelPlaceholder")}
            />
          </div>
          <div className="flex items-center gap-2">
            <Button onClick={onSaveAI} disabled={busy}>{t("aiSave")}</Button>
            {aiMessage && <p className="text-sm text-green-600">{aiMessage}</p>}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
