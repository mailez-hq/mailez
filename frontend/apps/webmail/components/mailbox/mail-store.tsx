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
import { subscribeMailEvents } from "@/lib/events";
import { writeLastFolder } from "@/lib/preferences";
import { parseMergeRecipients } from "@/lib/mail-merge";
import {
  aiComposeDraft, aiComposeStream, aiDraft, aiDraftNew, aiStatus, aiSummarize,
  aiPrioritize, aiSearch,
  accounts, delegations, setActiveAccountId, setActiveDelegateEmail,
  contacts, mailFlag, mailMove, mailIdentities,
  logout as apiLogout,
  mailFolders, mailFolderCreate, mailFolderRename, mailFolderDelete, mailFolderClear,
  mailLabelDelete, mailLabelRename, mailLabelSave, mailLabels, mailMessage, mailMessages, mailSaveDraft, mailSearch, mailSearchSpec, mailSend, mailSendReply, mailThread, mailUnseen, mailUndoSend, mailUnsubscribe, mailScheduled,
  mailDelete,
  mailSnooze, mailSnoozed,
  mailReceipt, mailRecall, mailRecallApply, mailMerge, mailReadAll,
  aiReplies, uploadLargeAttachment,
  meProfile, updateMeSettings,
  pgpEncrypt, pgpLookup, pgpSign,
  type CalendarEvent, type Contact, type DraftTone, type MailAccount, type MailDelegation, type MailIdentity, type MailLabel, type MailMessage, type MailThread, type Me,
  type MailSearchSpec, type OutboundAttachment, type ScheduledSend, type SnoozedMessage,
} from "@/lib/api";
import {
  SYSTEM_FLAGS,
  PIN_FLAG,
  MUTE_FLAG,
  isPinned,
  isMuted,
  isSnoozed,
  snoozeUntil,
  isUserLabel,
  SAVED_SEARCH_KEY,
  type SavedSearch,
  MAX_ATTACHMENT_BYTES,
  escHtml,
  normalizeQuoteBody,
  readFileAsBase64,
  stripCollapseMarkers,
  textToHtml,
} from "@/components/mailbox/mail-utils";

// ---- in-memory view cache ----
// Switching between conversations should be instant: keep recently fetched
// message details and threads in memory for a short TTL. Flag changes are
// bounded by the TTL; opening a message invalidates its entry (marks read).
const detailCache = new Map<string, { at: number; data: MailMessage }>();
const threadCache = new Map<string, { at: number; data: MailThread }>();
const VIEW_CACHE_TTL = 30_000;
const VIEW_CACHE_MAX = 120;

function viewCacheGet<T>(m: Map<string, { at: number; data: T }>, key: string): T | null {
  const e = m.get(key);
  if (!e) return null;
  if (Date.now() - e.at > VIEW_CACHE_TTL) {
    m.delete(key);
    return null;
  }
  return e.data;
}

function viewCachePut<T>(m: Map<string, { at: number; data: T }>, key: string, data: T) {
  if (m.size >= VIEW_CACHE_MAX) {
    let oldestKey: string | null = null;
    let oldestAt = Infinity;
    for (const [k, v] of m) {
      if (v.at < oldestAt) {
        oldestAt = v.at;
        oldestKey = k;
      }
    }
    if (oldestKey) m.delete(oldestKey);
  }
  m.set(key, { at: Date.now(), data });
}

// The store context is intentionally untyped for now: every field mirrors a
// local inside the provider, and MailView stays a thin view on top of it.
// A full MailStore interface lands together with the store-splitting refactor.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const MailStoreContext = createContext<any>(null);

// composeSignature fingerprints the compose fields. It is compared against
// the baseline captured when compose opens, so a pristine reply/forward
// (only the auto-generated quote) is never mistaken for user content.
function composeSignature(vals: {
  to: string[];
  cc: string[];
  bcc: string[];
  subject: string;
  body: string;
  bodyText: string;
  attachments: {filename: string; size: number}[];
}): string {
  return JSON.stringify([
    vals.to,
    vals.cc,
    vals.bcc,
    vals.subject,
    vals.body,
    vals.bodyText,
    vals.attachments.map((a) => `${a.filename}:${a.size}`),
  ]);
}

// buildSearchSpec merges the free-text keywords with the visual builder
// conditions into a structured spec; null when nothing is active. The
// documented keyword syntax (from:, to:, subject:, is:unread, is:flagged,
// has:attachment, label:, before:, after:) is parsed into structured fields
// here, mirroring the backend's parseSearchQuery; bare words become text.
function applySearchSyntax(keywords: string, spec: MailSearchSpec): boolean {
  let active = false;
  const re = /([a-z]+):"([^"]*)"|([a-z]+):(\S+)|(\S+)/gi;
  let m: RegExpExecArray | null;
  while ((m = re.exec(keywords)) !== null) {
    let key = "";
    let val = "";
    if (m[1]) {
      key = m[1];
      val = m[2];
    } else if (m[3]) {
      key = m[3];
      val = m[4];
    } else {
      spec.text = [...(spec.text ?? []), m[5]];
      active = true;
      continue;
    }
    switch (key.toLowerCase()) {
      case "from":
        spec.from = [...(spec.from ?? []), val];
        active = true;
        break;
      case "to":
        spec.to = [...(spec.to ?? []), val];
        active = true;
        break;
      case "subject":
        spec.subject = [...(spec.subject ?? []), val];
        active = true;
        break;
      case "has":
        if (val.toLowerCase() === "attachment") {
          spec.hasAttachment = true;
          active = true;
        }
        break;
      case "is":
        if (val.toLowerCase() === "unread") {
          spec.unseen = true;
          active = true;
        }
        if (val.toLowerCase() === "flagged" || val.toLowerCase() === "starred") {
          spec.flagged = true;
          active = true;
        }
        break;
      case "label":
        if (val) {
          spec.labels = [...(spec.labels ?? []), val];
          active = true;
        }
        break;
      case "filename":
        if (val) {
          spec.filenames = [...(spec.filenames ?? []), val];
          active = true;
        }
        break;
      case "before": {
        const t = new Date(`${val}T00:00:00Z`);
        if (!Number.isNaN(t.getTime())) {
          spec.before = t.toISOString();
          active = true;
        }
        break;
      }
      case "after": {
        const t = new Date(`${val}T00:00:00Z`);
        if (!Number.isNaN(t.getTime())) {
          spec.after = t.toISOString();
          active = true;
        }
        break;
      }
    }
  }
  return active;
}

function buildSearchSpec(keywords: string, spec: MailSearchSpec | null): MailSearchSpec | null {
  const merged: MailSearchSpec = spec ? { ...spec } : {};
  const syntaxActive = applySearchSyntax(keywords, merged);
  const active = syntaxActive || Object.entries(merged).some(([, v]) =>
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
  const [sortBy, setSortBy] = useState("date");
  const [sortDir, setSortDir] = useState("desc");
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");
  // composeError / composeNotice are scoped to the compose panel only, so
  // AI draft/compose and send failures never leak into the message list or
  // the reading pane.
  const [composeError, setComposeError] = useState("");
  const [composeNotice, setComposeNotice] = useState("");

  // ---- aggregated external accounts (full aggregation client) ----
  const [accountList, setAccountList] = useState<MailAccount[]>([]);
  // null = the internal gateway account; a number = an external mailbox.
  const [activeAccount, setActiveAccount] = useState<number | null>(null);
  // ---- delegated mailboxes (shared mailboxes with full access) ----
  // delegateList holds every owner mailbox this user may open; activeDelegate
  // is the currently open owner email (null = the user's own mailbox).
  const [delegateList, setDelegateList] = useState<MailDelegation[]>([]);
  const [activeDelegate, setActiveDelegate] = useState<string | null>(null);
  const switchInFlight = useRef(false);

  const refreshAccounts = useCallback(async () => {
    try {
      setAccountList(await accounts());
    } catch {
      // account listing is optional; the internal account always works
    }
  }, []);

  const refreshDelegations = useCallback(async () => {
    try {
      const listing = await delegations();
      setDelegateList(listing.received.filter((d) => d.full_access));
    } catch {
      // delegation listing is optional; the user's own mailbox always works
    }
  }, []);

  // Load the account list once; keep the active account in the api module so
  // every /mail/* request is scoped to it.
  useEffect(() => {
    refreshAccounts();
  }, [refreshAccounts]);
  useEffect(() => {
    refreshDelegations();
  }, [refreshDelegations]);
  useEffect(() => {
    setActiveAccountId(activeAccount);
  }, [activeAccount]);
  useEffect(() => {
    setActiveDelegateEmail(activeDelegate);
  }, [activeDelegate]);

  // Switching accounts resets the mailbox view and re-scopes all requests.
  async function switchAccount(id: number | null) {
    if (id === activeAccount || switchInFlight.current) return;
    switchInFlight.current = true;
    setActiveAccount(id);
    setActiveDelegate(null);
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

  // Switching delegated mailboxes re-scopes every /mail/* request to the
  // owner's mailbox (X-Delegate-Email); the backend validates the grant.
  async function switchDelegate(email: string | null) {
    if (email === activeDelegate || switchInFlight.current) return;
    switchInFlight.current = true;
    setActiveAccount(null);
    setActiveDelegate(email);
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
    setActiveLabel("");
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
  const [receiptOn, setReceiptOn] = useState(false);
  const [mergeOn, setMergeOn] = useState(false);
  const [mergeText, setMergeText] = useState("");
  const [burnAfter, setBurnAfter] = useState(0);
  // Where the initial focus should land when the compose dialog opens:
  // "to" (new message / forward) or "editor" (reply / reply all).
  const [composeFocus, setComposeFocus] = useState<"to" | "editor">("to");
  // hasReplyTarget marks a reply/forward compose (original email available as
  // AI context). New-mail compose leaves it false and AI drafts from subject
  // + hints instead.
  const [hasReplyTarget, setHasReplyTarget] = useState(false);
  const [aiDraftHint, setAiDraftHint] = useState("");
  const [aiComposeBusy, setAiComposeBusy] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  // The settings section to land on when the dialog opens (e.g. "accounts"
  // from the sidebar account manager); defaults to the first section.
  const [settingsInitialSection, setSettingsInitialSection] = useState("appearance");
  const [contactsOpen, setContactsOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const [sieveOpen, setSieveOpen] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [calendarOpen, setCalendarOpen] = useState(false);
  // Event to focus when the calendar drawer opens (set by the workspace
  // home "today's schedule" list; consumed once by CalendarView).
  const [calendarFocus, setCalendarFocus] = useState<CalendarEvent | null>(null);
  const [driveOpen, setDriveOpen] = useState(false);
  const [listWidth, setListWidth] = useState(360);

  // Deep links from the app sidebar (?open=settings / ?open=contacts) open
  // the matching dialog when the mailbox mounts; the query is consumed once.
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const open = params.get("open");
    if (open === "settings") setSettingsOpen(true);
    if (open === "contacts") setContactsOpen(true);
    if (open) {
      const url = new URL(window.location.href);
      url.searchParams.delete("open");
      window.history.replaceState(null, "", url.toString());
    }
  }, []);

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
  const [savedSearches, setSavedSearches] = useState<SavedSearch[]>([]);
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
  // Snapshot of a closed-but-unsaved-completely compose session: closing the
  // editor persists the draft and remembers its content so the next "Write"
  // restores it instead of starting blank.
  const lastDraftRef = useRef<{
    to: string[]; cc: string[]; bcc: string[]; subject: string; body: string; bodyText: string;
    attachments: OutboundAttachment[]; uid: number | null;
  } | null>(null);
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
  // Same guard for search responses: rapid saved-search / label / typing
  // transitions fire overlapping /mail/search requests.
  const searchSeq = useRef(0);
  const pendingG = useRef(false);
  const draftUidRef = useRef<number | null>(null);
  const draftBaselineRef = useRef("");
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
      const res = await mailMessages(f, p, sortBy, sortDir, prefs.conversation);
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
  }, [sortBy, sortDir, prefs.conversation]);

  // changeSort updates the list ordering (date desc/asc, by sender, subject
  // or size) and reloads the current folder.
  function changeSort(key: string) {
    const [s, d] = key.split("-");
    const nextBy = s || "date";
    const nextDir = d === "asc" ? "asc" : nextBy === "date" ? "desc" : "asc";
    setSortBy(nextBy);
    setSortDir(nextDir);
    loadMessages(folder, 0, true);
  }

  useEffect(() => {
    loadFolders();
  }, [loadFolders]);

  // Auto-save the draft 30s after the user stops typing, replacing the
  // previous auto-save so one compose session keeps exactly one draft.
  // A pristine reply/forward (only the auto-generated quote, nothing typed)
  // is not user content and must not create a draft on its own.
  useEffect(() => {
    if (!composeOpen) return;
    const hasContent =
      to.length > 0 || cc.length > 0 || subject.trim() !== "" || bodyText.trim() !== "" || attachments.length > 0;
    if (!hasContent) return;
    const pristine =
      draftUidRef.current == null &&
      draftBaselineRef.current ===
        composeSignature({to, cc, bcc, subject, body, bodyText, attachments});
    if (pristine) return;
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
  }, [composeOpen, to, cc, bcc, subject, bodyText, body, attachments]);

  // saveDraftNow writes the draft immediately (manual "save draft" button);
  // auto-save also runs 30s after the user stops typing.
  async function saveDraftNow() {
    const hasContent =
      to.length > 0 || cc.length > 0 || subject.trim() !== "" || bodyText.trim() !== "" || attachments.length > 0;
    if (!hasContent) return;
    if (draftSavingRef.current) return; // ignore rapid repeated clicks; the first save persists
    draftSavingRef.current = true;
    setComposeError("");
    try {
      const res = await mailSaveDraft(subject, bodyText, body, draftUidRef.current ?? 0, to, cc, attachments);
      draftUidRef.current = res.uid || draftUidRef.current;
      setDraftSaved(true);
      refreshDraftsIfActive();
    } catch (e) {
      setComposeError(e instanceof Error ? e.message : "save draft failed");
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
    // Closing a pristine reply (auto quote, no edits, no existing draft)
    // dismisses it without polluting the Drafts folder.
    const pristine =
      draftUidRef.current == null &&
      draftBaselineRef.current ===
        composeSignature({to, cc, bcc, subject, body, bodyText, attachments});
    // A create (uid == null) must not race another in-flight create, otherwise
    // two drafts appear; updating an existing draft is always safe.
    if (hasContent && !pristine && (draftUidRef.current != null || !draftSavingRef.current)) {
      // Remember the content synchronously so a quick reopen restores it even
      // before the async save resolves.
      lastDraftRef.current = {
        to, cc, bcc, subject, body, bodyText, attachments,
        uid: draftUidRef.current,
      };
      mailSaveDraft(subject, bodyText, body, draftUidRef.current ?? 0, to, cc, attachments)
        .then((res) => {
          draftUidRef.current = res.uid || draftUidRef.current;
          if (lastDraftRef.current) lastDraftRef.current.uid = draftUidRef.current;
          refreshDraftsIfActive();
        })
        .catch(() => {});
    } else {
      lastDraftRef.current = null;
    }
    setComposeOpen(false);
  }

  // Opening the calendar drawer dismisses a composing editor first so the
  // drawer is never hidden behind it; the draft is persisted by closeCompose.
  function openCalendar() {
    if (composeOpen) closeCompose();
    setCalendarFocus(null);
    setCalendarOpen(true);
  }

  // Opening the calendar already focused on a specific event (e.g. clicking
  // a row on the workspace home): the drawer pops the event dialog directly.
  function openCalendarEvent(ev: CalendarEvent) {
    if (composeOpen) closeCompose();
    setCalendarFocus(ev);
    setCalendarOpen(true);
  }

  // Opening the drive drawer dismisses a composing editor first, like the
  // calendar drawer.
  function openDrive() {
    if (composeOpen) closeCompose();
    setDriveOpen(true);
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
    setSearchSpec(null);
    lastSearchRef.current = "";
    // A folder switch resets the transient view filters so a category chosen
    // in one folder can't silently filter (and empty) the next one.
    setCategoryFilter("");
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
  // null when the URL has no folder segment (bare /mail, which MailLayout
  // redirects to the last-visited folder); the inbox stays the fallback.
  const pathFolder = segs[1] ? decodeURIComponent(segs[1]) : null;
  const pathId = segs.length > 2 ? decodeURIComponent(segs[2]) : null;

  useEffect(() => {
    if (pathFolder && pathFolder !== folder) setFolder(pathFolder);
  }, [pathFolder, folder]);

  // Remember the last-visited folder so login / re-open can restore the
  // user's position instead of always landing in the inbox. Only /mail routes
  // count as folder visits — /home keeps the inbox fallback without clobbering
  // the stored position.
  useEffect(() => {
    if (folder && pathname?.startsWith("/mail")) writeLastFolder(folder);
  }, [folder, pathname]);

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
    if (row) {
      // Optimistic switch: show the new message's summary right away so the
      // pane leaves the previous message while the full body loads. Keeping
      // the stale message on screen until the fetch lands reads as "stuck".
      setSelected(row);
      setDetail(row);
    } else {
      // Row not loaded yet (deep link / list still fetching): clear the pane
      // so the loading state is visible instead of stale content.
      setSelected(null);
      setDetail(null);
    }
    // A numeric id is the degenerate uid fallback used when a message has no
    // Message-ID header; routing it through the uid path avoids a pointless
    // (and error-prone) reverse lookup.
    const numericId = /^\d+$/.test(pathId);
    // A pathId only exists when the URL carries a folder segment, so pathFolder
    // is always set here; TS can't see the invariant, hence the assertion.
    const pf = pathFolder!;
    const detailKey = `${pf}\x00${pathId}`;
    const cachedDetail = viewCacheGet(detailCache, detailKey);
    if (cachedDetail) {
      setSelected(cachedDetail);
      setDetail(cachedDetail);
      setDetailLoading(false);
      setSummary("");
      return;
    }
    const fetch = row
      ? mailMessage(pf, { uid: row.uid })
      : numericId
        ? mailMessage(pf, { uid: Number(pathId) })
        : mailMessage(pf, { id: pathId });
    fetch
      .then((full) => {
        if (cancelled) return;
        viewCachePut(detailCache, detailKey, full);
        setSelected(full);
        setDetail(full);
        setSummary("");
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

  // Auto-load the conversation when a message that belongs to a thread
  // opens, so the reading pane shows the Gmail-style thread view without any
  // extra click. The previous conversation is dropped immediately so
  // switching between threads never renders stale members.
  useEffect(() => {
    setThread(null);
    setThreadOpen(false);
    if (!prefs.conversation || !detail?.thread_id) {
      return;
    }
    const threadKey = `${folder}\x00${detail.thread_id}`;
    const cachedThread = viewCacheGet(threadCache, threadKey);
    if (cachedThread) {
      setThread(cachedThread);
      setThreadOpen(true);
      setThreadLoading(false);
      return;
    }
    let cancelled = false;
    setThreadLoading(true);
    mailThread(folder, detail.thread_id)
      .then((th) => {
        if (cancelled) return;
        viewCachePut(threadCache, threadKey, th);
        setThread(th);
        setThreadOpen(true);
      })
      .catch(() => {
        if (!cancelled) setThread(null);
      })
      .finally(() => {
        if (!cancelled) setThreadLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [detail?.thread_id, folder, prefs.conversation]);

  // Collect user labels (custom IMAP keywords) from loaded messages so the
  // sidebar and reader can offer them for quick tagging/filtering; the list
  // is merged with the server-side definitions in knownLabels below.
  useEffect(() => {
    setFlagLabels((prev) => {
      const set = new Set(prev);
      messages.forEach((m) =>
        m.flags.forEach((f) => {
          if (isUserLabel(f)) set.add(f);
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

  // Real-time mailbox updates: the backend pushes a "mail" event over SSE when
  // new mail arrives in a watched folder. The current folder is read through a
  // ref so switching folders doesn't tear down and recreate the connection.
  const folderRef = useRef(folder);
  folderRef.current = folder;
  useEffect(() => {
    if (!me.email) return;
    return subscribeMailEvents({
      onReady: () => {
        // (Re)connected: pick up anything that arrived while offline.
        refreshUnseen();
        void loadMessages(folderRef.current, 0, true);
      },
      onMail: (ev) => {
        refreshUnseen();
        const folders = ev.folders || [];
        const cur = folderRef.current;
        if (
          folders.length === 0 ||
          folders.some((f) => f.toLowerCase() === cur.toLowerCase())
        ) {
          void loadMessages(cur, 0, true);
        }
      },
    });
  }, [me.email, refreshUnseen, loadMessages]);

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
      // An empty submission with no builder conditions means "show the plain
      // folder": clear the search state and reload, otherwise stale filtered
      // results would stay on screen with no indicator.
      clearSearch();
      return;
    }
    // A real search replaces any active label filter.
    setActiveLabel("");
    await runSearchWithSpec(spec);
  }

  // runSearchWithSpec executes a structured search (visual builder or merged
  // keywords) straight against /mail/search — no syntax-string round-trip.
  async function runSearchWithSpec(spec: MailSearchSpec) {
    // Monotonic seq: a slow earlier search must not overwrite a newer one
    // (rapid saved-search / label / typing transitions fire overlapping
    // requests).
    const seq = ++searchSeq.current;
    // Invalidate any in-flight folder load: without this, a slow
    // loadMessages response landing after the search result would overwrite
    // the filtered list with the full folder (the two seq guards are
    // independent).
    loadSeq.current++;
    setSearching(true);
    setSelected(null);
    setDetail(null);
    setCursor(0);
    // Drop any opened-message segment from the URL: the reading pane is being
    // cleared with the search, and leaving a stale /mail/<folder>/<id> makes a
    // later click on the same row a no-op (pathId never changes, so the
    // pathId effect never re-runs).
    if (segs.length > 2) {
      router.replace(`/mail/${encodeURIComponent(folder)}`, { scroll: false });
    }
    try {
      const results = await mailSearchSpec(searchAll ? "all" : folder, spec);
      if (seq !== searchSeq.current) return;
      setMessages(results);
      lastSearchRef.current = spec.text?.join(" ") ?? "";
    } catch (err) {
      if (seq !== searchSeq.current) return;
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
    if (segs.length > 2) {
      router.replace(`/mail/${encodeURIComponent(folder)}`, { scroll: false });
    }
    loadMessages(folder);
  }

  // refreshMail reloads the current view without blanking the list: re-runs an
  // active search, otherwise refetches the folder plus unseen counts.
  async function refreshMail() {
    setRefreshing(true);
    try {
      if (searching) {
        if (activeLabel) {
          // Re-run the label filter instead of dropping back to the folder.
          await runSearchWithSpec({ labels: [activeLabel] });
        } else {
          await doSearch(query);
        }
      } else {
        await Promise.all([loadMessages(folder, 0, true), mailUnseen().then(setUnseen)]);
      }
    } finally {
      setRefreshing(false);
    }
  }

  // Label filter (clicking a tag in the sidebar).
  function selectLabel(label: string) {
    setActiveView("all");
    setActiveLabel(label);
    // Drop any stale builder conditions so they can't leak into the label
    // view or into a later search box submission.
    setSearchSpec(null);
    if (label) {
      // Run the structured search directly; keep the keyword box empty
      // instead of echoing the raw label: syntax into it.
      setQuery("");
      runSearchWithSpec({ labels: [label] });
    } else {
      setQuery("");
      clearSearch();
    }
  }

  // Saved searches (virtual folders): keyword queries and structured specs.
  useEffect(() => {
    try {
      const raw = localStorage.getItem(SAVED_SEARCH_KEY);
      if (!raw) return;
      const parsed: unknown = JSON.parse(raw);
      if (!Array.isArray(parsed)) return;
      // Legacy entries are plain query strings; migrate them to typed items.
      const migrated: SavedSearch[] = parsed.map((item, i) => {
        if (typeof item === "string") {
          return { id: i + 1, kind: "query", name: item, query: item };
        }
        const s = item as {
          id?: number;
          kind?: string;
          name?: string;
          query?: string;
          spec?: MailSearchSpec;
        };
        const id = s.id ?? i + 1;
        const name = s.name ?? s.query ?? "";
        if (s.kind === "spec" && s.spec) {
          return { id, kind: "spec", name, spec: s.spec };
        }
        return { id, kind: "query", name, query: s.query ?? "" };
      });
      setSavedSearches(migrated);
    } catch {
      // storage unavailable
    }
  }, []);

  function persistSearches(next: SavedSearch[]) {
    setSavedSearches(next);
    try {
      localStorage.setItem(SAVED_SEARCH_KEY, JSON.stringify(next));
    } catch {
      // storage unavailable
    }
  }

  function saveCurrentSearch() {
    const q = query.trim();
    if (!q || savedSearches.some((s) => s.kind === "query" && s.query === q)) return;
    persistSearches([...savedSearches, { id: Date.now(), kind: "query", name: q, query: q }]);
  }

  // saveSearchSpec persists a structured condition set from the search builder
  // as a named quick entry in the sidebar.
  function saveSearchSpec(name: string, spec: MailSearchSpec) {
    const n = name.trim();
    if (!n || savedSearches.some((s) => s.kind === "spec" && s.name === n)) return;
    persistSearches([...savedSearches, { id: Date.now(), kind: "spec", name: n, spec }]);
  }

  function removeSavedSearch(id: number) {
    persistSearches(savedSearches.filter((s) => s.id !== id));
  }

  function runSavedSearch(item: SavedSearch) {
    if (item.kind === "spec") {
      applySearchSpec(item.spec);
      return;
    }
    // A keyword saved search replaces, not merges with, any stale builder
    // conditions from an earlier search. Build the spec explicitly with no
    // prior conditions so the synchronous call below can't capture the old
    // searchSpec and fire a duplicate, stale-merged request.
    setSearchSpec(null);
    setQuery(item.query);
    const spec = buildSearchSpec(item.query, null);
    if (spec) {
      setActiveLabel("");
      runSearchWithSpec(spec);
    }
  }

  // logout signs out and returns to the sign-in page (full navigation so all
  // in-memory mailbox state is dropped).
  async function logout() {
    try {
      await apiLogout();
    } finally {
      window.location.href = "/";
    }
  }

  function openMessage(m: MailMessage, srcFolder = folder) {
    if (!m.flags.includes("\\Seen")) {
      detailCache.delete(`${srcFolder}\x00${m.id || m.uid}`);
      mailFlag(srcFolder, m.uid, "\\Seen", true).then(refreshUnseen).catch(() => {});
      m.flags.push("\\Seen");
      setMessages((ms) => ms.map((x) => (x.uid === m.uid ? applySeenState(x, true) : x)));
      // A multi-member thread's unread dot is a server-side aggregate that
      // cannot be derived from the opened message alone: reload the folder so
      // reading the last unread member clears the row's unread indicator.
      // Skipped while searching (results are not the folder list).
      const row = messages.find((x) => x.uid === m.uid);
      if (!searching && row?.thread_unread && row.thread_count && row.thread_count > 1) {
        void loadMessages(folder, 0, true);
      }
    }
    // Navigate to the message route; MailView follows the /mail/[folder]/[id]
    // URL (pathId effect) to load and show the reading pane. The stable id
    // keeps every view routable, shareable and survives mailbox moves. The
    // folder is URL-encoded so nested names (Parent/Child) stay one segment.
    router.push(`/mail/${encodeURIComponent(srcFolder)}/${m.id || m.uid}`);
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
    detailCache.delete(`${folder}\x00${m.id || m.uid}`);
    try {
      await mailFlag(folder, m.uid, "\\Seen", value);
      refreshUnseen();
      const apply = (x: MailMessage) => (x.uid === m.uid ? applySeenState(x, value) : x);
      setMessages((ms) => ms.map(apply));
      setDetail((d) => (d ? apply(d) : d));
    } catch (e) {
      setError(e instanceof Error ? e.message : "flag failed");
    }
  }

  // applySeenState returns a list row with the \Seen flag applied and, for
  // conversation rows, the thread aggregate resolved when it can be computed
  // locally: a single-message thread turns read/unread with the row. Rows of
  // multi-member threads keep their server-side aggregate until the folder
  // is reloaded (see openMessage).
  function applySeenState(x: MailMessage, seen: boolean): MailMessage {
    const flags = seen
      ? x.flags.includes("\\Seen")
        ? x.flags
        : [...x.flags, "\\Seen"]
      : x.flags.filter((f) => f !== "\\Seen");
    let thread_unread = x.thread_unread;
    if (x.thread_unread !== undefined) {
      if (!seen) thread_unread = true;
      else if (!x.thread_count || x.thread_count <= 1) thread_unread = false;
    }
    return {...x, flags, thread_unread};
  }

  async function toggleStar(m: MailMessage) {
    const starred = !m.flags.includes("\\Flagged");
    detailCache.delete(`${folder}\x00${m.id || m.uid}`);
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

  // openSnoozed switches to the Snoozed virtual view, listing every message
  // parked by the user until a later time.
  async function openSnoozed() {
    setActiveView("snoozed");
    setActiveLabel("");
    setSearchSpec(null);
    setQuery("");
    setSearching(false);
    setCursor(0);
    setSelected(null);
    setDetail(null);
    try {
      const list = await mailSnoozed();
      setSnoozedMsgs(list);
      setMessages(list);
      setTotal(list.length);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load snoozed failed");
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
    const lines = normalizeQuoteBody(d.text_body || "").trim();
    if (!lines) return "";
    const quoted = lines.split("\n").map((l) => `> ${l}`).join("\n");
    return `\n\nOn ${fmtDate(d.date)}, ${from} wrote:\n${quoted}`;
  };

  // selectedQuote quotes only the user's current selection in the reading
  // pane when replying; without a selection it falls back to the full body.
  const selectedQuote = (d: MailMessage) => {
    const sel = typeof window !== "undefined" ? window.getSelection()?.toString().trim() : "";
    if (sel && sel.length > 1 && !/^[\r\n\s]+$/.test(sel)) {
      const from = d.from.map((a) => a.name || a.email).join(", ");
      const quoted = normalizeQuoteBody(sel).split("\n").map((l) => `> ${l}`).join("\n");
      return `\n\nOn ${fmtDate(d.date)}, ${from} wrote:\n${quoted}`;
    }
    return quoteText(d);
  };

  // Focus the recipient field when the compose dialog opens in "to" mode
  // (new message / forward). A short delay keeps the focus from being
  // stolen by Base UI's dialog focus management.
  useEffect(() => {
    if (!composeOpen || composeFocus !== "to") return;
    const tm = setTimeout(() => toInputRef.current?.focus(), 60);
    return () => clearTimeout(tm);
  }, [composeOpen, composeFocus]);

  function openCompose(
    toAddr = "",
    subj = "",
    html = "",
    text = "",
    focus: "to" | "editor" = "to",
    replyTarget = false,
  ) {
    setHasReplyTarget(replyTarget);
    setComposeError("");
    setComposeNotice("");
    // A bare "Write" right after closing a draft restores the saved content
    // instead of starting from scratch (the close path persisted it).
    if (!toAddr && !subj && !html && !text && lastDraftRef.current) {
      const s = lastDraftRef.current;
      lastDraftRef.current = null;
      setTo(s.to);
      setCc(s.cc);
      setBcc(s.bcc);
      setCcExpanded(false);
      setAttachments(s.attachments);
      setSubject(s.subject);
      setBody(s.body);
      setBodyText(s.bodyText);
      setSignOn(false);
      setEncryptOn(false);
      setDraftSaved(false);
      draftUidRef.current = s.uid ?? draftUidRef.current;
      draftBaselineRef.current = composeSignature({
        to: s.to, cc: s.cc, bcc: s.bcc, subject: s.subject, body: s.body, bodyText: s.bodyText, attachments: s.attachments,
      });
      setComposeFocus(focus);
      setComposeOpen(true);
      return;
    }
    const identity = identities.find((i) => i.email === from);
    const sig = prefs.autoSignature
      ? identity?.signature?.trim() || me.signature?.trim()
      : "";
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
    const parsedTo = toAddr ? toAddr.split(",").map((s) => s.trim()).filter(Boolean) : [];
    setTo(parsedTo);
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
    // Baseline for pristine-reply detection: only real user edits (or an
    // existing draft) should trigger auto/close-save.
    draftBaselineRef.current = composeSignature({
      to: parsedTo,
      cc: [],
      bcc: [],
      subject: subj,
      body: finalHtml,
      bodyText: finalText,
      attachments: [],
    });
    setComposeFocus(focus);
    setComposeOpen(true);
  }

  async function addFiles(list: FileList | File[]) {
    try {
      const files = Array.from(list);
      const inline: File[] = [];
      for (const f of files) {
        if (f.size > MAX_ATTACHMENT_BYTES) {
          // 超大附件：relay the file to the server and embed a download link.
          try {
            const up = await uploadLargeAttachment(f);
            const link = `${up.filename}（超大附件）\n下载：${up.url}`;
            setBodyText((prev) => `${prev}\n\n${link}`);
            setBody((prev) => `${prev}<p>${escHtml(up.filename)}（超大附件）<br/><a href="${escHtml(up.url)}">下载</a></p>`);
            showToast(t("largeAttachmentUploaded", { name: up.filename }));
          } catch {
            setComposeError(t("largeAttachmentFailed", { name: f.name }));
          }
          continue;
        }
        inline.push(f);
      }
      if (inline.length === 0) return;
      const ready = await Promise.all(inline.map(readFileAsBase64));
      setAttachments((prev) => [...prev, ...ready]);
      setComposeError("");
    } catch {
      setComposeError(t("attachFailed"));
    }
  }

  async function toggleRead(m: MailMessage) {
    const value = !m.flags.includes("\\Seen");
    try {
      await mailFlag(m.folder || folder, m.uid, "\\Seen", value);
      const apply = (x: MailMessage) => (x.uid === m.uid ? applySeenState(x, value) : x);
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
      textToHtml(quoteText(m), {collapseQuote: prefs.collapseReplyQuote}),
      quoteText(m),
      "editor",
      true,
    );
  }

  // replyFrom / forwardFrom open compose for an arbitrary message (thread
  // members, row context menus) without first swapping the open detail, so
  // the thread reading-pane action buttons work on the clicked message.
  function replyFrom(m: MailMessage) {
    const quote = quoteText(m);
    openCompose(
      m.from[0]?.email || "",
      m.subject.startsWith("Re:") ? m.subject : `Re: ${m.subject}`,
      textToHtml(quote, {collapseQuote: prefs.collapseReplyQuote}),
      quote,
      "editor",
      true,
    );
  }

  function forwardFrom(m: MailMessage) {
    const from = m.from.map((a) => a.name || a.email).join(", ");
    const head = `---------- Forwarded message ----------\nFrom: ${from}\nDate: ${fmtDate(m.date)}\nSubject: ${m.subject}\n\n`;
    const quote = head + normalizeQuoteBody(m.text_body || "");
    openCompose(
      "",
      m.subject.startsWith("Fwd:") ? m.subject : `Fwd: ${m.subject}`,
      textToHtml(quote),
      quote,
      "to",
      true,
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
    const sig = prefs.autoSignature ? idn?.signature?.trim() : "";
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
    const quote = selectedQuote(detail);
    openCompose(
      detail.from[0]?.email || "",
      detail.subject.startsWith("Re:") ? detail.subject : `Re: ${detail.subject}`,
      textToHtml(quote, {collapseQuote: prefs.collapseReplyQuote}),
      quote,
      "editor",
      true,
    );
  }

  // editDraft reopens a saved draft in the compose editor and takes over its
  // uid, so the next save replaces the same draft instead of creating a new
  // one. Attachments are restored when the detail carries their payload.
  function editDraft() {
    if (!detail) return;
    const m = detail;
    const to = (m.to || []).map((a) => a.email).filter(Boolean);
    const cc = (m.cc || []).map((a) => a.email).filter(Boolean);
    const html = m.html_body || textToHtml(m.text_body || "");
    const text = m.text_body || "";
    const atts = (m.attachments || [])
      .filter((a) => a.data)
      .map((a) => ({ filename: a.filename, content_type: a.content_type, size: a.size, data: a.data as string }));
    setTo(to);
    setCc(cc);
    setBcc([]);
    setCcExpanded(cc.length > 0);
    setAttachments(atts);
    setSubject(m.subject || "");
    setBody(html);
    setBodyText(text);
    setSignOn(false);
    setEncryptOn(false);
    setDraftSaved(false);
    draftUidRef.current = m.uid;
    draftBaselineRef.current = composeSignature({
      to, cc, bcc: [], subject: m.subject || "", body: html, bodyText: text, attachments: atts,
    });
    setComposeFocus("editor");
    setComposeOpen(true);
  }

  // replyWithQuote opens a reply whose body quotes only the selected passage.
  function replyWithQuote(selection: string) {
    if (!detail) return;
    const s = selection.trim();
    const quote = s ? `> ${s.replace(/\n/g, "\n> ")}\n\n` : selectedQuote(detail);
    openCompose(
      detail.from[0]?.email || "",
      detail.subject.startsWith("Re:") ? detail.subject : `Re: ${detail.subject}`,
      textToHtml(quote),
      quote,
      "editor",
      true,
    );
  }

  function replyAll() {
    if (!detail) return;
    const recipients = new Set<string>();
    // Reply All addresses every participant of the original message except
    // the replier: sender, explicit recipients AND cc. (The row context-menu
    // variant already included cc; this one was dropping it.)
    [...detail.from, ...(detail.cc || []), ...detail.to].forEach((a) => {
      if (a.email && a.email.toLowerCase() !== me.email.toLowerCase()) recipients.add(a.email);
    });
    const quote = selectedQuote(detail);
    openCompose(
      [...recipients].join(", "),
      detail.subject.startsWith("Re:") ? detail.subject : `Re: ${detail.subject}`,
      textToHtml(quote, {collapseQuote: prefs.collapseReplyQuote}),
      quote,
      "editor",
      true,
    );
  }

  function forward() {
    if (!detail) return;
    const from = detail.from.map((a) => a.name || a.email).join(", ");
    const head = `---------- Forwarded message ----------\nFrom: ${from}\nDate: ${fmtDate(detail.date)}\nSubject: ${detail.subject}\n\n`;
    const quote = head + normalizeQuoteBody(detail.text_body || "");
    openCompose(
      "",
      detail.subject.startsWith("Fwd:") ? detail.subject : `Fwd: ${detail.subject}`,
      textToHtml(quote),
      quote,
      "to",
      true,
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
    setDrafting(true);
    setComposeError("");
    try {
      let res: { draft: string };
      if (hasReplyTarget && detail) {
        // Reply/forward: the original email body is the AI context.
        res = await aiDraft(detail.text_body || detail.html_body || "", draftTone);
      } else {
        // New mail: draft from the subject + what the sender wants to say.
        if (!subject.trim() && !aiDraftHint.trim()) {
          setComposeError(t("aiDraftNeedsInput"));
          setDrafting(false);
          return;
        }
        res = await aiDraftNew(subject, aiDraftHint, draftTone);
      }
      setBody(textToHtml(res.draft));
      setBodyText(res.draft);
    } catch (e) {
      setComposeError(e instanceof Error ? e.message : "ai draft failed");
    } finally {
      setDrafting(false);
    }
  }

  // aiCompose turns a free-form instruction ("给小明写封邮件，说我下周不去
  // 旅游了") into a complete compose. The panel opens immediately and the
  // recipients/subject/body stream in while the model is still writing.
  async function aiCompose(instruction: string) {
    setComposeError("");
    setComposeNotice("");
    setAiComposeBusy(true);
    // Fresh compose: clear the form, then open the panel so the user watches
    // the email being written.
    setTo([]);
    setCc([]);
    setBcc([]);
    setSubject("");
    setBody("");
    setBodyText("");
    draftBaselineRef.current = composeSignature({
      to: [], cc: [], bcc: [], subject: "", body: "", bodyText: "", attachments,
    });
    setComposeOpen(true);
    setComposeFocus("editor");
    try {
      let contactsList = allContacts;
      if (!contactsList) {
        try {
          contactsList = await contacts();
        } catch {
          contactsList = [];
        }
      }
      const bodyParts: string[] = [];
      let subjectAcc = "";
      const flushBody = () => {
        const text = bodyParts.join("");
        setBodyText(text);
        setBody(textToHtml(text));
      };
      // Throttle editor updates so long outputs stay smooth.
      const timer = setInterval(flushBody, 80);
      let gotContent = false;
      await aiComposeStream(instruction, (ev) => {
        if (ev.type === "to" || ev.type === "subject" || ev.type === "body") {
          gotContent = true;
        }
        if (ev.type === "to" && ev.text) {
          const resolved: string[] = [];
          const unresolved: string[] = [];
          for (const raw of ev.text.split(",")) {
            const name = (raw || "").trim();
            if (!name) continue;
            if (name.includes("@")) {
              resolved.push(name);
              continue;
            }
            const hit = (contactsList || []).find(
              (c) =>
                (c.name || "").toLowerCase() === name.toLowerCase() ||
                (c.name || "").toLowerCase().includes(name.toLowerCase()),
            );
            if (hit) resolved.push(hit.email);
            else unresolved.push(name);
          }
          if (resolved.length > 0) setTo(resolved);
          if (unresolved.length > 0) {
            setComposeNotice(t("aiComposeUnresolved", { names: unresolved.join("、") }));
          }
        } else if (ev.type === "subject" && ev.text) {
          subjectAcc += ev.text;
          setSubject(subjectAcc);
        } else if (ev.type === "body" && ev.text) {
          bodyParts.push(ev.text);
        } else if (ev.type === "error" && ev.text) {
          setComposeError(ev.text);
        }
      });
      clearInterval(timer);
      flushBody();
      if (!gotContent) {
        // Streaming produced nothing (proxy buffering or an empty reply):
        // fall back to the one-shot endpoint so the user still gets the mail.
        const draft = await aiComposeDraft(instruction);
        const resolved: string[] = [];
        const unresolved: string[] = [];
        for (const raw of draft.to || []) {
          const name = (raw || "").trim();
          if (!name) continue;
          if (name.includes("@")) {
            resolved.push(name);
            continue;
          }
          const hit = (contactsList || []).find(
            (c) =>
              (c.name || "").toLowerCase() === name.toLowerCase() ||
              (c.name || "").toLowerCase().includes(name.toLowerCase()),
          );
          if (hit) resolved.push(hit.email);
          else unresolved.push(name);
        }
        if (resolved.length > 0) setTo(resolved);
        if (unresolved.length > 0) {
          setComposeNotice(t("aiComposeUnresolved", { names: unresolved.join("、") }));
        }
        if (draft.subject) setSubject(draft.subject);
        if (draft.body) {
          setBodyText(draft.body);
          setBody(textToHtml(draft.body));
        }
      }
    } catch (e) {
      setComposeError(e instanceof Error ? e.message : t("aiComposeFailed"));
    } finally {
      setAiComposeBusy(false);
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
    setComposeError("");
    try {
      let text = bodyText;
      let html = stripCollapseMarkers(body);
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

      // Mail merge (逐封群发): each line is "email, 姓名" and the subject/body
      // may reference {{name}}, {{email}} and custom {{var}} placeholders.
      if (mergeOn) {
        const recipients = parseMergeRecipients(mergeText);
        if (recipients.length === 0) {
          setComposeError(t("mergeNoRecipients"));
          return;
        }
        const res = await mailMerge({
          subject: finalSubject,
          body: finalText,
          html: finalHtml,
          from: finalFrom,
          recipients,
        });
        setMergeText("");
        setMergeOn(false);
        closeCompose();
        showToast(t("mergeSent", { count: res.sent }));
        if (res.failed.length > 0) {
          setComposeError(t("mergeFailed", { count: res.failed.length }));
        }
        return;
      }
      // datetime-local value -> RFC3339 (interpreted as local time).
      const sendAt = scheduleAt ? new Date(scheduleAt).toISOString() : undefined;

      const resetCompose = () => {
        lastDraftRef.current = null;
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

      // Sending an edited draft must remove the original from Drafts; the
      // backend only handles the outbox, not draft cleanup.
      const removeEditedDraft = async (draftUid: number | null) => {
        if (!draftUid) return;
        try {
          await mailDelete("Drafts", draftUid);
          console.log("[draft-debug] delete ok, folder=", folder, "-> refresh", folder === "Drafts");
          refreshDraftsIfActive();
        } catch {
          // The draft may already be gone; the next folder load reconciles.
        }
      };

      // Scheduled send: the backend parks the message until send_at. Cancel it
      // from the Scheduled dialog, not the send/undo toast.
      if (sendAt) {
        await mailSend(finalTo, finalCc, finalBcc, finalSubject, finalText, finalHtml, finalFrom, finalAttachments, 0, sendAt, receiptOn, burnAfter);
        removeEditedDraft(draftUidRef.current);
        resetCompose();
        showToast(t("toastScheduled", { time: fmtDate(sendAt) }));
        return;
      }

      if (delay <= 0) {
        await mailSend(finalTo, finalCc, finalBcc, finalSubject, finalText, finalHtml, finalFrom, finalAttachments, 0, undefined, receiptOn, burnAfter);
        removeEditedDraft(draftUidRef.current);
        resetCompose();
        loadMessages(folder);
        return;
      }

      // Server-side undo window: the backend parks the message in its outbox
      // and delivers it when the window elapses, so closing the tab no longer
      // loses the send.
      const res = await mailSend(finalTo, finalCc, finalBcc, finalSubject, finalText, finalHtml, finalFrom, finalAttachments, delay, undefined, receiptOn, burnAfter);
      removeEditedDraft(draftUidRef.current);
      const outboxId = res?.outbox_id;
      resetCompose();

      showToast(t("sending"), () => {
        if (outboxId) mailUndoSend(outboxId).catch(() => {});
      }, delay * 1000);

      // The backend parks the message in the outbox until the undo window
      // elapses, then delivers it. Refresh the current folder after that
      // window so a sent-and-delivered message shows up without requiring a
      // manual refresh.
      setTimeout(() => {
        loadMessages(folder, 0, true);
        refreshUnseen();
      }, delay * 1000 + 2000);
    } catch (err) {
      setComposeError(err instanceof Error ? err.message : "send failed");
    }
  }

  function backToList() {
    // Navigate back to the folder; the pathId effect clears the reading pane.
    router.push(`/mail/${encodeURIComponent(folder)}`);
    setSelected(null);
    setDetail(null);
    setThreadOpen(false);
  }

  // sendQuickReply sends an inline quick reply from the reading pane without
  // opening the compose panel; threading headers keep it in the conversation.
  async function sendQuickReply(
    to: string[],
    cc: string[],
    subject: string,
    text: string,
    inReplyTo: string,
    references: string,
  ): Promise<boolean> {
    setError("");
    try {
      await mailSendReply(to, cc, subject, text, inReplyTo, references);
      showToast(t("toastSent"));
      refreshMail();
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : "send failed");
      return false;
    }
  }

  // sendReceipt answers a read-receipt request (RFC 3798): the backend mails
  // the disposition notification and marks $MDNSent on the message.
  async function sendReceipt(folderName: string, uid: number): Promise<boolean> {
    setError("");
    try {
      await mailReceipt(folderName, uid);
      showToast(t("receiptSent"));
      refreshMail();
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : "receipt failed");
      return false;
    }
  }

  // recallMessage recalls a sent message: the backend mails X-MS-Recall
  // notices to the recipients and flags the sent copy.
  async function recallMessage(folderName: string, uid: number): Promise<boolean> {
    setError("");
    try {
      await mailRecall(folderName, uid);
      showToast(t("recallSent"));
      refreshMail();
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : "recall failed");
      return false;
    }
  }

  // applyRecall deletes the original message a recall notice targets, then
  // removes the notice itself.
  async function applyRecall(messageId: string, noticeFolder: string, noticeUid: number): Promise<boolean> {
    setError("");
    try {
      const res = await mailRecallApply(messageId);
      if (noticeFolder && noticeUid) {
        await mailMove(noticeFolder, [noticeUid], "Trash").catch(() => {});
      }
      showToast(t("recallApplied", { removed: res.removed }));
      refreshMail();
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : "recall apply failed");
      return false;
    }
  }

  // markAllRead marks the current folder's messages as read.
  async function markAllRead(folderName: string): Promise<boolean> {
    setError("");
    try {
      await mailReadAll(folderName);
      showToast(t("markedAllRead"));
      refreshMail();
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : "mark all read failed");
      return false;
    }
  }

  function selectFolder(f: string) {
    // Clicking the folder that is already open (e.g. right after a saved
    // search that kept the same URL) must drop the active search conditions
    // and show the folder's full list; router.push to the identical path is
    // a no-op and would leave the filtered results on screen.
    if (
      f === folder &&
      (searching || searchSpec || query.trim() || activeLabel || categoryFilter)
    ) {
      clearSearch();
      setCategoryFilter("");
      return;
    }
    router.push(`/mail/${encodeURIComponent(f)}`);
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

  // toggleMute mutes/unmutes the current conversation: muting archives every
  // message of the thread and marks them $Muted (the sidebar badge shows the
  // state; future replies need a server rule to skip the inbox automatically).
  async function toggleMute(m: MailMessage) {
    if (!m.thread_id) {
      // Single message with no thread metadata: mute just this message.
      const muted = isMuted(m);
      try {
        await mailFlag(m.folder || folder, m.uid, MUTE_FLAG, !muted);
        if (!muted) await mailMove(m.folder || folder, [m.uid], "Archive");
        refreshMail();
        showToast(t(muted ? "toastUnmuted" : "toastMuted"));
      } catch (e) {
        setError(e instanceof Error ? e.message : "mute failed");
      }
      return;
    }
    try {
      const th = await mailThread(m.folder || folder, m.thread_id);
      const msgs = th.messages || [];
      const muted = msgs.some((x) => isMuted(x));
      const targets = msgs.length ? msgs : [m];
      const uids = targets.map((x) => x.uid);
      const src = m.folder || folder;
      for (const x of targets) {
        await mailFlag(src, x.uid, MUTE_FLAG, !muted);
      }
      if (!muted && uids.length) {
        await mailMove(src, uids, "Archive");
      }
      refreshMail();
      showToast(t(muted ? "toastUnmuted" : "toastMuted"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "mute failed");
    }
  }

  // bulkLabel applies a label to every selected message. A brand-new name is
  // persisted as a label definition first so the sidebar keeps it.
  async function bulkLabel(label: string) {
    if (selectedUids.size === 0 || !label.trim()) return;
    setError("");
    try {
      const name = label.trim();
      if (!labelDefs.some((d) => d.name === name)) {
        const saved = await mailLabelSave(name, "");
        setLabelDefs((ds) => [...ds, saved]);
      }
      for (const uid of selectedUids) {
        await mailFlag(folder, uid, name, true);
      }
      setSelectedUids(new Set());
      showToast(t("toastLabelApplied"));
      refreshMail();
    } catch (e) {
      setError(e instanceof Error ? e.message : "label failed");
    }
  }

  async function selectThreadMessage(uid: number) {
    if (!detail) return;
    setError("");
    try {
      const next = await mailMessage(folder, { uid });
      setDetail(next);
      // Keep the URL pinned to the opened thread member so it stays shareable.
      router.push(`/mail/${encodeURIComponent(folder)}/${next.id || next.uid}`);
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
    delegateList,
    activeDelegate,
    switchDelegate,
    refreshDelegations,
    settingsInitialSection,
    openSettingsSection,
    sidebarOpen,
    setSidebarOpen,
    calendarOpen,
    setCalendarOpen,
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
    total,
    searching,
    sortBy,
    sortDir,
    changeSort,
    toggleMute,
    bulkLabel,
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
    openSnoozed,
    archiveMessage,
    bulkDelete,
    bulkArchive,
    bulkSpam,
    bulkFlag,
    loadMore,
    error,
    composeError,
    composeNotice,
    saveCurrentSearch,
    saveSearchSpec,
    searchRef,
    ai,
    prioritizing,
    priorityOn,
    priorityCategories,
    togglePriority,
    aiSearching,
    refreshMail,
    logout,
    refreshing,
    openContextMenu,
    onResizeStart,
    onResizeMove,
    onResizeEnd,
    summary,
    summarizing,
    summarize,
    reply,
    replyWithQuote,
    replyAll,
    editDraft,
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
    receiptOn,
    setReceiptOn,
    mergeOn,
    setMergeOn,
    mergeText,
    setMergeText,
    burnAfter,
    setBurnAfter,
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
    bodyText,
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
    aiComposeBusy,
    aiCompose,
    hasReplyTarget,
    aiDraftHint,
    setAiDraftHint,
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
    sendQuickReply,
    sendReceipt,
    recallMessage,
    applyRecall,
    markAllRead,
    closeCompose,
    openCalendar,
    calendarFocus,
    openCalendarEvent,
    driveOpen,
    setDriveOpen,
    openDrive,
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
    replyFrom,
    forwardFrom,
    toggleRead,
    spamMessage,
    toast,
    setToast,
  };
  return <MailStoreContext.Provider value={value}>{children}</MailStoreContext.Provider>;
}
