"use client";

// MailStore homes the mailbox state, side-effects and action handlers behind
// a single context. Domain logic lives in the store/ hooks (list, folders,
// reading pane, search, labels, ai, compose, message actions, time-shifted
// mail, realtime, quick actions); this file owns the cross-domain state
// (current folder, error banner, category filter), the URL synchronization,
// the render-phase view resets and the wiring. MailView (mail-view.tsx) is a
// thin render shell on top of useMailStore().

import { createContext, useContext, useEffect, useMemo, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { usePathname, useRouter } from "next/navigation";
import { usePaletteActions } from "@/components/palette/palette-actions";
import { usePreferences } from "@/components/preferences-provider";
import { useNewMailNotification } from "@/components/mailbox/use-new-mail-notification";
import { writeLastFolder } from "@/lib/preferences";
import {
  mailMessage,
  logout as apiLogout,
  type CalendarEvent, type DriveEntry, type MailMessage, type Me,
} from "@/lib/api";
import { isPinned, isSnoozed } from "@/components/mailbox/mail-utils";
import { viewCacheGet, viewCachePut } from "@/lib/view-cache";
import { useToast } from "./store/use-toast";
import { useUiChrome } from "./store/use-ui-chrome";
import { useAccounts } from "./store/use-accounts";
import { useMailHotkeys, type HotkeyApi, type HotkeyState } from "./store/use-mail-hotkeys";
import { useMailList } from "./store/use-mail-list";
import { useFolderMgmt } from "./store/use-folder-mgmt";
import { useThreadDetail } from "./store/use-thread-detail";
import { useMailSearch } from "./store/use-mail-search";
import { useMailLabels } from "./store/use-mail-labels";
import { useMailAi } from "./store/use-mail-ai";
import { useCompose } from "./store/use-compose";
import { useRealtime } from "./store/use-realtime";
import { useMessageActions } from "./store/use-message-actions";
import { useScheduledSnooze } from "./store/use-scheduled-snooze";
import { useQuickActions } from "./store/use-quick-actions";
import { detailCache } from "./store/view-caches";

const MailStoreContext = createContext<MailStoreValue | null>(null);

export function useMailStore(): MailStoreValue {
  const ctx = useContext(MailStoreContext);
  if (!ctx) throw new Error("useMailStore must be used within MailStoreProvider");
  return ctx;
}

interface MailStoreProviderProps {
  me: Me;
  children: React.ReactNode;
}

export function useMailStoreValue(me: Me) {
  const t = useTranslations("mail");
  const tp = useTranslations("palette");
  const ts = useTranslations("settings");
  const router = useRouter();
  const { theme, setTheme, density, setDensity, prefs, setUndoSend } = usePreferences();
  useNewMailNotification(prefs.notifications, t);

  // ---- orchestration-owned state: current folder, error banner, filter ----
  const [folder, setFolder] = useState("Inbox");
  const [error, setError] = useState("");
  // Deterministic auto-category filter ("" = all) applied to the loaded list.
  const [categoryFilter, setCategoryFilter] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);

  // ---- workspace chrome: dialogs, list width, context menu ----
  const ui = useUiChrome();
  const {
    settingsOpen, setSettingsOpen, settingsInitialSection, openSettingsSection,
    contactsOpen, setContactsOpen, paletteOpen, setPaletteOpen,
    shortcutsOpen, setShortcutsOpen, sieveOpen, setSieveOpen,
    sidebarOpen, setSidebarOpen, calendarOpen, setCalendarOpen,
    calendarFocus, setCalendarFocus, driveOpen, setDriveOpen,
    listWidth, ctxMenu, setCtxMenu, openContextMenu,
    onResizeStart, onResizeMove, onResizeEnd,
  } = ui;

  const { toast, setToast, showToast } = useToast();

  // ---- message list: pages, loading, cursor, selection, sort ----
  const {
    messages, setMessages, total, setTotal, page, loading,
    cursor, setCursor, selectedUids, setSelectedUids,
    sortBy, sortDir, refreshing, setRefreshing,
    loadingMoreRef, loadSeq,
    loadMessages, changeSort, toggleSelect,
  } = useMailList({ folder, conversation: prefs.conversation, setError });

  // selectFolder depends on the search state (dropping an active search when
  // the open folder is re-clicked), but useFolderMgmt must run before the
  // search hook (search refreshes unseen counts). Route the late-bound
  // function through a ref, synced after every render — the same snapshot
  // pattern the keyboard handler uses below.
  const selectFolderRef = useRef<(f: string) => void>((f: string) => {
    router.push(`/mail/${encodeURIComponent(f)}`);
  });

  // ---- folders: list, unseen counts, CRUD ----
  const {
    folders, unseen, setUnseen, refreshUnseen, loadFolders,
    createFolder, renameFolder, deleteFolder, clearFolder, spamFolder,
  } = useFolderMgmt({
    email: me.email,
    folder,
    loadMessages,
    selectFolder: (f: string) => selectFolderRef.current(f),
    setError,
    showToast,
    t,
  });

  // The URL (/mail/[folder] or /mail/[folder]/[uid]) is the single source of
  // truth for the current folder and opened message. A route folder change is
  // applied to local state during render (React-endorsed adjustment — the
  // effect this replaces ran one paint later); a route uid opens that message
  // (optimistically from the current list row, then the full detail).
  const pathname = usePathname();
  const segs = (pathname ?? "").split("/").filter(Boolean);
  // null when the URL has no folder segment (bare /mail, which MailLayout
  // redirects to the last-visited folder); the inbox stays the fallback.
  const pathFolder = segs[1] ? decodeURIComponent(segs[1]) : null;
  const pathId = segs.length > 2 ? decodeURIComponent(segs[2]) : null;

  const [prevPathFolder, setPrevPathFolder] = useState(pathFolder);
  if (pathFolder !== prevPathFolder) {
    setPrevPathFolder(pathFolder);
    if (pathFolder && pathFolder !== folder) setFolder(pathFolder);
  }

  // ---- reading pane: opened message detail + conversation ----
  const {
    selected, setSelected, detail, setDetail,
    detailLoading, setDetailLoading,
    thread, setThread, threadOpen, setThreadOpen, threadLoading,
    toggleThread, selectThreadMessage, backToList,
  } = useThreadDetail({ folder, router, conversation: prefs.conversation, setError });

  // ---- search & virtual views (keyword box, specs, label filters, saved
  // searches, search-aware refresh) ----
  const {
    query, setQuery,
    searching, setSearching,
    searchSpec, setSearchSpec,
    activeView, setActiveView,
    activeLabel, setActiveLabel,
    savedSearches, searchAll, setSearchAll,
    lastSearchRef,
    searchSeqRef,
    doSearch, applySearchSpec,
    clearSearch, refreshMail, selectLabel,
    saveCurrentSearch, saveSearchSpec, removeSavedSearch, runSavedSearch,
  } = useMailSearch({
    folder,
    pathId,
    router,
    loadMessages,
    loadSeq,
    setMessages,
    setSelected,
    setDetail,
    setCursor,
    setUnseen,
    setError,
    setRefreshing,
  });

  // ---- accounts & delegated mailboxes ----
  // Switching either re-scopes every /mail/* request and resets the view.
  const {
    accountList, activeAccount, refreshAccounts,
    delegateList, activeDelegate, switchAccount, switchDelegate, refreshDelegations,
  } = useAccounts(router, (opts) => {
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
    if (opts.includeLabel) setActiveLabel("");
  }, loadFolders, loadMessages);

  // ---- labels (definitions, message keywords, colors, manager dialog) ----
  const {
    labelDefs,
    labelManagerOpen, setLabelManagerOpen,
    knownLabels, labelColors,
    toggleLabel, saveLabel, renameLabel, deleteLabel, bulkLabel,
  } = useMailLabels({
    messages,
    folder,
    activeLabel,
    selectLabel,
    selectedUids,
    setSelectedUids,
    setError,
    setMessages,
    setDetail,
    showToast,
    refreshMail,
    t,
  });

  // ---- ai (capability flags, reading-pane summary, priority inbox, NL search) ----
  const {
    summary, setSummary,
    summarizing,
    prioritizing,
    aiSearching,
    priorityOn, setPriorityOn,
    priorityCategories, setPriorityCategories,
    setBaseMessages,
    ai,
    summarize, togglePriority, doAiSearch,
  } = useMailAi({
    prefsAiEnabled: prefs.ai.enabled,
    prefsAi: prefs.ai,
    detail,
    messages,
    query,
    setError,
    setMessages,
    setSearching,
    setSelected,
    setDetail,
    setCursor,
  });

  // ---- compose: form, draft persistence, reply seeding, send pipeline ----
  const {
    composeOpen,
    composeError,
    composeNotice,
    receiptOn, setReceiptOn,
    mergeOn, setMergeOn,
    mergeText, setMergeText,
    burnAfter, setBurnAfter,
    composeFocus,
    hasReplyTarget,
    aiDraftHint, setAiDraftHint,
    aiComposeBusy,
    drafting,
    to, setTo,
    cc, setCc,
    bcc, setBcc,
    ccExpanded, setCcExpanded,
    attachments, setAttachments,
    dragOverCompose, setDragOverCompose,
    subject, setSubject,
    body, setBody,
    bodyText, setBodyText,
    draftTone, setDraftTone,
    signOn, setSignOn,
    encryptOn, setEncryptOn,
    draftSaved,
    scheduleAt, setScheduleAt,
    allContacts,
    identities,
    from,
    selectIdentity,
    toInputRef, fileInputRef,
    quoteText,
    saveDraftNow,
    closeCompose,
    openCompose,
    addFiles,
    replyAllFrom, replyFrom, forwardFrom,
    loadContactsOnce,
    reply, editDraft, replyWithQuote, replyAll, forward,
    aiDraftReply, aiCompose,
    send,
  } = useCompose({
    folder,
    detail,
    me,
    prefs,
    loadMessages,
    refreshUnseen,
  });

  // ---- realtime: online status, push subscription, SSE mail events ----
  const { online } = useRealtime({
    email: me.email,
    notifications: prefs.notifications,
    folder,
    refreshUnseen,
    loadMessages,
  });

  // ---- message actions: move family, flags, open, sender actions ----
  const {
    moveTo, moveSelectedTo, moveDetailTo,
    archiveMessage, spamMessage, removeMessage,
    bulkDelete, bulkArchive, bulkSpam, bulkFlag,
    setSeen, toggleRead, toggleStar, togglePin, toggleMute,
    openMessage, reportNotSpam, unsubscribeAction,
  } = useMessageActions({
    folder,
    messages,
    searching,
    selected,
    detail,
    selectedUids,
    spamFolder,
    setSelected,
    setDetail,
    setMessages,
    setSelectedUids,
    setError,
    showToast,
    t,
    router,
    refreshUnseen,
    refreshMail,
    loadMessages,
    openCompose,
  });

  // ---- scheduled sends & snoozed messages ----
  const {
    scheduled, scheduledOpen, setScheduledOpen, scheduledLoading,
    loadScheduled, cancelScheduled,
    snoozedMsgs, snoozeMessage, unsnooze, openSnoozed,
  } = useScheduledSnooze({
    folder,
    setMessages,
    setTotal,
    setSelected,
    setDetail,
    setCursor,
    setQuery,
    setSearching,
    setSearchSpec,
    setActiveView,
    setActiveLabel,
    setError,
    showToast,
    refreshMail,
    t,
  });

  // ---- one-shot reading-pane actions (reply/receipt/recall/mark-all) ----
  const {
    sendQuickReply, sendReceipt, recallMessage, applyRecall, markAllRead,
  } = useQuickActions({ setError, showToast, refreshMail, t });

  // Opening the calendar already focused on a specific event (e.g. clicking
  // a row on the workspace home): the drawer pops the event dialog directly.
  // The drawer is never hidden behind a compose; the draft persists via
  // closeCompose.
  function openCalendar() {
    if (composeOpen) closeCompose();
    setCalendarFocus(null);
    setCalendarOpen(true);
  }

  function openCalendarEvent(ev: CalendarEvent) {
    if (composeOpen) closeCompose();
    setCalendarFocus(ev);
    setCalendarOpen(true);
  }

  // Opening the drive drawer dismisses a composing editor first, like the
  // calendar drawer. driveFocus is the file the drawer should navigate to
  // (recent-files card on the workspace dashboard); plain openDrive clears it.
  const [driveFocus, setDriveFocus] = useState<DriveEntry | null>(null);

  function openDrive() {
    if (composeOpen) closeCompose();
    setDriveFocus(null);
    setDriveOpen(true);
  }

  function openDriveFile(entry: DriveEntry) {
    if (composeOpen) closeCompose();
    setDriveFocus(entry);
    setDriveOpen(true);
  }

  // A folder switch resets the transient view state (query, filters,
  // selection, reader pane). The reset runs during render as the React-
  // endorsed state adjustment — one render earlier than an effect, with no
  // cascading re-render.
  const [prevResetFolder, setPrevResetFolder] = useState(folder);
  if (folder !== prevResetFolder) {
    setPrevResetFolder(folder);
    setQuery("");
    setSearching(false);
    setSearchSpec(null);
    // A category chosen in one folder can't silently filter (and empty) the
    // next one.
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
  }

  useEffect(() => {
    // Fetch-on-mount: loadFolders resolves every setState after await.
    loadFolders();
  }, [loadFolders]);

  useEffect(() => {
    lastSearchRef.current = "";
    // A folder load supersedes any in-flight search: bump the search guard
    // so a slow /mail/search response cannot land afterwards and overwrite
    // the folder list (searches already invalidate folder loads the other
    // way via loadSeq).
    searchSeqRef.current++;
    // Fetch-on-folder-change: loadMessages resolves every setState after await.
    loadMessages(folder);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- searchSeqRef is a stable ref
  }, [folder, loadMessages, lastSearchRef, searchSeqRef]);

  // Remember the last-visited folder so login / re-open can restore the
  // user's position instead of always landing in the inbox. Only /mail routes
  // count as folder visits — /home keeps the inbox fallback without clobbering
  // the stored position.
  useEffect(() => {
    if (folder && pathname?.startsWith("/mail")) writeLastFolder(folder);
  }, [folder, pathname]);

  useEffect(() => {
    // Route-uid subscription: leaving a message must clear the reader pane,
    // and the URL is an external system (the router), so this sync belongs
    // in an effect. The setState calls are the intentional external-system
    // write, not a cascading render.
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

  // clamp cursor when the list shrinks (render-phase adjustment — one render
  // earlier than the effect this replaces, same clamp).
  const [prevMsgCount, setPrevMsgCount] = useState(messages.length);
  if (messages.length !== prevMsgCount) {
    setPrevMsgCount(messages.length);
    setCursor(Math.min(cursor, Math.max(0, messages.length - 1)));
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

  // shownMessages applies the category filter on top of displayMessages —
  // exactly what the list panel renders. Keyboard navigation must index
  // this same array: using the raw list let j/k highlight one row while
  // Enter/x/s acted on a different (reordered or hidden) message.
  const shownMessages = useMemo(() => {
    if (!categoryFilter) return displayMessages;
    return displayMessages.filter(
      (m) => (priorityCategories[String(m.uid)] ?? m.category) === categoryFilter,
    );
  }, [displayMessages, categoryFilter, priorityCategories]);

  function loadMore() {
    if (loadingMoreRef.current || searching || messages.length >= total) return;
    loadingMoreRef.current = true;
    loadMessages(folder, page + 1).finally(() => {
      loadingMoreRef.current = false;
    });
  }

  // Close the row context menu on Escape.
  useEffect(() => {
    if (!ctxMenu) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setCtxMenu(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [ctxMenu, setCtxMenu]);

  // logout signs out and returns to the sign-in page (full navigation so all
  // in-memory mailbox state is dropped).
  async function logout() {
    try {
      await apiLogout();
    } finally {
      // Full browser navigation on purpose: it drops every module-level
      // cache so a following login as a different user cannot see stale
      // per-mailbox data that survives an SPA route change.
      // eslint-disable-next-line @next/next/no-location-assign-relative-destination
      window.location.href = "/";
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
  // Keep the late-bound folder-mgmt redirect current (see selectFolderRef).
  useEffect(() => {
    selectFolderRef.current = selectFolder;
  });

  // openThenReply backs out to the list row under the cursor and opens the
  // reply/forward compose for it (keyboard shortcuts "r" / "a" / "f"). If the
  // row is already the open detail, reply immediately; otherwise open the
  // message first and compose once the detail has swapped in.
  async function openThenReply(kind: "reply" | "replyAll" | "forward") {
    const m = stateRef.current.shownMessages[stateRef.current.cursor];
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

  // ---- global keyboard shortcuts ----
  const stateRef = useRef<HotkeyState>({
    messages, shownMessages, cursor, folder, searching, detail,
    composeOpen, settingsOpen, contactsOpen, paletteOpen, shortcutsOpen,
  });
  const apiRef = useRef<HotkeyApi>({
    openMessage, toggleSelect, removeMessage, toggleStar, setSeen,
    archiveMessage, spamMessage,
    openCompose, openThenReply, selectFolder, backToList,
  });
  // Keep the keyboard handler's snapshot refs in sync after every render
  // (writing refs during render is a React 19 anti-pattern).
  useEffect(() => {
    stateRef.current = {
      messages, shownMessages, cursor, folder, searching, detail,
      composeOpen, settingsOpen, contactsOpen, paletteOpen, shortcutsOpen,
    };
    apiRef.current = {
      openMessage, toggleSelect, removeMessage, toggleStar, setSeen,
      archiveMessage, spamMessage,
      openCompose, openThenReply, selectFolder, backToList,
    };
  });

  useMailHotkeys({ stateRef, apiRef, searchRef, setPaletteOpen, setShortcutsOpen, setCursor });

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

  // The exposed value mirrors every local so MailView (and its sub-panels via
  // useMailStore) can read state and call actions without prop drilling. The
  // context type is derived from this return value (ReturnType below), so the
  // interface can never drift from the implementation.
  return {
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
    driveFocus,
    openDrive,
    openDriveFile,
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
}

export type MailStoreValue = ReturnType<typeof useMailStoreValue>;

export function MailStoreProvider({ me, children }: MailStoreProviderProps) {
  const value = useMailStoreValue(me);
  return <MailStoreContext.Provider value={value}>{children}</MailStoreContext.Provider>;
}
