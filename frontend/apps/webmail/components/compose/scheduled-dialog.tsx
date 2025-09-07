"use client";

import { useEffect, useState } from "react";
import { CalendarClock, Pencil, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { useMailStore } from "@/components/mailbox/mail-store";
import { mailScheduledToDraft, type ScheduledSend } from "@/lib/api";

// ScheduledDialog lists the user's queued scheduled sends: each row can be
// moved back to Drafts for viewing/editing (modern-style) or cancelled.
export function ScheduledDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useTranslations("mail");
  const { scheduled, scheduledLoading, loadScheduled, cancelScheduled } = useMailStore() as {
    scheduled: ScheduledSend[];
    scheduledLoading: boolean;
    loadScheduled: () => Promise<ScheduledSend[]>;
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

  const [toDraftBusy, setToDraftBusy] = useState<number | null>(null);

  // "Edit": pull the scheduled send back into Drafts (the raw message is
  // restored verbatim), where the normal draft flow lets the user view and
  // edit it, then reschedule or send.
  async function moveToDraft(s: ScheduledSend) {
    setToDraftBusy(s.id);
    try {
      await mailScheduledToDraft(s.id);
      await loadScheduled();
    } catch {
      // row stays visible; a failed call leaves the schedule intact
    } finally {
      setToDraftBusy(null);
    }
  }

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
                  title={t("toDraft")}
                  disabled={toDraftBusy === s.id}
                  className="size-7 shrink-0 p-0 text-muted-foreground hover:text-foreground"
                  onClick={() => moveToDraft(s)}
                >
                  <Pencil className="size-3.5" />
                </Button>
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
