"use client";

import { useEffect, useState } from "react";
import { Check, Pencil, Plus, Tag, Trash2, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
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
  const { labelDefs, knownLabels, labelColors, saveLabel, renameLabel, deleteLabel } = useMailStore();

  const [error, setError] = useState("");
  const [newName, setNewName] = useState("");
  const [newColor, setNewColor] = useState(LABEL_PALETTE[0]);
  const [colorsOpen, setColorsOpen] = useState("");
  const [renaming, setRenaming] = useState("");
  const [renameTo, setRenameTo] = useState("");
  const [confirming, setConfirming] = useState("");

  useEffect(() => {
    if (!open) {
      setError("");
      setNewName("");
      setColorsOpen("");
      setRenaming("");
      setConfirming("");
    }
  }, [open]);

  // Definitions first (alphabetical), then keywords seen on messages that
  // have no definition row yet.
  const defs = [...labelDefs].sort((a, b) => a.name.localeCompare(b.name));
  const extras = knownLabels.filter((l) => !labelDefs.some((d) => d.name === l));

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
          {defs.length === 0 && extras.length === 0 && (
            <p className="py-2 text-sm text-muted-foreground">{t("noLabels")}</p>
          )}
          {defs.map((label) => (
            <div key={label.name} className="rounded-lg px-1 py-1">
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  title={t("labelColor")}
                  onClick={() => setColorsOpen((v) => (v === label.name ? "" : label.name))}
                  className="size-3.5 shrink-0 rounded-full border border-border transition-transform hover:scale-110"
                  style={{ backgroundColor: labelColor(label.name, label.color) }}
                />
                {renaming === label.name ? (
                  <Input
                    autoFocus
                    value={renameTo}
                    onChange={(e) => setRenameTo(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && renameTo.trim() && renameTo.trim() !== label.name) {
                        run(async () => {
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
                {renaming === label.name ? (
                  <Button
                    size="xs"
                    variant="ghost"
                    className="size-7 shrink-0 p-0"
                    onClick={() => {
                      if (renameTo.trim() && renameTo.trim() !== label.name) {
                        run(async () => {
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
                      setRenaming(label.name);
                      setRenameTo(label.name);
                    }}
                  >
                    <Pencil className="size-3.5" />
                  </Button>
                )}
                {confirming === label.name ? (
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
                    onClick={() => setConfirming(label.name)}
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                )}
              </div>
              {colorsOpen === label.name && (
                <div className="mt-1.5">
                  <ColorSwatches
                    value={label.color}
                    onPick={(color) => run(() => saveLabel(label.name, color))}
                  />
                </div>
              )}
            </div>
          ))}
          {extras.map((name) => (
            <div key={name} className="flex items-center gap-2 rounded-lg px-1 py-1">
              <span
                className="size-3.5 shrink-0 rounded-full"
                style={{ backgroundColor: labelColor(name, labelColors[name]) }}
              />
              <span className="min-w-0 flex-1 truncate text-sm">{name}</span>
              <Button
                size="xs"
                variant="ghost"
                title={t("delete")}
                className="size-7 shrink-0 p-0 text-muted-foreground hover:text-destructive"
                onClick={() => run(() => deleteLabel(name))}
              >
                <Trash2 className="size-3.5" />
              </Button>
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
