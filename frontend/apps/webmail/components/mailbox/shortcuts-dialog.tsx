"use client";

import { useTranslations } from "next-intl";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";

export function ShortcutsDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const t = useTranslations("shortcuts");
  const rows: [string, string][] = [
    ["j", t("navDown")],
    ["k", t("navUp")],
    ["Enter / o", t("open")],
    ["x", t("select")],
    ["#", t("delete")],
    ["s", t("star")],
    ["e", t("archive")],
    ["!", t("spam")],
    ["Shift+I", t("markRead")],
    ["Shift+U", t("markUnread")],
    ["r", t("reply")],
    ["a", t("replyAll")],
    ["f", t("forward")],
    ["/", t("search")],
    ["Esc", t("close")],
    ["n", t("newMessage")],
    ["u", t("back")],
    ["⌘K / Ctrl+K", t("palette")],
    ["?", t("shortcuts")],
    ["g i", t("goInbox")],
    ["g s", t("goSent")],
  ];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
        </DialogHeader>
        <ScrollArea className="max-h-80">
          {/* Right padding keeps the shortcut keys clear of the overlay
              scrollbar, which otherwise covers the last few pixels of every
              kbd. */}
          <div className="divide-y divide-border pr-4">
            {rows.map(([key, label]) => (
              <div key={key} className="flex min-w-0 items-center justify-between gap-4 py-2">
                <span className="min-w-0 flex-1 truncate text-sm text-muted-foreground">
                  {label}
                </span>
                <kbd className="shrink-0 rounded border border-border bg-muted px-2 py-0.5 font-mono text-xs">
                  {key}
                </kbd>
              </div>
            ))}
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  );
}
