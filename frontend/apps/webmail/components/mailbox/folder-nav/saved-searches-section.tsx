"use client";

import { useTranslations } from "next-intl";
import { Search, X } from "lucide-react";
import type { SavedSearch } from "@/components/mailbox/mail-utils";

export function SavedSearchesSection({
  savedSearches,
  onSelectSavedSearch,
  onRemoveSavedSearch,
  onClose,
}: {
  savedSearches: SavedSearch[];
  onSelectSavedSearch: (item: SavedSearch) => void;
  onRemoveSavedSearch: (id: number) => void;
  onClose: () => void;
}) {
  const t = useTranslations("mail");

  return (
    <div className="mt-3 border-t border-sidebar-border pt-2">
      <p className="px-2.5 pb-1 text-[10px] font-medium tracking-wide text-muted-foreground uppercase">
        {t("savedSearches")}
      </p>
      {savedSearches.map((item) => (
        <div
          key={item.id}
          className="group flex w-full items-center gap-1 rounded-lg px-1 transition-colors hover:bg-sidebar-accent/60"
        >
          <button
            onClick={() => {
              onSelectSavedSearch(item);
              onClose();
            }}
            className="min-w-0 flex-1 rounded-lg px-1.5 py-1.5 text-left text-sm text-sidebar-foreground transition-colors hover:text-foreground"
          >
            <span className="flex items-center gap-2 truncate">
              <Search className="size-3.5 shrink-0 opacity-60" />
              <span className="truncate">{item.name}</span>
            </span>
          </button>
          <button
            onClick={() => onRemoveSavedSearch(item.id)}
            title={t("delete")}
            className="rounded p-1 text-muted-foreground opacity-0 transition-opacity hover:text-destructive group-hover:opacity-100"
          >
            <X className="size-3.5" />
          </button>
        </div>
      ))}
    </div>
  );
}
