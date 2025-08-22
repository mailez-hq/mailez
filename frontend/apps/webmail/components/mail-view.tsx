"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Inbox as InboxIcon, PenLine, Search, Send, Settings, Users, WifiOff } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ComposeEditor } from "@/components/compose-editor";
import { MailSettings } from "@/components/mail-settings";
import { MailContacts } from "@/components/mail-contacts";
import { FolderNav } from "@/components/mail/folder-nav";
import { MessageListPanel } from "@/components/mail/message-list-panel";
import { ReadingPane } from "@/components/mail/reading-pane";
import { CommandPalette, type PaletteAction } from "@/components/mail/command-palette";
import { ShortcutsDialog } from "@/components/mail/shortcuts-dialog";
import { SieveEditor } from "@/components/mail/sieve-editor";
import { usePreferences } from "@/components/preferences-provider";
import {
  aiDraft, aiStatus, aiSummarize,
  aiPrioritize, aiSearch,
  mailFlag, mailMove, mailIdentities,
  mailFolders, mailMessage, mailMessages, mailSearch, mailSend, mailThread,
  type MailIdentity, type MailMessage, type MailThread, type Me,
} from "@/lib/api";
import { cn } from "@/lib/utils";

// textToHtml converts a plain-text draft (reply quoting, AI drafts) into
// sanitized HTML for the rich-text editor: "> " lines become blockquotes,
// everything else becomes paragraphs.
function textToHtml(text: string): string {
  const esc = (s: string) =>
    s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  const out: string[] = [];
  let inQuote = false;
  const closeQuote = () => {
    if (inQuote) {
      out.push("</blockquote>");
      inQuote = false;
    }
  };
  for (const raw of text.replace(/\r\n/g, "\n").split("\n")) {
    const m = raw.match(/^>\s?(.*)$/);
    if (m) {
      if (!inQuote) {
        out.push("<blockquote>");
        inQuote = true;
      }
      out.push(`<p>${esc(m[1]) || "<br>"}</p>`);
    } else {
      closeQuote();
      out.push(`<p>${esc(raw) || "<br>"}</p>`);
    }
  }
  closeQuote();
  return out.join("");
}

export function MailView({ me }: { me: Me }) {
  const t = useTranslations("mail");
  const tp = useTranslations("palette");
  const ts = useTranslations("settings");
  const { theme, setTheme, density, setDensity } = usePreferences();

  const [folders, setFolders] = useState<string[]>([]);
  const [folder, setFolder] = useState("INBOX");
  const [messages, setMessages] = useState<MailMessage[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [cursor, setCursor] = useState(0);
  const [selected, setSelected] = useState<MailMessage | null>(null);
  const [detail, setDetail] = useState<MailMessage | null>(null);
  const [selectedUids, setSelectedUids] = useState<Set<number>>(new Set());
  const [query, setQuery] = useState("");
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState("");

  const [composeOpen, setComposeOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [contactsOpen, setContactsOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const [sieveOpen, setSieveOpen] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [listWidth, setListWidth] = useState(360);

  const [aiEnabled, setAiEnabled] = useState(false);
  const [summary, setSummary] = useState("");
  const [summarizing, setSummarizing] = useState(false);
  const [drafting, setDrafting] = useState(false);
  const [prioritizing, setPrioritizing] = useState(false);
  const [aiSearching, setAiSearching] = useState(false);
  const [priorityOn, setPriorityOn] = useState(false);
  const [baseMessages, setBaseMessages] = useState<MailMessage[] | null>(null);
  const [identities, setIdentities] = useState<MailIdentity[]>([]);
  const [from, setFrom] = useState("");
  const [toast, setToast] = useState<{ id: number; label: string; onUndo?: () => void } | null>(null);
  const [online, setOnline] = useState(true);
  const [thread, setThread] = useState<MailThread | null>(null);
  const [threadOpen, setThreadOpen] = useState(false);
  const [threadLoading, setThreadLoading] = useState(false);

  const [to, setTo] = useState("");
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [bodyText, setBodyText] = useState("");

  const searchRef = useRef<HTMLInputElement>(null);
  const loadingMoreRef = useRef(false);
  const pendingG = useRef(false);
  const resizeRef = useRef<{ x: number; w: number } | null>(null);
  const toastTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const loadFolders = useCallback(async () => {
    try {
      setFolders(await mailFolders());
    } catch (e) {
      setError(e instanceof Error ? e.message : "load folders failed");
    }
  }, []);

  const loadMessages = useCallback(async (f: string, p = 0) => {
    if (p === 0) setLoading(true);
    try {
      const res = await mailMessages(f, p);
      setMessages(p === 0 ? res.messages : (prev) => [...prev, ...res.messages]);
      setTotal(res.total);
      setPage(p);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load messages failed");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadFolders();
  }, [loadFolders]);

  useEffect(() => {
    setQuery("");
    setSearching(false);
    setSelectedUids(new Set());
    setCursor(0);
    setSelected(null);
    setDetail(null);
    setThread(null);
    setThreadOpen(false);
    setPriorityOn(false);
    setBaseMessages(null);
    loadMessages(folder);
  }, [folder, loadMessages]);

  // Prefer the existing junk folder name (Spam in some setups) over
  // creating a duplicate Junk mailbox.
  const spamFolder = useMemo(() => {
    const hit = folders.find((f) => /^(junk|spam)$/i.test(f));
    return hit || "Junk";
  }, [folders]);

  // clamp cursor when the list shrinks
  useEffect(() => {
    setCursor((c) => Math.min(c, Math.max(0, messages.length - 1)));
  }, [messages.length]);

  // load AI capability once; AI summary/draft only show when a provider is configured
  useEffect(() => {
    aiStatus().then((s) => setAiEnabled(s.enabled)).catch(() => setAiEnabled(false));
  }, []);

  // load the From identities (own address + aliases with DKIM status)
  useEffect(() => {
    mailIdentities()
      .then((ids) => {
        setIdentities(ids);
        setFrom((f) => f || ids[0]?.email || me.email);
      })
      .catch(() => setFrom(me.email));
  }, [me.email]);

  // connection status banner
  useEffect(() => {
    if (typeof navigator === "undefined") return;
    setOnline(navigator.onLine);
    const on = () => setOnline(true);
    const off = () => setOnline(false);
    window.addEventListener("online", on);
    window.addEventListener("offline", off);
    return () => {
      window.removeEventListener("online", on);
      window.removeEventListener("offline", off);
    };
  }, []);

  function showToast(label: string, onUndo?: () => void) {
    if (toastTimer.current) clearTimeout(toastTimer.current);
    setToast({ id: Date.now(), label, onUndo });
    toastTimer.current = setTimeout(() => setToast(null), 5000);
  }

  // moveTo moves messages to a folder and offers an undo that moves them back.
  async function moveTo(uids: number[], destination: string, successLabel: string) {
    setError("");
    try {
      await mailMove(folder, uids, destination);
      const uidSet = new Set(uids);
      setMessages((ms) => ms.filter((x) => !uidSet.has(x.uid)));
      setSelectedUids((prev) => {
        const next = new Set(prev);
        uids.forEach((u) => next.delete(u));
        return next;
      });
      if (selected && uidSet.has(selected.uid)) {
        setSelected(null);
        setDetail(null);
      }
      showToast(successLabel, () => {
        const src = folder;
        mailMove(destination, uids, src)
          .catch(() => {})
          .finally(() => loadMessages(src));
      });
    } catch (e) {
      setError(e instanceof Error ? e.message : "move failed");
    }
  }

  function archiveMessage(m: MailMessage) {
    return moveTo([m.uid], "Archive", t("toastArchived"));
  }

  function spamMessage(m: MailMessage) {
    return moveTo([m.uid], spamFolder, t("toastSpam"));
  }

  async function doSearch(e?: React.FormEvent) {
    e?.preventDefault();
    const q = query.trim();
    if (!q) return;
    setSearching(true);
    setSelected(null);
    setDetail(null);
    setCursor(0);
    try {
      setMessages(await mailSearch(folder, q));
    } catch (err) {
      setError(err instanceof Error ? err.message : "search failed");
    }
  }

  function clearSearch() {
    setSearching(false);
    setQuery("");
    setCursor(0);
    loadMessages(folder);
  }

  async function openMessage(m: MailMessage, srcFolder = folder) {
    setSelected(m);
    setDetail(null);
    setSummary("");
    setThreadOpen(false);
    if (!m.flags.includes("\\Seen")) {
      mailFlag(srcFolder, m.uid, "\\Seen", true).catch(() => {});
      m.flags.push("\\Seen");
      setMessages((ms) => ms.map((x) => (x.uid === m.uid ? m : x)));
    }
    try {
      setDetail(await mailMessage(srcFolder, m.uid));
    } catch (e) {
      setError(e instanceof Error ? e.message : "load message failed");
    }
  }

  function removeMessage(m: MailMessage) {
    return moveTo([m.uid], "Trash", t("toastDeleted"));
  }

  function toggleSelect(m: MailMessage) {
    setSelectedUids((prev) => {
      const next = new Set(prev);
      if (next.has(m.uid)) next.delete(m.uid);
      else next.add(m.uid);
      return next;
    });
  }

  async function bulkDelete() {
    const ids = [...selectedUids];
    await moveTo(ids, "Trash", t("toastDeleted"));
  }

  async function bulkArchive() {
    await moveTo([...selectedUids], "Archive", t("toastArchived"));
  }

  async function bulkSpam() {
    await moveTo([...selectedUids], spamFolder, t("toastSpam"));
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
            : m,
        ),
      );
      setSelectedUids(new Set());
    } catch (e) {
      setError(e instanceof Error ? e.message : "bulk flag failed");
    }
  }

  async function setSeen(m: MailMessage, value: boolean) {
    try {
      await mailFlag(folder, m.uid, "\\Seen", value);
      setMessages((ms) =>
        ms.map((x) =>
          x.uid === m.uid
            ? {
                ...x,
                flags: value
                  ? [...new Set([...x.flags, "\\Seen"])]
                  : x.flags.filter((f) => f !== "\\Seen"),
              }
            : x,
        ),
      );
      if (detail?.uid === m.uid) {
        setDetail((d) =>
          d
            ? {
                ...d,
                flags: value
                  ? [...new Set([...d.flags, "\\Seen"])]
                  : d.flags.filter((f) => f !== "\\Seen"),
              }
            : d,
        );
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "flag failed");
    }
  }

  async function toggleStar(m: MailMessage) {
    const starred = !m.flags.includes("\\Flagged");
    try {
      await mailFlag(folder, m.uid, "\\Flagged", starred);
      setMessages((ms) =>
        ms.map((x) =>
          x.uid === m.uid
            ? {
                ...x,
                flags: starred
                  ? [...new Set([...x.flags, "\\Flagged"])]
                  : x.flags.filter((f) => f !== "\\Flagged"),
              }
            : x,
        ),
      );
      if (detail?.uid === m.uid) {
        setDetail((d) =>
          d
            ? {
                ...d,
                flags: starred
                  ? [...new Set([...d.flags, "\\Flagged"])]
                  : d.flags.filter((f) => f !== "\\Flagged"),
              }
            : d,
        );
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "star failed");
    }
  }

  function loadMore() {
    if (loadingMoreRef.current || searching || messages.length >= total) return;
    loadingMoreRef.current = true;
    loadMessages(folder, page + 1).finally(() => {
      loadingMoreRef.current = false;
    });
  }

  // quoteText builds the quoted original message used in replies/forwards.
  const quoteText = (d: MailMessage) => {
    const from = d.from.map((a) => a.name || a.email).join(", ");
    const lines = (d.text_body || "").trim();
    if (!lines) return "";
    const quoted = lines.split("\n").map((l) => `> ${l}`).join("\n");
    return `\n\nOn ${fmtDate(d.date)}, ${from} wrote:\n${quoted}`;
  };

  function openCompose(toAddr = "", subj = "", html = "", text = "") {
    setTo(toAddr);
    setSubject(subj);
    setBody(html);
    setBodyText(text);
    setComposeOpen(true);
  }

  function reply() {
    if (!detail) return;
    openCompose(
      detail.from[0]?.email || "",
      detail.subject.startsWith("Re:") ? detail.subject : `Re: ${detail.subject}`,
      textToHtml(quoteText(detail)),
      quoteText(detail),
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
      textToHtml(quoteText(detail)),
      quoteText(detail),
    );
  }

  function forward() {
    if (!detail) return;
    const from = detail.from.map((a) => a.name || a.email).join(", ");
    const head = `---------- Forwarded message ----------\nFrom: ${from}\nDate: ${fmtDate(detail.date)}\nSubject: ${detail.subject}\n\n`;
    openCompose(
      "",
      detail.subject.startsWith("Fwd:") ? detail.subject : `Fwd: ${detail.subject}`,
      textToHtml(head + (detail.text_body || "")),
      head + (detail.text_body || ""),
    );
  }

  async function openThenReply(kind: "reply" | "replyAll" | "forward") {
    const m = stateRef.current.messages[stateRef.current.cursor];
    if (!m) return;
    if (stateRef.current.detail?.uid === m.uid) {
      (kind === "reply" ? reply : kind === "replyAll" ? replyAll : forward)();
      return;
    }
    await openMessage(m);
    setTimeout(() => {
      (kind === "reply" ? reply : kind === "replyAll" ? replyAll : forward)();
    }, 120);
  }

  async function aiDraftReply() {
    if (!detail) return;
    setDrafting(true);
    setError("");
    try {
      const res = await aiDraft(detail.text_body || detail.html_body || "");
      setBody(textToHtml(res.draft));
      setBodyText(res.draft);
    } catch (e) {
      setError(e instanceof Error ? e.message : "ai draft failed");
    } finally {
      setDrafting(false);
    }
  }

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

  async function togglePriority() {
    if (priorityOn) {
      if (baseMessages) setMessages(baseMessages);
      setPriorityOn(false);
      setBaseMessages(null);
      return;
    }
    if (!aiEnabled || messages.length === 0) return;
    setPrioritizing(true);
    setError("");
    try {
      const items = messages.map((m) => ({
        uid: m.uid,
        subject: m.subject,
        from: m.from[0]?.email || "",
      }));
      const { scores } = await aiPrioritize(items);
      setBaseMessages(messages);
      setMessages(
        [...messages].sort(
          (a, b) => (scores[String(b.uid)] ?? 0) - (scores[String(a.uid)] ?? 0),
        ),
      );
      setPriorityOn(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "prioritize failed");
    } finally {
      setPrioritizing(false);
    }
  }

  async function doAiSearch() {
    const q = query.trim();
    if (!q || !aiEnabled) return;
    setAiSearching(true);
    setError("");
    try {
      const res = await aiSearch(q);
      setSearching(true);
      setSelected(null);
      setDetail(null);
      setCursor(0);
      setMessages(res.messages);
    } catch (e) {
      setError(e instanceof Error ? e.message : "ai search failed");
    } finally {
      setAiSearching(false);
    }
  }

  async function send(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await mailSend(to, subject, bodyText, body, from);
      setComposeOpen(false);
      setTo("");
      setSubject("");
      setBody("");
      setBodyText("");
      loadMessages(folder);
    } catch (err) {
      setError(err instanceof Error ? err.message : "send failed");
    }
  }

  function backToList() {
    setSelected(null);
    setDetail(null);
  }

  async function toggleThread() {
    if (!detail?.thread_id) return;
    if (threadOpen && thread?.thread_id === detail.thread_id) {
      setThreadOpen(false);
      return;
    }
    setThreadOpen(true);
    if (!thread || thread.thread_id !== detail.thread_id) {
      setThreadLoading(true);
      setError("");
      try {
        setThread(await mailThread(folder, detail.thread_id));
      } catch (e) {
        setError(e instanceof Error ? e.message : "thread failed");
      } finally {
        setThreadLoading(false);
      }
    }
  }

  async function selectThreadMessage(uid: number) {
    if (!detail) return;
    setError("");
    try {
      const next = await mailMessage(folder, uid);
      setSelected(next);
      setDetail(next);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load message failed");
    }
  }

  // ---- global keyboard shortcuts ----
  const stateRef = useRef({
    messages, cursor, folder, searching, detail,
    composeOpen, settingsOpen, contactsOpen, paletteOpen, shortcutsOpen,
  });
  stateRef.current = {
    messages, cursor, folder, searching, detail,
    composeOpen, settingsOpen, contactsOpen, paletteOpen, shortcutsOpen,
  };
  const apiRef = useRef({
    openMessage, toggleSelect, removeMessage, toggleStar, setSeen,
    archiveMessage, spamMessage,
    openCompose, openThenReply, selectFolder: setFolder, backToList,
  });
  apiRef.current = {
    openMessage, toggleSelect, removeMessage, toggleStar, setSeen,
    archiveMessage, spamMessage,
    openCompose, openThenReply, selectFolder: setFolder, backToList,
  };

  useEffect(() => {
    function isEditable(target: EventTarget | null) {
      if (!(target instanceof HTMLElement)) return false;
      const tag = target.tagName;
      return tag === "INPUT" || tag === "TEXTAREA" || target.isContentEditable;
    }

    function onKey(e: KeyboardEvent) {
      const s = stateRef.current;
      if (s.paletteOpen || s.shortcutsOpen || s.composeOpen || s.settingsOpen || s.contactsOpen) {
        return;
      }
      if (isEditable(e.target)) return;
      const mod = e.metaKey || e.ctrlKey;
      if (mod && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen(true);
        return;
      }
      if (mod) return;

      const api = apiRef.current;
      const m = s.messages[s.cursor];

      if (e.shiftKey && e.key === "I") {
        if (m) api.setSeen(m, true);
        return;
      }
      if (e.shiftKey && e.key === "U") {
        if (m) api.setSeen(m, false);
        return;
      }
      if (e.key === "g") {
        pendingG.current = true;
        return;
      }
      if (pendingG.current) {
        pendingG.current = false;
        if (e.key === "i") api.selectFolder("INBOX");
        if (e.key === "s") api.selectFolder("Sent");
        return;
      }

      switch (e.key) {
        case "/":
          e.preventDefault();
          searchRef.current?.focus();
          break;
        case "?":
          setShortcutsOpen(true);
          break;
        case "n":
          api.openCompose();
          break;
        case "j":
          setCursor((c) => Math.min(c + 1, s.messages.length - 1));
          break;
        case "k":
          setCursor((c) => Math.max(c - 1, 0));
          break;
        case "Enter":
        case "o":
          if (m) api.openMessage(m);
          break;
        case "x":
          if (m) api.toggleSelect(m);
          break;
        case "#":
          if (m) api.removeMessage(m);
          break;
        case "s":
          if (m) api.toggleStar(m);
          break;
        case "e":
          if (m) api.archiveMessage(m);
          break;
        case "!":
          if (m) api.spamMessage(m);
          break;
        case "r":
          api.openThenReply("reply");
          break;
        case "a":
          api.openThenReply("replyAll");
          break;
        case "f":
          api.openThenReply("forward");
          break;
        case "u":
          api.backToList();
          break;
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // ---- list width drag ----
  function onResizeStart(e: React.PointerEvent<HTMLDivElement>) {
    resizeRef.current = { x: e.clientX, w: listWidth };
    e.currentTarget.setPointerCapture(e.pointerId);
  }
  function onResizeMove(e: React.PointerEvent<HTMLDivElement>) {
    const r = resizeRef.current;
    if (!r) return;
    setListWidth(Math.min(480, Math.max(300, r.w + e.clientX - r.x)));
  }
  function onResizeEnd() {
    resizeRef.current = null;
  }

  const paletteActions: PaletteAction[] = [
    {
      id: "write",
      label: t("write"),
      icon: <PenLine className="size-4" />,
      keywords: "new compose mail",
      run: () => openCompose(),
    },
    {
      id: "search",
      label: t("search"),
      icon: <Search className="size-4" />,
      keywords: "find",
      run: () => searchRef.current?.focus(),
    },
    {
      id: "inbox",
      label: t("folderInbox"),
      icon: <InboxIcon className="size-4" />,
      keywords: "inbox",
      run: () => setFolder("INBOX"),
    },
    {
      id: "sent",
      label: t("folderSent"),
      icon: <Send className="size-4" />,
      keywords: "sent",
      run: () => setFolder("Sent"),
    },
    {
      id: "settings",
      label: t("settings"),
      icon: <Settings className="size-4" />,
      run: () => setSettingsOpen(true),
    },
    {
      id: "contacts",
      label: t("contacts"),
      icon: <Users className="size-4" />,
      run: () => setContactsOpen(true),
    },
    {
      id: "theme",
      label: tp("theme"),
      icon: <PenLine className="size-4" />,
      run: () => setTheme(theme === "dark" ? "light" : "dark"),
    },
    {
      id: "density",
      label: `${tp("density")}: ${
        { compact: ts("densityCompact"), cozy: ts("densityCozy"), relaxed: ts("densityRelaxed") }[
          density
        ]
      }`,
      icon: <PenLine className="size-4" />,
      run: () => setDensity(density === "compact" ? "cozy" : density === "cozy" ? "relaxed" : "compact"),
    },
  ];

  const fmtDate = (d: string) =>
    new Date(d).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });

  return (
    <div className="flex h-screen overflow-hidden bg-background text-foreground">
      {!online && (
        <div className="fixed inset-x-0 top-0 z-50 flex items-center justify-center gap-1.5 bg-ai py-1 text-xs text-ai-foreground">
          <WifiOff className="size-3" />
          {t("offline")}
        </div>
      )}
      <FolderNav
        folders={folders}
        current={folder}
        email={me.email}
        open={sidebarOpen}
        onSelect={setFolder}
        onCompose={() => openCompose()}
        onSettings={() => setSettingsOpen(true)}
        onContacts={() => setContactsOpen(true)}
        onSieve={() => setSieveOpen(true)}
        onClose={() => setSidebarOpen(false)}
      />

      <div
        className={cn(
          "min-w-0 flex-col",
          detail ? "hidden md:flex" : "flex",
          "w-full md:w-[var(--list-w)]",
        )}
        style={{ "--list-w": `${listWidth}px` } as React.CSSProperties}
      >
        <MessageListPanel
          folder={folder}
          messages={messages}
          total={total}
          searching={searching}
          loading={loading}
          query={query}
          onQueryChange={setQuery}
          onSearch={doSearch}
          onClearSearch={clearSearch}
          selectedUids={selectedUids}
          cursor={cursor}
          openUid={selected?.uid}
          onOpen={openMessage}
          onToggleSelect={toggleSelect}
          onDelete={removeMessage}
          onStar={toggleStar}
          onArchive={archiveMessage}
          onBulkDelete={bulkDelete}
          onBulkArchive={bulkArchive}
          onBulkSpam={bulkSpam}
          onBulkFlag={bulkFlag}
          onLoadMore={loadMore}
          searchInputRef={searchRef}
          error={error}
          onMenu={() => setSidebarOpen(true)}
          aiEnabled={aiEnabled}
          prioritizing={prioritizing}
          priorityOn={priorityOn}
          onTogglePriority={togglePriority}
          aiSearching={aiSearching}
          onAiSearch={doAiSearch}
          className="flex-1"
        />
      </div>

      <div
        onPointerDown={onResizeStart}
        onPointerMove={onResizeMove}
        onPointerUp={onResizeEnd}
        className="hidden w-1.5 shrink-0 cursor-col-resize transition-colors hover:bg-accent md:block"
        title="Resize"
      />

      <div className={cn("min-w-0 flex-1", detail ? "flex" : "hidden md:flex")}>
        {detail ? (
          <ReadingPane
            key={detail.uid}
            detail={detail}
            aiEnabled={aiEnabled}
            summary={summary}
            summarizing={summarizing}
            onSummarize={summarize}
            onReply={reply}
            onReplyAll={replyAll}
            onForward={forward}
            onArchive={() => archiveMessage(detail)}
            onDelete={() => removeMessage(detail)}
            onStar={() => toggleStar(detail)}
            onBack={backToList}
            thread={thread}
            threadOpen={threadOpen}
            threadLoading={threadLoading}
            onToggleThread={toggleThread}
            onSelectThread={selectThreadMessage}
          />
        ) : (
          <div className="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-sm text-muted-foreground">
            <InboxIcon className="size-9 opacity-40" />
            {t("selectMessage")}
          </div>
        )}
      </div>

      {/* Compose dialog */}
      <Dialog open={composeOpen} onOpenChange={setComposeOpen}>
        <DialogContent className="max-w-xl">
          <form onSubmit={send} className="space-y-3">
            <DialogHeader><DialogTitle>{t("newMessage")}</DialogTitle></DialogHeader>
            {identities.length > 1 && (
              <div className="space-y-1">
                <Label>{t("fromLabel")}</Label>
                <div className="flex flex-wrap gap-1.5">
                  {identities.map((idn) => (
                    <button
                      key={idn.email}
                      type="button"
                      onClick={() => setFrom(idn.email)}
                      className={cn(
                        "flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors",
                        from === idn.email
                          ? "border-primary bg-accent font-medium text-accent-foreground"
                          : "border-border text-muted-foreground hover:bg-muted",
                      )}
                      title={idn.dkim_enabled ? t("dkimOk") : t("dkimMissing")}
                    >
                      <span
                        className={cn(
                          "size-1.5 shrink-0 rounded-full",
                          idn.dkim_enabled ? "bg-primary" : "bg-[#C9A227]",
                        )}
                      />
                      <span className="truncate">{idn.email}</span>
                    </button>
                  ))}
                </div>
              </div>
            )}
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
              <ComposeEditor
                value={body}
                onChange={(html, text) => { setBody(html); setBodyText(text); }}
                placeholder={t("bodyPlaceholder")}
              />
            </div>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <DialogFooter>
              {aiEnabled && detail && (
                <Button
                  type="button"
                  variant="outline"
                  onClick={aiDraftReply}
                  disabled={drafting}
                  className="mr-auto"
                >
                  {drafting ? t("drafting") : t("aiDraft")}
                </Button>
              )}
              <Button type="submit">{t("send")}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <MailSettings
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
        onSaved={() => loadMessages(folder)}
      />

      <MailContacts
        open={contactsOpen}
        onOpenChange={setContactsOpen}
        onPick={(email) => setTo((prev) => (prev ? `${prev}, ${email}` : email))}
        onOpenMessage={(m, src) => {
          openMessage(m, src);
          setContactsOpen(false);
        }}
      />

      <CommandPalette
        open={paletteOpen}
        onOpenChange={setPaletteOpen}
        actions={paletteActions}
      />

      <ShortcutsDialog open={shortcutsOpen} onOpenChange={setShortcutsOpen} />

      <SieveEditor open={sieveOpen} onOpenChange={setSieveOpen} />

      {toast && (
        <div
          key={toast.id}
          className="fixed bottom-4 left-1/2 z-[60] flex -translate-x-1/2 items-center gap-3 rounded-lg border border-border bg-popover px-4 py-2 text-sm text-popover-foreground shadow-lg"
        >
          <span>{toast.label}</span>
          {toast.onUndo && (
            <Button
              size="xs"
              variant="outline"
              onClick={() => {
                const undo = toast?.onUndo;
                setToast(null);
                undo?.();
              }}
            >
              {t("undo")}
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
