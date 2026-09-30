"use client";

import { useTranslations } from "next-intl";
import { CalendarDays, Filter, HardDrive, Users } from "lucide-react";
import { Button } from "@/components/ui/button";
import { LocaleSwitcher } from "@/components/locale-switcher";
import { cn } from "@/lib/utils";

export function SidebarFooter({
  quotaPercent,
  quotaBarColor,
  onContacts,
  onCalendar,
  onDrive,
  onSieve,
}: {
  quotaPercent: number | null;
  quotaBarColor: string;
  onContacts: () => void;
  onCalendar: () => void;
  onDrive: () => void;
  onSieve: () => void;
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
        <Button variant="ghost" size="sm" onClick={onSieve} title={t("filterRules")}>
          <Filter className="size-4" />
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
