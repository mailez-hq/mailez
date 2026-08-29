"use client";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import type { MailAccount } from "@/lib/api";
import { Segmented } from "./segmented";

// AccountsSection lists external aggregation accounts and the add/edit form.
export function AccountsSection({
  t,
  accountList,
  accName, setAccName,
  accEmail, setAccEmail,
  accImapHost, setAccImapHost,
  accImapPort, setAccImapPort,
  accImapSecurity, setAccImapSecurity,
  accSmtpHost, setAccSmtpHost,
  accSmtpSecurity, setAccSmtpSecurity,
  accUsername, setAccUsername,
  accPassword, setAccPassword,
  accSaving,
  accTesting,
  accTestMsg,
  onAddAccount,
  editingAccount,
  onStartEditAccount,
  onCancelEditAccount,
  onDeleteAccount,
  onToggleAccount,
  onTestAccount,
}: {
  t: (key: string) => string;
  accountList: MailAccount[] | null;
  accName: string; setAccName: (v: string) => void;
  accEmail: string; setAccEmail: (v: string) => void;
  accImapHost: string; setAccImapHost: (v: string) => void;
  accImapPort: number; setAccImapPort: (v: number) => void;
  accImapSecurity: string; setAccImapSecurity: (v: string) => void;
  accSmtpHost: string; setAccSmtpHost: (v: string) => void;
  accSmtpSecurity: string; setAccSmtpSecurity: (v: string) => void;
  accUsername: string; setAccUsername: (v: string) => void;
  accPassword: string; setAccPassword: (v: string) => void;
  accSaving: boolean;
  accTesting: number | null;
  accTestMsg: string;
  onAddAccount: () => void;
  editingAccount: MailAccount | null;
  onStartEditAccount: (a: MailAccount) => void;
  onCancelEditAccount: () => void;
  onDeleteAccount: (id: number) => void;
  onToggleAccount: (a: MailAccount) => void;
  onTestAccount: (id: number) => void;
}) {
  return (
    <div className="h-full space-y-5 overflow-y-auto p-5">
      <div className="flex items-center justify-between">
        <p className="text-sm font-medium">
          {editingAccount ? t("accountEditTitle") : t("accounts")}
        </p>
        {editingAccount && (
          <Button variant="ghost" size="xs" onClick={onCancelEditAccount}>
            {t("accountCancelEdit")}
          </Button>
        )}
      </div>
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
              <Button variant="outline" size="sm" onClick={() => onStartEditAccount(a)}>
                {t("accountEdit")}
              </Button>
              <Button variant="ghost" size="sm" className="text-destructive" onClick={() => onDeleteAccount(a.id)}>
                {t("accountDelete")}
              </Button>
            </div>
          </div>
        ))}
      </div>

      <div className="space-y-2.5 rounded-lg border border-border p-3">
        <p className="text-sm font-medium">
          {editingAccount ? t("accountEditTitle") : t("accountAdd")}
        </p>
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
          {accSaving
            ? t("accountSaving")
            : editingAccount
              ? t("accountSave")
              : t("accountAdd")}
        </Button>
      </div>
    </div>
  );
}
