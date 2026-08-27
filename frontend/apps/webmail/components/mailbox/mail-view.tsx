"use client";

// MailView is the thin render shell for the /mail area. All mailbox state and
// actions live in the MailStoreProvider in mail-store.tsx and are read here
// via useMailStore(), keeping the three-pane layout free of state logic.

import { useEffect, useState } from "react";
import { Inbox as InboxIcon, WifiOff } from "lucide-react";
import { Button } from "@/components/ui/button";
import { highlightTerms } from "@/components/mailbox/highlight";
import { MailSettings } from "@/components/settings/mail-settings";
import { MailContacts } from "@/components/contacts/mail-contacts";
import { FolderNav } from "@/components/mailbox/folder-nav";
import { LabelManager } from "@/components/mailbox/label-manager";
import { MessageListPanel } from "@/components/mailbox/message-list-panel";
import { ReadingPane } from "@/components/mailbox/reading-pane";
import { ComposePanel } from "@/components/compose/compose-panel";
import { ScheduledDialog } from "@/components/compose/scheduled-dialog";
import { CommandPalette } from "@/components/palette/command-palette";
import { ShortcutsDialog } from "@/components/mailbox/shortcuts-dialog";
import { SieveEditor } from "@/components/sieve/sieve-editor";
import { isMuted, textToHtml } from "@/components/mailbox/mail-utils";
import { buildFolderTree, flattenTree, folderLabel } from "@/components/mailbox/folder-tree";
import { mailAnnouncement, type MailAnnouncement, type OutboundAttachment } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useMailStore } from "@/components/mailbox/mail-store";

// First-run hint shown in the empty reading pane until dismissed. It nudges
// new users toward the shortcut help without adding a separate "home" page —
// the inbox stays the landing view (Gmail / FastMail convention).
const WELCOME_SEEN_KEY = "mailez.welcomeSeen";

function FirstRunHint({ t }: { t: (key: string) => string }) {
  const [show, setShow] = useState(false);

  useEffect(() => {
    try {
      if (!window.localStorage.getItem(WELCOME_SEEN_KEY)) setShow(true);
    } catch {
      // storage unavailable: skip the hint
    }
  }, []);

  if (!show) return null;
  return (
    <div className="flex flex-col items-center gap-2">
      <p className="max-w-xs text-center">{t("welcomeHint")}</p>
      <Button
        size="sm"
        variant="outline"
        onClick={() => {
          setShow(false);
          try {
            window.localStorage.setItem(WELCOME_SEEN_KEY, "1");
          } catch {
            // ignore
          }
        }}
      >
        {t("welcomeDismiss")}
      </Button>
    </div>
  );
}

export function MailView() {
  const {
    t,
    me,
    online,
    folders,
    unseen,
    knownLabels,
    labelColors,
    labelManagerOpen,
    setLabelManagerOpen,
    activeLabel,
    savedSearches,
    folder,
    sidebarOpen,
    setSidebarOpen,
    accountList,
    activeAccount,
    switchAccount,
    delegateList,
    activeDelegate,
    switchDelegate,
    openSettingsSection,
    settingsInitialSection,
    selectFolder,
    selectLabel,
    deleteLabel,
    runSavedSearch,
    removeSavedSearch,
    moveTo,
    moveSelectedTo,
    openCompose,
    setSettingsOpen,
    setContactsOpen,
    setSieveOpen,
    contactsOpen,
    detail,
    detailLoading,
    listWidth,
    messages,
    total,
    searching,
    loading,
    query,
    setQuery,
    setSearchAll,
    doSearch,
    clearSearch,
    selectedUids,
    cursor,
    selected,
    searchAll,
    sortBy,
    sortDir,
    changeSort,
    toggleMute,
    bulkLabel,
    searchSpec,
    applySearchSpec,
    saveSearchSpec,
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
    replyAll,
    forward,
    backToList,
    moveDetailTo,
    reportNotSpam,
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
    closeCompose,
    saveDraftNow,
    setTo,
    setCc,
    setBcc,
    setSubject,
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
    unsubscribeAction,
    toast,
    setToast,
  } = useMailStore();

  const [announcement, setAnnouncement] = useState<MailAnnouncement | null>(null);

  // The global admin announcement banner: fetched once per session, hidden
  // while the backend has none published.
  useEffect(() => {
    let cancelled = false;
    mailAnnouncement()
      .then((a) => {
        if (a && !cancelled) setAnnouncement(a);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className="flex h-screen overflow-hidden bg-background text-foreground">
      {announcement && (
        <div className="fixed inset-x-0 top-0 z-50 flex items-center justify-center gap-2 bg-primary px-4 py-1.5 text-xs text-primary-foreground">
          <span className="font-semibold">{announcement.subject}</span>
          {announcement.body && <span className="text-primary-foreground/85">{announcement.body}</span>}
        </div>
      )}
      {!online && (
        <div
          className={cn(
            "fixed inset-x-0 z-50 flex items-center justify-center gap-1.5 bg-ai py-1 text-xs text-ai-foreground",
            announcement ? "top-7" : "top-0",
          )}
        >
          <WifiOff className="size-3" />
          {t("offline")}
        </div>
      )}
      <FolderNav
        folders={folders}
        unseen={unseen}
        labels={knownLabels}
        activeLabel={activeLabel}
        savedSearches={savedSearches}
        current={folder}
        email={me.email}
        quotaBytes={me.quota_bytes}
        quotaUsed={me.quota_bytes_used}
        open={sidebarOpen}
        accountList={accountList}
        activeAccount={activeAccount}
        delegateList={delegateList}
        activeDelegate={activeDelegate}
        onSwitchAccount={switchAccount}
        onSwitchDelegate={switchDelegate}
        onManageAccounts={() => openSettingsSection("accounts")}
        onSelect={selectFolder}
        onSelectLabel={selectLabel}
        onManageLabels={() => setLabelManagerOpen(true)}
        onDeleteLabel={deleteLabel}
        onSelectSavedSearch={runSavedSearch}
        onRemoveSavedSearch={removeSavedSearch}
        onMoveToFolder={(dest, uid) => moveTo([uid], dest, t("toastMoved"))}
        onCreateFolder={createFolder}
        onRenameFolder={renameFolder}
        onDeleteFolder={deleteFolder}
        onClearFolder={clearFolder}
        onCompose={() => openCompose()}
        onSettings={() => openSettingsSection("appearance")}
        onContacts={() => setContactsOpen(true)}
        onSieve={() => setSieveOpen(true)}
        onLogout={logout}
        onScheduled={() => {
          setScheduledOpen(true);
          loadScheduled();
        }}
        onClose={() => setSidebarOpen(false)}
      />

      <div
        className={cn(
          "min-w-0 min-h-0 flex-col",
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
          searchSpec={searchSpec}
          onQueryChange={setQuery}
          onApplySpec={applySearchSpec}
          onSearch={doSearch}
          onClearSearch={clearSearch}
          selectedUids={selectedUids}
          cursor={cursor}
          openId={selected?.id}
          onOpen={(m) => openMessage(m, m.folder || folder)}
          onToggleSelect={toggleSelect}
          onDelete={removeMessage}
          onStar={toggleStar}
          onArchive={archiveMessage}
          onBulkDelete={bulkDelete}
          onBulkArchive={bulkArchive}
          onBulkSpam={bulkSpam}
          onBulkFlag={bulkFlag}
          onBulkLabel={bulkLabel}
          labels={knownLabels}
          onLoadMore={loadMore}
          searchInputRef={searchRef}
          error={error}
          folders={folders}
          onMoveToFolder={moveSelectedTo}
          onSaveSearch={saveCurrentSearch}
          onSaveSearchSpec={saveSearchSpec}
          searchAll={searchAll}
          sortBy={sortBy}
          sortDir={sortDir}
          onChangeSort={changeSort}
          onToggleSearchAll={() => setSearchAll((v: boolean) => !v)}
          category={categoryFilter}
          onCategoryChange={setCategoryFilter}
          onMenu={() => setSidebarOpen(true)}
          aiSearchEnabled={ai.search}
          aiPriorityEnabled={ai.priority}
          prioritizing={prioritizing}
          priorityOn={priorityOn}
          categories={priorityCategories}
          onTogglePriority={togglePriority}
          aiSearching={aiSearching}
          onAiSearch={doAiSearch}
          refreshing={refreshing}
          onRefresh={refreshMail}
          highlightTerms={highlightTerms(query)}
          onContextMenu={openContextMenu}
          className="flex-1"
        />
      </div>

      <div
        onPointerDown={onResizeStart}
        onPointerMove={onResizeMove}
        onPointerUp={onResizeEnd}
        className="hidden w-1.5 shrink-0 cursor-col-resize transition-colors hover:bg-accent md:block"
        title={t("resize")}
      />

      <div className={cn("min-w-0 flex-1", detail ? "flex" : "hidden md:flex")}>
        {detail ? (
          <ReadingPane
            detail={detail}
            detailLoading={detailLoading}
            aiEnabled={ai.summary}
            summary={summary}
            summarizing={summarizing}
            onSummarize={summarize}
            onReply={reply}
            onReplyAll={replyAll}
            onForward={forward}
            onArchive={() => archiveMessage(detail)}
            onDelete={() => removeMessage(detail)}
            onStar={() => toggleStar(detail)}
            onSpam={() => spamMessage(detail)}
            onBack={backToList}
            folder={folder}
            folders={folders}
            onMoveToFolder={moveDetailTo}
            showNotSpam={/^(junk|spam)$/i.test(folder)}
            onNotSpam={() => detail && reportNotSpam(detail)}
            onUnsubscribe={() => detail && unsubscribeAction(detail)}
            labels={knownLabels}
            labelColors={labelColors}
            onToggleLabel={(l) => detail && toggleLabel(detail, l)}
            thread={thread}
            threadOpen={threadOpen}
            threadLoading={threadLoading}
            onToggleThread={toggleThread}
            onSelectThread={selectThreadMessage}
            highlightTerms={highlightTerms(query)}
            readerFont={prefs.readerFont}
            paneWidth={prefs.paneWidth}
            meEmail={me.email}
            onQuickReply={sendQuickReply}
            muted={isMuted(detail)}
            onToggleMute={() => detail && toggleMute(detail)}
          />
        ) : (
          <div className="flex flex-1 flex-col items-center justify-center gap-2 bg-secondary p-6 text-sm text-muted-foreground dark:bg-background">
            <InboxIcon className="size-9 opacity-40" />
            <span>{t("selectMessage")}</span>
            <FirstRunHint t={t} />
          </div>
        )}
      </div>

      {/* Compose panel — non-modal right-side sheet */}
      {composeOpen && (
        <ComposePanel
          t={t}
          identities={identities}
          from={from}
          onSelectIdentity={selectIdentity}
          error={error}
          draftSaved={draftSaved}
          to={to}
          cc={cc}
          bcc={bcc}
          ccExpanded={ccExpanded}
          onToggleCc={() => setCcExpanded(true)}
          subject={subject}
          body={body}
          bodyText={bodyText}
          onBodyChange={(html, text) => {
            setBody(html);
            setBodyText(text);
          }}
          attachments={attachments}
          onRemoveAttachment={(i: number) => setAttachments((prev: OutboundAttachment[]) => prev.filter((_, j) => j !== i))}
          composeFocus={composeFocus}
          allContacts={allContacts}
          onLoadContacts={loadContactsOnce}
          onOpenContacts={() => setContactsOpen(true)}
          fileInputRef={fileInputRef}
          toInputRef={toInputRef}
          spellcheck={prefs.spellcheck}
          draftTone={draftTone}
          onDraftTone={setDraftTone}
          aiDraftEnabled={ai.draft}
          hasReplyTarget={!!detail}
          drafting={drafting}
          onAiDraft={aiDraftReply}
          undoSendSeconds={prefs.undoSendSeconds}
          onUndoSendSeconds={setUndoSend}
          scheduleAt={scheduleAt}
          onScheduleAt={setScheduleAt}
          signOn={signOn}
          encryptOn={encryptOn}
          onToggleSign={() => setSignOn((v: boolean) => !v)}
          onToggleEncrypt={() => setEncryptOn((v: boolean) => !v)}
          dragOverCompose={dragOverCompose}
          onDragOverChange={setDragOverCompose}
          onDropFiles={addFiles}
          onPickFiles={(files) => {
            if (files) addFiles(files);
          }}
          onSubmit={send}
          onClose={closeCompose}
          onSaveDraft={saveDraftNow}
          setTo={setTo}
          setCc={setCc}
          setBcc={setBcc}
          setCcExpanded={setCcExpanded}
          setSubject={setSubject}
        />
      )}

      <MailSettings
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
        initialSection={settingsInitialSection}
        onSaved={() => loadMessages(folder)}
      />

      <MailContacts
        open={contactsOpen}
        onOpenChange={setContactsOpen}
        onPick={(email: string) => setTo((prev: string[]) => [...prev, email])}
        onOpenMessage={(m, src) => {
          openMessage(m, src);
          setContactsOpen(false);
        }}
      />

      <CommandPalette
        open={paletteOpen}
        onOpenChange={setPaletteOpen}
        actions={paletteActions}
        aiSearchEnabled={ai.search}
        onAiSearch={(q) => {
          setQuery(q);
          doAiSearch(q);
        }}
      />

      <ShortcutsDialog open={shortcutsOpen} onOpenChange={setShortcutsOpen} />

      <SieveEditor open={sieveOpen} onOpenChange={setSieveOpen} />

      <LabelManager open={labelManagerOpen} onOpenChange={setLabelManagerOpen} />

      <ScheduledDialog open={scheduledOpen} onOpenChange={setScheduledOpen} />

      {ctxMenu && (() => {
        const m = ctxMenu.message;
        const src = m.folder || folder;
        return (
          <div
            className="fixed inset-0 z-[70]"
            onClick={() => setCtxMenu(null)}
            onContextMenu={(e) => {
              e.preventDefault();
              setCtxMenu(null);
            }}
          >
            <div
              className="absolute z-10 min-w-48 rounded-lg border border-border bg-popover p-1 text-sm shadow-xl"
              style={{
                left: Math.min(ctxMenu.x, window.innerWidth - 220),
                top: Math.min(ctxMenu.y, window.innerHeight - 380),
              }}
            >
              <button
                onClick={() => {
                  setCtxMenu(null);
                  openCompose(
                    m.from[0]?.email || "",
                    m.subject.startsWith("Re:") ? m.subject : `Re: ${m.subject}`,
                    textToHtml(quoteText(m)),
                    quoteText(m),
                    "editor",
                  );
                }}
                className="w-full rounded-md px-2 py-1.5 text-left transition-colors hover:bg-accent"
              >
                {t("reply")}
              </button>
              <button
                onClick={() => {
                  setCtxMenu(null);
                  replyAllFrom(m);
                }}
                className="w-full rounded-md px-2 py-1.5 text-left transition-colors hover:bg-accent"
              >
                {t("replyAll")}
              </button>
              <button
                onClick={() => {
                  setCtxMenu(null);
                  openCompose(
                    "",
                    m.subject.startsWith("Fwd:") ? m.subject : `Fwd: ${m.subject}`,
                    textToHtml(quoteText(m)),
                    quoteText(m),
                    "to",
                  );
                }}
                className="w-full rounded-md px-2 py-1.5 text-left transition-colors hover:bg-accent"
              >
                {t("forward")}
              </button>
              <div className="my-1 border-t border-border" />
              <button
                onClick={() => {
                  setCtxMenu(null);
                  toggleRead(m);
                }}
                className="w-full rounded-md px-2 py-1.5 text-left transition-colors hover:bg-accent"
              >
                {m.flags.includes("\\Seen") ? t("unread") : t("read")}
              </button>
              <button
                onClick={() => {
                  setCtxMenu(null);
                  toggleStar(m);
                }}
                className="w-full rounded-md px-2 py-1.5 text-left transition-colors hover:bg-accent"
              >
                {m.flags.includes("\\Flagged") ? t("unstar") : t("star")}
              </button>
              <div className="my-1 border-t border-border" />
              <button
                onClick={() => {
                  setCtxMenu(null);
                  archiveMessage(m);
                }}
                className="w-full rounded-md px-2 py-1.5 text-left transition-colors hover:bg-accent"
              >
                {t("archive")}
              </button>
              <button
                onClick={() => {
                  setCtxMenu(null);
                  spamMessage(m);
                }}
                className="w-full rounded-md px-2 py-1.5 text-left transition-colors hover:bg-accent"
              >
                {t("spam")}
              </button>
              <button
                onClick={() => {
                  setCtxMenu(null);
                  removeMessage(m);
                }}
                className="w-full rounded-md px-2 py-1.5 text-left text-destructive transition-colors hover:bg-accent"
              >
                {t("delete")}
              </button>
              <div className="my-1 border-t border-border" />
              <p className="px-2 py-1 text-[10px] font-medium tracking-wide text-muted-foreground uppercase">
                {t("moveTo")}
              </p>
              {flattenTree(buildFolderTree(folders), (leaf) => folderLabel(t, leaf))
                .filter((o) => o.value !== src)
                .map((o) => (
                  <button
                    key={o.value}
                    onClick={() => {
                      setCtxMenu(null);
                      moveTo([m.uid], o.value, t("toastMoved"));
                    }}
                    className="w-full truncate rounded-md px-2 py-1 text-left transition-colors hover:bg-accent"
                  >
                    {o.label}
                  </button>
                ))}
            </div>
          </div>
        );
      })()}

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
