"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { api, adminOrgFooters, adminOrgFooterSave, type OrgFooter } from "@/lib/api";

export default function OrgFooterPage() {
  const t = useTranslations("orgFooter");
  const ct = useTranslations("common");
  const [domains, setDomains] = useState<string[]>([]);
  const [rows, setRows] = useState<OrgFooter[]>([]);
  const [domain, setDomain] = useState("");
  const [body, setBody] = useState("");
  const [enabled, setEnabled] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [page, footers] = await Promise.all([
        api<{ data: Array<{ name: string }> }>("/domains?page=1&limit=500"),
        adminOrgFooters(),
      ]);
      const names = (page.data || []).map((d) => d.name);
      setDomains(names);
      setRows(footers);
      setDomain((cur) => cur || footers[0]?.domain || names[0] || "");
    } catch (e) {
      setError(e instanceof Error ? e.message : t("loadFailed"));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    const row = rows.find((r) => r.domain === domain);
    setBody(row?.body_html || "");
    setEnabled(!!row?.enabled);
  }, [domain, rows]);

  async function save() {
    if (!domain) return;
    setSaving(true);
    setError("");
    setNotice("");
    try {
      const saved = await adminOrgFooterSave({ domain, body_html: body, enabled });
      setRows((prev) => {
        const next = prev.filter((r) => r.domain !== saved.domain);
        return [...next, saved];
      });
      setEnabled(saved.enabled);
      setNotice(saved.enabled ? t("savedEnabled") : t("savedDisabled"));
    } catch (e) {
      setError(e instanceof Error ? e.message : t("saveFailed"));
    } finally {
      setSaving(false);
    }
  }

  const configured = rows.filter((r) => r.body_text || r.body_html);

  return (
    <div className="space-y-6">
      <PageHeader title={t("title")} description={t("description")} />

      <Card>
        <CardHeader>
          <CardTitle>{t("editor")}</CardTitle>
          <CardDescription>{t("editorHint")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {loading ? (
            <p className="text-sm text-muted-foreground">{ct("loading")}</p>
          ) : domains.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("noDomains")}</p>
          ) : (
            <>
              <div className="space-y-2">
                <Label htmlFor="org-footer-domain">{t("domain")}</Label>
                <select
                  id="org-footer-domain"
                  value={domain}
                  onChange={(e) => setDomain(e.target.value)}
                  className="h-9 w-full max-w-sm rounded-md border border-input bg-transparent px-2 text-sm"
                >
                  {domains.map((name) => (
                    <option key={name} value={name}>
                      {name}
                      {rows.some((r) => r.domain === name && r.enabled) ? ` — ${t("enabled")}` : ""}
                    </option>
                  ))}
                </select>
              </div>

              <div className="space-y-2">
                <Label htmlFor="org-footer-body">{t("body")}</Label>
                <textarea
                  id="org-footer-body"
                  value={body}
                  onChange={(e) => setBody(e.target.value)}
                  rows={6}
                  placeholder={t("bodyPlaceholder")}
                  className="w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs"
                />
                <p className="text-xs text-muted-foreground">{t("bodyHint")}</p>
              </div>

              <div className="flex items-center justify-between rounded-md border border-border px-3 py-2">
                <div>
                  <Label>{t("enabled")}</Label>
                  <p className="text-xs text-muted-foreground">{t("enabledHint")}</p>
                </div>
                <Switch checked={enabled} onCheckedChange={setEnabled} />
              </div>

              <div className="flex items-center gap-3">
                <Button type="button" onClick={save} disabled={saving}>
                  {saving ? ct("saving") : ct("save")}
                </Button>
                {notice && <span className="text-xs text-muted-foreground">{notice}</span>}
                {error && <span className="text-xs text-destructive">{error}</span>}
              </div>
            </>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("configured")}</CardTitle>
          <CardDescription>{t("configuredHint")}</CardDescription>
        </CardHeader>
        <CardContent>
          {configured.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("none")}</p>
          ) : (
            <ul className="space-y-2 text-sm">
              {configured.map((row) => (
                <li key={row.domain} className="flex items-center justify-between border-b border-border pb-2">
                  <span className="font-medium">{row.domain}</span>
                  <span className={row.enabled ? "text-primary" : "text-muted-foreground"}>
                    {row.enabled ? t("enabled") : t("disabled")}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
