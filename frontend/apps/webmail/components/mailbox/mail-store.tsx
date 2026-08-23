"use client";

// MailStore homes the mailbox state, side-effects and action handlers behind
// a single context. MailView (mail-view.tsx) is kept a thin render shell on
// top of useMailStore(), mirroring the backend where mailbox/compose/ai logic
// lives in its own domain package but is wired by the mailbox layer.

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { usePathname, useRouter } from "next/navigation";
import { usePaletteActions } from "@/components/palette/palette-actions";
import { usePreferences } from "@/components/preferences-provider";
import { useNewMailNotification } from "@/components/mailbox/use-new-mail-notification";
import { setupPushSubscription, teardownPushSubscription } from "@/lib/push";
import {
  aiDraft, aiStatus, aiSummarize,
  aiPrioritize, aiSearch,
  accounts, setActiveAccountId,
  contacts, mailFlag, mailMove, mailIdentities,
  mailFolders, mailFolderCreate, mailFolderRename, mailFolderDelete, mailFolderClear,
  mailLabelDelete, mailLabelRename, mailLabelSave, mailLabels, mailMessage, mailMessages, mailSaveDraft, mailSearch, mailSearchSpec, mailSend, mailThread, mailUnseen, mailUndoSend, mailUnsubscribe, mailScheduled,
  mailSnooze, mailSnoozed,
  meProfile, updateMeSettings,
  pgpEncrypt, pgpLookup, pgpSign,
  type Contact, type DraftTone, type MailAccount, type MailIdentity, type MailLabel, type MailMessage, type MailThread, type Me,
  type MailSearchSpec, type OutboundAttachment, type ScheduledSend, type SnoozedMessage,
} from "@/lib/api";
import {
  SYSTEM_FLAGS,
  PIN_FLAG,
  isPinned,
  isSnoozed,
  snoozeUntil,
  SAVED_SEARCH_KEY,
  MAX_ATTACHMENT_BYTES,
  escHtml,
  readFileAsBase64,
  textToHtml,
} from "@/components/mailbox/mail-utils";

// The store context is intentionally untyped for now: every field mirrors a
// local inside the provider, and MailView stays a thin view on top of it.
// A full MailStore interface lands together with the store-splitting refactor.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const MailStoreContext = createContext<any>(null);

// buildSearchSpec merges the free-text keywords with the visual builder
// conditions into a structured spec; null when nothing is active.
function buildSearchSpec(keywords: string, spec: MailSearchSpec | null): MailSearchSpec | null {
  const merged: MailSearchSpec = spec ? { ...spec } : {};
  const words = keywords.split(/\s+/).filter(Boolean);
  if (words.length) {
    merged.text = [...(spec?.text ?? []), ...words];
  }
  const active = Object.entries(merged).some(([, v]) =>
    Array.isArray(v) ? v.length > 0 : Boolean(v),
  );
  return active ? merged : null;
}

export function useMailStore() {
  return useContext(MailStoreContext);
}

interface MailStoreProviderProps {
  me: Me;
  children: React.ReactNode;
}

export function MailStoreProvider({ me, children }: MailStoreProviderProps) {
  const t = useTranslations("mail");
  const tp = useTranslations("palette");
  const ts = useTranslations("settings");
  const router = useRouter();
  const { theme, setTheme, density, setDensity, prefs, setUndoSend } = usePreferences();
  useNewMailNotification(prefs.notifications, t);

  // ---- mailbox: folder / list / selection ----
  const [folders, setFolders] = useState<string[]>([]);
  const [unseen, setUnseen] = useState<Record<string, number>>({});
  const [folder, setFolder] = useState("Inbox");
  const [messages, setMessages] = useState<MailMessage[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [cursor, setCursor] = useState(0);
  const [selected, setSelected] = useState<MailMessage | null>(null);
  const [detail, setDetail] = useState<MailMessage | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [selectedUids, setSelectedUids] = useState<Set<number>>(new Set());
  const [query, setQuery] = useState("");
  const [searchSpec, setSearchSpec] = useState<MailSearchSpec | null>(null);
  const [searching, setSearching] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");

  // ---- aggregated external accounts (full aggregation client) ----
  const [accountList, setAccountList] = useState<MailAccount[]>([]);
  // null = the internal gateway account; a number = an external mailbox.
  const [activeAccount, setActiveAccount] = useState<number | null>(null);
  const switchInFlight = useRef(false);

  const refreshAccounts = useCallback(async () => {
    try {
      setAccountList(await accounts());
    } catch {
      // account listing is optional; the internal account always works
    }
  }, []);

  // Load the account list once; keep the active account in the api module so
  // every /mail/* request is scoped to it.
  useEffect(() => {
    refreshAccounts();
  }, [refreshAccounts]);
  useEffect(() => {
    setActiveAccountId(activeAccount);
  }, [activeAccount]);

  // Switching accounts resets the mailbox view and re-scopes all requests.
  async function switchAccount(id: number | null) {
    if (id === activeAccount || switchInFlight.current) return;
    switchInFlight.current = true;
    setActiveAccount(id);
    setFolder("Inbox");
    setMessages([]);
    setSelected(null);
    setDetail(null);
    setThread(null);
    setThreadOpen(false);
    setQuery("");
    setSearching(false);
    setSelectedUids(new Set());
    setCursor(0);
    switchInFlight.current = false;
    router.push("/mail/Inbox");
    loadFolders();
  }

  // openSettingsSection opens the settings dialog on a specific section.
  function openSettingsSection(section: string) {
    setSettingsInitialSection(section);
    setSettingsOpen(true);
  }

  // ---- stack: compose / settings / contacts / palette / sieve / sidebar ----
  const [composeOpen, setComposeOpen] = useState(false);
  // Where the initial focus should land when the compose dialog opens:
  // "to" (new message / forward) or "editor" (reply / reply all).
  const [composeFocus, setComposeFocus] = useState<"to" | "editor">("to");
  const [settingsOpen, setSettingsOpen] = useState(false);
  // The settings section to land on when the dialog opens (e.g. "accounts"
  // from the sidebar account manager); defaults to the first section.
  const [settingsInitialSection, setSettingsInitialSection] = useState("appearance");
  const [contactsOpen, setContactsOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const [sieveOpen, setSieveOpen] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [listWidth, setListWidth] = useState(360);

  // ---- ai: capability, summary, priority, search ----
  const [aiEnabled, setAiEnabled] = useState(false);
  const [summary, setSummary] = useState("");
  const [summarizing, setSummarizing] = useState(false);
  const [drafting, setDrafting] = useState(false);
  const [prioritizing, setPrioritizing] = useState(false);
  const [aiSearching, setAiSearching] = useState(false);
  const [priorityOn, setPriorityOn] = useState(false);
  const [priorityCategories, setPriorityCategories] = useState<Record<string, string>>({});
  const [activeView, setActiveView] = useState("all");
  const [activeLabel, setActiveLabel] = useState("");
  // Deterministic auto-category filter ("" = all) applied to the loaded list.
  const [categoryFilter, setCategoryFilter] = useState("");
  // Label definitions (name + color) persisted server-side; flagLabels are
  // keywords seen on loaded messages. The union is what the UI offers.
  const [labelDefs, setLabelDefs] = useState<MailLabel[]>([]);
  const [flagLabels, setFlagLabels] = useState<string[]>([]);
  const [labelManagerOpen, setLabelManagerOpen] = useState(false);
  const [savedSearches, setSavedSearches] = useState<string[]>([]);
  const [searchAll, setSearchAll] = useState(false);
  const [baseMessages, setBaseMessages] = useState<MailMessage[] | null>(null);
  const [identities, setIdentities] = useState<MailIdentity[]>([]);
  const [from, setFrom] = useState("");
  const [toast, setToast] = useState<{ id: number; label: string; onUndo?: () => void } | null>(null);
  const [online, setOnline] = useState(true);
  const [thread, setThread] = useState<MailThread | null>(null);
  const [threadOpen, setThreadOpen] = useState(false);
  const [threadLoading, setThreadLoading] = useState(false);
  // Scheduled sends: the backend parks messages in the outbox until send_at;
  // this dialog lists and cancels them.
  const [scheduled, setScheduled] = useState<ScheduledSend[]>([]);
  const [scheduledOpen, setScheduledOpen] = useState(false);
  const [scheduledLoading, setScheduledLoading] = useState(false);
  // Snoozed messages: keyword-based; the backend resurfaced due ones lazily.
  const [snoozedMsgs, setSnoozedMsgs] = useState<SnoozedMessage[]>([]);
  const [snoozedLoading, setSnoozedLoading] = useState(false);

  // ---- compose: form fields ----
  const [to, setTo] = useState<string[]>([]);
  const [cc, setCc] = useState<string[]>([]);
  const [bcc, setBcc] = useState<string[]>([]);
  const [ccExpanded, setCcExpanded] = useState(false);
  const [attachments, setAttachments] = useState<OutboundAttachment[]>([]);
  const [dragOverCompose, setDragOverCompose] = useState(false);
  const [ctxMenu, setCtxMenu] = useState<{ x: number; y: number; message: MailMessage } | null>(null);
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [bodyText, setBodyText] = useState("");
  const [draftTone, setDraftTone] = useState<DraftTone>("formal");
  const [signOn, setSignOn] = useState(false);
  const [encryptOn, setEncryptOn] = useState(false);
  const [draftSaved, setDraftSaved] = useState(false);
  // Optional RFC3339 timestamp (from the compose datetime input) that turns a
  // send into a scheduled send; null means "send now".
  const [scheduleAt, setScheduleAt] = useState<string>("");
  const [allContacts, setAllContacts] = useState<Contact[] | null>(null);

  const searchRef = useRef<HTMLInputElement>(null);
  const toInputRef = useRef<HTMLInputElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const loadingMoreRef = useRef(false);
  // Guards loadMessages responses: increments on every call so an older
  // in-flight request can detect it has been superseded and drop its result.
  const loadSeq = useRef(0);
  const pendingG = useRef(false);
  const draftUidRef = useRef<number | null>(null);
  const draftTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Serializes draft saves: while a save is in flight the draft UID is not yet
  // known, so a second overlapping save would create a duplicate draft instead
  // of updating the first one.
  const draftSavingRef = useRef(false);
  const lastSearchRef = useRef("");
  const resizeRef = useRef<{ x: number; w: number } | null>(null);
  const toastTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // ---- core: unseen / folders / list loading ----
  const refreshUnseen = useCallback(() => {
    mailUnseen().then(setUnseen).catch(() => {});
  }, []);

  const loadFolders = useCallback(async () => {
    try {
      setFolders(await mailFolders());
    } catch (e) {
      setError(e instanceof Error ? e.message : "load folders failed");
    }
    refreshUnseen();
  }, [refreshUnseen]);

  // ---- folder management (create / rename / delete / clear) ----
  async function createFolder(name: string) {
    setError("");
    try {
      await mailFolderCreate(name);
      await loadFolders();
      showToast(t("toastFolderCreated", { name }));
    } catch (e) {
      setError(e instanceof Error ? e.message : "create folder failed");
    }
  }

  async function renameFolder(name: string, newName: string) {
    setError("");
    try {
      await mailFolderRename(name, newName);
      await loadFolders();
      if (folder.toLowerCase() === name.toLowerCase()) selectFolder(newName);
      showToast(t("toastFolderRenamed", { name }));
    } catch (e) {
      setError(e instanceof Error ? e.message : "rename folder failed");
    }
  }

  async function deleteFolder(name: string) {
    setError("");
    try {
      await mailFolderDelete(name);
      await loadFolders();
      if (folder.toLowerCase() === name.toLowerCase()) selectFolder("Inbox");
      showToast(t("toastFolderDeleted", { name }));
    } catch (e) {
      setError(e instanceof Error ? e.message : "delete folder failed");
    }
  }

  async function clearFolder(name: string) {
    setError("");
    try {
      await mailFolderClear(name);
      refreshUnseen();
      await loadMessages(folder);
      showToast(t("toastFolderCleared", { name }));
    } catch (e) {
      setError(e instanceof Error ? e.message : "clear folder failed");
    }
  }

  // ---- scheduled sends (list / cancel) ----
  const loadScheduled = useCallback(async () => {
    setScheduledLoading(true);
    try {
      setScheduled(await mailScheduled());
    } catch (e) {
      setError(e instanceof Error ? e.message : "load scheduled failed");
    } finally {
      setScheduledLoading(false);
    }
  }, []);

  async function cancelScheduled(id: number) {
    setError("");
    try {
      await mailUndoSend(id);
      await loadScheduled();
      showToast(t("toastScheduledCancelled"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "cancel scheduled failed");
    }
  }

  const loadMessages = useCallback(async (f: string, p = 0, silent = false) => {
    // A monotonically increasing seq lets stale responses drop themselves: the
    // mount effect loads the initial folder before the pathname effect has
    // corrected it, so two requests race and the slower (older) one must not
    // overwrite the current folder's list.
    const seq = ++loadSeq.current;
    if (p === 0 && !silent) setLoading(true);
    try {
      const res = await mailMessages(f, p);
      if (seq !== loadSeq.current) return;
      setMessages(p === 0 ? res.messages : (prev) => [...prev, ...res.messages]);
      setTotal(res.total);
      setPage(p);
    } catch (e) {
      if (seq !== loadSeq.current) return;
      setError(e instanceof Error ? e.message : "load messages failed");
    } finally {
      if (seq === loadSeq.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadFolders();
  }, [loadFolders]);

  // Auto-save the draft 30s after the user stops typing, replacing the
  // previous auto-save so one compose session keeps exactly one draft.
  useEffect(() => {
    if (!composeOpen) return;
    const hasContent =
      to.length > 0 || cc.length > 0 || subject.trim() !== "" || bodyText.trim() !== "" || attachments.length > 0;
    if (!hasContent) return;
    setDraftSaved(false);
    if (draftTimerRef.current) clearTimeout(draftTimerRef.current);
    draftTimerRef.current = setTimeout(async () => {
      if (draftSavingRef.current) return; // a manual save already persists this content
      draftSavingRef.current = true;
      try {
        const res = await mailSaveDraft(subject, bodyText, body, draftUidRef.current ?? 0, to, cc, attachments);
        draftUidRef.current = res.uid || draftUidRef.current;
        setDraftSaved(true);
      } catch {
        // silent: keep editing, the next idle window retries
      } finally {
        draftSavingRef.current = false;
      }
    }, 30000);
    return () => {
      if (draftTimerRef.current) clearTimeout(draftTimerRef.current);
    };
  }, [composeOpen, to, cc, subject, bodyText, body, attachments]);

  // saveDraftNow writes the draft immediately (manual "save draft" button);
  // auto-save also runs 30s after the user stops typing.
  async function saveDraftNow() {
    const hasContent =
      to.length > 0 || cc.length > 0 || subject.trim() !== "" || bodyText.trim() !== "" || attachments.length > 0;
    if (!hasContent) return;
    if (draftSavingRef.current) return; // ignore rapid repeated clicks; the first save persists
    draftSavingRef.current = true;
    setError("");
    try {
      const res = await mailSaveDraft(subject, bodyText, body, draftUidRef.current ?? 0, to, cc, attachments);
      draftUidRef.current = res.uid || draftUidRef.current;
      setDraftSaved(true);
      refreshDraftsIfActive();
    } catch (e) {
      setError(e instanceof Error ? e.message : "save draft failed");
    } finally {
      draftSavingRef.current = false;
    }
  }

  // A draft save lands in the Drafts folder; if the user is browsing Drafts,
  // silently reload its list so the new/updated draft appears right away.
  const refreshDraftsIfActive = useCallback(() => {
    if (folder === "Drafts") void loadMessages("Drafts", 0, true);
  }, [folder, loadMessages]);

  // closeCompose saves the draft (fire-and-forget) and dismisses the panel.
  // Without this, closing within the 30s auto-save window would lose edits.
  function closeCompose() {
    const hasContent =
      to.length > 0 || cc.length > 0 || subject.trim() !== "" || bodyText.trim() !== "" || attachments.length > 0;
    // A create (uid == null) must not race another in-flight create, otherwise
    // two drafts appear; updating an existing draft is always safe.
    if (hasContent && (draftUidRef.current != null || !draftSavingRef.current)) {
      mailSaveDraft(subject, bodyText, body, draftUidRef.current ?? 0, to, cc, attachments)
        .then((res) => {
          draftUidRef.current = res.uid || draftUidRef.current;
          refreshDraftsIfActive();
        })
        .catch(() => {});
    }
    setComposeOpen(false);
  }

  // Esc dismisses the compose panel (no Base UI dialog to handle it anymore).
  useEffect(() => {
    if (!composeOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") closeCompose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [composeOpen, to, cc, subject, bodyText, body, attachments]);

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
    setPriorityCategories({});
    setActiveView("all");
    setActiveLabel("");
    setBaseMessages(null);
    loadMessages(folder);
  }, [folder, loadMessages]);

  // The URL (/mail/[folder] or /mail/[folder]/[uid]) is the single source of
  // truth for the current folder and opened message. A route folder change is
  // applied to local state (which then loads the message list); a route uid
  // opens that message (optimistically from the current list row, then the
  // full detail).
  const pathname = usePathname();
  const segs = (pathname ?? "").split("/").filter(Boolean);
  const pathFolder = segs[1] || "Inbox";
  const pathId = segs.length > 2 ? segs[2] : null;

  useEffect(() => {
    if (pathFolder !== folder) setFolder(pathFolder);
  }, [pathFolder, folder]);

  useEffect(() => {
    if (!pathId) {
      setSelected(null);
      setDetail(null);
      setThreadOpen(false);
      setDetailLoading(false);
      return;
    }
    let cancelled = false;
    setDetailLoading(true);
    // Optimistically show the row message (if already loaded) while the full
    // detail is fetched. Prefer the row's UID (we already have it, no reverse
    // lookup needed and it can't "not found"); only fall back to the stable id
    // reverse lookup when the row isn't in the mounted list (e.g. fresh deep
    // link where the list hasn't loaded yet).
    const row = messages.find((m) => m.id === pathId || m.uid === Number(pathId));
    if (row) setSelected(row);
    // A numeric id is the degenerate uid fallback used when a message has no
    // Message-ID header; routing it through the uid path avoids a pointless
    // (and error-prone) reverse lookup.
    const numericId = /^\d+$/.test(pathId);
    const fetch = row
      ? mailMessage(pathFolder, { uid: row.uid })
      : numericId
        ? mailMessage(pathFolder, { uid: Number(pathId) })
        : mailMessage(pathFolder, { id: pathId });
    fetch
      .then((full) => {
        if (cancelled) return;
        setSelected(full);
        setDetail(full);
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : "load message failed");
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pathId]);

  // Prefer the existing junk folder name (Spam in some setups) over
  // creating a duplicate Junk mailbox.
  const spamFolder = useMemo(() => {
    const hit = folders.find((f) => /^(junk|spam)$/i.test(f));
    return hit || "Junk";
  }, [folders]);

  // Collect user labels (custom IMAP keywords) from loaded messages so the
  // sidebar and reader can offer them for quick tagging/filtering; the list
  // is merged with the server-side definitions in knownLabels below.
  useEffect(() => {
    setFlagLabels((prev) => {
      const set = new Set(prev);
      messages.forEach((m) =>
        m.flags.forEach((f) => {
          if (!f.startsWith("\\") && !SYSTEM_FLAGS.has(f)) set.add(f);
        }),
      );
      return [...set];
    });
  }, [messages]);

  const knownLabels = useMemo(() => {
    const set = new Set<string>(labelDefs.map((d) => d.name));
    flagLabels.forEach((f) => set.add(f));
    return [...set];
  }, [labelDefs, flagLabels]);

  const labelColors = useMemo(() => {
    const out: Record<string, string> = {};
    labelDefs.forEach((d) => {
      if (d.color) out[d.name] = d.color;
    });
    return out;
  }, [labelDefs]);

  // Load label definitions once so the sidebar lists labels even when no
  // currently loaded message carries the keyword.
  useEffect(() => {
    mailLabels().then(setLabelDefs).catch(() => {});
  }, []);

  async function toggleLabel(m: MailMessage, label: string) {
    const has = m.flags.includes(label);
    setError("");
    // Applying a brand-new tag creates its definition so it stays listed and
    // gets a stable color; the palette fallback covers races.
    if (!has && !labelDefs.some((d) => d.name === label)) {
      mailLabelSave(label, "")
        .then((saved) => setLabelDefs((ds) => [...ds, saved]))
        .catch(() => {});
    }
    try {
      await mailFlag(folder, m.uid, label, !has);
      setMessages((ms) =>
        ms.map((x) =>
          x.uid === m.uid
            ? { ...x, flags: has ? x.flags.filter((f) => f !== label) : [...x.flags, label] }
            : x,
        ),
      );
      setDetail((d) =>
        d && d.uid === m.uid
          ? { ...d, flags: has ? d.flags.filter((f) => f !== label) : [...d.flags, label] }
          : d,
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "label failed");
    }
  }

  // saveLabel creates/updates a label definition (name + color).
  async function saveLabel(name: string, color: string) {
    const saved = await mailLabelSave(name, color);
    setLabelDefs((ds) => {
      const i = ds.findIndex((d) => d.name === name);
      if (i < 0) return [...ds, saved];
      const copy = [...ds];
      copy[i] = saved;
      return copy;
    });
  }

  // renameLabel renames the keyword on every message (server-side) and
  // updates the local definitions and flags optimistically.
  async function renameLabel(from: string, to: string) {
    await mailLabelRename(from, to);
    setLabelDefs((ds) => ds.map((d) => (d.name === from ? { ...d, name: to } : d)));
    const swap = (flags: string[]) => flags.map((f) => (f === from ? to : f));
    setMessages((ms) => ms.map((x) => (x.flags.includes(from) ? { ...x, flags: swap(x.flags) } : x)));
    setDetail((d) => (d && d.flags.includes(from) ? { ...d, flags: swap(d.flags) } : d));
    setFlagLabels((fs) => fs.map((f) => (f === from ? to : f)));
    if (activeLabel === from) selectLabel(to);
  }

  // deleteLabel strips the keyword from every message (server-side) and
  // removes the definition plus the local flag occurrences.
  async function deleteLabel(name: string) {
    await mailLabelDelete(name);
    setLabelDefs((ds) => ds.filter((d) => d.name !== name));
    setFlagLabels((fs) => fs.filter((f) => f !== name));
    setMessages((ms) => ms.map((x) => ({ ...x, flags: x.flags.filter((f) => f !== name) })));
    setDetail((d) => (d ? { ...d, flags: d.flags.filter((f) => f !== name) } : d));
    if (activeLabel === name) selectLabel("");
  }

  // clamp cursor when the list shrinks
  useEffect(() => {
    setCursor((c) => Math.min(c, Math.max(0, messages.length - 1)));
  }, [messages.length]);

  // load AI capability once; AI summary/draft only show when a provider is configured
  useEffect(() => {
    aiStatus().then((s) => setAiEnabled(s.enabled)).catch(() => setAiEnabled(false));
  }, []);

  // Effective per-feature AI flags: backend availability AND the user toggles.
  const ai = useMemo(
    () => ({
      summary: aiEnabled && prefs.ai.enabled && prefs.ai.summary,
      draft: aiEnabled && prefs.ai.enabled && prefs.ai.draft,
      priority: aiEnabled && prefs.ai.enabled && prefs.ai.priority,
      search: aiEnabled && prefs.ai.enabled && prefs.ai.search,
    }),
    [aiEnabled, prefs.ai],
  );

  // load the From identities (own address + aliases with DKIM status)
  useEffect(() => {
    mailIdentities()
      .then((ids) => {
        setIdentities(ids);
        setFrom((f) => f || ids[0]?.email || me.email);
      })
      .catch(() => setFrom(me.email));
  }, [me.email]);

  // Register/unregister the push subscription with the backend when the
  // notification preference changes.
  useEffect(() => {
    if (!me.email) return;
    if (prefs.notifications) {
      setupPushSubscription().catch(() => {});
    } else {
      teardownPushSubscription().catch(() => {});
    }
  }, [me.email, prefs.notifications]);

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

  function showToast(label: string, onUndo?: () => void, duration = 5000) {
    if (toastTimer.current) clearTimeout(toastTimer.current);
    setToast({ id: Date.now(), label, onUndo });
    toastTimer.current = setTimeout(() => setToast(null), duration);
  }

  // moveTo moves messages to a folder and offers an undo that moves them back.
  async function moveTo(uids: number[], destination: string, successLabel: string) {
    setError("");
    try {
      await mailMove(folder, uids, destination);
      refreshUnseen();
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

  // Report not-spam: whitelist the sender and move the message back to Inbox.
  async function reportNotSpam(m: MailMessage) {
    const sender = m.from[0]?.email;
    setError("");
    try {
      if (sender) {
        const profile = await meProfile();
        const current = (profile.whitelist || "").split(",").map((s) => s.trim()).filter(Boolean);
        if (!current.includes(sender)) current.push(sender);
        await updateMeSettings({ whitelist: current.join(", ") });
      }
      await moveTo([m.uid], "Inbox", t("toastNotSpam"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "not spam failed");
    }
  }

  // unsubscribeAction follows the sender's List-Unsubscribe header: mailto
  // opens a pre-filled compose, https is triggered server-side.
  async function unsubscribeAction(m: MailMessage) {
    const url = m.unsubscribe_url;
    if (!url) return;
    if (url.startsWith("mailto:")) {
      const addr = url.slice("mailto:".length).split("?")[0];
      openCompose(addr, "Unsubscribe", "", "", "editor");
      return;
    }
    setError("");
    try {
      await mailUnsubscribe(url, !!m.unsubscribe_post);
      showToast(t("toastUnsubscribed"));
    } catch (e) {
      setError(e instanceof Error ? e.message : t("toastUnsubscribeFailed"));
    }
  }

  async function doSearch(e?: React.FormEvent | string) {
    if (typeof e !== "string") e?.preventDefault();
    const q = (typeof e === "string" ? e : query).trim();
    const spec = buildSearchSpec(q, searchSpec);
    if (!spec) {
      setSearching(false);
      setActiveView("all");
      setActiveLabel("");
      return;
    }
    await runSearchWithSpec(spec);
  }

  // runSearchWithSpec executes a structured search (visual builder or merged
  // keywords) straight against /mail/search — no syntax-string round-trip.
  async function runSearchWithSpec(spec: MailSearchSpec) {
    setSearching(true);
    setSelected(null);
    setDetail(null);
    setCursor(0);
    try {
      setMessages(await mailSearchSpec(searchAll ? "all" : folder, spec));
      lastSearchRef.current = spec.text?.join(" ") ?? "";
    } catch (err) {
      setError(err instanceof Error ? err.message : "search failed");
    }
  }

  // applySearchSpec is the visual builder's submit: it replaces the keyword
  // box with the structured conditions and searches immediately.
  function applySearchSpec(spec: MailSearchSpec) {
    setSearchSpec(spec);
    setQuery("");
    setActiveView("all");
    setActiveLabel("");
    runSearchWithSpec(spec);
  }

  // Instant search: debounce manual typing so results appear live (except when
  // a virtual view / label filter already ran its own explicit search).
  useEffect(() => {
    const q = query.trim();
    if (q === "" || activeView !== "all" || activeLabel !== "" || q === lastSearchRef.current) return;
    const id = setTimeout(() => doSearch(q), 400);
    return () => clearTimeout(id);
  }, [query, activeView, activeLabel]);

  function clearSearch() {
    setSearching(false);
    setQuery("");
    setSearchSpec(null);
    setActiveView("all");
    setActiveLabel("");
    lastSearchRef.current = "";
    setCursor(0);
    loadMessages(folder);
  }

  // refreshMail reloads the current view without blanking the list: re-runs an
  // active search, otherwise refetches the folder plus unseen counts.
  async function refreshMail() {
    setRefreshing(true);
    try {
      if (searching) {
        await doSearch(query);
      } else {
        await Promise.all([loadMessages(folder, 0, true), mailUnseen().then(setUnseen)]);
      }
    } finally {
      setRefreshing(false);
    }
  }

  // Virtual views: unread / starred / attachments / snoozed as FastMail-style
  // filters. Snoozed is a dedicated fetch (keyword-driven), not a search.
  function selectView(view: string) {
    setActiveView(view);
    if (view === "snoozed") {
      setQuery("");
      setSearching(false);
      loadSnoozed();
      return;
    }
    const q =
      view === "unread"
        ? "is:unread"
        : view === "flagged"
          ? "is:flagged"
          : view === "attachment"
            ? "has:attachment"
            : "";
    if (q) {
      setQuery(q);
      doSearch(q);
    } else {
      setQuery("");
      clearSearch();
    }
  }

  // Label filter (clicking a tag in the sidebar).
  function selectLabel(label: string) {
    setActiveView("all");
    setActiveLabel(label);
    const q = label ? `label:${label}` : "";
    if (q) {
      setQuery(q);
      doSearch(q);
    } else {
      setQuery("");
      clearSearch();
    }
  }

  // Saved searches (virtual folders).
  useEffect(() => {
    try {
      const raw = localStorage.getItem(SAVED_SEARCH_KEY);
      if (raw) setSavedSearches(JSON.parse(raw));
    } catch {
      // storage unavailable
    }
  }, []);

  function persistSearches(next: string[]) {
    setSavedSearches(next);
    try {
      localStorage.setItem(SAVED_SEARCH_KEY, JSON.stringify(next));
    } catch {
      // storage unavailable
    }
  }

  function saveCurrentSearch() {
    const q = query.trim();
    if (!q || savedSearches.includes(q)) return;
    persistSearches([...savedSearches, q]);
  }

  function removeSavedSearch(q: string) {
    persistSearches(savedSearches.filter((s) => s !== q));
  }

  function runSavedSearch(q: string) {
    setQuery(q);
    doSearch(q);
  }

  function openMessage(m: MailMessage, srcFolder = folder) {
    if (!m.flags.includes("\\Seen")) {
      mailFlag(srcFolder, m.uid, "\\Seen", true).then(refreshUnseen).catch(() => {});
      m.flags.push("\\Seen");
      setMessages((ms) => ms.map((x) => (x.uid === m.uid ? m : x)));
    }
    // Navigate to the message route; MailView follows the /mail/[folder]/[id]
    // URL (pathId effect) to load and show the reading pane. The stable id
    // keeps every view routable, shareable and survives mailbox moves.
    router.push(`/mail/${srcFolder}/${m.id || m.uid}`);
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

  function moveSelectedTo(destination: string) {
    return moveTo([...selectedUids], destination, t("toastMoved"));
  }

  function moveDetailTo(destination: string) {
    if (detail) return moveTo([detail.uid], destination, t("toastMoved"));
  }

  async function bulkFlag(flag: string, value: boolean) {
    const ids = [...selectedUids];
    setError("");
    try {
      await Promise.all(ids.map((uid) => mailFlag(folder, uid, flag, value)));
      refreshUnseen();
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
      refreshUnseen();
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

  // Pinned messages stay at the top of the list via the $Pin keyword.
  async function togglePin(m: MailMessage) {
    const pinned = !isPinned(m);
    try {
      await mailFlag(folder, m.uid, PIN_FLAG, pinned);
      setMessages((ms) =>
        ms.map((x) =>
          x.uid === m.uid
            ? {
                ...x,
                flags: pinned
                  ? [...new Set([...x.flags, PIN_FLAG])]
                  : x.flags.filter((f) => f !== PIN_FLAG),
              }
            : x,
        ),
      );
      if (detail?.uid === m.uid) {
        setDetail((d) =>
          d
            ? {
                ...d,
                flags: pinned
                  ? [...new Set([...d.flags, PIN_FLAG])]
                  : d.flags.filter((f) => f !== PIN_FLAG),
              }
            : d,
        );
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "pin failed");
    }
  }

  async function loadSnoozed() {
    setSnoozedLoading(true);
    setError("");
    try {
      setSnoozedMsgs(await mailSnoozed());
    } catch (e) {
      setError(e instanceof Error ? e.message : "load snoozed failed");
    } finally {
      setSnoozedLoading(false);
    }
  }

  // Snooze hides a message until a given time; untilMs is 0 to wake it up.
  async function snoozeMessage(m: MailMessage, untilMs: number) {
    const f = m.folder || folder;
    try {
      await mailSnooze(f, m.uid, untilMs > 0 ? Math.floor(untilMs / 1000) : null);
      if (untilMs > 0) {
        // Hide from the current list; it now lives under the Snoozed view.
        setMessages((ms) => ms.filter((x) => x.uid !== m.uid));
        showToast(t("toastSnoozed"));
      } else {
        setSnoozedMsgs((ms) => ms.filter((x) => x.uid !== m.uid));
        refreshMail();
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "snooze failed");
    }
  }

  // Wake a snoozed message back into its folder.
  async function unsnooze(sm: SnoozedMessage) {
    const f = sm.folder || folder;
    try {
      await mailSnooze(f, sm.uid, null);
      setSnoozedMsgs((ms) => ms.filter((x) => x.uid !== sm.uid));
      refreshMail();
    } catch (e) {
      setError(e instanceof Error ? e.message : "unsnooze failed");
    }
  }

  // displayMessages drops snoozed messages from normal views and floats
  // pinned ones to the top, without mutating the raw list state.
  const displayMessages = useMemo(() => {
    const visible = activeView === "snoozed" ? messages : messages.filter((m) => !isSnoozed(m));
    if (!visible.some(isPinned)) return visible;
    const pinned: MailMessage[] = [];
    const rest: MailMessage[] = [];
    for (const m of visible) (isPinned(m) ? pinned : rest).push(m);
    return [...pinned, ...rest];
  }, [messages, activeView]);

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

  // Focus the recipient field when the compose dialog opens in "to" mode
  // (new message / forward). A short delay keeps the focus from being
  // stolen by Base UI's dialog focus management.
  useEffect(() => {
    if (!composeOpen || composeFocus !== "to") return;
    const tm = setTimeout(() => toInputRef.current?.focus(), 60);
    return () => clearTimeout(tm);
  }, [composeOpen, composeFocus]);

  function openCompose(toAddr = "", subj = "", html = "", text = "", focus: "to" | "editor" = "to") {
    const identity = identities.find((i) => i.email === from);
    const sig = identity?.signature?.trim() || me.signature?.trim();
    let finalHtml = html;
    let finalText = text;
    if (sig) {
      const sigHtml = sig
        .split("\n")
        .map((l) => (l.trim() ? `<p>${escHtml(l)}</p>` : "<p><br></p>"))
        .join("");
      finalHtml = `${html}<p><br></p><p>--</p>${sigHtml}`;
      finalText = text ? `${text}\n\n-- \n${sig}` : `-- \n${sig}`;
    }
    setTo(toAddr ? toAddr.split(",").map((s) => s.trim()).filter(Boolean) : []);
    setCc([]);
    setBcc([]);
    setCcExpanded(false);
    setAttachments([]);
    setSubject(subj);
    setBody(finalHtml);
    setBodyText(finalText);
    setSignOn(false);
    setEncryptOn(false);
    setDraftSaved(false);
    draftUidRef.current = null;
    setComposeFocus(focus);
    setComposeOpen(true);
  }

  async function addFiles(list: FileList | File[]) {
    try {
      const files = Array.from(list);
      const oversized = files.find((f) => f.size > MAX_ATTACHMENT_BYTES);
      if (oversized) {
        setError(t("attachmentTooLarge", { name: oversized.name }));
        return;
      }
      const ready = await Promise.all(files.map(readFileAsBase64));
      setAttachments((prev) => [...prev, ...ready]);
      setError("");
    } catch {
      setError(t("attachFailed"));
    }
  }

  async function toggleRead(m: MailMessage) {
    const value = !m.flags.includes("\\Seen");
    try {
      await mailFlag(m.folder || folder, m.uid, "\\Seen", value);
      const apply = (x: MailMessage) =>
        x.uid === m.uid
          ? { ...x, flags: value ? [...x.flags, "\\Seen"] : x.flags.filter((f) => f !== "\\Seen") }
          : x;
      setMessages((ms) => ms.map(apply));
      setDetail((d) => (d && d.uid === m.uid ? apply(d) : d));
    } catch (e) {
      setError(e instanceof Error ? e.message : "mark read failed");
    }
  }

  function openContextMenu(e: React.MouseEvent, m: MailMessage) {
    e.preventDefault();
    setCtxMenu({ x: e.clientX, y: e.clientY, message: m });
  }

  function replyAllFrom(m: MailMessage) {
    const recipients = new Set<string>();
    [...m.from, ...(m.cc || []), ...(m.to || [])].forEach((a) => {
      if (a.email && a.email.toLowerCase() !== me.email.toLowerCase()) recipients.add(a.email);
    });
    openCompose(
      [...recipients].join(", "),
      m.subject.startsWith("Re:") ? m.subject : `Re: ${m.subject}`,
      textToHtml(quoteText(m)),
      quoteText(m),
      "editor",
    );
  }

  // Close the row context menu on Escape.
  useEffect(() => {
    if (!ctxMenu) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setCtxMenu(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [ctxMenu]);

  // Swapping the From identity replaces the appended signature in place.
  function applySignature(text: string, html: string, sig: string) {
    const sigHtml = sig
      .split("\n")
      .map((l) => (l.trim() ? `<p>${escHtml(l)}</p>` : "<p><br></p>"))
      .join("");
    const cleanText = text.replace(/\n\n-- \n[\s\S]*$/, "");
    const marker = "<p><br></p><p>--</p>";
    const cleanHtml = (() => {
      const idx = html.lastIndexOf(marker);
      return idx >= 0 ? html.slice(0, idx) : html;
    })();
    return {
      text: cleanText ? `${cleanText}\n\n-- \n${sig}` : `-- \n${sig}`,
      html: `${cleanHtml}${marker}${sigHtml}`,
    };
  }

  function selectIdentity(email: string) {
    setFrom(email);
    const idn = identities.find((i) => i.email === email);
    const sig = idn?.signature?.trim();
    if (sig) {
      const next = applySignature(bodyText, body, sig);
      setBodyText(next.text);
      setBody(next.html);
    }
  }

  // Recipient auto-suggest: lazily load the address book once and match the
  // last comma-separated token typed in the To field.
  function loadContactsOnce() {
    if (allContacts === null) {
      contacts().then(setAllContacts).catch(() => {});
    }
  }

  function reply() {
    if (!detail) return;
    openCompose(
      detail.from[0]?.email || "",
      detail.subject.startsWith("Re:") ? detail.subject : `Re: ${detail.subject}`,
      textToHtml(quoteText(detail)),
      quoteText(detail),
      "editor",
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
      "editor",
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
      const res = await aiDraft(detail.text_body || detail.html_body || "", draftTone);
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
      setPriorityCategories({});
      setBaseMessages(null);
      return;
    }
    if (!ai.priority || messages.length === 0) return;
    setPrioritizing(true);
    setError("");
    try {
      const items = messages.map((m) => ({
        uid: m.uid,
        subject: m.subject,
        from: m.from[0]?.email || "",
      }));
      const { scores, categories } = await aiPrioritize(items);
      setBaseMessages(messages);
      setMessages(
        [...messages].sort(
          (a, b) => (scores[String(b.uid)] ?? 0) - (scores[String(a.uid)] ?? 0),
        ),
      );
      setPriorityCategories(categories ?? {});
      setPriorityOn(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "prioritize failed");
    } finally {
      setPrioritizing(false);
    }
  }

  async function doAiSearch(q?: string) {
    const searchQuery = (q ?? query).trim();
    if (!searchQuery || !ai.search) return;
    setAiSearching(true);
    setError("");
    try {
      const res = await aiSearch(searchQuery);
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
      let text = bodyText;
      let html = body;
      if (signOn) {
        const { signature } = await pgpSign(text || " ");
        text = `${text}\n\n${signature}`;
        html = textToHtml(text);
      }
      if (encryptOn) {
        const recipient = to[0]?.trim();
        if (!recipient) throw new Error(t("pgpNoRecipient"));
        let publicKey: string;
        try {
          publicKey = (await pgpLookup(recipient)).public_key;
        } catch {
          throw new Error(t("pgpNoKeyForRecipient"));
        }
        const { encrypted } = await pgpEncrypt(text || " ", publicKey);
        text = encrypted;
        html = "";
      }

      const finalTo = [...to];
      const finalCc = [...cc];
      const finalBcc = [...bcc];
      const finalAttachments = [...attachments];
      const finalSubject = subject;
      const finalText = text;
      const finalHtml = html;
      const finalFrom = from;
      const delay = prefs.undoSendSeconds;
      // datetime-local value -> RFC3339 (interpreted as local time).
      const sendAt = scheduleAt ? new Date(scheduleAt).toISOString() : undefined;

      const resetCompose = () => {
        setComposeOpen(false);
        setTo([]);
        setCc([]);
        setBcc([]);
        setCcExpanded(false);
        setAttachments([]);
        setSubject("");
        setBody("");
        setBodyText("");
        setScheduleAt("");
        setDraftSaved(false);
        draftUidRef.current = null;
      };

      // Scheduled send: the backend parks the message until send_at. Cancel it
      // from the Scheduled dialog, not the send/undo toast.
      if (sendAt) {
        await mailSend(finalTo, finalCc, finalBcc, finalSubject, finalText, finalHtml, finalFrom, finalAttachments, 0, sendAt);
        resetCompose();
        showToast(t("toastScheduled", { time: fmtDate(sendAt) }));
        return;
      }

      if (delay <= 0) {
        await mailSend(finalTo, finalCc, finalBcc, finalSubject, finalText, finalHtml, finalFrom, finalAttachments);
        resetCompose();
        loadMessages(folder);
        return;
      }

      // Server-side undo window: the backend parks the message in its outbox
      // and delivers it when the window elapses, so closing the tab no longer
      // loses the send.
      const res = await mailSend(finalTo, finalCc, finalBcc, finalSubject, finalText, finalHtml, finalFrom, finalAttachments, delay);
      const outboxId = res.outbox_id;
      resetCompose();

      showToast(t("sending"), () => {
        if (outboxId) mailUndoSend(outboxId).catch(() => {});
      }, delay * 1000);
    } catch (err) {
      setError(err instanceof Error ? err.message : "send failed");
    }
  }

  function backToList() {
    // Navigate back to the folder; the pathId effect clears the reading pane.
    router.push(`/mail/${folder}`);
    setSelected(null);
    setDetail(null);
    setThreadOpen(false);
  }

  function selectFolder(f: string) {
    router.push(`/mail/${f}`);
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
      const next = await mailMessage(folder, { uid });
      setDetail(next);
      // Keep the URL pinned to the opened thread member so it stays shareable.
      router.push(`/mail/${folder}/${next.id || next.uid}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load message failed");
    }
  }

  // ---- global keyboard shortcuts ----
  const stateRef = useRef({
    messages, cursor, folder, searching, detail,
    composeOpen, settingsOpen, contactsOpen, paletteOpen, shortcutsOpen,
  });
  const apiRef = useRef({
    openMessage, toggleSelect, removeMessage, toggleStar, setSeen,
    archiveMessage, spamMessage,
    openCompose, openThenReply, selectFolder, backToList,
  });
  // Keep the keyboard handler's snapshot refs in sync after every render
  // (writing refs during render is a React 19 anti-pattern).
  useEffect(() => {
    stateRef.current = {
      messages, cursor, folder, searching, detail,
      composeOpen, settingsOpen, contactsOpen, paletteOpen, shortcutsOpen,
    };
    apiRef.current = {
      openMessage, toggleSelect, removeMessage, toggleStar, setSeen,
      archiveMessage, spamMessage,
      openCompose, openThenReply, selectFolder, backToList,
    };
  });

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
        if (e.key === "i") api.selectFolder("Inbox");
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

  const paletteActions = usePaletteActions({
    t,
    tp,
    ts,
    openCompose,
    focusSearch: () => searchRef.current?.focus(),
    selectFolder,
    openSettings: () => setSettingsOpen(true),
    openContacts: () => setContactsOpen(true),
    theme,
    setTheme,
    density,
    setDensity,
  });

  const fmtDate = (d: string) =>
    new Date(d).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });

  // The exposed value mirrors every local so MailView (and its sub-panels via
  // useMailStore) can read state and call actions without prop drilling.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const value: any = {
    t,
    me,
    online,
    folders,
    unseen,
    knownLabels,
    labelDefs,
    labelColors,
    labelManagerOpen,
    setLabelManagerOpen,
    saveLabel,
    renameLabel,
    deleteLabel,
    activeLabel,
    savedSearches,
    folder,
    accountList,
    activeAccount,
    switchAccount,
    refreshAccounts,
    settingsInitialSection,
    openSettingsSection,
    sidebarOpen,
    setSidebarOpen,
    selectFolder,
    selectLabel,
    runSavedSearch,
    removeSavedSearch,
    moveTo,
    moveSelectedTo,
    openCompose,
    setSettingsOpen,
    setContactsOpen,
    setSieveOpen,
    detail,
    detailLoading,
    listWidth,
    messages,
    displayMessages,
    snoozedMsgs,
    snoozedLoading,
    total,
    searching,
    loading,
    query,
    setQuery,
    searchSpec,
    applySearchSpec,
    setSearchAll,
    doSearch,
    clearSearch,
    selectedUids,
    cursor,
    selected,
    searchAll,
    activeView,
    selectView,
    categoryFilter,
    setCategoryFilter,
    doAiSearch,
    loadMessages,
    createFolder,
    renameFolder,
    deleteFolder,
    clearFolder,
    openMessage,
    toggleSelect,
    removeMessage,
    toggleStar,
    togglePin,
    snoozeMessage,
    unsnooze,
    archiveMessage,
    bulkDelete,
    bulkArchive,
    bulkSpam,
    bulkFlag,
    loadMore,
    error,
    saveCurrentSearch,
    searchRef,
    ai,
    prioritizing,
    priorityOn,
    priorityCategories,
    togglePriority,
    aiSearching,
    refreshMail,
    refreshing,
    openContextMenu,
    onResizeStart,
    onResizeMove,
    onResizeEnd,
    summary,
    summarizing,
    summarize,
    reply,
    replyAll,
    forward,
    backToList,
    moveDetailTo,
    reportNotSpam,
    unsubscribeAction,
    toggleLabel,
    thread,
    threadOpen,
    threadLoading,
    toggleThread,
    selectThreadMessage,
    scheduled,
    scheduledOpen,
    setScheduledOpen,
    scheduledLoading,
    loadScheduled,
    cancelScheduled,
    composeOpen,
    identities,
    from,
    selectIdentity,
    draftSaved,
    to,
    cc,
    bcc,
    ccExpanded,
    setCcExpanded,
    subject,
    body,
    setBody,
    setBodyText,
    attachments,
    setAttachments,
    composeFocus,
    allContacts,
    loadContactsOnce,
    fileInputRef,
    toInputRef,
    draftTone,
    setDraftTone,
    drafting,
    aiDraftReply,
    prefs,
    setUndoSend,
    scheduleAt,
    setScheduleAt,
    signOn,
    encryptOn,
    setSignOn,
    setEncryptOn,
    dragOverCompose,
    setDragOverCompose,
    addFiles,
    send,
    closeCompose,
    saveDraftNow,
    setTo,
    setCc,
    setBcc,
    setSubject,
    contactsOpen,
    settingsOpen,
    paletteOpen,
    setPaletteOpen,
    paletteActions,
    shortcutsOpen,
    setShortcutsOpen,
    sieveOpen,
    ctxMenu,
    setCtxMenu,
    quoteText,
    replyAllFrom,
    toggleRead,
    spamMessage,
    toast,
    setToast,
  };
  return <MailStoreContext.Provider value={value}>{children}</MailStoreContext.Provider>;
}
