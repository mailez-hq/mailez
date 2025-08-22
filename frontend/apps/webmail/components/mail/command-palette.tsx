"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { Dialog, DialogContent } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

export type PaletteAction = {
  id: string;
  label: string;
  keywords?: string;
  icon?: React.ReactNode;
  run: () => void;
};

export function CommandPalette({
  open,
  onOpenChange,
  actions,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  actions: PaletteAction[];
}) {
  const t = useTranslations("palette");
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState(0);

  useEffect(() => {
    if (open) {
      setQuery("");
      setSelected(0);
    }
  }, [open]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return actions;
    return actions.filter(
      (a) =>
        a.label.toLowerCase().includes(q) ||
        (a.keywords || "").toLowerCase().includes(q),
    );
  }, [query, actions]);

  function run(action: PaletteAction | undefined) {
    if (!action) return;
    onOpenChange(false);
    action.run();
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="top-[18%] max-w-md gap-2 p-0 sm:max-w-md"
      >
        <div className="border-b border-border p-2">
          <Input
            autoFocus
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setSelected(0);
            }}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown") {
                e.preventDefault();
                setSelected((s) => Math.min(s + 1, filtered.length - 1));
              } else if (e.key === "ArrowUp") {
                e.preventDefault();
                setSelected((s) => Math.max(s - 1, 0));
              } else if (e.key === "Enter") {
                e.preventDefault();
                run(filtered[selected]);
              }
            }}
            placeholder={t("placeholder")}
            className="h-9 border-0 bg-transparent px-2 shadow-none focus-visible:ring-0"
          />
        </div>
        <div className="max-h-80 overflow-y-auto p-1.5">
          {filtered.map((a, i) => (
            <button
              key={a.id}
              onClick={() => run(a)}
              onMouseEnter={() => setSelected(i)}
              className={cn(
                "flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm transition-colors",
                i === selected ? "bg-accent text-accent-foreground" : "text-foreground",
              )}
            >
              {a.icon && <span className="shrink-0 opacity-70">{a.icon}</span>}
              <span className="truncate">{a.label}</span>
            </button>
          ))}
          {filtered.length === 0 && (
            <p className="p-3 text-sm text-muted-foreground">{t("noResults")}</p>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
