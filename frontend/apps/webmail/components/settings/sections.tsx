"use client";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  pgpDelete, pgpGenerate, totpDisable, totpEnable,
  type MeSettings, type PgpStatus, type TotpStatus,
} from "@/lib/api";
import type { Accent, Density, Preferences, Theme } from "@/lib/preferences";
import { cn } from "@/lib/utils";

const ACCENT_COLORS: Record<Accent, string> = {
  blue: "#2E6E8E",
  green: "#2F8E6C",
  purple: "#6B4FA0",
  orange: "#C4712A",
  rose: "#B0555B",
};

// Sections that share the single profile <form> and its save button; the
// PGP / 2FA / password sections manage their own state and actions.
const PROFILE_SECTIONS = new Set([
  "appearance", "ai", "notifications", "identity",
  "forwarding", "autoReply", "spam", "filters",
]);

export type SettingsSectionsProps = {
  t: (key: string) => string;
  section: string;
  profile: MeSettings | null;
  theme: Theme; setTheme: (v: Theme) => void;
  density: Density; setDensity: (v: Density) => void;
  accent: Accent; setAccent: (v: Accent) => void;
  prefs: Preferences; setAi: (ai: Preferences["ai"]) => void; setNotifications: (v: boolean) => void;
  displayedName: string; setDisplayedName: (v: string) => void;
  signature: string; setSignature: (v: string) => void;
  whitelist: string; setWhitelist: (v: string) => void;
  blacklist: string; setBlacklist: (v: string) => void;
  forwardEnabled: boolean; setForwardEnabled: (v: boolean) => void;
  forwardDestination: string; setForwardDestination: (v: string) => void;
  forwardKeep: boolean; setForwardKeep: (v: boolean) => void;
  replyEnabled: boolean; setReplyEnabled: (v: boolean) => void;
  replySubject: string; setReplySubject: (v: string) => void;
  replyBody: string; setReplyBody: (v: string) => void;
  replyStartdate: string; setReplyStartdate: (v: string) => void;
  replyEnddate: string; setReplyEnddate: (v: string) => void;
  spamEnabled: boolean; setSpamEnabled: (v: boolean) => void;
  spamMarkAsRead: boolean; setSpamMarkAsRead: (v: boolean) => void;
  spamThreshold: number; setSpamThreshold: (v: number) => void;
  saveSettings: (e: React.FormEvent) => void;
  savePassword: (e: React.FormEvent) => void;
  pgp: PgpStatus | null; setPgp: (v: PgpStatus | null) => void;
  generating: boolean; setGenerating: (v: boolean) => void;
  copied: boolean; setCopied: (v: boolean) => void;
  totp: TotpStatus | null; setTotp: (v: TotpStatus | null) => void;
  totpCode: string; setTotpCode: (v: string) => void;
  totpBusy: boolean; setTotpBusy: (v: boolean) => void;
  setError: (v: string) => void;
  oldPw: string; setOldPw: (v: string) => void;
  newPw: string; setNewPw: (v: string) => void;
  confirmPw: string; setConfirmPw: (v: string) => void;
};

// SettingsSections renders the active settings section content. The
// MailSettings shell owns all state and passes it down; the extracted JSX
// is byte-identical to the original single-file implementation.
export function SettingsSections(props: SettingsSectionsProps) {
  const { t, section, profile, theme, setTheme, density, setDensity, accent, setAccent, prefs, setAi, setNotifications, displayedName, setDisplayedName, signature, setSignature, whitelist, setWhitelist, blacklist, setBlacklist, forwardEnabled, setForwardEnabled, forwardDestination, setForwardDestination, forwardKeep, setForwardKeep, replyEnabled, setReplyEnabled, replySubject, setReplySubject, replyBody, setReplyBody, replyStartdate, setReplyStartdate, replyEnddate, setReplyEnddate, spamEnabled, setSpamEnabled, spamMarkAsRead, setSpamMarkAsRead, spamThreshold, setSpamThreshold, saveSettings, savePassword, pgp, setPgp, generating, setGenerating, copied, setCopied, totp, setTotp, totpCode, setTotpCode, totpBusy, setTotpBusy, setError, oldPw, setOldPw, newPw, setNewPw, confirmPw, setConfirmPw } = props;
  return (
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
                          <Input value={displayedName} onChange={(e) => setDisplayedName(e.target.value)} placeholder={profile?.email} />
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

