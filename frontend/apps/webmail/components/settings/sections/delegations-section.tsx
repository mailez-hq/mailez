"use client";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import type { DelegationListing, MailDelegation } from "@/lib/api";

// DelegationsSection shows who may act on this mailbox and on whose mailbox
// this user may act, plus the grant form.
export function DelegationsSection({
  t,
  delegationList,
  delEmail, setDelEmail,
  delCanSend, setDelCanSend,
  delFullAccess, setDelFullAccess,
  delSaving,
  onAddDelegation,
  onUpdateDelegation,
  onDeleteDelegation,
}: {
  t: (key: string) => string;
  delegationList: DelegationListing | null;
  delEmail: string; setDelEmail: (v: string) => void;
  delCanSend: boolean; setDelCanSend: (v: boolean) => void;
  delFullAccess: boolean; setDelFullAccess: (v: boolean) => void;
  delSaving: boolean;
  onAddDelegation: () => void;
  onUpdateDelegation: (d: MailDelegation) => void;
  onDeleteDelegation: (id: number) => void;
}) {
  return (
    <div className="h-full space-y-5 overflow-y-auto p-5">
      <p className="text-sm font-medium">{t("delegations")}</p>
      <p className="text-xs text-muted-foreground">{t("delegationIntro")}</p>

      <div className="space-y-2">
        <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
          {t("delegationGranted")}
        </p>
        {delegationList === null && <p className="text-sm text-muted-foreground">{t("loading")}</p>}
        {delegationList !== null && delegationList.granted.length === 0 && (
          <p className="text-sm text-muted-foreground">{t("delegationGrantedEmpty")}</p>
        )}
        {(delegationList?.granted || []).map((d) => (
          <div key={d.id} className="rounded-lg border border-border p-3">
            <div className="flex items-center gap-2">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{d.delegate_email}</p>
                <p className="truncate text-xs text-muted-foreground">
                  {d.delegate_name && <>{d.delegate_name} · </>}
                  {d.full_access
                    ? t("delegationFull")
                    : d.can_send
                      ? t("delegationSendOnly")
                      : ""}
                </p>
              </div>
              <div className="flex items-center gap-2">
                <div className="flex items-center gap-1.5">
                  <Switch
                    checked={d.can_send}
                    disabled={d.full_access}
                    onCheckedChange={() => onUpdateDelegation({ ...d, can_send: !d.can_send })}
                  />
                  <span className="text-xs text-muted-foreground">{t("delegationCanSend")}</span>
                </div>
                <div className="flex items-center gap-1.5">
                  <Switch
                    checked={d.full_access}
                    onCheckedChange={() => onUpdateDelegation({ ...d, full_access: !d.full_access })}
                  />
                  <span className="text-xs text-muted-foreground">{t("delegationFullAccess")}</span>
                </div>
                <Button variant="ghost" size="sm" className="text-destructive" onClick={() => onDeleteDelegation(d.id)}>
                  {t("delegationRevoke")}
                </Button>
              </div>
            </div>
          </div>
        ))}
      </div>

      <div className="space-y-2.5 rounded-lg border border-border p-3">
        <p className="text-sm font-medium">{t("delegationAdd")}</p>
        <div className="space-y-1">
          <Label>{t("delegationDelegate")}</Label>
          <Input
            value={delEmail}
            onChange={(e) => setDelEmail(e.target.value)}
            placeholder="teammate@example.com"
          />
        </div>
        <div className="flex flex-wrap items-center gap-4">
          <div className="flex items-center gap-1.5">
            <Switch
              checked={delCanSend}
              disabled={delFullAccess}
              onCheckedChange={setDelCanSend}
            />
            <span className="text-sm text-muted-foreground">{t("delegationCanSend")}</span>
          </div>
          <div className="flex items-center gap-1.5">
            <Switch checked={delFullAccess} onCheckedChange={setDelFullAccess} />
            <span className="text-sm text-muted-foreground">{t("delegationFullAccess")}</span>
          </div>
        </div>
        <p className="text-xs text-muted-foreground">{t("delegationFullHint")}</p>
        <Button disabled={delSaving || !delEmail.trim()} onClick={onAddDelegation}>
          {delSaving ? t("delegationSaving") : t("delegationGrant")}
        </Button>
      </div>

      <div className="space-y-2">
        <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
          {t("delegationReceived")}
        </p>
        {delegationList !== null && delegationList.received.length === 0 && (
          <p className="text-sm text-muted-foreground">{t("delegationReceivedEmpty")}</p>
        )}
        {(delegationList?.received || []).map((d) => (
          <div key={d.id} className="rounded-lg border border-border p-3">
            <p className="truncate text-sm font-medium">
              {d.owner_email}
              {d.owner_name && <span className="text-xs text-muted-foreground"> · {d.owner_name}</span>}
            </p>
            <p className="text-xs text-muted-foreground">
              {d.full_access ? t("delegationFull") : t("delegationSendOnly")}
            </p>
          </div>
        ))}
      </div>
    </div>
  );
}
