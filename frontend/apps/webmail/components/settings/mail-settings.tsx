"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import {
  Bell, Filter, Forward, KeyRound, Lock, MessageSquareReply,
  Palette, ShieldAlert, ShieldCheck, Sparkles, User,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { usePreferences } from "@/components/preferences-provider";
import {
  changePassword, meProfile, updateMeSettings,
  pgpStatus, pgpGenerate, pgpDelete,
  totpStatus, totpEnable, totpDisable, type TotpStatus,
  type PgpStatus, type MeSettings,
} from "@/lib/api";
import type { Accent, Density, Theme } from "@/lib/preferences";
import { cn } from "@/lib/utils";

const ACCENT_COLORS: Record<Accent, string> = {
  blue: "#2E6E8E",
  green: "#2F8E6C",
  purple: "#6B4FA0",
  orange: "#C4712A",
  rose: "#B0555B",
};

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

// Sections that share the single profile <form> and its save button; the
// PGP / 2FA / password sections manage their own state and actions.
const PROFILE_SECTIONS = new Set([
  "appearance", "ai", "notifications", "identity",
  "forwarding", "autoReply", "spam", "filters",
]);

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

            <div className="min-w-0 flex-1">
              {PROFILE_SECTIONS.has(section) && (
                <form onSubmit={saveSettings} className="flex h-full min-h-0 flex-col">
                  <div key={section} className="min-h-0 flex-1 space-y-5 overflow-y-auto p-5">
                    {section === "appearance" && (
                      <div className="space-y-3">
                        <p className="text-sm font-medium">{t("appearance")}</p>
                        <div className="space-y-1.5">
                          <Label>{t("theme")}</Label>
                          <Segmented
                            value={theme}
                            options={[
                              { value: "light", label: t("themeLight") },
                              { value: "dark", label: t("themeDark") },
                              { value: "system", label: t("themeSystem") },
                            ]}
                            onChange={(v) => setTheme(v as Theme)}
                          />
                        </div>
                        <div className="space-y-1.5">
                          <Label>{t("density")}</Label>
                          <Segmented
                            value={density}
                            options={[
                              { value: "compact", label: t("densityCompact") },
                              { value: "cozy", label: t("densityCozy") },
                              { value: "relaxed", label: t("densityRelaxed") },
                            ]}
                            onChange={(v) => setDensity(v as Density)}
                          />
                        </div>
                        <div className="space-y-1.5">
                          <Label>{t("accent")}</Label>
                          <div className="flex gap-1.5">
                            {(["blue", "green", "purple", "orange", "rose"] as Accent[]).map((a) => (
                              <button
                                key={a}
                                type="button"
                                onClick={() => setAccent(a)}
                                title={t(`accent${a.charAt(0).toUpperCase()}${a.slice(1)}`)}
                                className={cn(
                                  "size-6 rounded-full border-2 transition-transform",
                                  accent === a ? "scale-110 border-foreground" : "border-transparent hover:scale-105",
                                )}
                                style={{ backgroundColor: ACCENT_COLORS[a] }}
                              />
                            ))}
                          </div>
                        </div>
                      </div>
                    )}

                    {section === "ai" && (
                      <div className="space-y-2.5">
                        <p className="text-sm font-medium">{t("aiFeatures")}</p>
                        <div className="flex items-center justify-between">
                          <Label>{t("aiEnabled")}</Label>
                          <Switch
                            checked={prefs.ai.enabled}
                            onCheckedChange={(v) => setAi({ ...prefs.ai, enabled: v })}
                          />
                        </div>
                        {prefs.ai.enabled && (
                          <>
                            <div className="flex items-center justify-between">
                              <Label>{t("aiSummary")}</Label>
                              <Switch
                                checked={prefs.ai.summary}
                                onCheckedChange={(v) => setAi({ ...prefs.ai, summary: v })}
                              />
                            </div>
                            <div className="flex items-center justify-between">
                              <Label>{t("aiDraft")}</Label>
                              <Switch
                                checked={prefs.ai.draft}
                                onCheckedChange={(v) => setAi({ ...prefs.ai, draft: v })}
                              />
                            </div>
                            <div className="flex items-center justify-between">
                              <Label>{t("aiPriority")}</Label>
                              <Switch
                                checked={prefs.ai.priority}
                                onCheckedChange={(v) => setAi({ ...prefs.ai, priority: v })}
                              />
                            </div>
                            <div className="flex items-center justify-between">
                              <Label>{t("aiSearch")}</Label>
                              <Switch
                                checked={prefs.ai.search}
                                onCheckedChange={(v) => setAi({ ...prefs.ai, search: v })}
                              />
                            </div>
                          </>
                        )}
                        <p className="text-xs text-muted-foreground">{t("aiNote")}</p>
                      </div>
                    )}

                    {section === "notifications" && (
                      <div className="space-y-2">
                        <p className="text-sm font-medium">{t("notifications")}</p>
                        <div className="flex items-center justify-between">
                          <Label>{t("notifications")}</Label>
                          <Switch
                            checked={prefs.notifications}
                            onCheckedChange={(v) => {
                              setNotifications(v);
                              if (v && typeof Notification !== "undefined" && Notification.permission === "default") {
                                Notification.requestPermission().catch(() => {});
                              }
                            }}
                          />
                        </div>
                      </div>
                    )}

                    {section === "identity" && (
                      <div className="space-y-4">
                        <p className="text-sm font-medium">{t("identity")}</p>
                        <div className="space-y-2">
                          <Label>{t("displayedName")}</Label>
                          <Input value={displayedName} onChange={(e) => setDisplayedName(e.target.value)} placeholder={profile.email} />
                        </div>
                        <div className="space-y-2">
                          <Label>{t("signature")}</Label>
                          <textarea
                            value={signature}
                            onChange={(e) => setSignature(e.target.value)}
                            rows={4}
                            className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm"
                          />
                          <p className="text-xs text-muted-foreground">{t("signatureHint")}</p>
                        </div>
                      </div>
                    )}

                    {section === "forwarding" && (
                      <div className="space-y-2">
                        <p className="text-sm font-medium">{t("forwarding")}</p>
                        <div className="flex items-center justify-between">
                          <Label>{t("forwardMail")}</Label>
                          <Switch checked={forwardEnabled} onCheckedChange={setForwardEnabled} />
                        </div>
                        {forwardEnabled && (
                          <>
                            <div className="space-y-2">
                              <Label>{t("forwardTo")}</Label>
                              <Input value={forwardDestination} onChange={(e) => setForwardDestination(e.target.value)} placeholder="other@example.com" />
                            </div>
                            <div className="flex items-center justify-between">
                              <Label>{t("forwardKeep")}</Label>
                              <Switch checked={forwardKeep} onCheckedChange={setForwardKeep} />
                            </div>
                          </>
                        )}
                      </div>
                    )}

                    {section === "autoReply" && (
                      <div className="space-y-2">
                        <p className="text-sm font-medium">{t("autoReply")}</p>
                        <div className="flex items-center justify-between">
                          <Label>{t("enableAutoReply")}</Label>
                          <Switch checked={replyEnabled} onCheckedChange={setReplyEnabled} />
                        </div>
                        {replyEnabled && (
                          <>
                            <div className="space-y-2">
                              <Label>{t("subject")}</Label>
                              <Input value={replySubject} onChange={(e) => setReplySubject(e.target.value)} placeholder="Re: away" />
                            </div>
                            <div className="space-y-2">
                              <Label>{t("body")}</Label>
                              <textarea
                                value={replyBody}
                                onChange={(e) => setReplyBody(e.target.value)}
                                rows={4}
                                className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm"
                              />
                            </div>
                            <div className="grid grid-cols-2 gap-4">
                              <div className="space-y-2">
                                <Label>{t("startDate")}</Label>
                                <Input type="date" value={replyStartdate} onChange={(e) => setReplyStartdate(e.target.value)} />
                              </div>
                              <div className="space-y-2">
                                <Label>{t("endDate")}</Label>
                                <Input type="date" value={replyEnddate} onChange={(e) => setReplyEnddate(e.target.value)} />
                              </div>
                            </div>
                          </>
                        )}
                      </div>
                    )}

                    {section === "spam" && (
                      <div className="space-y-2">
                        <p className="text-sm font-medium">{t("spamFilter")}</p>
                        <div className="flex items-center justify-between">
                          <Label>{t("enableSpam")}</Label>
                          <Switch checked={spamEnabled} onCheckedChange={setSpamEnabled} />
                        </div>
                        {spamEnabled && (
                          <>
                            <div className="flex items-center justify-between">
                              <Label>{t("markAsRead")}</Label>
                              <Switch checked={spamMarkAsRead} onCheckedChange={setSpamMarkAsRead} />
                            </div>
                            <div className="space-y-2">
                              <Label>{t("threshold")}</Label>
                              <Input type="number" min={0} max={100} value={spamThreshold} onChange={(e) => setSpamThreshold(Number(e.target.value))} />
                            </div>
                          </>
                        )}
                      </div>
                    )}

                    {section === "filters" && (
                      <div className="space-y-5">
                        <p className="text-sm font-medium">{t("filters")}</p>
                        <div className="space-y-2">
                          <Label>{t("whitelist")}</Label>
                          <textarea
                            value={whitelist}
                            onChange={(e) => setWhitelist(e.target.value)}
                            rows={2}
                            className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm"
                          />
                          <p className="text-xs text-muted-foreground">{t("whitelistHint")}</p>
                        </div>
                        <div className="space-y-2">
                          <Label>{t("blacklist")}</Label>
                          <textarea
                            value={blacklist}
                            onChange={(e) => setBlacklist(e.target.value)}
                            rows={2}
                            className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm"
                          />
                          <p className="text-xs text-muted-foreground">{t("blacklistHint")}</p>
                        </div>
                      </div>
                    )}
                  </div>
                  <div className="flex shrink-0 justify-end border-t bg-muted/40 px-5 py-3">
                    <Button type="submit">{t("saveSettings")}</Button>
                  </div>
                </form>
              )}

              {section === "pgp" && (
                <div key={section} className="h-full space-y-3 overflow-y-auto p-5">
                  <p className="text-sm font-medium">{t("pgp")}</p>
                  {pgp === null && <p className="text-sm text-muted-foreground">{t("loading")}</p>}
                  {pgp && !pgp.has_key && (
                    <div className="space-y-2">
                      <p className="text-xs text-muted-foreground">{t("pgpNoKey")}</p>
                      <Button
                        type="button"
                        size="sm"
                        onClick={async () => {
                          setGenerating(true);
                          setError("");
                          try {
                            setPgp(await pgpGenerate());
                          } catch (e) {
                            setError(e instanceof Error ? e.message : "pgp generate failed");
                          } finally {
                            setGenerating(false);
                          }
                        }}
                        disabled={generating}
                      >
                        {generating ? t("pgpGenerating") : t("pgpGenerate")}
                      </Button>
                    </div>
                  )}
                  {pgp && pgp.has_key && (
                    <div className="space-y-2">
                      <p className="font-mono text-xs text-muted-foreground">
                        {t("pgpFingerprint")}: {pgp.fingerprint}
                      </p>
                      <div className="flex flex-wrap gap-1">
                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          onClick={() => {
                            if (!pgp.public_key) return;
                            navigator.clipboard?.writeText(pgp.public_key).then(() => {
                              setCopied(true);
                              setTimeout(() => setCopied(false), 1500);
                            });
                          }}
                        >
                          {copied ? t("pgpCopied") : t("pgpCopyPublicKey")}
                        </Button>
                        <Button
                          type="button"
                          size="sm"
                          variant="ghost"
                          className="text-muted-foreground hover:text-destructive"
                          onClick={async () => {
                            if (!window.confirm(t("pgpDeleteConfirm"))) return;
                            setError("");
                            try {
                              await pgpDelete();
                              setPgp({ has_key: false });
                            } catch (e) {
                              setError(e instanceof Error ? e.message : "pgp delete failed");
                            }
                          }}
                        >
                          {t("pgpDeleteKey")}
                        </Button>
                      </div>
                      <p className="text-xs text-muted-foreground">{t("pgpNote")}</p>
                    </div>
                  )}
                </div>
              )}

              {section === "twoFactor" && (
                <div key={section} className="h-full space-y-3 overflow-y-auto p-5">
                  <p className="text-sm font-medium">{t("twoFactor")}</p>
                  {totp === null && <p className="text-sm text-muted-foreground">{t("loading")}</p>}
                  {totp && totp.enabled && (
                    <div className="space-y-2">
                      <p className="text-xs text-muted-foreground">{t("twoFactorEnabled")}</p>
                      <div className="flex gap-1.5">
                        <Input
                          value={totpCode}
                          onChange={(e) => setTotpCode(e.target.value)}
                          placeholder="123456"
                          inputMode="numeric"
                          className="h-8 w-28"
                        />
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={totpBusy}
                          onClick={async () => {
                            setTotpBusy(true);
                            setError("");
                            try {
                              await totpDisable(totpCode.trim());
                              setTotp({ enabled: false });
                              setTotpCode("");
                            } catch (e) {
                              setError(e instanceof Error ? e.message : "2fa disable failed");
                            } finally {
                              setTotpBusy(false);
                            }
                          }}
                        >
                          {t("disable")}
                        </Button>
                      </div>
                    </div>
                  )}
                  {totp && !totp.enabled && (
                    <div className="space-y-2">
                      <p className="text-xs text-muted-foreground">{t("twoFactorHint")}</p>
                      {totp.otpauth && (
                        <p className="rounded-md border border-border bg-muted/40 p-2 font-mono text-[10px] break-all text-muted-foreground">
                          {totp.otpauth}
                        </p>
                      )}
                      <div className="flex gap-1.5">
                        <Input
                          value={totpCode}
                          onChange={(e) => setTotpCode(e.target.value)}
                          placeholder="123456"
                          inputMode="numeric"
                          className="h-8 w-28"
                        />
                        <Button
                          size="sm"
                          disabled={totpBusy}
                          onClick={async () => {
                            setTotpBusy(true);
                            setError("");
                            try {
                              await totpEnable(totpCode.trim());
                              setTotp({ enabled: true });
                              setTotpCode("");
                            } catch (e) {
                              setError(e instanceof Error ? e.message : "2fa enable failed");
                            } finally {
                              setTotpBusy(false);
                            }
                          }}
                        >
                          {t("enable")}
                        </Button>
                      </div>
                    </div>
                  )}
                </div>
              )}

              {section === "password" && (
                <form key={section} onSubmit={savePassword} className="h-full space-y-4 overflow-y-auto p-5">
                  <p className="text-sm font-medium">{t("changePassword")}</p>
                  <div className="space-y-2">
                    <Label>{t("currentPassword")}</Label>
                    <Input type="password" value={oldPw} onChange={(e) => setOldPw(e.target.value)} required />
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label>{t("newPassword")}</Label>
                      <Input type="password" value={newPw} onChange={(e) => setNewPw(e.target.value)} required />
                    </div>
                    <div className="space-y-2">
                      <Label>{t("confirmPassword")}</Label>
                      <Input type="password" value={confirmPw} onChange={(e) => setConfirmPw(e.target.value)} required />
                    </div>
                  </div>
                  <div className="flex justify-end">
                    <Button type="submit">{t("changePasswordBtn")}</Button>
                  </div>
                </form>
              )}
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

function Segmented({
  value,
  options,
  onChange,
}: {
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
}) {
  return (
    <div className="flex rounded-lg border border-border bg-muted/40 p-0.5">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          onClick={() => onChange(o.value)}
          className={cn(
            "flex-1 rounded-md px-2 py-1 text-xs transition-colors",
            value === o.value
              ? "bg-background font-medium text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
