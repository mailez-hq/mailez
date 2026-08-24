"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Megaphone } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { announcement, clearAnnouncement, saveAnnouncement } from "@/lib/api";

// AnnouncementPage manages the single global notice shown to every webmail
// user (Mailu parity). Reads are open to any signed-in user; this page is
// reachable by global admins only via the sidebar.
export default function AnnouncementPage() {
  const t = useTranslations("announcement");
  const ct = useTranslations("common");
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      const a = await announcement();
      if (a) {
        setSubject(a.subject);
        setBody(a.body);
        setEnabled(a.enabled);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoaded(true);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError("");
    try {
      await saveAnnouncement(subject, body, enabled);
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch (err) {
      setError(err instanceof Error ? err.message : "save failed");
    } finally {
      setSaving(false);
    }
  }

  async function clear() {
    if (!confirm(t("clearConfirm"))) return;
    try {
      await clearAnnouncement();
      setSubject("");
      setBody("");
      setEnabled(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "clear failed");
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader title={t("title")} description={t("desc")} />
      <Card className="max-w-2xl">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Megaphone className="size-4" />
            {t("formTitle")}
          </CardTitle>
          <CardDescription>{t("formDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={save} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="subject">{t("subject")}</Label>
              <Input
                id="subject"
                value={subject}
                onChange={(e) => setSubject(e.target.value)}
                placeholder={t("subjectPlaceholder")}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="body">{t("body")}</Label>
              <textarea
                id="body"
                value={body}
                onChange={(e) => setBody(e.target.value)}
                placeholder={t("bodyPlaceholder")}
                rows={4}
                className="flex min-h-24 w-full rounded-lg border border-input bg-transparent px-2.5 py-2 text-sm transition-colors outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
              />
            </div>
            <label className="flex w-fit cursor-pointer items-center gap-2 text-sm">
              <Switch checked={enabled} onCheckedChange={setEnabled} />
              {t("enabled")}
            </label>
            {error && <p className="text-sm text-destructive">{error}</p>}
            {saved && <p className="text-sm text-primary">{t("saved")}</p>}
            <div className="flex gap-2">
              <Button type="submit" disabled={saving}>
                {saving ? t("saving") : ct("save")}
              </Button>
              <Button type="button" variant="outline" onClick={clear}>
                {t("clear")}
              </Button>
            </div>
          </form>
          {!loaded && <p className="mt-4 text-sm text-muted-foreground">{ct("loading")}</p>}
        </CardContent>
      </Card>
    </div>
  );
}
