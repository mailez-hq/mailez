"use client";

import { type FormEvent } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { FolderDialog } from "./shared";

export function FolderDialogs({
  dialog,
  onClose,
  nameInput,
  onNameInput,
  parentInput,
  onParentInput,
  parentOptions,
  onSubmit,
  onConfirmDestructive,
  nameRef,
}: {
  dialog: FolderDialog | null;
  onClose: () => void;
  nameInput: string;
  onNameInput: (value: string) => void;
  parentInput: string;
  onParentInput: (value: string) => void;
  parentOptions: string[];
  onSubmit: (e: FormEvent) => void;
  onConfirmDestructive: () => void;
  nameRef: { current: HTMLInputElement | null };
}) {
  const t = useTranslations("mail");

  return (
    <>
      {/* folder name dialog (create / rename) */}
      <Dialog
        open={dialog !== null && (dialog.mode === "create" || dialog.mode === "rename")}
        onOpenChange={(o) => !o && onClose()}
      >
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>
              {dialog?.mode === "rename" ? t("renameFolder") : t("newFolder")}
            </DialogTitle>
          </DialogHeader>
          <form onSubmit={onSubmit} className="grid gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="folder-parent">{t("folderParent")}</Label>
              <select
                id="folder-parent"
                value={parentInput}
                onChange={(e) => onParentInput(e.target.value)}
                className="rounded-md border border-input bg-background px-2 py-1.5 text-sm"
              >
                <option value="">{t("folderParentRoot")}</option>
                {parentOptions.map((f) => (
                  <option key={f} value={f}>
                    {f}
                  </option>
                ))}
              </select>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="folder-name" className="sr-only">
                {t("folderNamePlaceholder")}
              </Label>
              <Input
                id="folder-name"
                ref={nameRef}
                value={nameInput}
                onChange={(e) => onNameInput(e.target.value)}
                placeholder={t("folderNamePlaceholder")}
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose}>
                {t("cancel")}
              </Button>
              <Button type="submit" disabled={!nameInput.trim()}>
                {t("save")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* confirm dialog (delete / clear) */}
      <Dialog
        open={dialog !== null && (dialog.mode === "delete" || dialog.mode === "clear")}
        onOpenChange={(o) => !o && onClose()}
      >
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>
              {dialog?.mode === "delete" ? t("deleteFolder") : t("clearFolder")}
            </DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            {dialog && dialog.mode !== "create"
              ? dialog.mode === "delete"
                ? t("confirmDeleteFolder", { name: dialog.name })
                : t("confirmClearFolder", { name: dialog.name })
              : ""}
          </p>
          <DialogFooter>
            <Button variant="outline" onClick={onClose}>
              {t("cancel")}
            </Button>
            <Button variant="destructive" onClick={onConfirmDestructive}>
              {t("confirmDelete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
