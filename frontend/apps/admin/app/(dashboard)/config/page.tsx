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
  exportConfig, getAIConfig, getLDAPConfig, importConfig, putAIConfig, putLDAPConfig, syncLDAP, testLDAP,
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
  const [ldap, setLdap] = useState({
    enabled: false, host: "", port: 389, security: "none", base_dn: "", bind_dn: "",
    bind_password: "", user_filter: "(objectClass=person)", mail_attr: "mail", uid_attr: "uid",
    upn_attr: "userPrincipalName", email_domain: "",
    name_attr: "displayName", dept_attr: "department", title_attr: "title", phone_attr: "telephoneNumber",
    auto_create: true, sync_groups: false, group_filter: "(objectClass=groupOfNames)",
    group_name_attr: "cn", group_mail_attr: "mail", group_member_attr: "member",
    sync_minutes: 60, has_bind_pw: false,
  });
  const [ldapMessage, setLdapMessage] = useState("");
  const [ldapBusy, setLdapBusy] = useState(false);

  useEffect(() => {
    getAIConfig()
      .then((c) => {
        setAiEnabled(c.enabled);
        setAiBaseUrl(c.base_url);
        setAiModel(c.model);
        setAiHasKey(!!c.has_api_key);
      })
      .catch(() => {});
    getLDAPConfig()
      .then((c) =>
        setLdap({
          enabled: c.enabled, host: c.host, port: c.port || 389, security: c.security || "none",
          base_dn: c.base_dn, bind_dn: c.bind_dn, bind_password: "",
          user_filter: c.user_filter || "(objectClass=person)", mail_attr: c.mail_attr || "mail",
          uid_attr: c.uid_attr || "uid", upn_attr: c.upn_attr || "userPrincipalName",
          email_domain: c.email_domain || "",
          name_attr: c.name_attr || "displayName",
          dept_attr: c.dept_attr || "department", title_attr: c.title_attr || "title",
          phone_attr: c.phone_attr || "telephoneNumber", auto_create: c.auto_create,
          sync_groups: c.sync_groups, group_filter: c.group_filter || "(objectClass=groupOfNames)",
          group_name_attr: c.group_name_attr || "cn", group_mail_attr: c.group_mail_attr || "mail",
          group_member_attr: c.group_member_attr || "member",
          sync_minutes: c.sync_minutes || 60, has_bind_pw: !!c.has_bind_pw,
        }),
      )
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

  async function onSaveLDAP() {
    setError(""); setLdapMessage("");
    setLdapBusy(true);
    try {
      await putLDAPConfig({
        enabled: ldap.enabled, host: ldap.host, port: ldap.port, security: ldap.security,
        base_dn: ldap.base_dn, bind_dn: ldap.bind_dn, bind_password: ldap.bind_password,
        user_filter: ldap.user_filter, mail_attr: ldap.mail_attr, uid_attr: ldap.uid_attr,
        upn_attr: ldap.upn_attr, email_domain: ldap.email_domain,
        name_attr: ldap.name_attr, dept_attr: ldap.dept_attr, title_attr: ldap.title_attr,
        phone_attr: ldap.phone_attr, auto_create: ldap.auto_create,
        sync_groups: ldap.sync_groups, group_filter: ldap.group_filter,
        group_name_attr: ldap.group_name_attr, group_mail_attr: ldap.group_mail_attr,
        group_member_attr: ldap.group_member_attr,
        sync_minutes: ldap.sync_minutes,
      });
      setLdap((l) => ({ ...l, bind_password: "", has_bind_pw: true }));
      setLdapMessage(t("ldapSaved"));
    } catch (e) {
      setError(e instanceof Error ? e.message : t("ldapFailed"));
    } finally {
      setLdapBusy(false);
    }
  }

  async function onTestLDAP() {
    setError(""); setLdapMessage("");
    setLdapBusy(true);
    try {
      await testLDAP({
        host: ldap.host, port: ldap.port, security: ldap.security, base_dn: ldap.base_dn,
        bind_dn: ldap.bind_dn, bind_password: ldap.bind_password, user_filter: ldap.user_filter,
      });
      setLdapMessage(t("ldapTestOk"));
    } catch (e) {
      setError(e instanceof Error ? e.message : t("ldapTestFail"));
    } finally {
      setLdapBusy(false);
    }
  }

  async function onSyncLDAP() {
    setError(""); setLdapMessage("");
    setLdapBusy(true);
    try {
      const r = await syncLDAP();
      setLdapMessage(t("ldapSynced", { added: r.added, updated: r.updated }));
    } catch (e) {
      setError(e instanceof Error ? e.message : t("ldapSyncFail"));
    } finally {
      setLdapBusy(false);
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
      <Card className="max-w-xl">
        <CardHeader>
          <CardTitle className="text-base">{t("ldap")}</CardTitle>
          <CardDescription>{t("ldapHint")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <Label htmlFor="ldap-enabled">{t("ldapEnabled")}</Label>
            <Switch id="ldap-enabled" checked={ldap.enabled} onCheckedChange={(v) => setLdap((l) => ({ ...l, enabled: v }))} />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="ldap-host">{t("ldapHost")}</Label>
              <Input id="ldap-host" value={ldap.host} onChange={(e) => setLdap((l) => ({ ...l, host: e.target.value }))} placeholder="ldap.example.com" />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="ldap-port">{t("ldapPort")}</Label>
              <Input id="ldap-port" type="number" value={String(ldap.port)} onChange={(e) => setLdap((l) => ({ ...l, port: Number(e.target.value) }))} />
            </div>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="ldap-security">{t("ldapSecurity")}</Label>
            <select
              id="ldap-security"
              value={ldap.security}
              onChange={(e) => setLdap((l) => ({ ...l, security: e.target.value }))}
              className="rounded-md border border-input bg-background px-2 py-1.5 text-sm"
            >
              <option value="none">None</option>
              <option value="starttls">STARTTLS</option>
              <option value="tls">LDAPS</option>
            </select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="ldap-base-dn">{t("ldapBaseDn")}</Label>
            <Input id="ldap-base-dn" value={ldap.base_dn} onChange={(e) => setLdap((l) => ({ ...l, base_dn: e.target.value }))} placeholder="dc=example,dc=com" />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="ldap-bind-dn">{t("ldapBindDn")}</Label>
              <Input id="ldap-bind-dn" value={ldap.bind_dn} onChange={(e) => setLdap((l) => ({ ...l, bind_dn: e.target.value }))} placeholder="cn=admin,dc=example,dc=com" />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="ldap-bind-pw">
                {t("ldapBindPw")}
                {ldap.has_bind_pw && !ldap.bind_password && (
                  <span className="ml-2 text-xs text-muted-foreground">({t("ldapPwSet")})</span>
                )}
              </Label>
              <Input id="ldap-bind-pw" type="password" value={ldap.bind_password} onChange={(e) => setLdap((l) => ({ ...l, bind_password: e.target.value }))} placeholder={t("ldapPwPlaceholder")} />
            </div>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="ldap-filter">{t("ldapFilter")}</Label>
            <Input id="ldap-filter" value={ldap.user_filter} onChange={(e) => setLdap((l) => ({ ...l, user_filter: e.target.value }))} />
          </div>
          <div className="grid grid-cols-3 gap-3">
            {([
              ["mail_attr", t("ldapMailAttr")],
              ["uid_attr", t("ldapUidAttr")],
              ["name_attr", t("ldapNameAttr")],
            ] as const).map(([key, label]) => (
              <div key={key} className="grid gap-1.5">
                <Label htmlFor={`ldap-${key}`}>{label}</Label>
                <Input id={`ldap-${key}`} value={ldap[key]} onChange={(e) => setLdap((l) => ({ ...l, [key]: e.target.value }))} />
              </div>
            ))}
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="ldap-upn-attr">{t("ldapUpnAttr")}</Label>
              <Input id="ldap-upn-attr" value={ldap.upn_attr} onChange={(e) => setLdap((l) => ({ ...l, upn_attr: e.target.value }))} />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="ldap-email-domain">{t("ldapEmailDomain")}</Label>
              <Input id="ldap-email-domain" value={ldap.email_domain} onChange={(e) => setLdap((l) => ({ ...l, email_domain: e.target.value }))} placeholder="example.com" />
            </div>
          </div>
          <div className="grid grid-cols-3 gap-3">
            {([
              ["dept_attr", t("ldapDeptAttr")],
              ["title_attr", t("ldapTitleAttr")],
              ["phone_attr", t("ldapPhoneAttr")],
            ] as const).map(([key, label]) => (
              <div key={key} className="grid gap-1.5">
                <Label htmlFor={`ldap-${key}`}>{label}</Label>
                <Input id={`ldap-${key}`} value={ldap[key]} onChange={(e) => setLdap((l) => ({ ...l, [key]: e.target.value }))} />
              </div>
            ))}
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="flex items-center justify-between">
              <Label htmlFor="ldap-auto">{t("ldapAutoCreate")}</Label>
              <Switch id="ldap-auto" checked={ldap.auto_create} onCheckedChange={(v) => setLdap((l) => ({ ...l, auto_create: v }))} />
            </div>
            <div className="flex items-center justify-between">
              <Label htmlFor="ldap-groups">{t("ldapSyncGroups")}</Label>
              <Switch id="ldap-groups" checked={ldap.sync_groups} onCheckedChange={(v) => setLdap((l) => ({ ...l, sync_groups: v }))} />
            </div>
          </div>
          {ldap.sync_groups && (
            <div className="space-y-3">
              <div className="grid gap-1.5">
                <Label htmlFor="ldap-group-filter">{t("ldapGroupFilter")}</Label>
                <Input id="ldap-group-filter" value={ldap.group_filter} onChange={(e) => setLdap((l) => ({ ...l, group_filter: e.target.value }))} />
              </div>
              <div className="grid grid-cols-3 gap-3">
                {([
                  ["group_name_attr", t("ldapGroupNameAttr")],
                  ["group_mail_attr", t("ldapGroupMailAttr")],
                  ["group_member_attr", t("ldapGroupMemberAttr")],
                ] as const).map(([key, label]) => (
                  <div key={key} className="grid gap-1.5">
                    <Label htmlFor={`ldap-${key}`}>{label}</Label>
                    <Input id={`ldap-${key}`} value={ldap[key]} onChange={(e) => setLdap((l) => ({ ...l, [key]: e.target.value }))} />
                  </div>
                ))}
              </div>
            </div>
          )}
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="ldap-sync-min">{t("ldapSyncMinutes")}</Label>
              <Input id="ldap-sync-min" type="number" value={String(ldap.sync_minutes)} onChange={(e) => setLdap((l) => ({ ...l, sync_minutes: Number(e.target.value) }))} />
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button onClick={onSaveLDAP} disabled={ldapBusy}>{t("ldapSave")}</Button>
            <Button variant="outline" onClick={onTestLDAP} disabled={ldapBusy}>{t("ldapTest")}</Button>
            <Button variant="outline" onClick={onSyncLDAP} disabled={ldapBusy}>{t("ldapSync")}</Button>
            {ldapMessage && <p className="text-sm text-green-600">{ldapMessage}</p>}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
