"use client";

// MailShell is the shared application chrome for the signed-in area: the
// FolderNav sidebar, the announcement / offline banners, and every global
// overlay (compose sheet, settings, contacts, calendar, drive, palette, ...).
// Both /home (workspace dashboard) and /mail (MailView) render inside it as
// the content area, so drawers and compose state survive navigation between
// them. All mailbox state lives in the MailStoreProvider that mounts this.

import { useEffect, useState } from "react";
import { Sparkles, WifiOff } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { MailSettings } from "@/components/settings/mail-settings";
import { MailContacts } from "@/components/contacts/mail-contacts";
import { FolderNav } from "@/components/mailbox/folder-nav";
import { LabelManager } from "@/components/mailbox/label-manager";
import { ComposePanel } from "@/components/compose/compose-panel";
import { ScheduledDialog } from "@/components/compose/scheduled-dialog";
import { CommandPalette } from "@/components/palette/command-palette";
import { ShortcutsDialog } from "@/components/mailbox/shortcuts-dialog";
import { SieveEditor } from "@/components/sieve/sieve-editor";
import { CalendarDrawer } from "@/components/calendar/calendar-drawer";
import { DriveDrawer } from "@/components/drive/drive-drawer";
import { textToHtml } from "@/components/mailbox/mail-utils";
import { buildFolderTree, flattenTree, folderLabel } from "@/components/mailbox/folder-tree";
import { mailAnnouncement, type MailAnnouncement, type OutboundAttachment } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useMailStore } from "@/components/mailbox/mail-store";

export function MailShell({ children }: { children: React.ReactNode }) {
  const [aiPromptOpen, setAiPromptOpen] = useState(false);
  const [aiPrompt, setAiPrompt] = useState("");
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
    openCompose,
    setSettingsOpen,
    setContactsOpen,
    setSieveOpen,
    contactsOpen,
    createFolder,
    renameFolder,
    deleteFolder,
    clearFolder,
    error,
    detail,
    ai,
    draftSaved,
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
    closeCompose,
    saveDraftNow,
    setTo,
    setCc,
    setBcc,
    setSubject,
    openMessage,
    loadMessages,
    setQuery,
    doAiSearch,
    settingsOpen,
    paletteOpen,
    setPaletteOpen,
    paletteActions,
    shortcutsOpen,
    setShortcutsOpen,
    sieveOpen,
    calendarOpen,
    openCalendar,
    calendarFocus,
    driveOpen,
    openDrive,
    setDriveOpen,
    setCalendarOpen,
    ctxMenu,
    setCtxMenu,
    quoteText,
    replyAllFrom,
    toggleRead,
    toggleStar,
    archiveMessage,
    spamMessage,
    removeMessage,
    toast,
    setToast,
    logout,
    scheduled,
    scheduledOpen,
    setScheduledOpen,
    loadScheduled,
    openSnoozed,
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
        <div className="flex shrink-0 items-center justify-center gap-2 bg-primary px-4 py-1.5 text-xs text-primary-foreground">
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
        aiComposeEnabled={ai.draft}
        aiComposeBusy={aiComposeBusy}
        onAiCompose={() => {
          setAiPrompt("");
          setAiPromptOpen(true);
        }}
        onSettings={() => openSettingsSection("appearance")}
        onContacts={() => setContactsOpen(true)}
        onSieve={() => setSieveOpen(true)}
        onCalendar={openCalendar}
        onDrive={openDrive}
        onLogout={logout}
        onScheduled={() => {
          setScheduledOpen(true);
          loadScheduled();
        }}
        onSnoozed={openSnoozed}
        onClose={() => setSidebarOpen(false)}
      />

      {/* Content area: MailView on /mail, the workspace dashboard on /home.
          Must stay a flex row so MailView's list + reading pane sit side by
          side; without it the reading pane stacks below the list (blank). */}
      <div className="flex min-w-0 flex-1">{children}</div>

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
          hasReplyTarget={hasReplyTarget}
          aiDraftHint={aiDraftHint}
          onAiDraftHint={setAiDraftHint}
          aiComposeBusy={aiComposeBusy}
          onAiCompose={aiCompose}
          drafting={drafting}
          onAiDraft={aiDraftReply}
          undoSendSeconds={prefs.undoSendSeconds}
          onUndoSendSeconds={setUndoSend}
          scheduleAt={scheduleAt}
          onScheduleAt={setScheduleAt}
          receiptOn={receiptOn}
          onReceiptOn={setReceiptOn}
          mergeOn={mergeOn}
          onMergeOn={setMergeOn}
          mergeText={mergeText}
          onMergeText={setMergeText}
          burnAfter={burnAfter}
          onBurnAfter={setBurnAfter}
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

      {calendarOpen && <CalendarDrawer onClose={() => setCalendarOpen(false)} initialEvent={calendarFocus} />}
      {driveOpen && <DriveDrawer onClose={() => setDriveOpen(false)} />}

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

      <Dialog open={aiPromptOpen} onOpenChange={setAiPromptOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Sparkles className="size-4 text-ai" />
              {t("aiCompose")}
            </DialogTitle>
            <DialogDescription>{t("aiComposeHint")}</DialogDescription>
          </DialogHeader>
          <Textarea
            value={aiPrompt}
            onChange={(e) => setAiPrompt(e.target.value)}
            placeholder={t("aiComposePlaceholder")}
            rows={4}
            autoFocus
          />
          <DialogFooter>
            <Button
              type="button"
              disabled={aiComposeBusy || !aiPrompt.trim()}
              onClick={() => {
                aiCompose(aiPrompt.trim());
                setAiPromptOpen(false);
              }}
            >
              {aiComposeBusy ? t("aiComposeBusy") : t("aiComposeGenerate")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
