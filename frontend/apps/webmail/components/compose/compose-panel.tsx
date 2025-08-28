"use client";

import { useState, type RefObject } from "react";
import { CalendarClock, Check, Flame, LayoutTemplate, Loader2, Lock, Paperclip, PenLine, Undo2, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ComposeEditor } from "@/components/compose/compose-editor";
import { RecipientInput } from "@/components/compose/recipient-input";
import { TemplatesDialog } from "@/components/compose/templates-dialog";
import { fmtBytes } from "@/components/mailbox/mail-utils";
import { cn } from "@/lib/utils";
import {
  mailTemplates, type Contact, type MailIdentity, type MailTemplate, type OutboundAttachment,
} from "@/lib/api";

// datetime-local values are local wall time; toISOString() would emit UTC and
// land 8h in the past on UTC+8 (scheduled send would be rejected).
function localDateTime(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export interface ComposePanelProps {
  t: (key: string) => string;
  identities: MailIdentity[];
  from: string;
  onSelectIdentity: (email: string) => void;
  composeError: string;
  composeNotice: string;
  draftSaved: boolean;
  to: string[];
  cc: string[];
  bcc: string[];
  ccExpanded: boolean;
  onToggleCc: () => void;
  subject: string;
  body: string;
  bodyText: string;
  onBodyChange: (html: string, text: string) => void;
  attachments: OutboundAttachment[];
  onRemoveAttachment: (index: number) => void;
  composeFocus: "to" | "editor";
  allContacts: Contact[] | null;
  onLoadContacts: () => void;
  onOpenContacts: () => void;
  fileInputRef: RefObject<HTMLInputElement | null>;
  toInputRef: RefObject<HTMLInputElement | null>;
  spellcheck: boolean;
  aiComposeBusy: boolean;
  undoSendSeconds: number;
  onUndoSendSeconds: (seconds: number) => void;
  scheduleAt: string;
  onScheduleAt: (v: string) => void;
  receiptOn: boolean;
  onReceiptOn: (v: boolean) => void;
  mergeOn: boolean;
  onMergeOn: (v: boolean) => void;
  mergeText: string;
  onMergeText: (v: string) => void;
  burnAfter: number;
  onBurnAfter: (v: number) => void;
  signOn: boolean;
  encryptOn: boolean;
  onToggleSign: () => void;
  onToggleEncrypt: () => void;
  dragOverCompose: boolean;
  onDragOverChange: (v: boolean) => void;
  onDropFiles: (files: FileList) => void;
  onPickFiles: (files: FileList | null) => void;
  onSubmit: (e: React.FormEvent) => void;
  onClose: () => void;
  onSaveDraft: () => void;
  setTo: (v: string[]) => void;
  setCc: (v: string[]) => void;
  setBcc: (v: string[]) => void;
  setCcExpanded: (v: boolean) => void;
  setSubject: (v: string) => void;
}

// ComposePanel renders the non-modal right-side compose sheet.
// It is a pure view: all compose state and handlers live in MailView and are
// passed down as props so the panel stays a self-contained UI block.
export function ComposePanel(props: ComposePanelProps) {
  const {
    t,
    identities,
    from,
    onSelectIdentity,
    composeError,
    composeNotice,
    draftSaved,
    to,
    cc,
    bcc,
    ccExpanded,
    onToggleCc,
    subject,
    body,
    bodyText,
    onBodyChange,
    attachments,
    onRemoveAttachment,
    composeFocus,
    allContacts,
    onLoadContacts,
    onOpenContacts,
  fileInputRef,
  toInputRef,
  spellcheck,
    aiComposeBusy,
    undoSendSeconds,
    onUndoSendSeconds,
    scheduleAt,
    onScheduleAt,
    receiptOn,
    onReceiptOn,
    mergeOn,
    onMergeOn,
    mergeText,
    onMergeText,
    burnAfter,
    onBurnAfter,
    signOn,
    encryptOn,
    onToggleSign,
    onToggleEncrypt,
    dragOverCompose,
    onDragOverChange,
    onDropFiles,
    onPickFiles,
    onSubmit,
    onClose,
    onSaveDraft,
    setTo,
    setCc,
    setBcc,
    setCcExpanded,
    setSubject,
  } = props;
  const [templates, setTemplates] = useState<MailTemplate[]>([]);
  const [templatesOpen, setTemplatesOpen] = useState(false);
  const [templatesDialogOpen, setTemplatesDialogOpen] = useState(false);
  // Earliest schedulable moment, captured once so render stays pure (the
  // backend re-checks that send_at is in the future anyway).
  const [minScheduleAt] = useState(() => localDateTime(new Date(Date.now() + 60000)));

  function openTemplates() {
    setTemplatesOpen((v) => !v);
    if (templates.length === 0) {
      mailTemplates().then(setTemplates).catch(() => {});
    }
  }

  function insertTemplate(tpl: MailTemplate) {
    setTemplatesOpen(false);
    onBodyChange(`${body}${tpl.html}`, `${bodyText}${tpl.text || ""}`.trim());
  }

  return (
    <>
      {/* Click-away scrim: tapping outside the editor closes it (the close
          path persists the draft). */}
      <div className="fixed inset-0 z-40 bg-black/20" onClick={onClose} aria-hidden="true" />
      <div
        className="fixed inset-y-0 right-0 z-50 flex w-full flex-col border-l border-border bg-card shadow-2xl md:w-2/3"
        role="dialog"
        aria-modal="true"
        aria-label={t("newMessage")}
      >
      <form
        onSubmit={onSubmit}
        className="flex min-h-0 flex-1 flex-col overflow-y-auto"
        onDragOver={(e) => {
          e.preventDefault();
          onDragOverChange(true);
        }}
        onDragLeave={() => onDragOverChange(false)}
        onDrop={(e) => {
          e.preventDefault();
          onDragOverChange(false);
          if (e.dataTransfer.files?.length) onDropFiles(e.dataTransfer.files);
        }}
      >
        <input
          ref={fileInputRef}
          type="file"
          multiple
          className="hidden"
          onChange={(e) => {
            onPickFiles(e.target.files);
            e.target.value = "";
          }}
        />
        {dragOverCompose && (
          <div className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center border-4 border-dashed border-primary/50 bg-background/80">
            <p className="rounded-full bg-background px-4 py-2 text-sm font-medium shadow-sm">
              {t("dragDropFiles")}
            </p>
          </div>
        )}
        <header className="flex shrink-0 items-center justify-between border-b px-4 py-3">
          <h2 className="text-base font-semibold">{t("newMessage")}</h2>
          <Button variant="ghost" size="icon-sm" onClick={onClose} title={t("close")}>
            <X className="size-4" />
          </Button>
        </header>
        <div className="shrink-0 space-y-3 px-4 pt-3">
          {identities.length > 1 && (
            <div className="space-y-1">
              <Label>{t("fromLabel")}</Label>
              <div className="flex flex-wrap gap-1.5">
                {identities.map((idn) => (
                  <button
                    key={idn.email}
                    type="button"
                    onClick={() => onSelectIdentity(idn.email)}
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
                        idn.delegated ? "bg-violet-500" : idn.dkim_enabled ? "bg-primary" : "bg-[#C9A227]",
                      )}
                    />
                    <span className="truncate">{idn.email}</span>
                    {idn.delegated && (
                      <span className="rounded bg-violet-500/10 px-1 py-px text-[10px] font-medium text-violet-600 dark:text-violet-400">
                        {t("delegated")}
                      </span>
                    )}
                  </button>
                ))}
              </div>
            </div>
          )}
          <div className="space-y-1">
            <div className="flex items-center justify-between">
              <Label>{t("to")}</Label>
              <div className="flex items-center gap-1">
                {!ccExpanded ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={onToggleCc}
                  >
                    {t("expandRecipients")}
                  </Button>
                ) : (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => setCcExpanded(false)}
                  >
                    {t("collapseRecipients")}
                  </Button>
                )}
                <Button type="button" variant="ghost" size="sm" onClick={onOpenContacts}>
                  {t("contacts")}
                </Button>
              </div>
            </div>
            <div onFocusCapture={onLoadContacts}>
              <RecipientInput
                value={to}
                onChange={setTo}
                placeholder="user@example.com"
                inputRef={toInputRef}
                autoFocus={composeFocus === "to"}
                suggestions={allContacts || []}
              />
            </div>
          </div>
          {ccExpanded && (
            <>
              <div className="space-y-1">
                <Label>{t("cc")}</Label>
                <RecipientInput value={cc} onChange={setCc} suggestions={allContacts || []} />
              </div>
              <div className="space-y-1">
                <Label>{t("bcc")}</Label>
                <RecipientInput value={bcc} onChange={setBcc} suggestions={allContacts || []} />
              </div>
            </>
          )}
          <div className="space-y-1">
            <Label>{t("subject")}</Label>
            <Input value={subject} onChange={(e) => setSubject(e.target.value)} />
          </div>
        </div>
        {attachments.length > 0 && (
          <div className="flex shrink-0 flex-wrap gap-2 px-4 py-2">
            {attachments.map((a, i) => (
              <span
                key={`${a.filename}-${i}`}
                className="inline-flex max-w-60 items-center gap-1.5 rounded-lg border border-border bg-muted/50 px-2 py-1 text-xs"
              >
                <Paperclip className="size-3 shrink-0 text-muted-foreground" />
                <span className="truncate">{a.filename}</span>
                <span className="shrink-0 text-muted-foreground">{fmtBytes(a.size)}</span>
                <button
                  type="button"
                  onClick={() => onRemoveAttachment(i)}
                  title={t("removeAttachment")}
                  className="shrink-0 text-muted-foreground hover:text-destructive"
                >
                  <X className="size-3" />
                </button>
              </span>
            ))}
          </div>
        )}
        <div className="flex min-h-0 flex-1 flex-col px-4 py-3">
          <div className="flex min-h-0 flex-1 flex-col">
            {mergeOn && (
              <div className="mb-2 rounded-lg border border-border bg-muted/30 p-2">
                <p className="mb-1 text-[11px] text-muted-foreground">
                  {t("mergeHint")}
                </p>
                <textarea
                  value={mergeText}
                  onChange={(e) => onMergeText(e.target.value)}
                  placeholder={t("mergePlaceholder")}
                  rows={5}
                  className="w-full resize-y rounded-md border border-border bg-background p-2 text-xs outline-none focus:border-accent"
                />
              </div>
            )}
            {aiComposeBusy && !bodyText.trim() && (
              <div className="mb-2 flex items-center gap-2 rounded-md border border-ai/30 bg-ai/5 px-3 py-2 text-xs text-ai">
                <Loader2 className="size-3.5 shrink-0 animate-spin" />
                <span>{t("aiComposeStreaming")}</span>
              </div>
            )}
            <ComposeEditor
              value={body}
              onChange={onBodyChange}
              placeholder={t("bodyPlaceholder")}
              autoFocus={composeFocus === "editor"}
              spellcheck={spellcheck}
            />
          </div>
          {composeError && <p className="mt-2 text-sm text-destructive">{composeError}</p>}
          {composeNotice && <p className="mt-2 text-sm text-amber-600">{composeNotice}</p>}
          {draftSaved && !composeError && (
            <p className="mt-2 flex items-center gap-1 text-xs text-muted-foreground">
              <Check className="size-3" />
              {t("draftSaved")}
            </p>
          )}
        </div>
        <div className="sticky bottom-0 z-10 flex shrink-0 flex-wrap items-center gap-2 border-t bg-muted/40 px-4 py-2.5">
          <label className="flex cursor-pointer items-center gap-1 text-[11px] text-muted-foreground">
            <input
              type="checkbox"
              checked={receiptOn}
              onChange={(e) => onReceiptOn(e.target.checked)}
              className="size-3"
            />
            {t("receiptRequest")}
          </label>
          <div className="flex items-center gap-1 rounded-full border border-border px-2 py-1">
            <Flame className="size-3 text-muted-foreground" />
            <select
              value={burnAfter}
              onChange={(e) => onBurnAfter(Number(e.target.value))}
              title={t("burnAfter")}
              className="bg-transparent text-[11px] outline-none"
            >
              <option value={0}>{t("burnAfterOff")}</option>
              <option value={5}>{t("burnAfter5")}</option>
              <option value={30}>{t("burnAfter30")}</option>
              <option value={60}>{t("burnAfter60")}</option>
              <option value={1440}>{t("burnAfter1d")}</option>
            </select>
          </div>
          <button
            type="button"
            onClick={() => {
              onMergeOn(!mergeOn);
              if (!mergeOn) onReceiptOn(false);
            }}
            className={cn(
              "rounded-full border px-2 py-0.5 text-[11px] transition-colors",
              mergeOn
                ? "border-accent bg-accent font-medium text-accent-foreground"
                : "border-border text-muted-foreground hover:text-foreground",
            )}
            title={t("mergeTitle")}
          >
            {t("mergeToggle")}
          </button>
          <div className="relative">
            <Button type="button" variant="ghost" size="sm" onClick={openTemplates} title={t("templates")}>
              <LayoutTemplate className="size-4" />
              <span className="hidden sm:inline">{t("templates")}</span>
            </Button>
            {templatesOpen && (
              <div className="absolute bottom-full left-0 z-30 mb-1 w-64 rounded-lg border border-border bg-popover p-1 text-sm shadow-lg">
                {templates.length === 0 && (
                  <p className="px-2 py-1.5 text-xs text-muted-foreground">{t("noTemplates")}</p>
                )}
                {templates.map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => insertTemplate(item)}
                    className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-muted"
                  >
                    <span className="truncate">{item.name}</span>
                  </button>
                ))}
                <button
                  type="button"
                  onClick={() => {
                    setTemplatesOpen(false);
                    setTemplatesDialogOpen(true);
                  }}
                  className="mt-0.5 flex w-full items-center gap-2 rounded-md border-t border-border px-2 py-1.5 text-left text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  {t("manageTemplates")}
                </button>
              </div>
            )}
          </div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => fileInputRef.current?.click()}
            title={t("attach")}
          >
            <Paperclip className="size-4" />
            <span className="hidden sm:inline">{t("attach")}</span>
          </Button>
          <div className="flex items-center gap-1 rounded-full border border-border px-2 py-1">
            <Undo2 className="size-3 text-muted-foreground" />
            <select
              value={undoSendSeconds}
              onChange={(e) => onUndoSendSeconds(Number(e.target.value))}
              title={t("undoSendLabel")}
              disabled={!!scheduleAt}
              className="bg-transparent text-[11px] outline-none disabled:opacity-50"
            >
              <option value={0}>{t("undoSendOff")}</option>
              <option value={5}>5s</option>
              <option value={10}>10s</option>
              <option value={20}>20s</option>
              <option value={30}>30s</option>
            </select>
          </div>
          <div className="flex items-center gap-1 rounded-full border border-border px-2 py-1">
            <CalendarClock className="size-3 text-muted-foreground" />
            {scheduleAt ? (
              <input
                type="datetime-local"
                value={scheduleAt}
                onChange={(e) => onScheduleAt(e.target.value)}
                className="w-36 bg-transparent text-[11px] outline-none"
                min={minScheduleAt}
              />
            ) : (
              <button
                type="button"
                onClick={() => onScheduleAt(minScheduleAt)}
                title={t("scheduleSend")}
                className="text-[11px] text-muted-foreground transition-colors hover:text-foreground"
              >
                {t("scheduleSend")}
              </button>
            )}
          </div>
          <div className="ml-auto flex items-center gap-1">
            <button
              type="button"
              onClick={onToggleSign}
              className={cn(
                "flex items-center gap-1 rounded-full border px-2 py-1 text-[11px] transition-colors",
                signOn
                  ? "border-ai bg-ai/10 font-medium text-ai"
                  : "border-border text-muted-foreground hover:bg-muted",
              )}
            >
              <PenLine className="size-3" />
              {t("pgpSign")}
            </button>
            <button
              type="button"
              onClick={onToggleEncrypt}
              className={cn(
                "flex items-center gap-1 rounded-full border px-2 py-1 text-[11px] transition-colors",
                encryptOn
                  ? "border-ai bg-ai/10 font-medium text-ai"
                  : "border-border text-muted-foreground hover:bg-muted",
              )}
            >
              <Lock className="size-3" />
              {t("pgpEncrypt")}
            </button>
            <Button type="button" variant="outline" onClick={onSaveDraft}>
              {t("saveDraft")}
            </Button>
            <Button type="submit" disabled={aiComposeBusy}>
              {aiComposeBusy ? t("aiComposeBusy") : scheduleAt ? t("schedule") : t("send")}
            </Button>
          </div>
        </div>
      </form>
        <TemplatesDialog open={templatesDialogOpen} onOpenChange={setTemplatesDialogOpen} />
      </div>
    </>
  );
}
