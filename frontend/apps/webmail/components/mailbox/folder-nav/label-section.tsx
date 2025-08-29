"use client";

import { useTranslations } from "next-intl";
import { Settings2, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { labelColor } from "@/components/mailbox/mail-utils";
import { cn } from "@/lib/utils";

export function LabelSection({
  labels,
  labelColors,
  activeLabel,
  confirmLabel,
  onConfirmLabel,
  onSelectLabel,
  onClose,
  onManageLabels,
  onDeleteLabel,
}: {
  labels: string[];
  labelColors?: Record<string, string>;
  activeLabel?: string;
  confirmLabel: string | null;
  onConfirmLabel: (label: string | null) => void;
  onSelectLabel: (label: string) => void;
  onClose: () => void;
  onManageLabels?: () => void;
  onDeleteLabel?: (label: string) => void;
}) {
  const t = useTranslations("mail");

  return (
    <div className="mt-3 border-t border-sidebar-border pt-2">
      <div className="flex items-center justify-between pr-1">
        <p className="px-2.5 pb-1 text-[10px] font-medium tracking-wide text-muted-foreground uppercase">
          {t("labels")}
        </p>
        {onManageLabels && (
          <button
            onClick={onManageLabels}
            title={t("manageLabels")}
            className="rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
          >
            <Settings2 className="size-3.5" />
          </button>
        )}
      </div>
      {labels.map((label) => (
        <div key={label} className="group relative flex w-full items-center">
          <button
            onClick={() => {
              onSelectLabel(label);
              onClose();
            }}
            className={cn(
              "flex min-w-0 flex-1 items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm transition-colors",
              activeLabel === label
                ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
                : "text-sidebar-foreground hover:bg-sidebar-accent/60 hover:text-foreground",
            )}
          >
            <span
              className="size-2 shrink-0 rounded-full"
              style={{ backgroundColor: labelColor(label, labelColors?.[label]) }}
            />
            <span className="truncate">{label}</span>
          </button>
          {onDeleteLabel && (
            <div className="absolute right-1 top-1/2 z-30 -translate-y-1/2">
              <Button
                variant="ghost"
                size="icon-sm"
                title={t("delete")}
                className={cn(
                  "size-6 rounded-md",
                  confirmLabel === label
                    ? "opacity-100"
                    : "opacity-0 group-hover:opacity-100",
                )}
                onClick={() =>
                  onConfirmLabel(confirmLabel === label ? null : label)
                }
              >
                <Trash2 className="size-3.5" />
                <span className="sr-only">{t("delete")}</span>
              </Button>
              {confirmLabel === label && (
                <div className="absolute right-0 top-full z-30 mt-1 w-40 rounded-lg border border-border bg-popover p-1 text-sm shadow-lg">
                  <p className="px-2 py-1 text-xs text-muted-foreground">
                    {t("confirmDeleteLabel", { name: label })}
                  </p>
                  <div className="flex items-center gap-1">
                    <Button
                      size="xs"
                      variant="destructive"
                      className="flex-1"
                      onClick={() => {
                        onDeleteLabel(label);
                        onConfirmLabel(null);
                      }}
                    >
                      {t("delete")}
                    </Button>
                    <Button
                      size="xs"
                      variant="ghost"
                      onClick={() => onConfirmLabel(null)}
                    >
                      {t("cancel")}
                    </Button>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      ))}
      {labels.length === 0 && onManageLabels && (
        <button
          onClick={onManageLabels}
          className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-sidebar-accent/60 hover:text-foreground"
        >
          <span className="size-2 shrink-0 rounded-full border border-border" />
          {t("newLabel")}
        </button>
      )}
    </div>
  );
}
