"use client";

import { useEffect, useState } from "react";
import { LayoutTemplate, PenLine, Plus, Trash2, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { mailTemplateDelete, mailTemplateSave, mailTemplates, type MailTemplate } from "@/lib/api";

// Manage reusable compose templates (canned responses): list, add, delete.
export function TemplatesDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const t = useTranslations("mail");
  const [list, setList] = useState<MailTemplate[]>([]);
  const [name, setName] = useState("");
  const [subject, setSubject] = useState("");
  const [html, setHtml] = useState("");
  const [error, setError] = useState("");
  const [editingId, setEditingId] = useState<number | null>(null);

  useEffect(() => {
    if (!open) return;
    setError("");
    mailTemplates().then(setList).catch(() => {});
  }, [open]);

  function startEdit(item: MailTemplate) {
    setEditingId(item.id);
    setName(item.name);
    setSubject(item.subject);
    setHtml(item.html);
    setError("");
  }

  function cancelEdit() {
    setEditingId(null);
    setName("");
    setSubject("");
    setHtml("");
    setError("");
  }

  async function save() {
    setError("");
    if (!name.trim()) return;
    try {
      const saved = await mailTemplateSave({
        id: editingId ?? undefined,
        name: name.trim(),
        subject,
        html,
        text: "",
      });
      setList((ls) =>
        editingId
          ? ls.map((x) => (x.id === saved.id ? saved : x))
          : [...ls, saved],
      );
      cancelEdit();
    } catch (e) {
      setError(e instanceof Error ? e.message : "save failed");
    }
  }

  async function remove(id: number) {
    try {
      await mailTemplateDelete(id);
      setList((ls) => ls.filter((x) => x.id !== id));
    } catch (e) {
      setError(e instanceof Error ? e.message : "delete failed");
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <LayoutTemplate className="size-4" />
            {t("templates")}
          </DialogTitle>
        </DialogHeader>
        {error && <p className="text-sm text-destructive">{error}</p>}
        <div className="max-h-56 space-y-1 overflow-y-auto">
          {list.length === 0 && (
            <p className="py-2 text-sm text-muted-foreground">{t("noTemplates")}</p>
          )}
          {list.map((item) => (
            <div key={item.id} className="flex items-center gap-2 rounded-lg border border-border px-2.5 py-1.5">
              <span className="min-w-0 flex-1 truncate text-sm">{item.name}</span>
              <Button
                type="button"
                size="xs"
                variant="ghost"
                title={t("templateEdit")}
                className="size-7 shrink-0 p-0 text-muted-foreground hover:text-foreground"
                onClick={() => startEdit(item)}
              >
                <PenLine className="size-3.5" />
              </Button>
              <Button
                type="button"
                size="xs"
                variant="ghost"
                title={t("delete")}
                className="size-7 shrink-0 p-0 text-muted-foreground hover:text-destructive"
                onClick={() => remove(item.id)}
              >
                <Trash2 className="size-3.5" />
              </Button>
            </div>
          ))}
        </div>
        <div className="space-y-2 border-t border-border pt-3">
          <div className="grid grid-cols-2 gap-2">
            <div className="space-y-1">
              <Label>{t("templateName")}</Label>
              <Input value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <div className="space-y-1">
              <Label>{t("subject")}</Label>
              <Input value={subject} onChange={(e) => setSubject(e.target.value)} />
            </div>
          </div>
          <div className="space-y-1">
            <Label>{t("templateBody")}</Label>
            <Textarea value={html} onChange={(e) => setHtml(e.target.value)} rows={4} />
          </div>
          <div className="flex items-center gap-2">
            <Button type="button" size="sm" onClick={save} disabled={!name.trim()}>
              <Plus className="size-3.5" />
              {editingId ? t("templateSave") : t("addTemplate")}
            </Button>
            {editingId && (
              <Button type="button" size="sm" variant="ghost" onClick={cancelEdit}>
                {t("templateCancel")}
              </Button>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
