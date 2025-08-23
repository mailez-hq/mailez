"use client";

import { useEffect } from "react";
import { CalendarClock, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { useMailStore } from "@/components/mailbox/mail-store";

// ScheduledDialog lists the user's queued scheduled sends with a cancel
// button per row. Data is loaded fresh each time the dialog opens.
export function ScheduledDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useTranslations("mail");
  const { scheduled, scheduledLoading, loadScheduled, cancelScheduled } = useMailStore() as {
    scheduled: { id: number; subject: string; send_at: string; recipients: string[] }[];
    scheduledLoading: boolean;
    loadScheduled: () => Promise<void>;
    cancelScheduled: (id: number) => Promise<void>;
  };

  useEffect(() => {
    if (open) loadScheduled();
  }, [open, loadScheduled]);

  const fmt = (iso: string) =>
    new Date(iso).toLocaleString(undefined, {
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[80vh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <CalendarClock className="size-4" />
            {t("scheduled")}
          </DialogTitle>
        </DialogHeader>

        {scheduledLoading ? (
          <p className="py-4 text-sm text-muted-foreground">{t("loading")}</p>
        ) : scheduled.length === 0 ? (
          <p className="py-4 text-sm text-muted-foreground">{t("scheduledEmpty")}</p>
        ) : (
          <div className="space-y-1">
            {scheduled.map((s) => (
              <div
                key={s.id}
                className="group flex items-center gap-2 rounded-lg border border-border px-2.5 py-2"
              >
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">
                    {s.subject || t("noSubject")}
                  </p>
                  <p className="truncate text-xs text-muted-foreground">
                    {t("scheduledAt", { time: fmt(s.send_at) })}
                    <span className="mx-1">·</span>
                    {s.recipients.join(", ")}
                  </p>
                </div>
                <Button
                  size="xs"
                  variant="ghost"
                  title={t("cancelSchedule")}
                  className="size-7 shrink-0 p-0 text-muted-foreground hover:text-destructive"
                  onClick={() => cancelScheduled(s.id)}
                >
                  <X className="size-3.5" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
