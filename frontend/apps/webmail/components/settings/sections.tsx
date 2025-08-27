"use client";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  pgpDelete, pgpGenerate, totpDisable, totpEnable,
  type MailAccount, type MeSettings, type PgpKey, type PgpStatus, type SmimeCert, type SmimeStatus,
  type TotpStatus, type Webhook,
} from "@/lib/api";
import type { Accent, Density, Preferences, ReaderFontSize, ReadingPaneWidth, Theme } from "@/lib/preferences";
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
  spellcheck: boolean; setSpellcheck: (v: boolean) => void;
  readerFont: ReaderFontSize; setReaderFont: (v: ReaderFontSize) => void;
  paneWidth: ReadingPaneWidth; setPaneWidth: (v: ReadingPaneWidth) => void;
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
  pgpKeys: PgpKey[] | null;
  pgpImportEmail: string; setPgpImportEmail: (v: string) => void;
  pgpImportKeyText: string; setPgpImportKeyText: (v: string) => void;
  pgpImporting: boolean;
  pgpDeleting: number | null;
  onImportPgpKey: () => void;
  onDeletePgpKey: (id: number) => void;
  totp: TotpStatus | null; setTotp: (v: TotpStatus | null) => void;
  totpCode: string; setTotpCode: (v: string) => void;
  totpBusy: boolean; setTotpBusy: (v: boolean) => void;
  webhooks: Webhook[] | null;
  whUrl: string; setWhUrl: (v: string) => void;
  whSecret: string; setWhSecret: (v: string) => void;
  whEvents: string; setWhEvents: (v: string) => void;
  whEnabled: boolean; setWhEnabled: (v: boolean) => void;
  whSaving: boolean;
  whTesting: number | null;
  whTestMsg: string;
  onAddWebhook: () => void;
  onDeleteWebhook: (id: number) => void;
  onToggleWebhook: (w: Webhook) => void;
  onTestWebhook: (id: number) => void;
  smime: SmimeStatus | null;
  smimeCerts: SmimeCert[] | null;
  smimeImportMode: "pem" | "p12"; setSmimeImportMode: (v: "pem" | "p12") => void;
  smimeCertPem: string; setSmimeCertPem: (v: string) => void;
  smimeKeyPem: string; setSmimeKeyPem: (v: string) => void;
  smimeP12B64: string; setSmimeP12B64: (v: string) => void;
  smimeP12Password: string; setSmimeP12Password: (v: string) => void;
  smimeImporting: boolean;
  smimeKeyringEmail: string; setSmimeKeyringEmail: (v: string) => void;
  smimeKeyringCert: string; setSmimeKeyringCert: (v: string) => void;
  smimeKeyringImporting: boolean;
  smimeKeyringDeleting: number | null;
  onImportSmime: () => void;
  onDeleteSmime: () => void;
  onImportSmimeCert: () => void;
  onDeleteSmimeCert: (id: number) => void;
  accountList: MailAccount[] | null;
  accName: string; setAccName: (v: string) => void;
  accEmail: string; setAccEmail: (v: string) => void;
  accImapHost: string; setAccImapHost: (v: string) => void;
  accImapPort: number; setAccImapPort: (v: number) => void;
  accImapSecurity: string; setAccImapSecurity: (v: string) => void;
  accSmtpHost: string; setAccSmtpHost: (v: string) => void;
  accSmtpPort: number; setAccSmtpPort: (v: number) => void;
  accSmtpSecurity: string; setAccSmtpSecurity: (v: string) => void;
  accUsername: string; setAccUsername: (v: string) => void;
  accPassword: string; setAccPassword: (v: string) => void;
  accSaving: boolean;
  accTesting: number | null;
  accTestMsg: string;
  onAddAccount: () => void;
  onDeleteAccount: (id: number) => void;
  onToggleAccount: (a: MailAccount) => void;
  onTestAccount: (id: number) => void;
  setError: (v: string) => void;
  oldPw: string; setOldPw: (v: string) => void;
  newPw: string; setNewPw: (v: string) => void;
  confirmPw: string; setConfirmPw: (v: string) => void;
};

// SettingsSections renders the active settings section content. The
// MailSettings shell owns all state and passes it down; the extracted JSX
// is byte-identical to the original single-file implementation.
export function SettingsSections(props: SettingsSectionsProps) {
  const { t, section, profile, theme, setTheme, density, setDensity, accent, setAccent, spellcheck, setSpellcheck, readerFont, setReaderFont, paneWidth, setPaneWidth, prefs, setAi, setNotifications, displayedName, setDisplayedName, signature, setSignature, whitelist, setWhitelist, blacklist, setBlacklist, forwardEnabled, setForwardEnabled, forwardDestination, setForwardDestination, forwardKeep, setForwardKeep, replyEnabled, setReplyEnabled, replySubject, setReplySubject, replyBody, setReplyBody, replyStartdate, setReplyStartdate, replyEnddate, setReplyEnddate, spamEnabled, setSpamEnabled, spamMarkAsRead, setSpamMarkAsRead, spamThreshold, setSpamThreshold, saveSettings, savePassword, pgp, setPgp, generating, setGenerating, copied, setCopied, pgpKeys, pgpImportEmail, setPgpImportEmail, pgpImportKeyText, setPgpImportKeyText, pgpImporting, pgpDeleting, onImportPgpKey, onDeletePgpKey, totp, setTotp, totpCode, setTotpCode, totpBusy, setTotpBusy, webhooks, whUrl, setWhUrl, whSecret, setWhSecret, whEvents, setWhEvents, whEnabled, setWhEnabled, whSaving, whTesting, whTestMsg, onAddWebhook, onDeleteWebhook, onToggleWebhook, onTestWebhook, smime, smimeCerts, smimeImportMode, setSmimeImportMode, smimeCertPem, setSmimeCertPem, smimeKeyPem, setSmimeKeyPem, smimeP12B64, setSmimeP12B64, smimeP12Password, setSmimeP12Password, smimeImporting, smimeKeyringEmail, setSmimeKeyringEmail, smimeKeyringCert, setSmimeKeyringCert, smimeKeyringImporting, smimeKeyringDeleting, onImportSmime, onDeleteSmime, onImportSmimeCert, onDeleteSmimeCert, accountList, accName, setAccName, accEmail, setAccEmail, accImapHost, setAccImapHost, accImapPort, setAccImapPort, accImapSecurity, setAccImapSecurity, accSmtpHost, setAccSmtpHost, accSmtpPort, setAccSmtpPort, accSmtpSecurity, setAccSmtpSecurity, accUsername, setAccUsername, accPassword, setAccPassword, accSaving, accTesting, accTestMsg, onAddAccount, onDeleteAccount, onToggleAccount, onTestAccount, setError, oldPw, setOldPw, newPw, setNewPw, confirmPw, setConfirmPw } = props;
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
                        <div className="space-y-1.5">
                          <Label>{t("readerFont")}</Label>
                          <Segmented
                            value={readerFont}
                            options={[
                              { value: "sm", label: t("readerFontSm") },
                              { value: "md", label: t("readerFontMd") },
                              { value: "lg", label: t("readerFontLg") },
                              { value: "xl", label: t("readerFontXl") },
                            ]}
                            onChange={(v) => setReaderFont(v as ReaderFontSize)}
                          />
                        </div>
                        <div className="space-y-1.5">
                          <Label>{t("paneWidth")}</Label>
                          <Segmented
                            value={paneWidth}
                            options={[
                              { value: "narrow", label: t("paneNarrow") },
                              { value: "md", label: t("paneMd") },
                              { value: "wide", label: t("paneWide") },
                            ]}
                            onChange={(v) => setPaneWidth(v as ReadingPaneWidth)}
                          />
                        </div>
                        <div className="flex items-center justify-between">
                          <Label>{t("spellcheck")}</Label>
                          <Switch checked={spellcheck} onCheckedChange={setSpellcheck} />
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

              {section === "accounts" && (
                <div key={section} className="h-full space-y-5 overflow-y-auto p-5">
                  <p className="text-sm font-medium">{t("accounts")}</p>
                  <p className="text-xs text-muted-foreground">{t("accountIntro")}</p>

                  {accTestMsg && (
                    <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">{accTestMsg}</p>
                  )}

                  <div className="space-y-2">
                    {(accountList || []).length === 0 && (
                      <p className="text-sm text-muted-foreground">{t("accountEmpty")}</p>
                    )}
                    {(accountList || []).map((a) => (
                      <div key={a.id} className="rounded-lg border border-border p-3">
                        <div className="flex items-center gap-2">
                          <div className="min-w-0 flex-1">
                            <p className="truncate text-sm font-medium">
                              {a.name || a.email}
                              {!a.enabled && (
                                <span className="ml-2 rounded-full bg-muted px-1.5 py-px text-[10px] text-muted-foreground">
                                  {t("accountDisabled")}
                                </span>
                              )}
                            </p>
                            <p className="truncate text-xs text-muted-foreground">
                              {a.email} · {a.imap_host}:{a.imap_port}
                            </p>
                          </div>
                          <Switch checked={a.enabled} onCheckedChange={() => onToggleAccount(a)} />
                          <Button variant="outline" size="sm" disabled={accTesting === a.id} onClick={() => onTestAccount(a.id)}>
                            {accTesting === a.id ? t("accountTesting") : t("accountTest")}
                          </Button>
                          <Button variant="ghost" size="sm" className="text-destructive" onClick={() => onDeleteAccount(a.id)}>
                            {t("accountDelete")}
                          </Button>
                        </div>
                      </div>
                    ))}
                  </div>

                  <div className="space-y-2.5 rounded-lg border border-border p-3">
                    <p className="text-sm font-medium">{t("accountAdd")}</p>
                    <div className="grid grid-cols-2 gap-2.5">
                      <div className="space-y-1">
                        <Label>{t("accountName")}</Label>
                        <Input value={accName} onChange={(e) => setAccName(e.target.value)} placeholder={t("accountNamePlaceholder")} />
                      </div>
                      <div className="space-y-1">
                        <Label>{t("accountEmail")}</Label>
                        <Input value={accEmail} onChange={(e) => setAccEmail(e.target.value)} placeholder="you@example.com" />
                      </div>
                      <div className="space-y-1">
                        <Label>{t("accountImap")}</Label>
                        <Input value={accImapHost} onChange={(e) => setAccImapHost(e.target.value)} placeholder="imap.example.com" />
                      </div>
                      <div className="grid grid-cols-[1fr_100px] gap-2">
                        <div className="space-y-1">
                          <Label>{t("accountUsername")}</Label>
                          <Input value={accUsername} onChange={(e) => setAccUsername(e.target.value)} placeholder="user@example.com" />
                        </div>
                        <div className="space-y-1">
                          <Label>{t("accountPort")}</Label>
                          <Input type="number" value={String(accImapPort)} onChange={(e) => setAccImapPort(Number(e.target.value))} />
                        </div>
                      </div>
                      <div className="space-y-1">
                        <Label>{t("accountImapSecurity")}</Label>
                        <Segmented
                          value={accImapSecurity}
                          options={[
                            { value: "tls", label: t("securityTls") },
                            { value: "starttls", label: t("securityStartTls") },
                            { value: "none", label: t("securityNone") },
                          ]}
                          onChange={setAccImapSecurity}
                        />
                      </div>
                      <div className="space-y-1">
                        <Label>{t("accountSmtp")}</Label>
                        <Input value={accSmtpHost} onChange={(e) => setAccSmtpHost(e.target.value)} placeholder="smtp.example.com" />
                      </div>
                      <div className="space-y-1">
                        <Label>{t("accountPassword")}</Label>
                        <Input type="password" value={accPassword} onChange={(e) => setAccPassword(e.target.value)} autoComplete="off" />
                      </div>
                      <div className="space-y-1">
                        <Label>{t("accountSmtpSecurity")}</Label>
                        <Segmented
                          value={accSmtpSecurity}
                          options={[
                            { value: "starttls", label: t("securityStartTls") },
                            { value: "tls", label: t("securityTls") },
                            { value: "none", label: t("securityNone") },
                          ]}
                          onChange={setAccSmtpSecurity}
                        />
                      </div>
                    </div>
                    <Button disabled={accSaving} onClick={onAddAccount}>
                      {accSaving ? t("accountSaving") : t("accountAdd")}
                    </Button>
                  </div>
                </div>
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

                  {/* Keyring: imported public keys used to encrypt to
                      external recipients. Independent of the user's own key. */}
                  <div className="space-y-2 border-t pt-3">
                    <p className="text-sm font-medium">{t("pgpKeyring")}</p>
                    <div className="space-y-1.5">
                      <Label>{t("pgpImportEmail")}</Label>
                      <Input
                        value={pgpImportEmail}
                        onChange={(e) => setPgpImportEmail(e.target.value)}
                        placeholder="alice@example.com"
                        className="text-sm"
                      />
                    </div>
                    <div className="space-y-1.5">
                      <Label>{t("pgpImportKey")}</Label>
                      <textarea
                        value={pgpImportKeyText}
                        onChange={(e) => setPgpImportKeyText(e.target.value)}
                        rows={4}
                        placeholder="-----BEGIN PGP PUBLIC KEY BLOCK-----"
                        className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs shadow-sm"
                      />
                    </div>
                    <Button type="button" size="sm" onClick={onImportPgpKey} disabled={pgpImporting}>
                      {pgpImporting ? t("pgpImporting") : t("pgpImport")}
                    </Button>
                    {pgpKeys === null && <p className="text-xs text-muted-foreground">{t("loading")}</p>}
                    {pgpKeys !== null && pgpKeys.length === 0 && (
                      <p className="text-xs text-muted-foreground">{t("pgpKeyringEmpty")}</p>
                    )}
                    <ul className="space-y-1.5">
                      {pgpKeys?.map((k) => (
                        <li key={k.id} className="flex items-center gap-2 rounded-md border border-border px-2.5 py-1.5">
                          <div className="min-w-0 flex-1">
                            <p className="truncate text-sm">{k.email}</p>
                            <p className="truncate font-mono text-[11px] text-muted-foreground">
                              {k.fingerprint.slice(0, 40)}
                            </p>
                          </div>
                          <Button
                            type="button"
                            size="sm"
                            variant="ghost"
                            className="text-muted-foreground hover:text-destructive"
                            onClick={() => onDeletePgpKey(k.id)}
                            disabled={pgpDeleting === k.id}
                          >
                            {pgpDeleting === k.id ? t("pgpDeleting") : t("pgpDeleteKey")}
                          </Button>
                        </li>
                      ))}
                    </ul>
                    <p className="text-xs text-muted-foreground">{t("pgpKeyringNote")}</p>
                  </div>
                </div>
              )}

              {section === "smime" && (
                <div key={section} className="h-full space-y-3 overflow-y-auto p-5">
                  <p className="text-sm font-medium">{t("smime")}</p>
                  {smime === null && <p className="text-sm text-muted-foreground">{t("loading")}</p>}

                  {smime && !smime.has_cert && (
                    <div className="space-y-2">
                      <p className="text-xs text-muted-foreground">{t("smimeNoCert")}</p>
                      <div className="flex gap-1.5">
                        <Button
                          type="button"
                          size="sm"
                          variant={smimeImportMode === "pem" ? "default" : "outline"}
                          onClick={() => setSmimeImportMode("pem")}
                        >
                          {t("smimeImportPem")}
                        </Button>
                        <Button
                          type="button"
                          size="sm"
                          variant={smimeImportMode === "p12" ? "default" : "outline"}
                          onClick={() => setSmimeImportMode("p12")}
                        >
                          {t("smimeImportP12")}
                        </Button>
                      </div>
                      {smimeImportMode === "pem" ? (
                        <>
                          <div className="space-y-1.5">
                            <Label>{t("smimeCert")}</Label>
                            <textarea
                              value={smimeCertPem}
                              onChange={(e) => setSmimeCertPem(e.target.value)}
                              rows={4}
                              placeholder="-----BEGIN CERTIFICATE-----"
                              className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs shadow-sm"
                            />
                          </div>
                          <div className="space-y-1.5">
                            <Label>{t("smimePrivateKey")}</Label>
                            <textarea
                              value={smimeKeyPem}
                              onChange={(e) => setSmimeKeyPem(e.target.value)}
                              rows={4}
                              placeholder="-----BEGIN PRIVATE KEY-----"
                              className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs shadow-sm"
                            />
                          </div>
                        </>
                      ) : (
                        <>
                          <div className="space-y-1.5">
                            <Label>{t("smimeP12")}</Label>
                            <textarea
                              value={smimeP12B64}
                              onChange={(e) => setSmimeP12B64(e.target.value)}
                              rows={3}
                              placeholder={t("smimeP12Placeholder")}
                              className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs shadow-sm"
                            />
                          </div>
                          <div className="space-y-1.5">
                            <Label>{t("smimeP12Password")}</Label>
                            <Input
                              type="password"
                              value={smimeP12Password}
                              onChange={(e) => setSmimeP12Password(e.target.value)}
                              className="text-sm"
                            />
                          </div>
                        </>
                      )}
                      <Button type="button" size="sm" onClick={onImportSmime} disabled={smimeImporting}>
                        {smimeImporting ? t("smimeImporting") : t("smimeImport")}
                      </Button>
                    </div>
                  )}

                  {smime && smime.has_cert && (
                    <div className="space-y-2">
                      <p className="text-xs text-muted-foreground">
                        {smime.email && <>{smime.email} · </>}
                        {t("smimeFingerprint")}: <span className="font-mono">{smime.fingerprint}</span>
                      </p>
                      {smime.issuer && <p className="text-xs text-muted-foreground">{t("smimeIssuer")}: {smime.issuer}</p>}
                      {smime.not_after && (
                        <p className="text-xs text-muted-foreground">
                          {t("smimeExpires")}: {new Date(smime.not_after).toLocaleDateString()}
                        </p>
                      )}
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        className="text-muted-foreground hover:text-destructive"
                        onClick={onDeleteSmime}
                      >
                        {t("smimeDelete")}
                      </Button>
                    </div>
                  )}

                  {/* Certificate keyring: imported certs for external recipients */}
                  <div className="space-y-2 border-t pt-3">
                    <p className="text-sm font-medium">{t("smimeKeyring")}</p>
                    <div className="space-y-1.5">
                      <Label>{t("smimeKeyringEmail")}</Label>
                      <Input
                        value={smimeKeyringEmail}
                        onChange={(e) => setSmimeKeyringEmail(e.target.value)}
                        placeholder="alice@example.com"
                        className="text-sm"
                      />
                    </div>
                    <div className="space-y-1.5">
                      <Label>{t("smimeKeyringCert")}</Label>
                      <textarea
                        value={smimeKeyringCert}
                        onChange={(e) => setSmimeKeyringCert(e.target.value)}
                        rows={4}
                        placeholder="-----BEGIN CERTIFICATE-----"
                        className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs shadow-sm"
                      />
                    </div>
                    <Button
                      type="button"
                      size="sm"
                      onClick={onImportSmimeCert}
                      disabled={smimeKeyringImporting}
                    >
                      {smimeKeyringImporting ? t("smimeImporting") : t("smimeImport")}
                    </Button>
                    {smimeCerts === null && <p className="text-xs text-muted-foreground">{t("loading")}</p>}
                    {smimeCerts !== null && smimeCerts.length === 0 && (
                      <p className="text-xs text-muted-foreground">{t("smimeKeyringEmpty")}</p>
                    )}
                    <ul className="space-y-1.5">
                      {smimeCerts?.map((c) => (
                        <li key={c.id} className="flex items-center gap-2 rounded-md border border-border px-2.5 py-1.5">
                          <div className="min-w-0 flex-1">
                            <p className="truncate text-sm">{c.email}</p>
                            <p className="truncate font-mono text-[11px] text-muted-foreground">
                              {c.fingerprint.slice(0, 40)}
                            </p>
                          </div>
                          <Button
                            type="button"
                            size="sm"
                            variant="ghost"
                            className="text-muted-foreground hover:text-destructive"
                            onClick={() => onDeleteSmimeCert(c.id)}
                            disabled={smimeKeyringDeleting === c.id}
                          >
                            {smimeKeyringDeleting === c.id ? t("pgpDeleting") : t("webhookDelete")}
                          </Button>
                        </li>
                      ))}
                    </ul>
                  </div>
                </div>
              )}

              {section === "webhooks" && (
                <div key={section} className="h-full space-y-3 overflow-y-auto p-5">
                  <p className="text-sm font-medium">{t("webhooks")}</p>
                  <p className="text-xs text-muted-foreground">{t("webhookIntro")}</p>

                  {/* New webhook form */}
                  <div className="space-y-1.5 rounded-md border border-border p-3">
                    <div className="space-y-1.5">
                      <Label>{t("webhookUrl")}</Label>
                      <Input
                        value={whUrl}
                        onChange={(e) => setWhUrl(e.target.value)}
                        placeholder="https://example.com/hooks/mail"
                        className="text-sm"
                      />
                    </div>
                    <div className="space-y-1.5">
                      <Label>{t("webhookSecret")}</Label>
                      <Input
                        value={whSecret}
                        onChange={(e) => setWhSecret(e.target.value)}
                        placeholder={t("webhookSecretPlaceholder")}
                        className="text-sm font-mono"
                      />
                    </div>
                    <div className="space-y-1.5">
                      <Label>{t("webhookEvents")}</Label>
                      <Input
                        value={whEvents}
                        onChange={(e) => setWhEvents(e.target.value)}
                        placeholder="mail.received"
                        className="text-sm font-mono"
                      />
                    </div>
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        <Switch checked={whEnabled} onCheckedChange={setWhEnabled} />
                        <span className="text-sm text-muted-foreground">{t("webhookEnabled")}</span>
                      </div>
                      <Button type="button" size="sm" onClick={onAddWebhook} disabled={whSaving}>
                        {whSaving ? t("webhookSaving") : t("webhookAdd")}
                      </Button>
                    </div>
                  </div>

                  {whTestMsg && <p className="text-xs text-muted-foreground">{whTestMsg}</p>}

                  {/* Existing webhooks */}
                  {webhooks === null && <p className="text-xs text-muted-foreground">{t("loading")}</p>}
                  {webhooks !== null && webhooks.length === 0 && (
                    <p className="text-xs text-muted-foreground">{t("webhookEmpty")}</p>
                  )}
                  <ul className="space-y-1.5">
                    {webhooks?.map((w) => (
                      <li key={w.id} className="flex items-center gap-2 rounded-md border border-border px-2.5 py-1.5">
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2">
                            <p className="truncate text-sm">{w.url}</p>
                            <span
                              className={cn(
                                "rounded px-1.5 py-0.5 text-[10px] font-medium",
                                w.enabled
                                  ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
                                  : "bg-muted text-muted-foreground",
                              )}
                            >
                              {w.enabled ? t("webhookEnabled") : t("webhookDisabled")}
                            </span>
                          </div>
                          <p className="truncate font-mono text-[11px] text-muted-foreground">
                            {w.events}
                            {w.last_status > 0 && ` · ${t("webhookLast")} ${w.last_status}`}
                          </p>
                          {w.last_error && (
                            <p className="truncate text-[11px] text-destructive">{w.last_error}</p>
                          )}
                        </div>
                        <Switch
                          checked={w.enabled}
                          onCheckedChange={() => onToggleWebhook(w)}
                          aria-label={t("webhookEnabled")}
                        />
                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          onClick={() => onTestWebhook(w.id)}
                          disabled={whTesting === w.id}
                        >
                          {whTesting === w.id ? t("webhookTesting") : t("webhookTest")}
                        </Button>
                        <Button
                          type="button"
                          size="sm"
                          variant="ghost"
                          className="text-muted-foreground hover:text-destructive"
                          onClick={() => onDeleteWebhook(w.id)}
                        >
                          {t("webhookDelete")}
                        </Button>
                      </li>
                    ))}
                  </ul>
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

