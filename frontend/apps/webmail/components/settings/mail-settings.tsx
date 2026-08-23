"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import {
  Bell, Filter, Forward, KeyRound, Lock, MessageSquareReply,
  Palette, ShieldAlert, ShieldCheck, Sparkles, User,
} from "lucide-react";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { usePreferences } from "@/components/preferences-provider";
import { SettingsSections } from "@/components/settings/sections";
import {
  changePassword, meProfile, updateMeSettings,
  pgpStatus, totpStatus,
  type TotpStatus,
  type PgpStatus, type MeSettings,
} from "@/lib/api";
import { cn } from "@/lib/utils";

// Sidebar navigation for the settings dialog, mirroring the FastMail layout:
// a section list on the left, the active section's form on the right.
const SETTINGS_SECTIONS = [
  { id: "appearance", icon: Palette, label: "appearance" },
  { id: "ai", icon: Sparkles, label: "aiFeatures" },
  { id: "notifications", icon: Bell, label: "notifications" },
  { id: "identity", icon: User, label: "identity" },
  { id: "forwarding", icon: Forward, label: "forwarding" },
  { id: "autoReply", icon: MessageSquareReply, label: "autoReply" },
  { id: "spam", icon: ShieldAlert, label: "spamFilter" },
  { id: "filters", icon: Filter, label: "filters" },
  { id: "pgp", icon: KeyRound, label: "pgp" },
  { id: "twoFactor", icon: ShieldCheck, label: "twoFactor" },
  { id: "password", icon: Lock, label: "changePassword" },
] as const;

// backend serializes time.Time as RFC3339; date inputs need yyyy-mm-dd
const toDateInput = (s: string) => (s && !s.startsWith("0001") ? s.slice(0, 10) : "");

export function MailSettings({ open, onOpenChange, onSaved }: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onSaved: () => void;
}) {
  const t = useTranslations("settings");
  const { theme, setTheme, density, setDensity, accent, setAccent, prefs, setAi, setNotifications } = usePreferences();
  const [profile, setProfile] = useState<MeSettings | null>(null);
  const [error, setError] = useState("");
  const [section, setSection] = useState<string>("appearance");

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

  // two-factor authentication
  const [totp, setTotp] = useState<TotpStatus | null>(null);
  const [totpCode, setTotpCode] = useState("");
  const [totpBusy, setTotpBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    setError("");
    totpStatus().then(setTotp).catch(() => setTotp(null));
    pgpStatus().then(setPgp).catch(() => setPgp(null));
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

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] max-w-3xl flex-col overflow-hidden p-0">
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
              totp={totp}
              setTotp={setTotp}
              totpCode={totpCode}
              setTotpCode={setTotpCode}
              totpBusy={totpBusy}
              setTotpBusy={setTotpBusy}
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

