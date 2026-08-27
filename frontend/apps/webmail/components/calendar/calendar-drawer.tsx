"use client";

import { useEffect, useState } from "react";
import { X } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { CalendarView } from "@/components/calendar/calendar-view";
import { cn } from "@/lib/utils";

// CalendarDrawer slides the calendar in from the right edge of the mailbox
// (Gmail-style side panel) so the user never leaves the message view.
export function CalendarDrawer({ onClose }: { onClose: () => void }) {
  const t = useTranslations("calendar");
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const raf = requestAnimationFrame(() => setVisible(true));
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener("keydown", onKey);
    };
  }, [onClose]);

  return (
    <div className="fixed inset-0 z-40" role="presentation">
      {/* Click-away scrim: transparent so the mailbox stays visible. */}
      <div className="absolute inset-0" onClick={onClose} aria-hidden="true" />
      <div
        className={cn(
          "absolute inset-y-0 right-0 flex w-full max-w-[600px] flex-col border-l border-border bg-background shadow-2xl transition-transform duration-200",
          visible ? "translate-x-0" : "translate-x-full",
        )}
        role="dialog"
        aria-modal="false"
        aria-label={t("title")}
      >
        <div className="flex shrink-0 items-center justify-between border-b px-3 py-2">
          <h2 className="text-sm font-semibold">{t("title")}</h2>
          <Button variant="ghost" size="icon-sm" onClick={onClose} title={t("close")}>
            <X className="size-4" />
          </Button>
        </div>
        <div className="min-h-0 flex-1">
          <CalendarView />
        </div>
      </div>
    </div>
  );
}

