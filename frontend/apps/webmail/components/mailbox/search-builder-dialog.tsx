"use client";

import { useMemo, useState } from "react";
import { Plus, SlidersHorizontal, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

type Field =
  | "from" | "to" | "subject" | "text" | "label"
  | "hasAttachment" | "unread" | "flagged" | "before" | "after";

type Condition = { id: number; field: Field; value: string };

const VALUE_FIELDS: Record<Field, boolean> = {
  from: true, to: true, subject: true, text: true, label: true,
  hasAttachment: false, unread: false, flagged: false,
  before: true, after: true,
};

let nextId = 1;

// compileConditions converts the visual conditions into the same query
// expression the search box accepts (AND semantics, matching SearchQuery).
function compileConditions(conditions: Condition[]): string {
  const parts: string[] = [];
  for (const c of conditions) {
    const v = c.value.trim();
    switch (c.field) {
      case "hasAttachment":
        parts.push("has:attachment");
        break;
      case "unread":
        parts.push("is:unread");
        break;
      case "flagged":
        parts.push("is:flagged");
        break;
      case "before":
      case "after":
        if (/^\d{4}-\d{2}-\d{2}$/.test(v)) parts.push(`${c.field}:${v}`);
        break;
      case "text":
        // bare words are ANDed by the text parser; split a multi-word phrase
        if (v) for (const w of v.split(/\s+/)) parts.push(w);
        break;
      default:
        if (v) parts.push(`${c.field}:"${v.replace(/"/g, "")}"`);
    }
  }
  return parts.join(" ");
}

export function SearchBuilderDialog({
  open,
  onOpenChange,
  onApply,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onApply: (query: string) => void;
}) {
  const t = useTranslations("mail");
  const [conditions, setConditions] = useState<Condition[]>([]);

  const fieldLabel = (f: Field) => t(`sb${f.charAt(0).toUpperCase()}${f.slice(1)}`);

  const query = useMemo(() => compileConditions(conditions), [conditions]);

  const update = (id: number, patch: Partial<Condition>) =>
    setConditions((cs) => cs.map((c) => (c.id === id ? { ...c, ...patch } : c)));

  const addCondition = () =>
    setConditions((cs) => [...cs, { id: nextId++, field: "from", value: "" }]);

  const runSearch = () => {
    onApply(query);
    onOpenChange(false);
  };

  const clearAll = () => setConditions([]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <SlidersHorizontal className="size-4" />
            {t("advancedSearch")}
          </DialogTitle>
        </DialogHeader>

        <p className="text-xs text-muted-foreground">{t("sbHint")}</p>

        {conditions.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("sbEmpty")}</p>
        ) : (
          <div className="space-y-1.5">
            {conditions.map((c) => (
              <div key={c.id} className="flex items-center gap-1.5">
                <select
                  value={c.field}
                  onChange={(e) =>
                    update(c.id, { field: e.target.value as Field, value: "" })
                  }
                  className="h-8 w-32 shrink-0 rounded-md border border-border bg-transparent px-1.5 text-sm text-foreground outline-none focus-visible:border-ring"
                >
                  {(
                    ["from", "to", "subject", "text", "label", "hasAttachment", "unread", "flagged", "before", "after"] as Field[]
                  ).map((f) => (
                    <option key={f} value={f}>
                      {fieldLabel(f)}
                    </option>
                  ))}
                </select>
                {VALUE_FIELDS[c.field] ? (
                  <Input
                    type={c.field === "before" || c.field === "after" ? "date" : "text"}
                    value={c.value}
                    onChange={(e) => update(c.id, { value: e.target.value })}
                    placeholder={fieldLabel(c.field)}
                    className="h-8 flex-1 text-sm"
                  />
                ) : (
                  <span className="flex-1 text-sm text-muted-foreground">
                    {fieldLabel(c.field)}
                  </span>
                )}
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  title={t("delete")}
                  className="size-7 shrink-0 p-0 text-muted-foreground hover:text-destructive"
                  onClick={() =>
                    setConditions((cs) => cs.filter((x) => x.id !== c.id))
                  }
                >
                  <X className="size-3.5" />
                </Button>
              </div>
            ))}
          </div>
        )}

        <Button type="button" variant="outline" size="sm" onClick={addCondition}>
          <Plus className="size-3.5" />
          {t("sbAddCondition")}
        </Button>

        <div className="space-y-1.5">
          <p className="text-xs text-muted-foreground">{t("sbPreview")}</p>
          <p
            className={cn(
              "min-h-8 rounded-md border border-border bg-muted/40 px-2.5 py-1.5 font-mono text-xs break-words",
              query ? "text-foreground" : "text-muted-foreground",
            )}
          >
            {query || "—"}
          </p>
        </div>

        <DialogFooter>
          <Button type="button" variant="ghost" onClick={clearAll} disabled={conditions.length === 0}>
            {t("sbClear")}
          </Button>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {t("cancel")}
          </Button>
          <Button type="button" onClick={runSearch} disabled={!query}>
            {t("sbSearch")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
