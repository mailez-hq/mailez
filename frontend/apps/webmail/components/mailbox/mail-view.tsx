"use client";

// MailView is the content area of the /mail segment: the message list column
// plus the reading pane. All mailbox state and actions live in the
// MailStoreProvider in mail-store.tsx; the surrounding chrome (FolderNav
// sidebar, banners, global overlays) is MailShell in mail-shell.tsx, which
// also hosts the /home workspace as a sibling content area.

import { useEffect, useState } from "react";
import { Inbox as InboxIcon, Loader2, TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { highlightTerms } from "@/components/mailbox/highlight";
import { MessageListPanel } from "@/components/mailbox/message-list-panel";
import { ReadingPane } from "@/components/mailbox/reading-pane";
import { isMuted } from "@/components/mailbox/mail-utils";
import { cn } from "@/lib/utils";
import { useMailStore } from "@/components/mailbox/mail-store";

// First-run hint shown in the empty reading pane until dismissed. It nudges
// new users toward the shortcut help.
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
    folders,
    knownLabels,
    labelColors,
    folder,
    sidebarOpen,
    setSidebarOpen,
    moveSelectedTo,
    openMessage,
    detail,
    detailLoading,
    listWidth,
    messages,
    displayMessages,
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
    replyFrom,
    replyAllFrom,
    forwardFrom,
    editDraft,
    forward,
    backToList,
    moveDetailTo,
    reportNotSpam,
    toggleLabel,
    thread,
    threadOpen,
    threadLoading,
    toggleThread,
    snoozeMessage,
    prefs,
    sendQuickReply,
    recallMessage,
    applyRecall,
    sendReceipt,
    markAllRead,
    spamMessage,
    unsubscribeAction,
  } = useMailStore();

  return (
    <>
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
          displayMessages={displayMessages}
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
          onMarkAllRead={() => markAllRead(folder)}
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
            onReplyWithQuote={replyWithQuote}
            onReplyAll={replyAll}
            onForward={forward}
            onReplyThread={replyFrom}
            onReplyAllThread={replyAllFrom}
            onForwardThread={forwardFrom}
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
            conversationEnabled={prefs.conversation}
            onToggleThread={toggleThread}
            highlightTerms={highlightTerms(query)}
            readerFont={prefs.readerFont}
            paneWidth={prefs.paneWidth}
            meEmail={me.email}
            onQuickReply={sendQuickReply}
            muted={isMuted(detail)}
            onToggleMute={() => detail && toggleMute(detail)}
            onRecall={recallMessage}
            onSendReceipt={sendReceipt}
            onApplyRecall={applyRecall}
            onEditDraft={editDraft}
            onSnooze={(untilMs) => detail && snoozeMessage(detail, untilMs)}
          />
        ) : detailLoading ? (
          <div className="flex flex-1 flex-col items-center justify-center gap-2 bg-secondary p-6 text-sm text-muted-foreground dark:bg-background">
            <Loader2 className="size-9 animate-spin opacity-40" />
            <span>{t("loading")}</span>
          </div>
        ) : error ? (
          <div className="flex flex-1 flex-col items-center justify-center gap-2 bg-secondary p-6 text-sm text-destructive dark:bg-background">
            <TriangleAlert className="size-9 opacity-60" />
            <span>{error}</span>
          </div>
        ) : (
          <div className="flex flex-1 flex-col items-center justify-center gap-2 bg-secondary p-6 text-sm text-muted-foreground dark:bg-background">
            <InboxIcon className="size-9 opacity-40" />
            <span>{t("selectMessage")}</span>
            <FirstRunHint t={t} />
          </div>
        )}
      </div>
    </>
  );
}
