"use client";

import { useEffect, useMemo, useState } from "react";
import { Bookmark, Plus, SlidersHorizontal, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from "@/components/ui/dialog";
import type { MailSearchSpec } from "@/lib/api";

// Guided search builder: users tap a condition ("发件人", "有附件", ...),
// fill in one value at a time, and the result is a structured MailSearchSpec
// POSTed straight to /mail/search — no syntax strings anywhere.

type Field =
  | "from" | "to" | "subject" | "label"
  | "hasAttachment" | "unread" | "flagged" | "before" | "after";

type Condition = { id: number; field: Field; value: string };

const BOOLEAN_FIELDS: Field[] = ["hasAttachment", "unread", "flagged"];
const DATE_FIELDS: Field[] = ["before", "after"];
const ALL_FIELDS: Field[] = [
  "from", "to", "subject", "hasAttachment", "unread", "flagged", "before", "after", "label",
];

let nextId = 1;

// toSpec converts the selected conditions into a structured search spec.
// Dates are sent as RFC 3339 (the backend SearchQuery unmarshals time.Time).
function toSpec(conditions: Condition[]): MailSearchSpec {
  const spec: MailSearchSpec = {};
  const push = (key: "from" | "to" | "subject" | "labels", v: string) => {
    if (!v.trim()) return;
    if (key === "labels") {
      spec.labels = [...(spec.labels ?? []), v.trim()];
    } else {
      spec[key] = [...(spec[key] ?? []), v.trim()];
    }
  };
  for (const c of conditions) {
    switch (c.field) {
      case "from":
        push("from", c.value);
        break;
      case "to":
        push("to", c.value);
        break;
      case "subject":
        push("subject", c.value);
        break;
      case "label":
        push("labels", c.value);
        break;
      case "hasAttachment":
        spec.hasAttachment = true;
        break;
      case "unread":
        spec.unseen = true;
        break;
      case "flagged":
        spec.flagged = true;
        break;
      case "before":
        if (c.value) spec.before = new Date(c.value + "T00:00:00Z").toISOString();
        break;
      case "after":
        if (c.value) spec.after = new Date(c.value + "T00:00:00Z").toISOString();
        break;
    }
  }
  return spec;
}

export function SearchBuilderDialog({
  open,
  onOpenChange,
  onApply,
  onSave,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onApply: (spec: MailSearchSpec) => void;
  onSave: (name: string, spec: MailSearchSpec) => void;
}) {
  const t = useTranslations("mail");
  const [conditions, setConditions] = useState<Condition[]>([]);
  const [editing, setEditing] = useState<Field | null>(null);
  const [draft, setDraft] = useState("");
  const [saveName, setSaveName] = useState("");

  // Reset local editing state each time the dialog opens: without this, a
  // condition committed in a previous session (e.g. From) silently leaks
  // into the next search and into saved searches.
  useEffect(() => {
    if (open) {
      setConditions([]);
      setEditing(null);
      setDraft("");
      setSaveName("");
    }
  }, [open]);

  const fieldLabel = (f: Field) => t(`sb${f.charAt(0).toUpperCase()}${f.slice(1)}`);
  const activeCount = useMemo(() => conditions.length, [conditions]);

  const startEdit = (f: Field) => {
    if (editing === f) {
      setEditing(null);
      setDraft("");
      return;
    }
    setEditing(f);
    setDraft("");
  };

  // Boolean conditions behave like toggles: tapping an active one removes it
  // instead of stacking a duplicate chip.
  const toggleBoolean = (f: Field) => {
    setConditions((cs) => {
      if (cs.some((c) => c.field === f)) {
        return cs.filter((c) => c.field !== f);
      }
      return [...cs, { id: nextId++, field: f, value: "" }];
    });
  };

  const isActive = (f: Field) => conditions.some((c) => c.field === f);

  const commitInput = () => {
    if (!editing) return;
    if (draft.trim()) {
      setConditions((cs) => [...cs, { id: nextId++, field: editing, value: draft.trim() }]);
    }
    setDraft("");
    setEditing(null);
  };

  const remove = (id: number) =>
    setConditions((cs) => cs.filter((c) => c.id !== id));

  const clearAll = () => {
    setConditions([]);
    setEditing(null);
    setDraft("");
    setSaveName("");
  };

  const apply = () => {
    // Commit any pending draft first so a typed value is never silently
    // dropped when the user hits Apply while editing a condition.
    const finalConditions =
      editing && draft.trim()
        ? [...conditions, { id: nextId++, field: editing, value: draft.trim() }]
        : conditions;
    onApply(toSpec(finalConditions));
    onOpenChange(false);
  };

  // save persists the current conditions as a named quick entry in the
  // sidebar; it also closes the dialog so the new entry is visible.
  const save = () => {
    if (!saveName.trim() || activeCount === 0) return;
    onSave(saveName.trim(), toSpec(conditions));
    setSaveName("");
    onOpenChange(false);
  };

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

        {/* Condition buttons — tap one, type a value, hit Enter */}
        <div className="flex flex-wrap gap-1.5">
          {ALL_FIELDS.map((f) => (
            <Button
              key={f}
              type="button"
              size="sm"
              variant={editing === f ? "default" : isActive(f) ? "secondary" : "outline"}
              className="h-7 gap-1 px-2 text-xs"
              onClick={() => (BOOLEAN_FIELDS.includes(f) ? toggleBoolean(f) : startEdit(f))}
            >
              {fieldLabel(f)}
              {editing === f && <X className="size-3" />}
            </Button>
          ))}
        </div>

        {/* Inline input for the active value condition */}
        {editing && (
          <div className="flex items-center gap-1.5">
            <span className="shrink-0 text-sm font-medium">{fieldLabel(editing)}</span>
            <Input
              autoFocus
              type={DATE_FIELDS.includes(editing) ? "date" : "text"}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") commitInput();
                if (e.key === "Escape") {
                  setEditing(null);
                  setDraft("");
                }
              }}
              placeholder={DATE_FIELDS.includes(editing) ? "2026-01-01" : fieldLabel(editing)}
              className="h-8 flex-1 text-sm"
            />
            <Button type="button" size="sm" className="h-8" onClick={commitInput}>
              {t("sbAdd")}
            </Button>
          </div>
        )}

        {/* Selected conditions as removable chips */}
        {conditions.length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            {conditions.map((c) => (
              <span
                key={c.id}
                className="inline-flex items-center gap-1 rounded-full border border-border bg-muted px-2.5 py-1 text-xs text-foreground"
              >
                <span className="text-muted-foreground">{fieldLabel(c.field)}</span>
                {c.value ? (
                  <span className="font-medium">{c.value}</span>
                ) : (
                  <Plus className="size-3" />
                )}
                <button
                  type="button"
                  onClick={() => remove(c.id)}
                  className="rounded-full p-0.5 text-muted-foreground transition-colors hover:bg-muted-foreground/20 hover:text-foreground"
                  aria-label={t("delete")}
                >
                  <X className="size-3" />
                </button>
              </span>
            ))}
          </div>
        )}

        <DialogFooter className="flex items-center justify-between gap-2">
          <div className="flex min-w-0 flex-1 items-center gap-1.5">
            {activeCount > 0 ? (
              <>
                <Button type="button" variant="ghost" size="sm" onClick={clearAll}>
                  {t("sbClear")}
                </Button>
                <Input
                  value={saveName}
                  onChange={(e) => setSaveName(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && saveName.trim()) save();
                  }}
                  placeholder={t("saveSearchName")}
                  className="h-8 min-w-24 flex-1 text-sm"
                />
                <Button type="button" size="sm" onClick={save} disabled={!saveName.trim()}>
                  <Bookmark className="size-3.5" />
                  {t("saveSearch")}
                </Button>
              </>
            ) : (
              <span />
            )}
          </div>
          <Button type="button" onClick={apply} disabled={activeCount === 0}>
            {t("sbApply")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
