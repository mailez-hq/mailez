"use client";

import { useCallback, useEffect, useState } from "react";
import { Loader2, Mail, MessageSquare, PenLine, Trash2 } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  contacts, createContact, deleteContact, mailSearch,
  type Contact, type MailMessage,
} from "@/lib/api";
import { cn } from "@/lib/utils";

function fmtShort(d: string) {
  const date = new Date(d);
  if (Number.isNaN(date.getTime())) return "";
  const now = new Date();
  if (date.toDateString() === now.toDateString())
    return date.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  if (date.getFullYear() === now.getFullYear())
    return date.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  return date.toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

export function MailContacts({
  open,
  onOpenChange,
  onPick,
  onOpenMessage,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onPick: (email: string) => void;
  onOpenMessage: (m: MailMessage, folder: string) => void;
}) {
  const t = useTranslations("contacts");
  const tm = useTranslations("mail");
  const [list, setList] = useState<Contact[]>([]);
  const [selectedId, setSelectedId] = useState<number | null>(null);
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
      setName("");
      setEmail("");
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "create failed");
    }
  }

  async function remove(id: number) {
    try {
      await deleteContact(id);
      if (selectedId === id) setSelectedId(null);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "delete failed");
    }
  }

  function pick(c: Contact) {
    onPick(c.email);
    onOpenChange(false);
  }

  const selected = list.find((c) => c.id === selectedId) || null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] max-w-2xl">
        <DialogHeader><DialogTitle>{t("title")}</DialogTitle></DialogHeader>
        {error && <p className="text-sm text-destructive">{error}</p>}
        <div className="grid gap-4 md:grid-cols-[230px_1fr]">
          {/* left: add + list */}
          <div className="min-w-0">
            <form onSubmit={add} className="space-y-2">
              <div className="flex gap-1.5">
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={t("name")}
                  className="h-8"
                />
                <Button type="submit" size="sm" className="shrink-0">
                  {t("add")}
                </Button>
              </div>
              <Input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder={t("email")}
                className="h-8"
                required
              />
            </form>
            <ScrollArea className="mt-2 max-h-80">
              <div className="space-y-1 pr-1">
                {list.map((c) => (
                  <button
                    key={c.id}
                    type="button"
                    onClick={() => setSelectedId(c.id)}
                    className={cn(
                      "flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left transition-colors",
                      selectedId === c.id
                        ? "bg-accent text-accent-foreground"
                        : "hover:bg-muted",
                    )}
                  >
                    <span
                      className={cn(
                        "flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold",
                        selectedId === c.id
                          ? "bg-primary/20 text-primary"
                          : "bg-muted text-muted-foreground",
                      )}
                    >
                      {(c.name || c.email).charAt(0).toUpperCase()}
                    </span>
                    <span className="min-w-0">
                      <span className="block truncate text-sm font-medium">{c.name || c.email}</span>
                      {c.name && (
                        <span className="block truncate text-xs text-muted-foreground">
                          {c.email}
                        </span>
                      )}
                    </span>
                  </button>
                ))}
                {list.length === 0 && (
                  <p className="p-3 text-sm text-muted-foreground">{t("noContacts")}</p>
                )}
              </div>
            </ScrollArea>
          </div>

          {/* right: contact detail */}
          <div className="min-w-0">
            {selected ? (
              <ContactDetail
                key={selected.id}
                contact={selected}
                onPick={() => pick(selected)}
                onDelete={() => remove(selected.id)}
                onOpenMessage={onOpenMessage}
              />
            ) : (
              <div className="flex h-40 items-center justify-center text-sm text-muted-foreground">
                <MessageSquare className="mr-2 size-4 opacity-50" />
                {t("selectHint")}
              </div>
            )}
          </div>
        </div>
        <DialogFooter />
      </DialogContent>
    </Dialog>
  );
}

function ContactDetail({
  contact,
  onPick,
  onDelete,
  onOpenMessage,
}: {
  contact: Contact;
  onPick: () => void;
  onDelete: () => void;
  onOpenMessage: (m: MailMessage, folder: string) => void;
}) {
  const t = useTranslations("contacts");
  const tm = useTranslations("mail");
  const [history, setHistory] = useState<MailMessage[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    setHistory(null);
    setError("");
    let cancelled = false;
    setLoading(true);
    Promise.all([
      mailSearch("INBOX", `from:${contact.email}`),
      mailSearch("INBOX", `to:${contact.email}`),
    ])
      .then(([from, to]) => {
        if (cancelled) return;
        const seen = new Set<number>();
        const merged: MailMessage[] = [];
        for (const m of [...from, ...to]) {
          if (!seen.has(m.uid)) {
            seen.add(m.uid);
            merged.push(m);
          }
        }
        merged.sort((a, b) => new Date(b.date).getTime() - new Date(a.date).getTime());
        setHistory(merged.slice(0, 20));
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : "history failed");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [contact.email]);

  return (
    <div className="rounded-lg border border-border p-3">
      <div className="flex items-start gap-3">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-full bg-accent text-sm font-semibold text-accent-foreground">
          {(contact.name || contact.email).charAt(0).toUpperCase()}
        </span>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold">{contact.name || contact.email}</p>
          <p className="truncate text-xs text-muted-foreground">{contact.email}</p>
          {contact.comment && (
            <p className="mt-1 text-xs text-muted-foreground">{contact.comment}</p>
          )}
        </div>
      </div>
      <div className="mt-3 flex flex-wrap gap-1">
        <Button size="sm" onClick={onPick}>
          <PenLine className="size-3.5" />
          {t("writeEmail")}
        </Button>
        <Button size="sm" variant="ghost" onClick={onDelete} className="text-muted-foreground hover:text-destructive">
          <Trash2 className="size-3.5" />
          {t("delete")}
        </Button>
      </div>

      <div className="mt-4">
        <p className="mb-1.5 flex items-center gap-1 text-xs font-medium text-muted-foreground">
          <Mail className="size-3" />
          {t("history")}
        </p>
        {loading && (
          <p className="flex items-center gap-1.5 p-2 text-xs text-muted-foreground">
            <Loader2 className="size-3 animate-spin" />
            {t("loadingHistory")}
          </p>
        )}
        {error && <p className="p-2 text-xs text-destructive">{error}</p>}
        {!loading && !error && history && history.length === 0 && (
          <p className="p-2 text-xs text-muted-foreground">{t("noHistory")}</p>
        )}
        {!loading && history && history.length > 0 && (
          <div className="max-h-48 divide-y divide-border overflow-y-auto rounded-md border border-border">
            {history.map((m) => (
              <button
                key={m.uid}
                type="button"
                onClick={() => onOpenMessage(m, "INBOX")}
                className="flex w-full items-center gap-2 px-2.5 py-2 text-left text-xs transition-colors hover:bg-muted/60"
              >
                <span className="min-w-0 flex-1 truncate">
                  <span className="block truncate font-medium">{m.subject || tm("noSubject")}</span>
                  <span className="block truncate text-muted-foreground">
                    {m.from[0]?.name || m.from[0]?.email || "?"}
                  </span>
                </span>
                <span className="shrink-0 text-muted-foreground">{fmtShort(m.date)}</span>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
