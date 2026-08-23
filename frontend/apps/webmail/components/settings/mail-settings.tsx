"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import {
  AtSign, Bell, Filter, Forward, IdCard, KeyRound, Lock, MessageSquareReply,
  Palette, ShieldAlert, ShieldCheck, Sparkles, User, Webhook as WebhookIcon,
} from "lucide-react";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { usePreferences } from "@/components/preferences-provider";
import { SettingsSections } from "@/components/settings/sections";
import {
  changePassword, meProfile, updateMeSettings,
  accounts, accountCreate, accountDelete, accountTest, accountUpdate,
  pgpDeleteKey, pgpImportKey, pgpListKeys, pgpStatus, totpStatus,
  smimeDelete, smimeDeleteCert, smimeImport, smimeImportCert, smimeListCerts, smimeStatus,
  webhookCreate, webhookDelete, webhookList, webhookTest, webhookUpdate,
  type MailAccount, type PgpKey, type SmimeCert, type SmimeStatus, type TotpStatus,
  type PgpStatus, type MeSettings, type Webhook,
} from "@/lib/api";
import { cn } from "@/lib/utils";

// Sidebar navigation for the settings dialog, mirroring the FastMail layout:
// a section list on the left, the active section's form on the right.
const SETTINGS_SECTIONS = [
  { id: "appearance", icon: Palette, label: "appearance" },
  { id: "accounts", icon: AtSign, label: "accounts" },
  { id: "ai", icon: Sparkles, label: "aiFeatures" },
  { id: "notifications", icon: Bell, label: "notifications" },
  { id: "identity", icon: User, label: "identity" },
  { id: "forwarding", icon: Forward, label: "forwarding" },
  { id: "autoReply", icon: MessageSquareReply, label: "autoReply" },
  { id: "spam", icon: ShieldAlert, label: "spamFilter" },
  { id: "filters", icon: Filter, label: "filters" },
  { id: "pgp", icon: KeyRound, label: "pgp" },
  { id: "smime", icon: IdCard, label: "smime" },
  { id: "webhooks", icon: WebhookIcon, label: "webhooks" },
  { id: "twoFactor", icon: ShieldCheck, label: "twoFactor" },
  { id: "password", icon: Lock, label: "changePassword" },
] as const;

// backend serializes time.Time as RFC3339; date inputs need yyyy-mm-dd
const toDateInput = (s: string) => (s && !s.startsWith("0001") ? s.slice(0, 10) : "");

export function MailSettings({ open, onOpenChange, initialSection = "appearance", onSaved }: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  initialSection?: string;
  onSaved: () => void;
}) {
  const t = useTranslations("settings");
  const { theme, setTheme, density, setDensity, accent, setAccent, prefs, setAi, setNotifications } = usePreferences();
  const [profile, setProfile] = useState<MeSettings | null>(null);
  const [error, setError] = useState("");
  const [section, setSection] = useState<string>("appearance");

  // Land on the requested section each time the dialog opens (e.g. "accounts"
  // from the sidebar account manager).
  useEffect(() => {
    if (open) setSection(initialSection);
  }, [open, initialSection]);

  const [displayedName, setDisplayedName] = useState("");
  const [signature, setSignature] = useState("");
  const [whitelist, setWhitelist] = useState("");
  const [blacklist, setBlacklist] = useState("");
  const [forwardEnabled, setForwardEnabled] = useState(false);
  const [forwardDestination, setForwardDestination] = useState("");
  const [forwardKeep, setForwardKeep] = useState(true);
  const [replyEnabled, setReplyEnabled] = useState(false);
  const [replySubject, setReplySubject] = useState("");
  const [replyBody, setReplyBody] = useState("");
  const [replyStartdate, setReplyStartdate] = useState("");
  const [replyEnddate, setReplyEnddate] = useState("");
  const [spamEnabled, setSpamEnabled] = useState(true);
  const [spamMarkAsRead, setSpamMarkAsRead] = useState(true);
  const [spamThreshold, setSpamThreshold] = useState(80);

  // change password
  const [oldPw, setOldPw] = useState("");
  const [newPw, setNewPw] = useState("");
  const [confirmPw, setConfirmPw] = useState("");

  // PGP key management
  const [pgp, setPgp] = useState<PgpStatus | null>(null);
  const [generating, setGenerating] = useState(false);
  const [copied, setCopied] = useState(false);
  const [pgpKeys, setPgpKeys] = useState<PgpKey[] | null>(null);
  const [pgpImportEmail, setPgpImportEmail] = useState("");
  const [pgpImportKeyText, setPgpImportKeyText] = useState("");
  const [pgpImporting, setPgpImporting] = useState(false);
  const [pgpDeleting, setPgpDeleting] = useState<number | null>(null);

  // two-factor authentication
  const [totp, setTotp] = useState<TotpStatus | null>(null);
  const [totpCode, setTotpCode] = useState("");
  const [totpBusy, setTotpBusy] = useState(false);

  // webhook event callbacks
  const [webhooks, setWebhooks] = useState<Webhook[] | null>(null);
  const [whUrl, setWhUrl] = useState("");
  const [whSecret, setWhSecret] = useState("");
  const [whEvents, setWhEvents] = useState("mail.received");
  const [whEnabled, setWhEnabled] = useState(true);
  const [whSaving, setWhSaving] = useState(false);
  const [whTesting, setWhTesting] = useState<number | null>(null);
  const [whTestMsg, setWhTestMsg] = useState("");

  // S/MIME certificate management
  const [smime, setSmime] = useState<SmimeStatus | null>(null);
  const [smimeCerts, setSmimeCerts] = useState<SmimeCert[] | null>(null);
  const [smimeImportMode, setSmimeImportMode] = useState<"pem" | "p12">("pem");
  const [smimeCertPem, setSmimeCertPem] = useState("");
  const [smimeKeyPem, setSmimeKeyPem] = useState("");
  const [smimeP12B64, setSmimeP12B64] = useState("");
  const [smimeP12Password, setSmimeP12Password] = useState("");
  const [smimeImporting, setSmimeImporting] = useState(false);
  const [smimeKeyringEmail, setSmimeKeyringEmail] = useState("");
  const [smimeKeyringCert, setSmimeKeyringCert] = useState("");
  const [smimeKeyringImporting, setSmimeKeyringImporting] = useState(false);
  const [smimeKeyringDeleting, setSmimeKeyringDeleting] = useState<number | null>(null);

  // aggregated external accounts (full aggregation client)
  const [accountList, setAccountList] = useState<MailAccount[] | null>(null);
  const [accName, setAccName] = useState("");
  const [accEmail, setAccEmail] = useState("");
  const [accImapHost, setAccImapHost] = useState("");
  const [accImapPort, setAccImapPort] = useState(993);
  const [accImapSecurity, setAccImapSecurity] = useState("tls");
  const [accSmtpHost, setAccSmtpHost] = useState("");
  const [accSmtpPort, setAccSmtpPort] = useState(587);
  const [accSmtpSecurity, setAccSmtpSecurity] = useState("starttls");
  const [accUsername, setAccUsername] = useState("");
  const [accPassword, setAccPassword] = useState("");
  const [accSaving, setAccSaving] = useState(false);
  const [accTesting, setAccTesting] = useState<number | null>(null);
  const [accTestMsg, setAccTestMsg] = useState("");

  useEffect(() => {
    if (!open) return;
    setError("");
    totpStatus().then(setTotp).catch(() => setTotp(null));
    pgpStatus().then(setPgp).catch(() => setPgp(null));
    pgpListKeys().then(setPgpKeys).catch(() => setPgpKeys([]));
    smimeStatus().then(setSmime).catch(() => setSmime(null));
    smimeListCerts().then(setSmimeCerts).catch(() => setSmimeCerts([]));
    accounts().then(setAccountList).catch(() => setAccountList([]));
    webhookList().then(setWebhooks).catch(() => setWebhooks([]));
    meProfile().then((p) => {
      setProfile(p);
      setDisplayedName(p.displayed_name);
      setSignature(p.signature || "");
      setWhitelist(p.whitelist || "");
      setBlacklist(p.blacklist || "");
      setForwardEnabled(p.forward_enabled);
      setForwardDestination(p.forward_destination);
      setForwardKeep(p.forward_keep);
      setReplyEnabled(p.reply_enabled);
      setReplySubject(p.reply_subject);
      setReplyBody(p.reply_body);
      setReplyStartdate(toDateInput(p.reply_startdate));
      setReplyEnddate(toDateInput(p.reply_enddate));
      setSpamEnabled(p.spam_enabled);
      setSpamMarkAsRead(p.spam_mark_as_read);
      setSpamThreshold(p.spam_threshold);
    }).catch((e) => setError(e instanceof Error ? e.message : "load profile failed"));
  }, [open]);

  async function saveSettings(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await updateMeSettings({
        displayed_name: displayedName,
        signature,
        whitelist,
        blacklist,
        forward_enabled: forwardEnabled,
        forward_destination: forwardDestination,
        forward_keep: forwardKeep,
        reply_enabled: replyEnabled,
        reply_subject: replySubject,
        reply_body: replyBody,
        reply_startdate: replyStartdate,
        reply_enddate: replyEnddate,
        spam_enabled: spamEnabled,
        spam_mark_as_read: spamMarkAsRead,
        spam_threshold: spamThreshold,
      });
      onSaved();
      onOpenChange(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : "save failed");
    }
  }

  async function savePassword(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (newPw !== confirmPw) {
      setError(t("passwordsDoNotMatch"));
      return;
    }
    try {
      await changePassword(oldPw, newPw);
      setOldPw(""); setNewPw(""); setConfirmPw("");
      onSaved();
    } catch (e) {
      setError(e instanceof Error ? e.message : "password change failed");
    }
  }

  async function importPgpKey() {
    if (!pgpImportEmail.trim() || !pgpImportKeyText.trim()) return;
    setPgpImporting(true);
    setError("");
    try {
      const k = await pgpImportKey(pgpImportEmail.trim(), pgpImportKeyText.trim());
      setPgpKeys((ks) => [k, ...(ks || [])]);
      setPgpImportEmail("");
      setPgpImportKeyText("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "pgp import failed");
    } finally {
      setPgpImporting(false);
    }
  }

  async function deletePgpKey(id: number) {
    if (!window.confirm(t("pgpKeyDeleteConfirm"))) return;
    setPgpDeleting(id);
    setError("");
    try {
      await pgpDeleteKey(id);
      setPgpKeys((ks) => (ks || []).filter((k) => k.id !== id));
    } catch (e) {
      setError(e instanceof Error ? e.message : "pgp key delete failed");
    } finally {
      setPgpDeleting(null);
    }
  }

  async function addWebhook() {
    if (!whUrl.trim() || !whEvents.trim()) return;
    setWhSaving(true);
    setError("");
    setWhTestMsg("");
    try {
      const w = await webhookCreate({
        url: whUrl.trim(),
        secret: whSecret,
        events: whEvents.trim(),
        enabled: whEnabled,
      });
      setWebhooks((ws) => [w, ...(ws || [])]);
      setWhUrl("");
      setWhSecret("");
      setWhEvents("mail.received");
      setWhEnabled(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "webhook create failed");
    } finally {
      setWhSaving(false);
    }
  }

  async function deleteWebhook(id: number) {
    if (!window.confirm(t("webhookDeleteConfirm"))) return;
    setError("");
    setWhTestMsg("");
    try {
      await webhookDelete(id);
      setWebhooks((ws) => (ws || []).filter((w) => w.id !== id));
    } catch (e) {
      setError(e instanceof Error ? e.message : "webhook delete failed");
    }
  }

  async function toggleWebhook(w: Webhook) {
    setError("");
    try {
      const updated = await webhookUpdate(w.id, { enabled: !w.enabled });
      setWebhooks((ws) => (ws || []).map((x) => (x.id === updated.id ? updated : x)));
    } catch (e) {
      setError(e instanceof Error ? e.message : "webhook update failed");
    }
  }

  async function testWebhook(id: number) {
    setWhTesting(id);
    setError("");
    setWhTestMsg("");
    try {
      const r = await webhookTest(id);
      setWhTestMsg(r.ok
        ? `${t("webhookTestOk")} (${r.status})`
        : `${t("webhookTestFail")} (${r.status}${r.error ? `: ${r.error}` : ""})`);
    } catch (e) {
      setWhTestMsg(e instanceof Error ? e.message : "webhook test failed");
    } finally {
      setWhTesting(null);
    }
  }

  async function importSmime() {
    setSmimeImporting(true);
    setError("");
    try {
      const body = smimeImportMode === "pem"
        ? { cert_pem: smimeCertPem.trim(), private_key: smimeKeyPem.trim() }
        : { p12_b64: smimeP12B64.trim(), p12_password: smimeP12Password };
      const s = await smimeImport(body);
      setSmime(s);
      setSmimeCertPem("");
      setSmimeKeyPem("");
      setSmimeP12B64("");
      setSmimeP12Password("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "smime import failed");
    } finally {
      setSmimeImporting(false);
    }
  }

  async function deleteSmime() {
    if (!window.confirm(t("smimeDeleteConfirm"))) return;
    setError("");
    try {
      await smimeDelete();
      setSmime({ has_cert: false });
    } catch (e) {
      setError(e instanceof Error ? e.message : "smime delete failed");
    }
  }

  async function importSmimeCert() {
    if (!smimeKeyringEmail.trim() || !smimeKeyringCert.trim()) return;
    setSmimeKeyringImporting(true);
    setError("");
    try {
      const c = await smimeImportCert(smimeKeyringEmail.trim(), smimeKeyringCert.trim());
      setSmimeCerts((cs) => [c, ...(cs || [])]);
      setSmimeKeyringEmail("");
      setSmimeKeyringCert("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "smime cert import failed");
    } finally {
      setSmimeKeyringImporting(false);
    }
  }

  async function deleteSmimeCert(id: number) {
    if (!window.confirm(t("smimeCertDeleteConfirm"))) return;
    setSmimeKeyringDeleting(id);
    setError("");
    try {
      await smimeDeleteCert(id);
      setSmimeCerts((cs) => (cs || []).filter((c) => c.id !== id));
    } catch (e) {
      setError(e instanceof Error ? e.message : "smime cert delete failed");
    } finally {
      setSmimeKeyringDeleting(null);
    }
  }

  async function addAccount() {
    if (!accName.trim() || !accEmail.trim() || !accImapHost.trim()) return;
    setAccSaving(true);
    setError("");
    setAccTestMsg("");
    try {
      const a = await accountCreate({
        name: accName.trim(),
        email: accEmail.trim(),
        imap_host: accImapHost.trim(),
        imap_port: accImapPort,
        imap_security: accImapSecurity,
        smtp_host: accSmtpHost.trim() || undefined,
        smtp_port: accSmtpPort || undefined,
        smtp_security: accSmtpSecurity,
        username: accUsername.trim(),
        password: accPassword,
      });
      setAccountList((as) => [a, ...(as || [])]);
      setAccName("");
      setAccEmail("");
      setAccImapHost("");
      setAccSmtpHost("");
      setAccUsername("");
      setAccPassword("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "account create failed");
    } finally {
      setAccSaving(false);
    }
  }

  async function removeAccount(id: number) {
    if (!window.confirm(t("accountDeleteConfirm"))) return;
    setError("");
    try {
      await accountDelete(id);
      setAccountList((as) => (as || []).filter((a) => a.id !== id));
    } catch (e) {
      setError(e instanceof Error ? e.message : "account delete failed");
    }
  }

  async function toggleAccount(a: MailAccount) {
    setError("");
    try {
      const updated = await accountUpdate(a.id, { enabled: !a.enabled });
      setAccountList((as) => (as || []).map((x) => (x.id === updated.id ? updated : x)));
    } catch (e) {
      setError(e instanceof Error ? e.message : "account update failed");
    }
  }

  async function testAccount(id: number) {
    setAccTesting(id);
    setError("");
    setAccTestMsg("");
    try {
      const r = await accountTest(id);
      setAccTestMsg(r.ok ? t("accountTestOk") : t("accountTestFail"));
    } catch (e) {
      setAccTestMsg(e instanceof Error ? e.message : "test failed");
    } finally {
      setAccTesting(null);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] flex-col overflow-hidden p-0 sm:max-w-3xl">
        <DialogHeader className="shrink-0 border-b px-5 py-4">
          <DialogTitle>{t("title")}</DialogTitle>
          {error && <p className="text-sm text-destructive">{error}</p>}
        </DialogHeader>
        {!profile && !error && <p className="p-5 text-sm text-muted-foreground">{t("loading")}</p>}

        {profile && (
          <div className="flex min-h-0 flex-1">
            <aside className="w-48 shrink-0 overflow-y-auto border-r p-2">
              <nav className="space-y-0.5">
                {SETTINGS_SECTIONS.map((s) => (
                  <button
                    key={s.id}
                    type="button"
                    onClick={() => setSection(s.id)}
                    className={cn(
                      "flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm transition-colors",
                      section === s.id
                        ? "bg-accent font-medium text-accent-foreground"
                        : "text-muted-foreground hover:bg-muted hover:text-foreground",
                    )}
                  >
                    <s.icon className="size-4 shrink-0" />
                    <span className="truncate">{t(s.label)}</span>
                  </button>
                ))}
              </nav>
            </aside>

            <SettingsSections
              t={t}
              section={section}
              profile={profile}
              theme={theme}
              setTheme={setTheme}
              density={density}
              setDensity={setDensity}
              accent={accent}
              setAccent={setAccent}
              prefs={prefs}
              setAi={setAi}
              setNotifications={setNotifications}
              displayedName={displayedName}
              setDisplayedName={setDisplayedName}
              signature={signature}
              setSignature={setSignature}
              whitelist={whitelist}
              setWhitelist={setWhitelist}
              blacklist={blacklist}
              setBlacklist={setBlacklist}
              forwardEnabled={forwardEnabled}
              setForwardEnabled={setForwardEnabled}
              forwardDestination={forwardDestination}
              setForwardDestination={setForwardDestination}
              forwardKeep={forwardKeep}
              setForwardKeep={setForwardKeep}
              replyEnabled={replyEnabled}
              setReplyEnabled={setReplyEnabled}
              replySubject={replySubject}
              setReplySubject={setReplySubject}
              replyBody={replyBody}
              setReplyBody={setReplyBody}
              replyStartdate={replyStartdate}
              setReplyStartdate={setReplyStartdate}
              replyEnddate={replyEnddate}
              setReplyEnddate={setReplyEnddate}
              spamEnabled={spamEnabled}
              setSpamEnabled={setSpamEnabled}
              spamMarkAsRead={spamMarkAsRead}
              setSpamMarkAsRead={setSpamMarkAsRead}
              spamThreshold={spamThreshold}
              setSpamThreshold={setSpamThreshold}
              saveSettings={saveSettings}
              savePassword={savePassword}
              pgp={pgp}
              setPgp={setPgp}
              generating={generating}
              setGenerating={setGenerating}
              copied={copied}
              setCopied={setCopied}
              pgpKeys={pgpKeys}
              pgpImportEmail={pgpImportEmail}
              setPgpImportEmail={setPgpImportEmail}
              pgpImportKeyText={pgpImportKeyText}
              setPgpImportKeyText={setPgpImportKeyText}
              pgpImporting={pgpImporting}
              pgpDeleting={pgpDeleting}
              onImportPgpKey={importPgpKey}
              onDeletePgpKey={deletePgpKey}
              totp={totp}
              setTotp={setTotp}
              totpCode={totpCode}
              setTotpCode={setTotpCode}
              totpBusy={totpBusy}
              setTotpBusy={setTotpBusy}
              webhooks={webhooks}
              whUrl={whUrl}
              setWhUrl={setWhUrl}
              whSecret={whSecret}
              setWhSecret={setWhSecret}
              whEvents={whEvents}
              setWhEvents={setWhEvents}
              whEnabled={whEnabled}
              setWhEnabled={setWhEnabled}
              whSaving={whSaving}
              whTesting={whTesting}
              whTestMsg={whTestMsg}
              onAddWebhook={addWebhook}
              onDeleteWebhook={deleteWebhook}
              onToggleWebhook={toggleWebhook}
              onTestWebhook={testWebhook}
              smime={smime}
              smimeCerts={smimeCerts}
              smimeImportMode={smimeImportMode}
              setSmimeImportMode={setSmimeImportMode}
              smimeCertPem={smimeCertPem}
              setSmimeCertPem={setSmimeCertPem}
              smimeKeyPem={smimeKeyPem}
              setSmimeKeyPem={setSmimeKeyPem}
              smimeP12B64={smimeP12B64}
              setSmimeP12B64={setSmimeP12B64}
              smimeP12Password={smimeP12Password}
              setSmimeP12Password={setSmimeP12Password}
              smimeImporting={smimeImporting}
              smimeKeyringEmail={smimeKeyringEmail}
              setSmimeKeyringEmail={setSmimeKeyringEmail}
              smimeKeyringCert={smimeKeyringCert}
              setSmimeKeyringCert={setSmimeKeyringCert}
              smimeKeyringImporting={smimeKeyringImporting}
              smimeKeyringDeleting={smimeKeyringDeleting}
              onImportSmime={importSmime}
              onDeleteSmime={deleteSmime}
              onImportSmimeCert={importSmimeCert}
              onDeleteSmimeCert={deleteSmimeCert}
              accountList={accountList}
              accName={accName}
              setAccName={setAccName}
              accEmail={accEmail}
              setAccEmail={setAccEmail}
              accImapHost={accImapHost}
              setAccImapHost={setAccImapHost}
              accImapPort={accImapPort}
              setAccImapPort={setAccImapPort}
              accImapSecurity={accImapSecurity}
              setAccImapSecurity={setAccImapSecurity}
              accSmtpHost={accSmtpHost}
              setAccSmtpHost={setAccSmtpHost}
              accSmtpPort={accSmtpPort}
              setAccSmtpPort={setAccSmtpPort}
              accSmtpSecurity={accSmtpSecurity}
              setAccSmtpSecurity={setAccSmtpSecurity}
              accUsername={accUsername}
              setAccUsername={setAccUsername}
              accPassword={accPassword}
              setAccPassword={setAccPassword}
              accSaving={accSaving}
              accTesting={accTesting}
              accTestMsg={accTestMsg}
              onAddAccount={addAccount}
              onDeleteAccount={removeAccount}
              onToggleAccount={toggleAccount}
              onTestAccount={testAccount}
              setError={setError}
              oldPw={oldPw}
              setOldPw={setOldPw}
              newPw={newPw}
              setNewPw={setNewPw}
              confirmPw={confirmPw}
              setConfirmPw={setConfirmPw}
            />
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

