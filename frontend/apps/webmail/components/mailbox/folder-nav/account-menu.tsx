"use client";

import { useEffect } from "react";
import { useTranslations } from "next-intl";
import { ChevronsUpDown, Plus } from "lucide-react";
import type { MailAccount, MailDelegation } from "@/lib/api";
import { cn } from "@/lib/utils";

// Account switcher: own mailbox + delegated mailboxes + external accounts.
export function AccountMenu({
  email,
  currentAccountEmail,
  accountList,
  delegateList,
  activeAccount,
  activeDelegate,
  open,
  onOpenChange,
  onSwitchAccount,
  onSwitchDelegate,
  onManageAccounts,
}: {
  email: string;
  currentAccountEmail: string;
  accountList?: MailAccount[];
  delegateList?: MailDelegation[];
  activeAccount?: number | null;
  activeDelegate?: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSwitchAccount?: (id: number | null) => void;
  onSwitchDelegate?: (email: string | null) => void;
  onManageAccounts?: () => void;
}) {
  const t = useTranslations("mail");

  // Close the account switcher on Escape / outside click.
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onOpenChange(false);
    const onDown = (e: MouseEvent) => {
      if (!(e.target instanceof Element) || !e.target.closest("[data-account-menu]")) {
        onOpenChange(false);
      }
    };
    window.addEventListener("keydown", onKey);
    window.addEventListener("mousedown", onDown);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("mousedown", onDown);
    };
  }, [open, onOpenChange]);

  return (
    <div className="relative px-3 pb-2" data-account-menu>
      <button
        onClick={() => onOpenChange(!open)}
        className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-sm transition-colors hover:bg-sidebar-accent/60"
        title={currentAccountEmail}
      >
        <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary text-[10px] font-bold text-primary-foreground">
          {currentAccountEmail.charAt(0).toUpperCase()}
        </span>
        <span className="min-w-0 flex-1 truncate text-left">{currentAccountEmail}</span>
        <ChevronsUpDown className="size-3.5 shrink-0 opacity-50" />
      </button>
      {open && (
        <div className="absolute left-3 right-3 top-full z-30 mt-1 rounded-lg border border-border bg-popover p-1 text-sm shadow-lg">
          <button
            onClick={() => {
              onSwitchAccount?.(null);
              onOpenChange(false);
            }}
            className={cn(
              "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-muted",
              activeAccount === null && activeDelegate === null && "bg-muted font-medium",
            )}
          >
            <span className="min-w-0 flex-1 truncate">{email}</span>
            <span className="text-[10px] text-muted-foreground">{t("myAccount")}</span>
          </button>
          {delegateList && delegateList.length > 0 && (
            <div className="mt-0.5 border-t border-border pt-0.5">
              <p className="px-2 pb-1 pt-1 text-[10px] font-medium tracking-wide text-muted-foreground uppercase">
                {t("delegatedMailboxes")}
              </p>
              {delegateList.map((d) => (
                <button
                  key={d.id}
                  className={cn(
                    "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-muted",
                    activeDelegate === d.owner_email && "bg-muted font-medium",
                  )}
                  onClick={() => {
                    onSwitchDelegate?.(d.owner_email);
                    onOpenChange(false);
                  }}
                >
                  <span className="min-w-0 flex-1 truncate">{d.owner_email}</span>
                  <span className="rounded bg-accent px-1.5 py-px text-[10px] text-accent-foreground">
                    {t("delegated")}
                  </span>
                </button>
              ))}
            </div>
          )}
          {accountList?.map((a) => (
            <button
              key={a.id}
              className={cn(
                "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-muted",
                activeAccount === a.id && "bg-muted font-medium",
              )}
              onClick={() => {
                onSwitchAccount?.(a.id);
                onOpenChange(false);
              }}
            >
              <span className="min-w-0 flex-1 truncate">{a.email}</span>
              {!a.enabled && <span className="text-[10px] text-destructive">{t("accountOff")}</span>}
            </button>
          ))}
          <button
            className="mt-0.5 flex w-full items-center gap-2 rounded-md border-t border-border px-2 py-1.5 text-left text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            onClick={() => {
              onOpenChange(false);
              onManageAccounts?.();
            }}
          >
            <Plus className="size-3.5" />
            {t("manageAccounts")}
          </button>
        </div>
      )}
    </div>
  );
}
