"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Loader2, MessageSquarePlus, Reply, Sparkles } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { aiReplies, aiTranslate, mailAttachmentsZip, mailFlag, mailRaw, pgpDecrypt } from "@/lib/api";
import type { MailMessage, MailThread } from "@/lib/api";
import { AttachmentList } from "@/components/mailbox/reader/attachment-list";
import { AiSummary } from "@/modules/ai-summary";
import { MessageActions } from "@/components/mailbox/reader/message-actions";
import { MessageBody } from "@/components/mailbox/reader/message-body";
import { MessageDetails, MessageMeta } from "@/components/mailbox/reader/message-meta";
import { MessageHeader } from "@/components/mailbox/reader/message-header";
import {
  BurnGate, PgpBanner, RecallNotice, ReceiptBanner, RemoteImageBanner, TranslationPanel,
} from "@/components/mailbox/reader/notice-banners";
import { QuickReply } from "@/components/mailbox/reader/quick-reply";
import { useMounted } from "@/components/mailbox/reader/use-mounted";
import { ThreadMessage } from "@/components/mailbox/reader/thread-message";
import { foldHtmlQuotes } from "@/components/mailbox/reader/html-quotes";
import { InvitationBanner } from "@/components/mailbox/reader/invitation-banner";
import { escHtml, isSnoozed } from "@/components/mailbox/mail-utils";
import type { ReaderFontSize, ReadingPaneWidth } from "@/lib/preferences";
import {
  blockRemoteImages, hasRemoteImages, rememberedRemoteSenders,
} from "@/components/mailbox/reader/remote-images";
import {
  fmtFullDate, parseBody,
} from "@/components/mailbox/reader/body";
import { sanitizeMailHTML } from "@/lib/sanitize";

export function ReadingPane({
  detail,
  detailLoading,
  aiEnabled,
  aiLocked,
  summary,
  summarizing,
  onSummarize,
  onReply,
  onReplyAll,
  onForward,
  onArchive,
  onDelete,
  onStar,
  onSpam,
  onBack,
  folder,
  folders,
  onMoveToFolder,
  showNotSpam,
  onNotSpam,
  onUnsubscribe,
  labels,
  labelColors,
  onToggleLabel,
  thread,
  conversationEnabled,
  onReplyThread,
  onForwardThread,
  highlightTerms,
  readerFont,
  paneWidth,
  meEmail,
  onQuickReply,
  muted,
  onToggleMute,
  onRecall,
  onSendReceipt,
  onApplyRecall,
  onReplyWithQuote,
  onEditDraft,
  onSnooze,
}: {
  detail: MailMessage;
  detailLoading: boolean;
  aiEnabled: boolean;
  /** The AI module is not enabled: render locked AI entry points instead of hiding them. */
  aiLocked?: boolean;
  summary: string;
  summarizing: boolean;
  onSummarize: (threadText?: string) => void;
  onReply: () => void;
  onReplyAll: () => void;
  onForward: () => void;
  onArchive: () => void;
  onDelete: () => void;
  onStar: () => void;
  onSpam: () => void;
  onBack: () => void;
  folder: string;
  folders: string[];
  onMoveToFolder: (destination: string) => void;
  showNotSpam: boolean;
  onNotSpam: () => void;
  onUnsubscribe: () => void;
  labels: string[];
  labelColors?: Record<string, string>;
  onToggleLabel: (label: string) => void;
  thread: MailThread | null;
  threadOpen: boolean;
  threadLoading: boolean;
  conversationEnabled: boolean;
  onToggleThread: () => void;
  onReplyThread: (m: MailMessage) => void;
  onForwardThread: (m: MailMessage) => void;
  highlightTerms?: string[];
  readerFont?: ReaderFontSize;
  paneWidth?: ReadingPaneWidth;
  meEmail: string;
  onQuickReply: (
    to: string[], cc: string[], subject: string, text: string, inReplyTo: string, references: string,
  ) => Promise<boolean>;
  muted: boolean;
  onToggleMute: () => void;
  onRecall: (folder: string, uid: number) => Promise<boolean>;
  onSendReceipt: (folder: string, uid: number) => Promise<boolean>;
  onApplyRecall: (messageId: string, folder: string, uid: number) => Promise<boolean>;
  onReplyWithQuote: (selection: string) => void;
  onEditDraft: () => void;
  onSnooze: (untilMs: number) => void;
}) {
  const t = useTranslations("mail");
  const fontPx = readerFont === "sm" ? 13 : readerFont === "lg" ? 16 : readerFont === "xl" ? 18 : 14;
  const paneMax = paneWidth === "narrow" ? 720 : paneWidth === "wide" ? 1100 : 0;
  const isDraft = /^drafts$/i.test(folder) || (detail.flags || []).some((f) => /draft/i.test(f));
  const snoozed = isSnoozed(detail);
  const mounted = useMounted();
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [expandedQuotes, setExpandedQuotes] = useState<Set<number>>(new Set());
  const [summaryCollapsed, setSummaryCollapsed] = useState(false);
  const [summaryOpen, setSummaryOpen] = useState(false);
  const [feedback, setFeedback] = useState<"up" | "down" | null>(null);
  const [quickReplyOpen, setQuickReplyOpen] = useState(false);
  const [receiptBusy, setReceiptBusy] = useState(false);
  const [snoozeOpen, setSnoozeOpen] = useState(false);
  const [snoozeCustom, setSnoozeCustom] = useState("");
  const [recallBusy, setRecallBusy] = useState(false);
  const [recallHandled, setRecallHandled] = useState(false);
  const [replies, setReplies] = useState<string[]>([]);
  const [repliesLoading, setRepliesLoading] = useState(false);
  const [burnRevealed, setBurnRevealed] = useState(false);
  const [pgpPlaintext, setPgpPlaintext] = useState<string | null>(null);
  const [decrypting, setDecrypting] = useState(false);
  const [pgpError, setPgpError] = useState("");
  const [labelOpen, setLabelOpen] = useState(false);
  const [newLabel, setNewLabel] = useState("");
  const labelMenuRef = useRef<HTMLDivElement>(null);
  const [rawOpen, setRawOpen] = useState(false);
  const [rawText, setRawText] = useState("");
  const [rawLoading, setRawLoading] = useState(false);
  const [rawError, setRawError] = useState("");
  const [quoteSel, setQuoteSel] = useState<{ x: number; y: number; text: string } | null>(null);
  const bodyRef = useRef<HTMLDivElement>(null);
  const [quickReplyText, setQuickReplyText] = useState("");
  const [quickReplyAll, setQuickReplyAll] = useState(false);
  const [quickSending, setQuickSending] = useState(false);
  const [translation, setTranslation] = useState("");
  const [translating, setTranslating] = useState(false);
  const [translatedView, setTranslatedView] = useState(false);

  // Track which thread message is expanded
  const [expandedUid, setExpandedUid] = useState<number | null>(detail.uid);
  // Inline quick reply in thread view: which member the box targets (conversation
  // model - the reply box opens under the message being answered, not as an
  // overlay that hides the conversation). null = newest member.
  const [quickReplyTargetUid, setQuickReplyTargetUid] = useState<number | null>(null);

  const senderDomain = (detail.from[0]?.email || "").split("@").pop() || "";
  const [remoteLoaded, setRemoteLoaded] = useState(() =>
    rememberedRemoteSenders().includes(senderDomain),
  );

  // Reset transient panel state when a different mail is opened. The panel is
  // kept mounted (no key remount) so switching mails doesn't flash empty
  // content behind it; we only clear the per-message state here. The reset
  // runs during render (React's "adjust state when props change" pattern)
  // instead of inside an effect, so the fresh message never renders with the
  // previous message's panel state.
  const detailKey = `${detail.uid}\u0000${detail.from[0]?.email || ""}`;
  const [prevDetailKey, setPrevDetailKey] = useState(detailKey);
  if (prevDetailKey !== detailKey) {
    setPrevDetailKey(detailKey);
    const domain = (detail.from[0]?.email || "").split("@").pop() || "";
    setExpandedUid(detail.uid);
    setDetailsOpen(false);
    setExpandedQuotes(new Set());
    setSummaryCollapsed(false);
    setSummaryOpen(false);
    setFeedback(null);
    setQuickReplyOpen(false);
    setQuickReplyTargetUid(null);
    setReplies([]);
    setPgpPlaintext(null);
    setPgpError("");
    setRawText("");
    setRemoteLoaded(rememberedRemoteSenders().includes(domain));
  }

  // Track text selections inside the message body and offer "reply with
  // this quote" (引用选中段落回复).
  useEffect(() => {
    if (!detail) return;
    const onMouseUp = () => {
      const sel = window.getSelection();
      const text = sel?.toString().trim() || "";
      if (!text || !sel || sel.rangeCount === 0) {
        setQuoteSel(null);
        return;
      }
      if (!bodyRef.current?.contains(sel.anchorNode)) {
        setQuoteSel(null);
        return;
      }
      const rect = sel.getRangeAt(0).getBoundingClientRect();
      setQuoteSel({ x: rect.left + rect.width / 2, y: rect.bottom + 6, text: text.slice(0, 400) });
    };
    const onScroll = () => setQuoteSel(null);
    document.addEventListener("mouseup", onMouseUp);
    document.addEventListener("scroll", onScroll, true);
    return () => {
      document.removeEventListener("mouseup", onMouseUp);
      document.removeEventListener("scroll", onScroll, true);
    };
  }, [detail]);

  // Close the label menu when clicking outside it or pressing Escape; the
  // menu is a plain popover without a modal backdrop.
  useEffect(() => {
    if (!labelOpen) return;
    function onMouseDown(e: MouseEvent) {
      if (labelMenuRef.current && !labelMenuRef.current.contains(e.target as Node)) {
        setLabelOpen(false);
      }
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") setLabelOpen(false);
    }
    document.addEventListener("mousedown", onMouseDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onMouseDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [labelOpen]);

  async function loadRaw() {
    setRawOpen(true);
    setRawLoading(true);
    setRawError("");
    try {
      const res = await mailRaw(folder, detail.uid);
      setRawText(res.raw);
    } catch (e) {
      setRawError(e instanceof Error ? e.message : "load raw failed");
    } finally {
      setRawLoading(false);
    }
  }

  async function downloadRaw() {
    try {
      const res = await mailRaw(folder, detail.uid);
      const blob = new Blob([res.raw], { type: "message/rfc822" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${(detail.subject || "message").replace(/[^\w\s-]/g, "").slice(0, 60) || "message"}.eml`;
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      // ignore download failures
    }
  }

  const isPgpEncrypted = /-----BEGIN PGP MESSAGE-----/.test(detail.text_body || "");

  async function decryptPgp() {
    if (!detail.text_body) return;
    setDecrypting(true);
    setPgpError("");
    try {
      const res = await pgpDecrypt(detail.text_body);
      setPgpPlaintext(res.plaintext);
    } catch (e) {
      setPgpError(e instanceof Error ? e.message : "pgp decrypt failed");
    } finally {
      setDecrypting(false);
    }
  }

  const segments = useMemo(
    () => parseBody(pgpPlaintext ?? (detail.text_body || "")),
    [detail.text_body, pgpPlaintext],
  );
  const remoteImages = useMemo(
    () => hasRemoteImages(detail.html_body),
    [detail.html_body],
  );
  const htmlBody = useMemo(() => {
    if (!detail.html_body) return "";
    if (pgpPlaintext !== null || isPgpEncrypted) return "";
    const clean = sanitizeMailHTML(detail.html_body);
    const guarded =
      remoteImages && !remoteLoaded
        ? blockRemoteImages(clean)
        : clean;
    // Fold quoted history into <details> (same as thread members): the
    // engine's auto-generated html carries the whole "> " chain as nested
    // blockquotes that would otherwise render in full.
    return foldHtmlQuotes(guarded, t("quotedText"));
  }, [detail.html_body, remoteImages, remoteLoaded, pgpPlaintext, isPgpEncrypted, t]);

  const starred = detail.flags.includes("\\Flagged");
  const sender = detail.from[0];
  const senderName = sender?.name || sender?.email || "?";

  // Only trust the loaded conversation when it belongs to the opened message.
  // While a new conversation loads, show a loading placeholder instead of a
  // stale thread or a flash of the single-message view.
  const threadForDetail = thread?.thread_id === detail.thread_id ? thread : null;
  const threadMessages = threadForDetail?.messages ?? [detail];
  const isThreadView = threadMessages.length > 1;
  // Thread-level quick reply and Smart Reply suggestions target the newest
  // member (conversation semantics); the AI summary covers the whole conversation.
  const lastThreadMsg = threadMessages[threadMessages.length - 1];
  const threadText = useMemo(
    () =>
      threadMessages
        .map((m) => (m.text_body || "").trim())
        .filter(Boolean)
        .join("\n\n"),
    [threadMessages],
  );
  // The conversation view must stay in sync with the list: when the setting
  // is off, the reading pane shows the single message even if the message
  // belongs to a thread.
  const isConversation = conversationEnabled && !!detail.thread_id;

  function handleToggleMessage(uid: number) {
    setExpandedUid((prev) => (prev === uid ? null : uid));
  }

  function toggleQuote(i: number) {
    setExpandedQuotes((prev) => {
      const next = new Set(prev);
      if (next.has(i)) next.delete(i);
      else next.add(i);
      return next;
    });
  }

  async function doTranslate() {
    const text = (detail.text_body || "").trim();
    if (!text) return;
    setTranslating(true);
    try {
      const target = /[\u4e00-\u9fff]/.test(text) ? "English" : "简体中文";
      const res = await aiTranslate(text, target);
      setTranslation(res.translation);
      setTranslatedView(true);
    } catch {
      setTranslation("");
      setTranslatedView(false);
    } finally {
      setTranslating(false);
    }
  }

  // loadReplies fetches Smart Reply suggestions; forText lets the thread
  // view request suggestions for the newest member instead of the opened one.
  async function loadReplies(forText?: string) {
    // Guard the input: a bare onClick pass-through would hand us a
    // MouseEvent, and any non-string must fall back rather than crash
    // on .trim().
    const source = typeof forText === "string" ? forText : detail.text_body;
    const text = (typeof source === "string" ? source : "").trim();
    if (!text) return;
    setRepliesLoading(true);
    try {
      const res = await aiReplies(text);
      setReplies(res.replies || []);
    } catch {
      setReplies([]);
    } finally {
      setRepliesLoading(false);
    }
  }

  // downloadAllAttachments packs every attachment of the message into a zip.
  async function downloadAllAttachments() {
    try {
      const blob = await mailAttachmentsZip(detail.folder || folder, detail.uid);
      const a = document.createElement("a");
      a.href = URL.createObjectURL(blob);
      a.download = `${detail.subject || "attachments"}.zip`;
      a.click();
      URL.revokeObjectURL(a.href);
    } catch {
      // ignore: the user can still download attachments individually
    }
  }

  // revealBurn unlocks a burn-after-read message once (flags it $BurnRead).
  async function revealBurn() {
    setBurnRevealed(true);
    try {
      await mailFlag(detail.folder || folder, detail.uid, "$BurnRead", true);
    } catch {
      // non-fatal: the body is shown for this session regardless
    }
  }

  function doPrint() {
    const w = window.open("", "_blank");
    if (!w) return;
    const sender = detail.from.map((a) => a.name || a.email).join(", ");
    const recipients = [...detail.to, ...(detail.cc || [])].map((a) => a.email).join(", ");
    // The print window executes scripts (window.onload=print), so the body
    // MUST go through the same sanitization as the reading pane: raw
    // html_body could carry inline handlers that would run same-origin.
    const body =
      (detail.html_body ? sanitizeMailHTML(detail.html_body) : "") ||
      `<pre style="white-space:pre-wrap;font-family:inherit">${escHtml(detail.text_body || "")}</pre>`;
    w.document.write(
      `<!doctype html><html><head><meta charset="utf-8"><title>${escHtml(detail.subject)}</title>` +
        `<style>body{font-family:system-ui,sans-serif;max-width:760px;margin:24px auto;padding:0 16px;color:#111}` +
        `h1{font-size:20px;line-height:1.4}header{color:#555;font-size:13px;margin-bottom:16px;padding-bottom:12px;border-bottom:1px solid #ddd}` +
        `img{max-width:100%}blockquote{border-left:3px solid #ccc;margin:8px 0;padding-left:12px;color:#555}</style>` +
        `</head><body><h1>${escHtml(detail.subject)}</h1><header>From: ${escHtml(sender)}<br>Date: ${escHtml(fmtFullDate(detail.date))}<br>To: ${escHtml(recipients)}</header>` +
        `<div>${body}</div><script>window.onload=()=>window.print()<\/script></body></html>`,
    );
    w.document.close();
  }

  // doQuickReplyFor sends the inline quick reply for a specific message —
  // the opened detail in single view, the newest member in thread view.
  async function doQuickReplyFor(target: MailMessage) {
    const text = quickReplyText.trim();
    if (!text) return;
    setQuickSending(true);
    try {
      const sender = target.from[0]?.email;
      if (!sender) return;
      const meLower = meEmail.toLowerCase();
      const recipients = new Set<string>();
      if (quickReplyAll) {
        [...target.from, ...(target.cc || []), ...target.to].forEach((a) => {
          if (a.email && a.email.toLowerCase() !== meLower) recipients.add(a.email);
        });
      } else {
        recipients.add(sender);
      }
      const subject = target.subject.startsWith("Re:") ? target.subject : `Re: ${target.subject}`;
      const ok = await onQuickReply(
        [...recipients],
        [],
        subject,
        text,
        target.id,
        target.id,
      );
      if (ok) setQuickReplyText("");
    } finally {
      setQuickSending(false);
    }
  }

  function doQuickReply() {
    return doQuickReplyFor(detail);
  }

  return (
    <main
      className="mail-scroll min-w-0 flex-1 overflow-y-auto bg-card"
      style={paneMax ? {maxWidth: paneMax, width: "100%", marginInline: "auto"} : undefined}
    >
      {quoteSel && (
        <Button
          size="xs"
          variant="outline"
          className="fixed z-50 shadow-md"
          style={{ left: quoteSel.x, top: quoteSel.y, transform: "translateX(-50%)" }}
          onClick={() => {
            onReplyWithQuote(quoteSel.text);
            setQuoteSel(null);
          }}
        >
          <Reply className="size-3" />
          {t("replyWithQuote")}
        </Button>
      )}
      {/* Subject header */}
      <MessageHeader
        detail={detail}
        folder={folder}
        isDraft={isDraft}
        starred={starred}
        muted={muted}
        snoozed={snoozed}
        labels={labels}
        labelColors={labelColors}
        aiEnabled={aiEnabled}
        highlightTerms={highlightTerms}
        translating={translating}
        showNotSpam={showNotSpam}
        onBack={onBack}
        onStar={onStar}
        onEditDraft={onEditDraft}
        onRecall={onRecall}
        onArchive={onArchive}
        onSpam={onSpam}
        onDelete={onDelete}
        onTranslate={doTranslate}
        onToggleMute={onToggleMute}
        onPrint={doPrint}
        onSnooze={onSnooze}
        onToggleLabel={onToggleLabel}
        snoozeOpen={snoozeOpen}
        setSnoozeOpen={setSnoozeOpen}
        snoozeCustom={snoozeCustom}
        setSnoozeCustom={setSnoozeCustom}
        labelOpen={labelOpen}
        setLabelOpen={setLabelOpen}
        newLabel={newLabel}
        setNewLabel={setNewLabel}
        labelMenuRef={labelMenuRef}
      />

      {/* Message content area */}
      <div className="px-4 py-4 md:px-6">
        {/* Conversation view: loading placeholder, full thread, or single */}
        {isConversation && !threadForDetail ? (
          <p className="flex items-center gap-1.5 py-4 text-sm text-muted-foreground">
            <Loader2 className="size-3.5 animate-spin" />
            {t("loading")}
          </p>
        ) : isThreadView ? (
          <div className="space-y-0">
            {threadMessages.map((msg, idx) => {
              // Conversation model: the inline reply box lives under the message
              // being answered (newest by default, the clicked member
              // otherwise) instead of an overlay that hides the thread.
              const quickTarget =
                quickReplyTargetUid === null
                  ? threadMessages[threadMessages.length - 1]
                  : threadMessages.find((m) => m.uid === quickReplyTargetUid);
              const isQuickTarget = quickReplyOpen && quickTarget?.uid === msg.uid;
              return (
              <ThreadMessage
                key={msg.uid}
                message={msg}
                isExpanded={expandedUid === msg.uid}
                loading={detailLoading && msg.uid === detail.uid}
                onToggleExpand={() => handleToggleMessage(msg.uid)}
                onClose={() => setExpandedUid(null)}
                detailsOpen={detailsOpen}
                onToggleDetails={() => setDetailsOpen((v) => !v)}
                expandedQuotes={expandedQuotes}
                onToggleQuote={toggleQuote}
                highlightTerms={highlightTerms}
                remoteLoaded={remoteLoaded}
                onReply={() => {
                  setQuickReplyTargetUid(msg.uid);
                  setQuickReplyAll(false);
                  setQuickReplyOpen(true);
                }}
                onReplyAll={() => {
                  setQuickReplyTargetUid(msg.uid);
                  setQuickReplyAll(true);
                  setQuickReplyOpen(true);
                }}
                onForward={() => onForwardThread(msg)}
                actionRowExtra={
                  idx === threadMessages.length - 1 ? (
                    <>
                      {/* Thread-level controls, unified with the single-message
                          view, inline after the forward button of the newest
                          member: quick reply (to this member) and the AI
                          summary (over the whole conversation). */}
                      <Button
                        size="sm"
                        variant={isQuickTarget ? "secondary" : "outline"}
                        onClick={() => {
                          // The thread-level button always resets to the
                          // newest member; per-message 回复 retargets above.
                          setQuickReplyTargetUid(null);
                          setQuickReplyOpen((v) => !v);
                        }}
                      >
                        <MessageSquarePlus className="size-3.5" />
                        {t("quickReply")}
                      </Button>
                      {aiEnabled && (
                        <Button
                          size="sm"
                          variant={summaryOpen || summarizing || summary ? "secondary" : "outline"}
                          onClick={() => {
                            if (summaryOpen) {
                              setSummaryOpen(false);
                            } else {
                              setSummaryOpen(true);
                              if (!summary) onSummarize(threadText);
                            }
                          }}
                          disabled={summarizing}
                        >
                          <Sparkles className="size-3.5" />
                          {summarizing ? t("summarizing") : summary ? t("aiSummary") : t("summarize")}
                        </Button>
                      )}
                    </>
                  ) : undefined
                }
                belowActions={
                  isQuickTarget && quickTarget ? (
                    <QuickReply
                      replies={replies}
                      setQuickReplyText={setQuickReplyText}
                      quickReplyAll={quickReplyAll}
                      setQuickReplyAll={setQuickReplyAll}
                      aiEnabled={aiEnabled}
                      onLoadReplies={() => loadReplies(quickTarget.text_body)}
                      repliesLoading={repliesLoading}
                      setQuickReplyOpen={setQuickReplyOpen}
                      quickReplyText={quickReplyText}
                      onSend={() => doQuickReplyFor(quickTarget)}
                      quickSending={quickSending}
                      onPopOut={() => onReplyThread(quickTarget)}
                    />
                  ) : undefined
                }
              />
              );
            })}
          </div>
        ) : (
          /* Single message view (no thread) */
          <div>
            {/* Sender header */}
            <MessageMeta
              detail={detail}
              senderName={senderName}
              sender={sender}
              detailsOpen={detailsOpen}
              setDetailsOpen={setDetailsOpen}
            />

            {/* Meeting invitation */}
            {detail.invitation && detail.invitation.method === "REQUEST" && (
              <InvitationBanner invitation={detail.invitation} />
            )}

            {/* Read-receipt request (RFC 3798) */}
            <ReceiptBanner
              detail={detail}
              folder={folder}
              receiptBusy={receiptBusy}
              setReceiptBusy={setReceiptBusy}
              onSendReceipt={onSendReceipt}
            />

            {/* Recall notice (X-MS-Recall) */}
            <RecallNotice
              detail={detail}
              folder={folder}
              recallBusy={recallBusy}
              setRecallBusy={setRecallBusy}
              recallHandled={recallHandled}
              setRecallHandled={setRecallHandled}
              onApplyRecall={onApplyRecall}
            />

            {/* Expanded details */}
            {detailsOpen && (
              <MessageDetails detail={detail} sender={sender} />
            )}

            {/* Action buttons */}
            <MessageActions
              showNotSpam={showNotSpam}
              onNotSpam={onNotSpam}
              detail={detail}
              onUnsubscribe={onUnsubscribe}
              onReply={onReply}
              onReplyAll={onReplyAll}
              onForward={onForward}
              quickReplyOpen={quickReplyOpen}
              setQuickReplyOpen={setQuickReplyOpen}
              aiEnabled={aiEnabled}
              aiLocked={aiLocked}
              summaryOpen={summaryOpen}
              setSummaryOpen={setSummaryOpen}
              summarizing={summarizing}
              summary={summary}
              onSummarize={onSummarize}
              onLoadRaw={loadRaw}
              onDownloadRaw={downloadRaw}
              folders={folders}
              onMoveToFolder={onMoveToFolder}
            />

            {/* Inline quick reply: only rendered when the action-bar button
                toggles it open. */}
            {quickReplyOpen && (
              <QuickReply
                replies={replies}
                setQuickReplyText={setQuickReplyText}
                quickReplyAll={quickReplyAll}
                setQuickReplyAll={setQuickReplyAll}
                aiEnabled={aiEnabled}
                onLoadReplies={() => loadReplies()}
                repliesLoading={repliesLoading}
                setQuickReplyOpen={setQuickReplyOpen}
                quickReplyText={quickReplyText}
                onSend={doQuickReply}
                quickSending={quickSending}
              />
            )}

            {/* PGP encrypted message */}
            <PgpBanner
              isPgpEncrypted={isPgpEncrypted}
              pgpPlaintext={pgpPlaintext}
              onDecrypt={decryptPgp}
              decrypting={decrypting}
              pgpError={pgpError}
            />

            {/* Remote image policy */}
            <RemoteImageBanner
              remoteImages={remoteImages}
              remoteLoaded={remoteLoaded}
              setRemoteLoaded={setRemoteLoaded}
              senderDomain={senderDomain}
            />

            {/* Translation panel */}
            <TranslationPanel
              translatedView={translatedView}
              translation={translation}
              setTranslatedView={setTranslatedView}
            />

            {/* Burn-after-read gate: reveal once, then flag $BurnRead */}
            <BurnGate
              detail={detail}
              burnRevealed={burnRevealed}
              onReveal={revealBurn}
            />

            {/* Message body: HTML preferred, plain text as fallback */}
            <MessageBody
              mounted={mounted}
              htmlBody={htmlBody}
              segments={segments}
              detailLoading={detailLoading}
              fontPx={fontPx}
              burnRevealed={burnRevealed}
              detail={detail}
              meEmail={meEmail}
              highlightTerms={highlightTerms}
              expandedQuotes={expandedQuotes}
              onToggleQuote={toggleQuote}
              bodyRef={bodyRef}
            />

            {/* Attachments */}
            <AttachmentList detail={detail} onDownloadAll={downloadAllAttachments} />
          </div>
        )}

        {/* AI summary: only rendered when the action-bar button toggles it. */}
        {aiEnabled && (summaryOpen || summarizing || summary) && (
          <AiSummary
            setSummaryOpen={setSummaryOpen}
            summarizing={summarizing}
            summary={summary}
            summaryCollapsed={summaryCollapsed}
            setSummaryCollapsed={setSummaryCollapsed}
            feedback={feedback}
            setFeedback={setFeedback}
            onSummarize={onSummarize}
          />
        )}
      </div>

      {/* Raw message dialog */}
      <Dialog open={rawOpen} onOpenChange={setRawOpen}>
        <DialogContent className="max-h-[85vh] sm:max-w-3xl">
          <DialogHeader><DialogTitle>{t("viewRaw")}</DialogTitle></DialogHeader>
          {rawLoading && <p className="text-sm text-muted-foreground">{t("loading")}</p>}
          {rawError && <p className="text-sm text-destructive">{rawError}</p>}
          {!rawLoading && !rawError && (
            <pre className="mail-scroll max-h-[65vh] overflow-auto rounded-lg border border-border bg-muted/40 p-3 font-mono text-[11px] leading-4 break-all whitespace-pre-wrap">
              {rawText}
            </pre>
          )}
        </DialogContent>
      </Dialog>
    </main>
  );
}
