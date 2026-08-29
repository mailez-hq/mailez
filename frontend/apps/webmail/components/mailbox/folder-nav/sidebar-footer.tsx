"use client";

import { useTranslations } from "next-intl";
import {
  CalendarDays,
  Ellipsis,
  ExternalLink,
  Filter,
  HardDrive,
  LogOut,
  Settings,
  Users,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { LocaleSwitcher } from "@/components/locale-switcher";
import { cn } from "@/lib/utils";

// The admin console lives behind the same nginx front in deployments (/admin);
// local dev can override with NEXT_PUBLIC_ADMIN_URL (e.g. http://localhost:3000).
const ADMIN_URL = process.env.NEXT_PUBLIC_ADMIN_URL || "/admin";

export function SidebarFooter({
  quotaPercent,
  quotaBarColor,
  onContacts,
  onCalendar,
  onDrive,
  onSettings,
  onSieve,
  onLogout,
}: {
  quotaPercent: number | null;
  quotaBarColor: string;
  onContacts: () => void;
  onCalendar: () => void;
  onDrive: () => void;
  onSettings: () => void;
  onSieve: () => void;
  onLogout: () => void;
}) {
  const t = useTranslations("mail");

  return (
    <div className="border-t border-sidebar-border p-2">
      <div className="mb-1 flex items-center justify-center">
        <LocaleSwitcher />
      </div>
      {/* Divider between the language switcher and the user info /
          account controls below. */}
      <div className="mx-2 mb-1.5 border-t border-sidebar-border" />
      <div className="mb-1 flex items-center justify-center gap-0.5">
        <Button variant="ghost" size="sm" onClick={onContacts} title={t("contacts")}>
          <Users className="size-4" />
        </Button>
        <Button variant="ghost" size="sm" onClick={onCalendar} title={t("calendar")}>
          <CalendarDays className="size-4" />
        </Button>
        <Button variant="ghost" size="sm" onClick={onDrive} title={t("drive")}>
          <HardDrive className="size-4" />
        </Button>
        <Button variant="ghost" size="sm" onClick={onSettings} title={t("settings")}>
          <Settings className="size-4" />
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button variant="ghost" size="sm" title={t("more")}>
                <Ellipsis className="size-4" />
              </Button>
            }
          />
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={onSieve}>
              <Filter className="size-4" />
              {t("filterRules")}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => window.open(ADMIN_URL, "_blank", "noopener,noreferrer")}
            >
              <ExternalLink className="size-4" />
              {t("adminConsole")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <Button variant="ghost" size="sm" onClick={onLogout} title={t("logout")}>
          <LogOut className="size-4" />
        </Button>
      </div>
      {quotaPercent !== null && (
        <div className="mt-1.5 px-3">
          <div className="h-1 w-full overflow-hidden rounded-full bg-sidebar-accent">
            <div
              className={cn("h-full rounded-full transition-all", quotaBarColor)}
              style={{ width: `${quotaPercent}%` }}
            />
          </div>
          <p className="mt-1 text-center text-[10px] text-muted-foreground">
            {t("quotaUsed", { percent: quotaPercent })}
          </p>
        </div>
      )}
    </div>
  );
}
