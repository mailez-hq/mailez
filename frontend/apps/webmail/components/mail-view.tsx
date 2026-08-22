"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { ScrollArea } from "@/components/ui/scroll-area";
import { MailSettings } from "@/components/mail-settings";
import { MailContacts } from "@/components/mail-contacts";
import {
  aiDraft, aiStatus, aiSummarize,
  mailDelete, mailFlag,
  mailFolders, mailMessage, mailMessages, mailSearch, mailSend, type MailMessage, type Me,
} from "@/lib/api";

export function MailView({ me }: { me: Me }) {
  const t = useTranslations("mail");
  const [folders, setFolders] = useState<string[]>([]);
  const [folder, setFolder] = useState("INBOX");
  const [messages, setMessages] = useState<MailMessage[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [cursor, setCursor] = useState(0);
  const [selected, setSelected] = useState<MailMessage | null>(null);
  const [detail, setDetail] = useState<MailMessage | null>(null);
  const [composeOpen, setComposeOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [contactsOpen, setContactsOpen] = useState(false);
  const [error, setError] = useState("");
  const [aiEnabled, setAiEnabled] = useState(false);
  const [summary, setSummary] = useState("");
  const [summarizing, setSummarizing] = useState(false);
  const [drafting, setDrafting] = useState(false);
  const [query, setQuery] = useState("");
  const [searching, setSearching] = useState(false);
  const [selectedUids, setSelectedUids] = useState<Set<number>>(new Set());

  const [to, setTo] = useState("");
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");

  const loadFolders = useCallback(async () => {
    try {
      setFolders(await mailFolders());
    } catch (e) {
      setError(e instanceof Error ? e.message : "load folders failed");
    }
  }, []);

  const loadMessages = useCallback(async (f: string, p = 0) => {
    try {
      const res = await mailMessages(f, p);
      setMessages(p === 0 ? res.messages : (prev) => [...prev, ...res.messages]);
      setTotal(res.total);
      setPage(p);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load messages failed");
    }
  }, []);

  useEffect(() => {
    loadFolders();
  }, [loadFolders]);

  useEffect(() => {
    setQuery("");
    setSearching(false);
    setSelectedUids(new Set());
    loadMessages(folder);
    setSelected(null);
    setDetail(null);
  }, [folder, loadMessages]);

  // load AI capability once; AI summary/draft only show when a provider is configured
  useEffect(() => {
    aiStatus().then((s) => setAiEnabled(s.enabled)).catch(() => setAiEnabled(false));
  }, []);

  async function doSearch(e: React.FormEvent) {
    e.preventDefault();
    const q = query.trim();
    if (!q) return;
    setSearching(true);
    setSelected(null);
    setDetail(null);
    try {
      setMessages(await mailSearch(folder, q));
    } catch (err) {
      setError(err instanceof Error ? err.message : "search failed");
    }
  }

  async function openMessage(m: MailMessage) {
    setSelected(m);
    setDetail(null);
    setSummary("");
    if (!m.flags.includes("\\Seen")) {
      mailFlag(folder, m.uid, "\\Seen", true).catch(() => {});
      m.flags.push("\\Seen");
    }
    try {
      setDetail(await mailMessage(folder, m.uid));
    } catch (e) {
      setError(e instanceof Error ? e.message : "load message failed");
    }
  }

  async function removeMessage(m: MailMessage) {
    try {
      await mailDelete(folder, m.uid);
      setMessages((ms) => ms.filter((x) => x.uid !== m.uid));
      if (selected?.uid === m.uid) {
        setSelected(null);
        setDetail(null);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "delete failed");
    }
  }

  function toggleSelect(uid: number) {
    setSelectedUids((prev) => {
      const next = new Set(prev);
      if (next.has(uid)) next.delete(uid); else next.add(uid);
      return next;
    });
  }

  async function bulkDelete() {
    const ids = [...selectedUids];
    setError("");
    try {
      await Promise.all(ids.map((uid) => mailDelete(folder, uid)));
      setMessages((ms) => ms.filter((x) => !selectedUids.has(x.uid)));
      if (selected && selectedUids.has(selected.uid)) {
        setSelected(null);
        setDetail(null);
      }
      setSelectedUids(new Set());
    } catch (e) {
      setError(e instanceof Error ? e.message : "bulk delete failed");
    }
  }

  async function bulkFlag(flag: string, value: boolean) {
    const ids = [...selectedUids];
    setError("");
    try {
      await Promise.all(ids.map((uid) => mailFlag(folder, uid, flag, value)));
      setMessages((ms) =>
        ms.map((m) =>
          selectedUids.has(m.uid)
            ? { ...m, flags: value ? [...m.flags, flag] : m.flags.filter((f) => f !== flag) }
            : m
        )
      );
      setSelectedUids(new Set());
    } catch (e) {
      setError(e instanceof Error ? e.message : "bulk flag failed");
    }
  }

  // quoteText builds the quoted original message used in replies/forwards.
  const quoteText = (d: MailMessage) => {
    const from = d.from.map((a) => a.name || a.email).join(", ");
    const lines = (d.text_body || "").trim();
    if (!lines) return "";
    const quoted = lines.split("\n").map((l) => `> ${l}`).join("\n");
    return `\n\nOn ${fmtDate(d.date)}, ${from} wrote:\n${quoted}`;
  };

  function openCompose(to: string, subject: string, body: string) {
    setTo(to);
    setSubject(subject);
    setBody(body);
    setComposeOpen(true);
  }

  function reply() {
    if (!detail) return;
    openCompose(
      detail.from[0]?.email || "",
      detail.subject.startsWith("Re:") ? detail.subject : `Re: ${detail.subject}`,
      quoteText(detail)
    );
  }

  function replyAll() {
    if (!detail) return;
    const recipients = new Set<string>();
    [...detail.from, ...detail.to].forEach((a) => {
      if (a.email && a.email.toLowerCase() !== me.email.toLowerCase()) recipients.add(a.email);
    });
    openCompose(
      [...recipients].join(", "),
      detail.subject.startsWith("Re:") ? detail.subject : `Re: ${detail.subject}`,
      quoteText(detail)
    );
  }

  function forward() {
    if (!detail) return;
    const from = detail.from.map((a) => a.name || a.email).join(", ");
    const head = `---------- Forwarded message ----------\nFrom: ${from}\nDate: ${fmtDate(detail.date)}\nSubject: ${detail.subject}\n\n`;
    openCompose(
      "",
      detail.subject.startsWith("Fwd:") ? detail.subject : `Fwd: ${detail.subject}`,
      head + (detail.text_body || "")
    );
  }

  async function aiDraftReply() {
    if (!detail) return;
    setDrafting(true);
    setError("");
    try {
      const res = await aiDraft(detail.text_body || detail.html_body || "");
      setBody(res.draft);
    } catch (e) {
      setError(e instanceof Error ? e.message : "ai draft failed");
    } finally {
      setDrafting(false);
    }
  }

  // keyboard navigation: j/k move, Enter open, r reply, # delete
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      if (messages.length === 0) return;
      if (e.key === "j") {
        setCursor((c) => Math.min(c + 1, messages.length - 1));
      } else if (e.key === "k") {
        setCursor((c) => Math.max(c - 1, 0));
      } else if (e.key === "Enter") {
        openMessage(messages[cursor]);
      } else if (e.key === "r") {
        openMessage(messages[cursor]).then(() => setTimeout(reply, 50));
      } else if (e.key === "#") {
        removeMessage(messages[cursor]);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  async function summarize() {
    if (!detail) return;
    setSummarizing(true);
    setSummary("");
    try {
      const res = await aiSummarize(detail.text_body || detail.html_body || "");
      setSummary(res.summary);
    } catch (e) {
      setError(e instanceof Error ? e.message : "summarize failed");
    } finally {
      setSummarizing(false);
    }
  }

  async function send(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await mailSend(to, subject, body);
      setComposeOpen(false);
      setTo(""); setSubject(""); setBody("");
      loadMessages(folder);
    } catch (err) {
      setError(err instanceof Error ? err.message : "send failed");
    }
  }

  const fmtDate = (d: string) =>
    new Date(d).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });

  const fmtSize = (n: number) =>
    n >= 1024 * 1024 ? `${(n / (1024 * 1024)).toFixed(1)} MB`
      : n >= 1024 ? `${Math.round(n / 1024)} KB`
        : `${n} B`;

  return (
    <div className="flex h-screen">
      {/* Folders */}
      <aside className="flex w-44 shrink-0 flex-col border-r bg-white dark:bg-zinc-900">
        <div className="flex items-center justify-between px-3 py-3">
          <span className="text-base font-semibold">mailess</span>
          <div className="flex gap-1">
            <Button size="sm" variant="ghost" onClick={() => setContactsOpen(true)}>{t("contacts")}</Button>
            <Button size="sm" variant="ghost" onClick={() => setSettingsOpen(true)}>{t("settings")}</Button>
            <Button size="sm" onClick={() => setComposeOpen(true)}>{t("write")}</Button>
          </div>
        </div>
        <ScrollArea className="flex-1">
          <nav className="space-y-1 px-2">
            {folders.map((f) => (
              <button
                key={f}
                onClick={() => setFolder(f)}
                className={`w-full rounded-md px-3 py-1.5 text-left text-sm ${
                  f === folder
                    ? "bg-zinc-100 font-medium text-zinc-900 dark:bg-zinc-800 dark:text-zinc-50"
                    : "text-zinc-600 hover:bg-zinc-100 dark:text-zinc-400 dark:hover:bg-zinc-800"
                }`}
              >
                {f}
              </button>
            ))}
          </nav>
        </ScrollArea>
        <div className="border-t px-3 py-2 text-xs text-zinc-500">{me.email}</div>
      </aside>

      {/* Message list */}
      <div className="flex w-96 shrink-0 flex-col border-r">
        <div className="border-b px-3 py-2">
          <form onSubmit={doSearch} className="flex items-center gap-1">
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t("search", { folder })}
              className="h-8"
            />
            {searching && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-8 px-2"
                onClick={() => { setSearching(false); setQuery(""); loadMessages(folder); }}
                title="Clear search"
              >
                ✕
              </Button>
            )}
          </form>
        </div>
        {selectedUids.size > 0 && (
          <div className="flex items-center gap-1 border-b px-3 py-2">
            <span className="mr-1 text-sm text-zinc-500">{t("selected", { count: selectedUids.size })}</span>
            <Button size="sm" variant="outline" onClick={bulkDelete}>{t("delete")}</Button>
            <Button size="sm" variant="outline" onClick={() => bulkFlag("\\Seen", true)}>{t("read")}</Button>
            <Button size="sm" variant="outline" onClick={() => bulkFlag("\\Seen", false)}>{t("unread")}</Button>
          </div>
        )}
        <ScrollArea className="flex-1">
          {messages.map((m, i) => {
            const unread = !m.flags.includes("\\Seen");
            return (
              <div
                key={m.uid}
                onClick={() => openMessage(m)}
                className={`flex cursor-pointer flex-col items-start gap-0.5 border-b px-3 py-2.5 hover:bg-zinc-50 dark:hover:bg-zinc-800/50 ${
                  selected?.uid === m.uid
                    ? "bg-zinc-100 dark:bg-zinc-800"
                    : cursor === i
                      ? "bg-zinc-50 dark:bg-zinc-800/40"
                      : ""
                }`}
              >
                <div className="flex w-full items-baseline justify-between">
                  <span className="flex min-w-0 items-center gap-2">
                    <input
                      type="checkbox"
                      checked={selectedUids.has(m.uid)}
                      onClick={(e) => e.stopPropagation()}
                      onChange={() => toggleSelect(m.uid)}
                      className="shrink-0 accent-zinc-900 dark:accent-zinc-100"
                      title="Select"
                    />
                    <span className={`truncate ${unread ? "font-semibold" : "font-normal"}`}>
                      {m.from[0]?.name || m.from[0]?.email || "unknown"}
                    </span>
                  </span>
                  <span className="flex shrink-0 items-center gap-2">
                    <span className="text-xs text-zinc-400">{fmtDate(m.date)}</span>
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        removeMessage(m);
                      }}
                      className="text-xs text-zinc-400 hover:text-red-500"
                      title="Delete (#)"
                    >
                      ✕
                    </button>
                  </span>
                </div>
                <span className={`truncate pl-6 text-sm ${unread ? "font-medium text-zinc-900 dark:text-zinc-100" : "text-zinc-600 dark:text-zinc-400"}`}>
                  {m.subject || "(no subject)"}
                </span>
              </div>
            );
          })}
          {messages.length === 0 && (
            <p className="p-4 text-sm text-zinc-400">{searching ? t("noMatches") : t("noMessages")}</p>
          )}
          {!searching && messages.length > 0 && messages.length < total && (
            <button
              onClick={() => loadMessages(folder, page + 1)}
              className="w-full p-3 text-center text-sm text-zinc-500 hover:bg-zinc-50 dark:hover:bg-zinc-800"
            >
              {t("loadMore", { count: total - messages.length })}
            </button>
          )}
        </ScrollArea>
      </div>

      {/* Reading pane */}
      <main className="flex-1 overflow-y-auto bg-white dark:bg-zinc-950">
        {detail ? (
          <article className="mx-auto max-w-3xl p-6">
            <h1 className="text-2xl font-semibold">{detail.subject}</h1>
            <div className="mt-2 flex items-center gap-2 text-sm text-zinc-500">
              <span className="font-medium text-zinc-900 dark:text-zinc-100">
                {detail.from.map((a) => a.name || a.email).join(", ")}
              </span>
              <span>&lt;{detail.from[0]?.email}&gt;</span>
              <span>·</span>
              <span>{fmtDate(detail.date)}</span>
            </div>
            <div className="mt-3 flex gap-1">
              <Button size="sm" variant="outline" onClick={reply}>{t("reply")}</Button>
              <Button size="sm" variant="outline" onClick={replyAll}>{t("replyAll")}</Button>
              <Button size="sm" variant="outline" onClick={forward}>{t("forward")}</Button>
            </div>
            {aiEnabled && (
              <div className="mt-4">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={summarize}
                  disabled={summarizing}
                >
                  {summarizing ? "Summarizing..." : "AI Summary"}
                </Button>
                {summary && (
                  <div className="mt-2 rounded-md border bg-zinc-50 p-3 text-sm leading-6 dark:bg-zinc-900">
                    <p className="mb-1 text-xs font-medium text-zinc-400">Summary</p>
                    <p className="whitespace-pre-wrap">{summary}</p>
                  </div>
                )}
              </div>
            )}
            <div className="mt-6 whitespace-pre-wrap text-[15px] leading-7">
              {detail.text_body || t("noTextBody")}
            </div>
            {detail.html_body && (
              <div className="mt-6 border-t pt-4">
                <p className="mb-2 text-xs text-zinc-400">{t("htmlVersion")}</p>
                <div
                  className="text-sm"
                  dangerouslySetInnerHTML={{ __html: detail.html_body }}
                />
              </div>
            )}
            {detail.attachments && detail.attachments.length > 0 && (
              <div className="mt-6 border-t pt-4">
                <p className="mb-2 text-xs font-medium text-zinc-400">{t("attachments", { count: detail.attachments.length })}</p>
                <div className="flex flex-wrap gap-2">
                  {detail.attachments.map((a, i) => (
                    <a
                      key={i}
                      href={`data:${a.content_type};base64,${a.data}`}
                      download={a.filename}
                      className="flex items-center gap-2 rounded-md border px-3 py-2 text-sm hover:bg-zinc-50 dark:hover:bg-zinc-800"
                    >
                      <span className="max-w-48 truncate font-medium">{a.filename}</span>
                      <span className="shrink-0 text-xs text-zinc-400">{fmtSize(a.size)}</span>
                    </a>
                  ))}
                </div>
              </div>
            )}
          </article>
        ) : (
          <div className="flex h-full items-center justify-center text-sm text-zinc-400">
            {t("selectMessage")}
          </div>
        )}
      </main>

      {/* Compose dialog */}
      <Dialog open={composeOpen} onOpenChange={setComposeOpen}>
        <DialogContent className="max-w-xl">
          <form onSubmit={send} className="space-y-3">
            <DialogHeader><DialogTitle>{t("newMessage")}</DialogTitle></DialogHeader>
            <div className="space-y-1">
              <div className="flex items-center justify-between">
                <Label>{t("to")}</Label>
                <Button type="button" variant="ghost" size="sm" onClick={() => setContactsOpen(true)}>
                  {t("contacts")}
                </Button>
              </div>
              <Input value={to} onChange={(e) => setTo(e.target.value)} placeholder="user@example.com" required />
            </div>
            <div className="space-y-1">
              <Label>{t("subject")}</Label>
              <Input value={subject} onChange={(e) => setSubject(e.target.value)} />
            </div>
            <div className="space-y-1">
              <Label>{t("body")}</Label>
              <Textarea value={body} onChange={(e) => setBody(e.target.value)} rows={8} placeholder="Write your message..." />
            </div>
            {error && <p className="text-sm text-red-600">{error}</p>}
            <DialogFooter>
              {aiEnabled && detail && (
                <Button
                  type="button"
                  variant="outline"
                  onClick={aiDraftReply}
                  disabled={drafting}
                  className="mr-auto"
                >
                  {drafting ? "Drafting..." : "AI Draft"}
                </Button>
              )}
              <Button type="submit">{t("send")}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Settings dialog */}
      <MailSettings
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
        onSaved={() => loadMessages(folder)}
      />

      {/* Contacts dialog */}
      <MailContacts
        open={contactsOpen}
        onOpenChange={setContactsOpen}
        onPick={(email) => setTo((prev) => (prev ? `${prev}, ${email}` : email))}
      />
    </div>
  );
}
