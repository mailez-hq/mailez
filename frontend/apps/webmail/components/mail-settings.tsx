"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { usePreferences } from "@/components/preferences-provider";
import {
  changePassword, meProfile, updateMeSettings, type MeSettings,
} from "@/lib/api";
import type { Density, Theme } from "@/lib/preferences";
import { cn } from "@/lib/utils";

// backend serializes time.Time as RFC3339; date inputs need yyyy-mm-dd
const toDateInput = (s: string) => (s && !s.startsWith("0001") ? s.slice(0, 10) : "");

export function MailSettings({ open, onOpenChange, onSaved }: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onSaved: () => void;
}) {
  const t = useTranslations("settings");
  const { theme, setTheme, density, setDensity } = usePreferences();
  const [profile, setProfile] = useState<MeSettings | null>(null);
  const [error, setError] = useState("");

  const [displayedName, setDisplayedName] = useState("");
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

  useEffect(() => {
    if (!open) return;
    setError("");
    meProfile().then((p) => {
      setProfile(p);
      setDisplayedName(p.displayed_name);
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
      <DialogContent className="max-h-[90vh] max-w-xl overflow-y-auto">
        <DialogHeader><DialogTitle>{t("title")}</DialogTitle></DialogHeader>
        {!profile && !error && <p className="text-sm text-muted-foreground">{t("loading")}</p>}

        {error && <p className="text-sm text-destructive">{error}</p>}

        {profile && (
          <div className="space-y-4">
            <form onSubmit={saveSettings} className="space-y-4">
              <Separator />
              <div>
                <p className="mb-2 text-sm font-medium">{t("appearance")}</p>
                <div className="space-y-3">
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
                </div>
              </div>

              <Separator />
              <div className="space-y-2">
                <Label>{t("displayedName")}</Label>
                <Input value={displayedName} onChange={(e) => setDisplayedName(e.target.value)} placeholder={profile.email} />
              </div>

              <Separator />
              <div>
                <p className="mb-2 text-sm font-medium">{t("forwarding")}</p>
                <div className="space-y-2">
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
              </div>

              <Separator />
              <div>
                <p className="mb-2 text-sm font-medium">{t("autoReply")}</p>
                <div className="space-y-2">
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
              </div>

              <Separator />
              <div>
                <p className="mb-2 text-sm font-medium">{t("spamFilter")}</p>
                <div className="space-y-2">
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
              </div>

              <DialogFooter><Button type="submit">{t("saveSettings")}</Button></DialogFooter>
            </form>

            <Separator />

            <form onSubmit={savePassword} className="space-y-4">
              <div>
                <p className="mb-2 text-sm font-medium">{t("changePassword")}</p>
                <div className="space-y-2">
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
                </div>
              </div>
              <DialogFooter><Button type="submit">{t("changePasswordBtn")}</Button></DialogFooter>
            </form>
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
