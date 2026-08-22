"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { contacts, createContact, deleteContact, type Contact } from "@/lib/api";

export function MailContacts({ open, onOpenChange, onPick }: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onPick: (email: string) => void;
}) {
  const t = useTranslations("contacts");
  const [list, setList] = useState<Contact[]>([]);
  const [error, setError] = useState("");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");

  const load = useCallback(async () => {
    try {
      setList(await contacts());
    } catch (e) {
      setError(e instanceof Error ? e.message : "load contacts failed");
    }
  }, []);

  useEffect(() => {
    if (open) {
      setError("");
      load();
    }
  }, [open, load]);

  async function add(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await createContact(name, email);
      setName(""); setEmail("");
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "create failed");
    }
  }

  async function remove(id: number) {
    try {
      await deleteContact(id);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "delete failed");
    }
  }

  function pick(c: Contact) {
    onPick(c.email);
    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[80vh] max-w-md">
        <DialogHeader><DialogTitle>{t("title")}</DialogTitle></DialogHeader>
        <form onSubmit={add} className="flex items-end gap-2">
          <div className="flex-1 space-y-1">
            <Label>{t("name")}</Label>
            <Input value={name} onChange={(e) => setName(e.target.value)} required />
          </div>
          <div className="flex-1 space-y-1">
            <Label>{t("email")}</Label>
            <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
          </div>
          <Button type="submit">{t("add")}</Button>
        </form>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <ScrollArea className="max-h-72">
          <div className="space-y-1">
            {list.map((c) => (
              <div
                key={c.id}
                className="flex items-center justify-between gap-2 rounded-md border px-3 py-2"
              >
                <button
                  type="button"
                  onClick={() => pick(c)}
                  className="min-w-0 flex-1 text-left"
                  title="Use as recipient"
                >
                  <p className="truncate text-sm font-medium">{c.name}</p>
                  <p className="truncate text-xs text-zinc-500">{c.email}</p>
                </button>
                <Button variant="ghost" size="sm" onClick={() => remove(c.id)}>✕</Button>
              </div>
            ))}
            {list.length === 0 && <p className="p-3 text-sm text-zinc-400">{t("noContacts")}</p>}
          </div>
        </ScrollArea>
        <DialogFooter />
      </DialogContent>
    </Dialog>
  );
}
