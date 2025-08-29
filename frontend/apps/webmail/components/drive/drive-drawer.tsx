"use client";

import { useEffect, useState } from "react";
import { X } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { DriveView } from "@/components/drive/drive-view";
import type { DriveEntry } from "@/lib/api";
import { cn } from "@/lib/utils";

// DriveDrawer slides the cloud drive in from the right edge of the mailbox,
// mirroring the calendar drawer pattern. `focus` (from the workspace
// recent-files card) tells DriveView which file to navigate to and highlight.
export function DriveDrawer({
  focus,
  onClose,
}: {
  focus?: DriveEntry | null;
  onClose: () => void;
}) {
  const t = useTranslations("drive");
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
      <div className="absolute inset-0" onClick={onClose} aria-hidden="true" />
      <div
        className={cn(
          "absolute inset-y-0 right-0 flex w-full max-w-[720px] flex-col border-l border-border bg-background shadow-2xl transition-transform duration-200",
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
          <DriveView focus={focus} />
        </div>
      </div>
    </div>
  );
}
