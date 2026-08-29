"use client";

import { useState } from "react";
import { Check, Pencil, Plus, Tag, Trash2, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import type { MailLabel } from "@/lib/api";
import { useMailStore } from "@/components/mailbox/mail-store";
import { LABEL_PALETTE, labelColor } from "@/components/mailbox/mail-utils";

function ColorSwatches({
  value,
  onPick,
}: {
  value: string;
  onPick: (color: string) => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-1.5 pl-6">
      {LABEL_PALETTE.map((c) => (
        <button
          key={c}
          type="button"
          onClick={() => onPick(c)}
          className="flex size-5 items-center justify-center rounded-full border border-border transition-transform hover:scale-110"
          style={{ backgroundColor: c }}
        >
          {value === c && <Check className="size-3 text-white" />}
        </button>
      ))}
    </div>
  );
}

export function LabelManager({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useTranslations("mail");
  // The store context is untyped (any); narrow the slice this dialog uses so
  // callbacks stay fully typed.
  const {
    labelDefs,
    knownLabels,
    saveLabel,
    renameLabel,
    deleteLabel,
  } = useMailStore() as {
    labelDefs: MailLabel[];
    knownLabels: string[];
    saveLabel: (name: string, color: string) => Promise<void>;
    renameLabel: (from: string, to: string) => Promise<void>;
    deleteLabel: (name: string) => Promise<void>;
  };

  const [error, setError] = useState("");
  const [newName, setNewName] = useState("");
  const [newColor, setNewColor] = useState(LABEL_PALETTE[0]);
  const [colorsOpen, setColorsOpen] = useState("");
  const [renaming, setRenaming] = useState("");
  const [renameTo, setRenameTo] = useState("");
  const [confirming, setConfirming] = useState("");

  // Clear editing state when the manager closes (render-phase adjustment —
  // React-recommended over an effect, catches every close path).
  const [prevOpen, setPrevOpen] = useState(open);
  if (open !== prevOpen) {
    setPrevOpen(open);
    if (!open) {
      setError("");
      setNewName("");
      setColorsOpen("");
      setRenaming("");
      setConfirming("");
    }
  }

  // Every label row is manageable: defined labels carry a persisted color,
  // while keywords seen on messages without a definition row (orphan tags)
  // start with the palette fallback. Editing an orphan (color or rename)
  // first creates its definition so the metadata has somewhere to live.
  const rows = [
    ...labelDefs.map((d) => ({ key: d.name, name: d.name, color: d.color })),
    ...knownLabels
      .filter((l) => !labelDefs.some((d) => d.name === l))
      .map((l) => ({ key: l, name: l, color: "" })),
  ].sort((a, b) => a.name.localeCompare(b.name));

  async function run(action: () => Promise<void>) {
    setError("");
    try {
      await action();
    } catch (e) {
      setError(e instanceof Error ? e.message : "label action failed");
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[80vh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Tag className="size-4" />
            {t("manageLabels")}
          </DialogTitle>
        </DialogHeader>

        {error && <p className="text-sm text-destructive">{error}</p>}

        <div className="space-y-1">
          {rows.length === 0 && (
            <p className="py-2 text-sm text-muted-foreground">{t("noLabels")}</p>
          )}
          {rows.map((label) => (
            <div key={label.key} className="rounded-lg px-1 py-1">
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  title={t("labelColor")}
                  onClick={() => setColorsOpen((v) => (v === label.key ? "" : label.key))}
                  className="size-3.5 shrink-0 rounded-full border border-border transition-transform hover:scale-110"
                  style={{ backgroundColor: labelColor(label.name, label.color) }}
                />
                {renaming === label.key ? (
                  <Input
                    autoFocus
                    value={renameTo}
                    onChange={(e) => setRenameTo(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && renameTo.trim() && renameTo.trim() !== label.name) {
                        run(async () => {
                          // Adopt an orphan keyword into a definition before
                          // renaming so the new name is persisted.
                          if (!labelDefs.some((d) => d.name === label.name)) {
                            await saveLabel(label.name, "");
                          }
                          await renameLabel(label.name, renameTo.trim());
                          setRenaming("");
                        });
                      }
                    }}
                    className="h-7 flex-1 text-sm"
                  />
                ) : (
                  <span className="min-w-0 flex-1 truncate text-sm">{label.name}</span>
                )}
                {renaming === label.key ? (
                  <Button
                    size="xs"
                    variant="ghost"
                    className="size-7 shrink-0 p-0"
                    onClick={() => {
                      if (renameTo.trim() && renameTo.trim() !== label.name) {
                        run(async () => {
                          if (!labelDefs.some((d) => d.name === label.name)) {
                            await saveLabel(label.name, "");
                          }
                          await renameLabel(label.name, renameTo.trim());
                          setRenaming("");
                        });
                      } else {
                        setRenaming("");
                      }
                    }}
                  >
                    <Check className="size-3.5" />
                  </Button>
                ) : (
                  <Button
                    size="xs"
                    variant="ghost"
                    title={t("renameLabel")}
                    className="size-7 shrink-0 p-0 text-muted-foreground"
                    onClick={() => {
                      setRenaming(label.key);
                      setRenameTo(label.name);
                    }}
                  >
                    <Pencil className="size-3.5" />
                  </Button>
                )}
                {confirming === label.key ? (
                  <Button
                    size="xs"
                    variant="ghost"
                    className="h-7 shrink-0 px-1.5 text-destructive"
                    onClick={() =>
                      run(async () => {
                        await deleteLabel(label.name);
                        setConfirming("");
                      })
                    }
                  >
                    <X className="size-3" />
                    {t("confirmDelete")}
                  </Button>
                ) : (
                  <Button
                    size="xs"
                    variant="ghost"
                    title={t("delete")}
                    className="size-7 shrink-0 p-0 text-muted-foreground hover:text-destructive"
                    onClick={() => setConfirming(label.key)}
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                )}
              </div>
              {colorsOpen === label.key && (
                <div className="mt-1.5">
                  <ColorSwatches
                    value={label.color}
                    onPick={(color) =>
                      run(async () => {
                        // Picking a color on an orphan keyword creates its
                        // definition so the choice persists.
                        await saveLabel(label.name, color);
                        setColorsOpen("");
                      })
                    }
                  />
                </div>
              )}
            </div>
          ))}
        </div>

        <div className="mt-3 border-t border-border pt-3">
          <div className="flex items-center gap-1.5">
            <button
              type="button"
              title={t("labelColor")}
              onClick={() =>
                setNewColor((c) => LABEL_PALETTE[(LABEL_PALETTE.indexOf(c) + 1) % LABEL_PALETTE.length])
              }
              className="size-5 shrink-0 rounded-full border border-border transition-transform hover:scale-110"
              style={{ backgroundColor: newColor }}
            />
            <Input
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && newName.trim()) {
                  run(async () => {
                    await saveLabel(newName.trim(), newColor);
                    setNewName("");
                  });
                }
              }}
              placeholder={t("newLabel")}
              className="h-8 flex-1 text-sm"
            />
            <Button
              size="sm"
              className="h-8 shrink-0 px-2"
              onClick={() => {
                if (newName.trim()) {
                  run(async () => {
                    await saveLabel(newName.trim(), newColor);
                    setNewName("");
                  });
                }
              }}
            >
              <Plus className="size-4" />
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
