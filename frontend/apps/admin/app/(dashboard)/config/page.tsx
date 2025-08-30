"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Download, Upload } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from "@/components/ui/card";
import { Drawer, DrawerContent, DrawerDescription, DrawerFooter, DrawerHeader, DrawerTitle } from "@/components/ui/drawer";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { cn } from "@/lib/utils";
import {
  IS_COMMUNITY_BUILD,
  exportConfig, getAIConfigs, getBranding, getLDAPConfig, importConfig,
  createAIConfig, deleteAIConfig, testAIConfig, updateAIConfig,
  putBranding, putLDAPConfig, syncLDAP, testLDAP,
  type AiConfigView,
  type BrandingConfigView, type ConfigBackup, type ConfigStats,
} from "@/lib/api";

type ConfigTab = "backup" | "branding" | "ai" | "ldap";

export default function ConfigPage() {
  const t = useTranslations("config");
  const fileRef = useRef<HTMLInputElement>(null);
  const [tab, setTab] = useState<ConfigTab>("backup");

  // Each tab owns its busy/error/message state so AI or LDAP errors never
  // leak into the backup panel (or vice versa).
  const [backupBusy, setBackupBusy] = useState(false);
  const [backupError, setBackupError] = useState("");
  const [backupMessage, setBackupMessage] = useState("");
  const [branding, setBranding] = useState({
    title: "", subtitle: "", tagline: "", feature1: "", feature2: "", feature3: "",
    logo_url: "", hero_url: "", copyright: "", contact: "",
  });
  const [brandingBusy, setBrandingBusy] = useState(false);
  const [brandingError, setBrandingError] = useState("");
  const [brandingMessage, setBrandingMessage] = useState("");
  const [aiProviders, setAiProviders] = useState<AiConfigView[]>([]);
  const [aiDrawer, setAiDrawer] = useState<AiConfigView | null>(null);
  const [aiBusy, setAiBusy] = useState(false);
  const [aiError, setAiError] = useState("");
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
  const [ldapBusy, setLdapBusy] = useState(false);
  const [ldapError, setLdapError] = useState("");
  const [ldapMessage, setLdapMessage] = useState("");

  useEffect(() => {
    getBranding()
      .then((b) => setBranding({
        title: b.title || "", subtitle: b.subtitle || "", tagline: b.tagline || "",
        feature1: b.feature1 || "", feature2: b.feature2 || "", feature3: b.feature3 || "",
        logo_url: b.logo_url || "", hero_url: b.hero_url || "", copyright: b.copyright || "",
        contact: b.contact || "",
      }))
      .catch(() => {});
    getAIConfigs()
      .then((list) => setAiProviders(list))
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

  async function onSaveBranding() {
    setBrandingError(""); setBrandingMessage("");
    setBrandingBusy(true);
    try {
      const saved: BrandingConfigView = await putBranding(branding);
      setBranding({
        title: saved.title || "", subtitle: saved.subtitle || "", tagline: saved.tagline || "",
        feature1: saved.feature1 || "", feature2: saved.feature2 || "", feature3: saved.feature3 || "",
        logo_url: saved.logo_url || "", hero_url: saved.hero_url || "", copyright: saved.copyright || "",
        contact: saved.contact || "",
      });
      setBrandingMessage(t("brandingSaved"));
    } catch (e) {
      setBrandingError(e instanceof Error ? e.message : t("brandingFailed"));
    } finally {
      setBrandingBusy(false);
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

  function replaceProvider(next: AiConfigView) {
    setAiProviders((list) => list.map((p) => (p.id === next.id ? next : p)));
  }

  function emptyAIDraft(): AiConfigView {
    return {
      id: 0, name: "", enabled: false, is_default: false, provider: "openai",
      base_url: "", model: "", has_api_key: false, api_key: "", last_test_ok: false,
    };
  }

  function openAIDrawer(p: AiConfigView | null) {
    setAiError(""); setAiMessage("");
    setAiDrawer(p ? { ...p, api_key: "" } : emptyAIDraft());
  }

  function patchAIDrawer(patch: Partial<AiConfigView>) {
    setAiDrawer((d) => (d ? { ...d, ...patch } : d));
  }

  // saveAIDrawer persists the drawer form (create or update) and refreshes
  // both the list and the drawer copy; it returns the persisted row.
  async function saveAIDrawer(): Promise<AiConfigView | null> {
    const d = aiDrawer;
    if (!d) return null;
    const isNew = d.id === 0;
    const saved = isNew
      ? await createAIConfig({
          name: d.name,
          base_url: d.base_url,
          api_key: d.api_key,
          model: d.model,
        })
      : await updateAIConfig(d.id, {
          name: d.name,
          base_url: d.base_url,
          api_key: d.api_key,
          model: d.model,
        });
    const withKey = { ...saved, api_key: "" };
    setAiDrawer(withKey);
    if (isNew) {
      setAiProviders((list) => [...list, withKey]);
    } else {
      replaceProvider(withKey);
    }
    return saved;
  }

  async function onSaveAIDrawer() {
    setAiError(""); setAiMessage("");
    setAiBusy(true);
    try {
      await saveAIDrawer();
      setAiMessage(t("aiSaved"));
    } catch (e) {
      setAiError(e instanceof Error ? e.message : t("aiFailed"));
    } finally {
      setAiBusy(false);
    }
  }

  async function onTestAIDrawer() {
    setAiError(""); setAiMessage("");
    setAiBusy(true);
    try {
      // Persist the current form first so the test runs against these
      // settings (a changed endpoint/key disables the provider until the
      // test passes, which is exactly what we want).
      const saved = await saveAIDrawer();
      if (!saved) return;
      const tested = await testAIConfig(saved.id);
      const withKey = { ...tested, api_key: "" };
      setAiDrawer(withKey);
      replaceProvider(withKey);
      if (tested.last_test_ok) {
        setAiMessage(t("aiTestOk"));
      } else {
        setAiError(t("aiTestFail") + (tested.last_test_error ? `：${tested.last_test_error}` : ""));
      }
    } catch (e) {
      setAiError(e instanceof Error ? e.message : t("aiFailed"));
    } finally {
      setAiBusy(false);
    }
  }

  async function onTestAIRow(p: AiConfigView) {
    setAiError(""); setAiMessage("");
    setAiBusy(true);
    try {
      const tested = await testAIConfig(p.id);
      replaceProvider(tested);
      if (tested.last_test_ok) {
        setAiMessage(t("aiTestOk"));
      } else {
        setAiError(t("aiTestFail") + (tested.last_test_error ? `：${tested.last_test_error}` : ""));
      }
    } catch (e) {
      setAiError(e instanceof Error ? e.message : t("aiFailed"));
    } finally {
      setAiBusy(false);
    }
  }

  async function onToggleAIEnabled(p: AiConfigView, enabled: boolean) {
    setAiError(""); setAiMessage("");
    setAiBusy(true);
    try {
      const saved = await updateAIConfig(p.id, { enabled });
      replaceProvider(saved);
      if (enabled) setAiMessage(t("aiEnabledSaved"));
    } catch (e) {
      setAiError(e instanceof Error ? e.message : t("aiFailed"));
    } finally {
      setAiBusy(false);
    }
  }

  async function onSetAIDefault(p: AiConfigView) {
    setAiError(""); setAiMessage("");
    setAiBusy(true);
    try {
      await updateAIConfig(p.id, { is_default: true });
      // Refresh the whole list: the other providers' default flags changed.
      setAiProviders(await getAIConfigs());
      setAiMessage(t("aiDefaultSaved"));
    } catch (e) {
      setAiError(e instanceof Error ? e.message : t("aiFailed"));
    } finally {
      setAiBusy(false);
    }
  }

  async function onDeleteAI(p: AiConfigView) {
    if (!confirm(t("aiDeleteConfirm", { name: p.name }))) return;
    setAiError(""); setAiMessage("");
    setAiBusy(true);
    try {
      await deleteAIConfig(p.id);
      setAiProviders((list) => list.filter((x) => x.id !== p.id));
      setAiMessage(t("aiDeleted"));
    } catch (e) {
      setAiError(e instanceof Error ? e.message : t("aiFailed"));
    } finally {
      setAiBusy(false);
    }
  }

  async function onSaveLDAP() {
    setLdapError(""); setLdapMessage("");
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
      setLdapError(e instanceof Error ? e.message : t("ldapFailed"));
    } finally {
      setLdapBusy(false);
    }
  }

  async function onTestLDAP() {
    setLdapError(""); setLdapMessage("");
    setLdapBusy(true);
    try {
      await testLDAP({
        host: ldap.host, port: ldap.port, security: ldap.security, base_dn: ldap.base_dn,
        bind_dn: ldap.bind_dn, bind_password: ldap.bind_password, user_filter: ldap.user_filter,
      });
      setLdapMessage(t("ldapTestOk"));
    } catch (e) {
      setLdapError(e instanceof Error ? e.message : t("ldapTestFail"));
    } finally {
      setLdapBusy(false);
    }
  }

  async function onSyncLDAP() {
    setLdapError(""); setLdapMessage("");
    setLdapBusy(true);
    try {
      const r = await syncLDAP();
      setLdapMessage(t("ldapSynced", { added: r.added, updated: r.updated }));
    } catch (e) {
      setLdapError(e instanceof Error ? e.message : t("ldapSyncFail"));
    } finally {
      setLdapBusy(false);
    }
  }

  // AI configuration and AD/LDAP directory integration are enterprise
  // surfaces; the community build neither fetches them (the endpoints do
  // not exist on a CE backend) nor shows the tabs.
  const tabs: { key: ConfigTab; label: string }[] = [
    { key: "backup", label: t("backup") },
    { key: "branding", label: t("branding") },
    ...(IS_COMMUNITY_BUILD
      ? []
      : [
          { key: "ai" as ConfigTab, label: t("ai") },
          { key: "ldap" as ConfigTab, label: t("ldap") },
        ]),
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

      {tab === "branding" && (
        <Card className="max-w-xl">
          <CardHeader>
            <CardTitle className="text-base">{t("branding")}</CardTitle>
            <CardDescription>{t("brandingHint")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-1.5">
                <Label htmlFor="branding-title">{t("brandingTitle")}</Label>
                <Input
                  id="branding-title"
                  value={branding.title}
                  onChange={(e) => setBranding((b) => ({ ...b, title: e.target.value }))}
                  placeholder="Mailez"
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="branding-subtitle">{t("brandingSubtitle")}</Label>
                <Input
                  id="branding-subtitle"
                  value={branding.subtitle}
                  onChange={(e) => setBranding((b) => ({ ...b, subtitle: e.target.value }))}
                  placeholder={t("brandingSubtitlePlaceholder")}
                />
              </div>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="branding-tagline">{t("brandingTagline")}</Label>
              <Input
                id="branding-tagline"
                value={branding.tagline}
                onChange={(e) => setBranding((b) => ({ ...b, tagline: e.target.value }))}
                placeholder={t("brandingTaglinePlaceholder")}
              />
            </div>
            <div className="grid grid-cols-3 gap-3">
              {([
                ["feature1", t("brandingFeature1")],
                ["feature2", t("brandingFeature2")],
                ["feature3", t("brandingFeature3")],
              ] as const).map(([key, label]) => (
                <div key={key} className="grid gap-1.5">
                  <Label htmlFor={`branding-${key}`}>{label}</Label>
                  <Input
                    id={`branding-${key}`}
                    value={branding[key]}
                    onChange={(e) => setBranding((b) => ({ ...b, [key]: e.target.value }))}
                  />
                </div>
              ))}
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-1.5">
                <Label htmlFor="branding-logo">{t("brandingLogoUrl")}</Label>
                <Input
                  id="branding-logo"
                  value={branding.logo_url}
                  onChange={(e) => setBranding((b) => ({ ...b, logo_url: e.target.value }))}
                  placeholder="https://example.com/logo.png"
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="branding-hero">{t("brandingHeroUrl")}</Label>
                <Input
                  id="branding-hero"
                  value={branding.hero_url}
                  onChange={(e) => setBranding((b) => ({ ...b, hero_url: e.target.value }))}
                  placeholder="https://example.com/banner.jpg"
                />
              </div>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="branding-copyright">{t("brandingCopyright")}</Label>
              <Input
                id="branding-copyright"
                value={branding.copyright}
                onChange={(e) => setBranding((b) => ({ ...b, copyright: e.target.value }))}
                placeholder="Copyright © example.com, All Rights Reserved"
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="branding-contact">{t("brandingContact")}</Label>
              <Input
                id="branding-contact"
                value={branding.contact}
                onChange={(e) => setBranding((b) => ({ ...b, contact: e.target.value }))}
                placeholder={t("brandingContactPlaceholder")}
              />
            </div>
            <p className="text-xs text-muted-foreground">{t("brandingPreviewHint")}</p>
            <div className="flex items-center gap-2">
              <Button onClick={onSaveBranding} disabled={brandingBusy}>{t("brandingSave")}</Button>
              {brandingError && <p className="text-sm text-red-600">{brandingError}</p>}
              {brandingMessage && <p className="text-sm text-green-600">{brandingMessage}</p>}
            </div>
          </CardContent>
        </Card>
      )}

      {tab === "ai" && (
        <div className="space-y-4">
          <div className="flex items-center justify-between gap-3">
            <div>
              <h3 className="text-base font-medium">{t("ai")}</h3>
              <p className="text-sm text-muted-foreground">{t("aiHint")}</p>
            </div>
            <Button onClick={() => openAIDrawer(null)} disabled={aiBusy}>
              {t("aiAdd")}
            </Button>
          </div>
          {(aiError || aiMessage) && (
            <div className="text-sm">
              {aiError && <p className="text-red-600">{aiError}</p>}
              {aiMessage && <p className="text-green-600">{aiMessage}</p>}
            </div>
          )}
          <Card>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("aiName")}</TableHead>
                  <TableHead>{t("aiBaseUrl")}</TableHead>
                  <TableHead>{t("aiModel")}</TableHead>
                  <TableHead>{t("aiStatus")}</TableHead>
                  <TableHead>{t("aiLastTest")}</TableHead>
                  <TableHead className="text-right">{t("aiActions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {aiProviders.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={6} className="py-6 text-center text-muted-foreground">
                      {t("aiEmpty")}
                    </TableCell>
                  </TableRow>
                )}
                {aiProviders.map((p) => (
                  <TableRow key={p.id}>
                    <TableCell>
                      <div className="flex items-center gap-2 font-medium">
                        {p.name}
                        {p.is_default && <Badge variant="secondary">{t("aiIsDefault")}</Badge>}
                      </div>
                      {!p.last_test_ok && (
                        <p className="text-xs text-amber-600">{t("aiEnabledRequiresTest")}</p>
                      )}
                    </TableCell>
                    <TableCell className="max-w-[200px] truncate text-muted-foreground">
                      {p.base_url || "https://api.openai.com/v1"}
                    </TableCell>
                    <TableCell>{p.model || "—"}</TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Switch
                          checked={p.enabled}
                          disabled={!p.last_test_ok || aiBusy}
                          onCheckedChange={(v) => onToggleAIEnabled(p, v)}
                        />
                        <span className="text-xs text-muted-foreground">
                          {p.enabled ? t("aiOn") : t("aiOff")}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>
                      {p.last_test_at ? (
                        <span className={p.last_test_ok ? "text-green-600" : "text-red-600"}>
                          {p.last_test_ok ? t("aiTestOkShort") : t("aiTestFailShort")}
                          <span className="text-muted-foreground">
                            {" · "}
                            {new Date(p.last_test_at).toLocaleString()}
                          </span>
                        </span>
                      ) : (
                        <span className="text-muted-foreground">{t("aiNeverTested")}</span>
                      )}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Button size="sm" variant="ghost" onClick={() => openAIDrawer(p)} disabled={aiBusy}>
                          {t("aiEdit")}
                        </Button>
                        <Button size="sm" variant="outline" onClick={() => onTestAIRow(p)} disabled={aiBusy}>
                          {t("aiTest")}
                        </Button>
                        {p.enabled && !p.is_default && (
                          <Button size="sm" variant="secondary" onClick={() => onSetAIDefault(p)} disabled={aiBusy}>
                            {t("aiSetDefault")}
                          </Button>
                        )}
                        <Button
                          size="sm"
                          variant="ghost"
                          className="text-red-600"
                          onClick={() => onDeleteAI(p)}
                          disabled={aiBusy}
                        >
                          {t("aiDelete")}
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Card>
        </div>
      )}

      <Drawer
        open={aiDrawer !== null}
        onOpenChange={(open) => {
          if (!open) setAiDrawer(null);
        }}
      >
        <DrawerContent>
          <DrawerHeader>
            <DrawerTitle>{aiDrawer && aiDrawer.id === 0 ? t("aiAdd") : t("aiEdit")}</DrawerTitle>
            <DrawerDescription>{t("aiDrawerHint")}</DrawerDescription>
          </DrawerHeader>
          {aiDrawer && (
            <div className="space-y-3">
              <div className="grid gap-1.5">
                <Label htmlFor="ai-drawer-name">{t("aiName")}</Label>
                <Input
                  id="ai-drawer-name"
                  value={aiDrawer.name}
                  placeholder={t("aiNamePlaceholder")}
                  onChange={(e) => patchAIDrawer({ name: e.target.value })}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="ai-drawer-url">{t("aiBaseUrl")}</Label>
                <Input
                  id="ai-drawer-url"
                  value={aiDrawer.base_url}
                  placeholder={t("aiBaseUrlPlaceholder")}
                  onChange={(e) => patchAIDrawer({ base_url: e.target.value })}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="ai-drawer-model">{t("aiModel")}</Label>
                <Input
                  id="ai-drawer-model"
                  value={aiDrawer.model}
                  placeholder={t("aiModelPlaceholder")}
                  onChange={(e) => patchAIDrawer({ model: e.target.value })}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="ai-drawer-key">
                  {t("aiApiKey")}
                  {aiDrawer.has_api_key && !aiDrawer.api_key && (
                    <span className="ml-2 text-xs text-muted-foreground">({t("aiApiKeySet")})</span>
                  )}
                </Label>
                <Input
                  id="ai-drawer-key"
                  type="password"
                  value={aiDrawer.api_key ?? ""}
                  placeholder={t("aiApiKeyPlaceholder")}
                  onChange={(e) => patchAIDrawer({ api_key: e.target.value })}
                />
              </div>
              {aiDrawer.id !== 0 && aiDrawer.last_test_at && (
                <p className="text-xs text-muted-foreground">
                  {t("aiLastTest")}:{" "}
                  {aiDrawer.last_test_ok ? t("aiTestOkShort") : t("aiTestFailShort")}
                  {" · "}
                  {new Date(aiDrawer.last_test_at).toLocaleString()}
                  {aiDrawer.last_test_error ? ` — ${aiDrawer.last_test_error}` : ""}
                </p>
              )}
            </div>
          )}
          <DrawerFooter>
            <Button variant="outline" onClick={() => setAiDrawer(null)} disabled={aiBusy}>
              {t("aiCancel")}
            </Button>
            <Button variant="outline" onClick={onTestAIDrawer} disabled={aiBusy}>
              {aiBusy ? t("aiTesting") : t("aiTest")}
            </Button>
            <Button onClick={onSaveAIDrawer} disabled={aiBusy}>
              {t("aiSave")}
            </Button>
          </DrawerFooter>
        </DrawerContent>
      </Drawer>

      {tab === "ldap" && (
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
              {ldapError && <p className="text-sm text-red-600">{ldapError}</p>}
              {ldapMessage && <p className="text-sm text-green-600">{ldapMessage}</p>}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
