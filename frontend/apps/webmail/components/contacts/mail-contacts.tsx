"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  Loader2, Mail, MessageSquare, PenLine, Trash2, Upload, Download, FolderOpen,
} from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  carddavGet, carddavSet, carddavSync, contacts, contactsDedupe,
  createContact, updateContact, deleteContact, exportContacts, importContacts,
  mailSearch, orgContacts,
  type Contact, type MailMessage, type OrgContact,
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

function Avatar({ contact, className }: { contact: Contact; className?: string }) {
  if (contact.avatar) {
    return (
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={contact.avatar}
        alt=""
        referrerPolicy="no-referrer"
        className={cn("shrink-0 rounded-full object-cover", className)}
      />
    );
  }
  return (
    <span
      className={cn(
        "flex shrink-0 items-center justify-center rounded-full text-xs font-semibold",
        className,
      )}
    >
      {(contact.name || contact.email).charAt(0).toUpperCase()}
    </span>
  );
}

function GroupBadges({ groups }: { groups: string }) {
  const items = groups.split(",").map((g) => g.trim()).filter(Boolean);
  if (items.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-1">
      {items.map((g, i) => (
        <span
          key={i}
          className="rounded-full bg-accent px-2 py-0.5 text-[10px] font-medium text-accent-foreground"
        >
          {g}
        </span>
      ))}
    </div>
  );
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
  const [info, setInfo] = useState("");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [groups, setGroups] = useState("");
  const [importing, setImporting] = useState(false);
  const [groupFilter, setGroupFilter] = useState("");
  const [deduping, setDeduping] = useState(false);
  const [carddavOpen, setCarddavOpen] = useState(false);
  const [carddavUrl, setCarddavUrl] = useState("");
  const [carddavUser, setCarddavUser] = useState("");
  const [carddavPw, setCarddavPw] = useState("");
  const [carddavSaving, setCarddavSaving] = useState(false);
  const [carddavSyncing, setCarddavSyncing] = useState(false);
  const [tab, setTab] = useState<"mine" | "org">("mine");
  const [orgList, setOrgList] = useState<OrgContact[]>([]);
  const [orgDept, setOrgDept] = useState("");
  const fileRef = useRef<HTMLInputElement>(null);

  const load = useCallback(async () => {
    try {
      setList(await contacts());
      orgContacts().then(setOrgList).catch(() => setOrgList([]));
    } catch (e) {
      setError(e instanceof Error ? e.message : "load contacts failed");
    }
  }, []);

  useEffect(() => {
    if (open) {
      setError("");
      setInfo("");
      load();
    }
  }, [open, load]);

  async function add(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setInfo("");
    try {
      await createContact(name, email, "", groups);
      setName("");
      setEmail("");
      setGroups("");
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

  async function handleExport() {
    try {
      const data = await exportContacts();
      const blob = new Blob([data], { type: "text/vcard" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = "contacts.vcf";
      a.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      setError(err instanceof Error ? err.message : "export failed");
    }
  }

  async function handleImportFile(f: File) {
    setImporting(true);
    setError("");
    setInfo("");
    try {
      const result = await importContacts(await f.text());
      if (result.added === 0) {
        setInfo(t("importEmpty"));
      } else {
        setInfo(t("importResult", { added: String(result.added), total: String(result.total) }));
      }
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("importFailed"));
    } finally {
      setImporting(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  async function handleDedupe() {
    setDeduping(true);
    setError("");
    try {
      const res = await contactsDedupe();
      setInfo(`合并 ${res.merged} 组重复项，删除 ${res.removed} 条`);
      load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "dedupe failed");
    } finally {
      setDeduping(false);
    }
  }

  async function openCardDAV() {
    setCarddavOpen(true);
    setError("");
    try {
      const cfg = await carddavGet();
      setCarddavUrl(cfg.url);
      setCarddavUser(cfg.username);
      setCarddavPw("");
    } catch {
      // default empty form
    }
  }

  async function saveCardDAV() {
    setCarddavSaving(true);
    setError("");
    try {
      await carddavSet({url: carddavUrl.trim(), username: carddavUser, password: carddavPw});
      setCarddavPw("");
      setInfo(t("carddavSaved"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "save failed");
    } finally {
      setCarddavSaving(false);
    }
  }

  async function syncCardDAV() {
    setCarddavSyncing(true);
    setError("");
    try {
      const res = await carddavSync();
      setInfo(t("carddavSynced", {added: res.added, updated: res.updated}));
      load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "sync failed");
    } finally {
      setCarddavSyncing(false);
    }
  }

  function pick(c: { email: string }) {
    onPick(c.email);
    onOpenChange(false);
  }

  const selected = list.find((c) => c.id === selectedId) || null;
  const groupList = [...new Set(list.flatMap((c) => (c.groups || "").split(",").map((g) => g.trim()).filter(Boolean)))];
  const visible = groupFilter ? list.filter((c) => (c.groups || "").split(",").map((g) => g.trim()).includes(groupFilter)) : list;
  const orgDepts = [...new Set(orgList.map((c) => c.department).filter(Boolean))].sort();
  const orgVisible = orgDept ? orgList.filter((c) => c.department === orgDept) : orgList;

  return (
    <>
    <Dialog open={open} onOpenChange={onOpenChange}>
      {/* Fixed dialog height: both tabs share the same body area so switching
          between 我的联系人 / 组织通讯录 never changes the dialog size. */}
      <DialogContent className="flex h-[75vh] flex-col sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {t("title")}
            <span className="flex-1" />
            <input
              ref={fileRef}
              type="file"
              accept=".vcf,.vcard,text/vcard,text/x-vcard"
              className="hidden"
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) handleImportFile(f);
              }}
            />
            <Button
              size="xs"
              variant="ghost"
              className="gap-1"
              disabled={importing}
              onClick={() => fileRef.current?.click()}
            >
              {importing ? <Loader2 className="size-3.5 animate-spin" /> : <Upload className="size-3.5" />}
              {t("import")}
            </Button>
            <Button size="xs" variant="ghost" className="gap-1" onClick={handleExport}>
              <Download className="size-3.5" />
              {t("export")}
            </Button>
            <Button size="xs" variant="ghost" className="gap-1" onClick={handleDedupe} disabled={deduping}>
              {deduping ? <Loader2 className="size-3.5 animate-spin" /> : <FolderOpen className="size-3.5" />}
              {t("dedupe")}
            </Button>
            <Button size="xs" variant="ghost" className="gap-1" onClick={openCardDAV}>
              CardDAV
            </Button>
          </DialogTitle>
        </DialogHeader>
        {error && <p className="text-sm text-destructive">{error}</p>}
        {info && <p className="text-sm text-muted-foreground">{info}</p>}
        <div className="mb-2 flex gap-1">
          <button
            type="button"
            onClick={() => setTab("mine")}
            className={cn(
              "rounded-full border px-2.5 py-0.5 text-xs transition-colors",
              tab === "mine" ? "bg-accent font-medium text-accent-foreground" : "border-border text-muted-foreground hover:bg-muted",
            )}
          >
            {t("myContacts")}
          </button>
          <button
            type="button"
            onClick={() => setTab("org")}
            className={cn(
              "rounded-full border px-2.5 py-0.5 text-xs transition-colors",
              tab === "org" ? "bg-accent font-medium text-accent-foreground" : "border-border text-muted-foreground hover:bg-muted",
            )}
          >
            {t("orgContacts")}
          </button>
        </div>
        <div className="min-h-0 flex-1">
        {tab === "org" ? (
          <div className="flex h-full min-h-0 flex-col">
            {orgDepts.length > 0 && (
              <div className="mb-2 flex shrink-0 flex-wrap gap-1">
                <button
                  type="button"
                  onClick={() => setOrgDept("")}
                  className={cn(
                    "rounded-full border px-2 py-0.5 text-[11px] transition-colors",
                    orgDept === "" ? "bg-accent font-medium text-accent-foreground" : "border-border text-muted-foreground hover:bg-muted",
                  )}
                >
                  {t("allDepartments")}
                </button>
                {orgDepts.map((d) => (
                  <button
                    key={d}
                    type="button"
                    onClick={() => setOrgDept(orgDept === d ? "" : d)}
                    className={cn(
                      "rounded-full border px-2 py-0.5 text-[11px] transition-colors",
                      orgDept === d ? "bg-accent font-medium text-accent-foreground" : "border-border text-muted-foreground hover:bg-muted",
                    )}
                  >
                    {d}
                  </button>
                ))}
              </div>
            )}
            <ScrollArea className="min-h-0 flex-1">
              <div className="space-y-1 pr-1">
                {orgList.length === 0 && <p className="text-sm text-muted-foreground">{t("orgEmpty")}</p>}
                {orgVisible.map((c) => (
                  <button
                    key={c.id}
                    type="button"
                    onClick={() => pick(c)}
                    className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left transition-colors hover:bg-muted"
                  >
                    <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-accent/60 text-xs font-semibold">
                      {(c.name || c.email).charAt(0).toUpperCase()}
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium">{c.name || c.email}</span>
                      <span className="block truncate text-[11px] text-muted-foreground">
                        {[c.department, c.title].filter(Boolean).join(" · ")}
                      </span>
                    </span>
                    <span className="shrink-0 text-[11px] text-muted-foreground">{c.email}</span>
                  </button>
                ))}
              </div>
            </ScrollArea>
          </div>
        ) : (
        <div className="grid h-full min-h-0 gap-4 md:grid-cols-[230px_1fr]">
          {/* left: add + list */}
          <div className="flex min-h-0 min-w-0 flex-col">
            {groupList.length > 0 && (
              <div className="mb-2 flex shrink-0 flex-wrap gap-1">
                <button
                  type="button"
                  onClick={() => setGroupFilter("")}
                  className={cn(
                    "rounded-full border px-2 py-0.5 text-[11px] transition-colors",
                    groupFilter === "" ? "bg-accent font-medium text-accent-foreground" : "border-border text-muted-foreground hover:bg-muted",
                  )}
                >
                  {t("allGroups")}
                </button>
                {groupList.map((g) => (
                  <button
                    key={g}
                    type="button"
                    onClick={() => setGroupFilter(groupFilter === g ? "" : g)}
                    className={cn(
                      "rounded-full border px-2 py-0.5 text-[11px] transition-colors",
                      groupFilter === g ? "bg-accent font-medium text-accent-foreground" : "border-border text-muted-foreground hover:bg-muted",
                    )}
                  >
                    {g}
                  </button>
                ))}
              </div>
            )}
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
              <Input
                value={groups}
                onChange={(e) => setGroups(e.target.value)}
                placeholder={t("groupsPlaceholder")}
                className="h-8"
              />
            </form>
            <ScrollArea className="mt-2 min-h-0 flex-1">
              <div className="space-y-1 pr-1">
                {visible.map((c) => (
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
                    <Avatar
                      contact={c}
                      className={cn(
                        "size-7",
                        selectedId === c.id ? "bg-primary/20 text-primary" : "bg-muted text-muted-foreground",
                      )}
                    />
                    <span className="min-w-0">
                      <span className="block truncate text-sm font-medium">{c.name || c.email}</span>
                      <span className="block truncate text-xs text-muted-foreground">
                        {c.name ? c.email : c.groups || ""}
                      </span>
                    </span>
                  </button>
                ))}
                {visible.length === 0 && (
                  <p className="p-3 text-sm text-muted-foreground">{t("noContacts")}</p>
                )}
              </div>
            </ScrollArea>
          </div>

          {/* right: contact detail */}
          <div className="min-h-0 min-w-0">
            {selected ? (
              <ScrollArea className="h-full min-h-0">
                <ContactDetail
                  key={selected.id}
                  contact={selected}
                  onPick={() => pick(selected)}
                  onDelete={() => remove(selected.id)}
                  onOpenMessage={onOpenMessage}
                  onSaved={(c) => {
                    setList((prev) => prev.map((x) => (x.id === c.id ? c : x)));
                    setInfo(t("saved"));
                  }}
                />
              </ScrollArea>
            ) : (
              <div className="flex h-40 items-center justify-center text-sm text-muted-foreground">
                <MessageSquare className="mr-2 size-4 opacity-50" />
                {t("selectHint")}
              </div>
            )}
          </div>
        </div>
        )}
        </div>
        <DialogFooter />
      </DialogContent>
    </Dialog>

    {/* CardDAV one-way import sync */}
    <Dialog open={carddavOpen} onOpenChange={setCarddavOpen}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>CardDAV</DialogTitle>
        </DialogHeader>
        <div className="space-y-2.5">
          <div className="space-y-1">
            <Label>{t("carddavUrl")}</Label>
            <Input value={carddavUrl} onChange={(e) => setCarddavUrl(e.target.value)} placeholder="https://dav.example.com/addressbooks/user/contacts/" />
          </div>
          <div className="space-y-1">
            <Label>{t("username")}</Label>
            <Input value={carddavUser} onChange={(e) => setCarddavUser(e.target.value)} autoComplete="off" />
          </div>
          <div className="space-y-1">
            <Label>{t("password")}</Label>
            <Input type="password" value={carddavPw} onChange={(e) => setCarddavPw(e.target.value)} autoComplete="new-password" />
          </div>
          <p className="text-xs text-muted-foreground">{t("carddavHint")}</p>
        </div>
        <DialogFooter>
          <Button variant="outline" size="sm" onClick={() => setCarddavOpen(false)}>
            {t("close")}
          </Button>
          <Button size="sm" variant="outline" onClick={syncCardDAV} disabled={carddavSyncing || !carddavUrl.trim()}>
            {carddavSyncing ? <Loader2 className="size-3.5 animate-spin" /> : null}
            {t("carddavSync")}
          </Button>
          <Button size="sm" onClick={saveCardDAV} disabled={carddavSaving}>
            {carddavSaving ? <Loader2 className="size-3.5 animate-spin" /> : null}
            {t("save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
    </>
  );
}

function ContactDetail({
  contact,
  onPick,
  onDelete,
  onOpenMessage,
  onSaved,
}: {
  contact: Contact;
  onPick: () => void;
  onDelete: () => void;
  onOpenMessage: (m: MailMessage, folder: string) => void;
  onSaved: (c: Contact) => void;
}) {
  const t = useTranslations("contacts");
  const tm = useTranslations("mail");
  const [history, setHistory] = useState<MailMessage[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [saveErr, setSaveErr] = useState("");
  const [comment, setComment] = useState(contact.comment);
  const [groups, setGroups] = useState(contact.groups);
  const [avatar, setAvatar] = useState(contact.avatar);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    setHistory(null);
    setError("");
    let cancelled = false;
    setLoading(true);
    Promise.all([
      mailSearch("Inbox", `from:${contact.email}`),
      mailSearch("Inbox", `to:${contact.email}`),
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

  useEffect(() => {
    setComment(contact.comment);
    setGroups(contact.groups);
    setAvatar(contact.avatar);
    setDirty(false);
  }, [contact]);

  async function save() {
    setSaving(true);
    setSaveErr("");
    try {
      const updated = await updateContact(contact.id, { comment, groups, avatar });
      setDirty(false);
      onSaved(updated);
    } catch (e) {
      setSaveErr(e instanceof Error ? e.message : "save failed");
    } finally {
      setSaving(false);
    }
  }

  const display = { ...contact, comment, groups, avatar };

  return (
    <div className="rounded-lg border border-border p-3">
      <div className="flex items-start gap-3">
        <Avatar
          contact={display}
          className="size-10 bg-accent text-sm font-semibold text-accent-foreground"
        />
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold">{contact.name || contact.email}</p>
          <p className="truncate text-xs text-muted-foreground">{contact.email}</p>
          <div className="mt-1">
            <GroupBadges groups={groups} />
          </div>
        </div>
      </div>

      <div className="mt-3 space-y-2">
        <div>
          <Label className="text-xs text-muted-foreground">{t("groups")}</Label>
          <Input
            value={groups}
            onChange={(e) => {
              setGroups(e.target.value);
              setDirty(true);
            }}
            placeholder={t("groupsPlaceholder")}
            className="mt-1 h-8"
          />
        </div>
        <div>
          <Label className="text-xs text-muted-foreground">{t("avatar")}</Label>
          <Input
            value={avatar}
            onChange={(e) => {
              setAvatar(e.target.value);
              setDirty(true);
            }}
            className="mt-1 h-8"
          />
        </div>
        <div>
          <Label className="text-xs text-muted-foreground">{t("comment")}</Label>
          <Input
            value={comment}
            onChange={(e) => {
              setComment(e.target.value);
              setDirty(true);
            }}
            className="mt-1 h-8"
          />
        </div>
      </div>

      <div className="mt-3 flex flex-wrap gap-1">
        <Button size="sm" onClick={onPick}>
          <PenLine className="size-3.5" />
          {t("writeEmail")}
        </Button>
        {dirty && (
          <Button size="sm" variant="secondary" disabled={saving} onClick={save}>
            {saving && <Loader2 className="mr-1 size-3.5 animate-spin" />}
            {t("save")}
          </Button>
        )}
        <Button size="sm" variant="ghost" onClick={onDelete} className="text-muted-foreground hover:text-destructive">
          <Trash2 className="size-3.5" />
          {t("delete")}
        </Button>
      </div>
      {saveErr && <p className="mt-2 text-xs text-destructive">{saveErr}</p>}

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
                onClick={() => onOpenMessage(m, "Inbox")}
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
