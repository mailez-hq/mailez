"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import {
  Archive,
  ArrowLeft,
  Ban,
  CalendarDays,
  ChevronDown,
  ChevronUp,
  Download,
  FileCode,
  Info,
  Languages,
  Loader2,
  Lock,
  MailCheck,
  MailWarning,
  Printer,
  RotateCw,
  Undo2,
  Volume2,
  VolumeX,
  Reply,
  ReplyAll,
  Send,
  ShieldCheck,
  Sparkles,
  Star,
  Tag,
  ThumbsDown,
  ThumbsUp,
  Trash2,
  X,
} from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { aiTranslate, inviteRespond, mailRaw, pgpDecrypt } from "@/lib/api";
import type { MailAttachment, MailInvitation, MailMessage, MailThread } from "@/lib/api";
import { AttachmentCard } from "@/components/mailbox/reader/attachment-card";
import { QuoteBlock } from "@/components/mailbox/reader/quote-block";
import { useMounted } from "@/components/mailbox/reader/use-mounted";
import { escHtml, labelColor } from "@/components/mailbox/mail-utils";
import { buildFolderTree, flattenTree, folderLabel } from "@/components/mailbox/folder-tree";
import type { ReaderFontSize, ReadingPaneWidth } from "@/lib/preferences";
import {
  blockRemoteImages, hasRemoteImages, rememberedRemoteSenders, rememberRemoteSender,
} from "@/components/mailbox/reader/remote-images";
import {
  fmtFullDate, fmtShort, getSnippet, parseBody, type Segment,
} from "@/components/mailbox/reader/body";
import { Highlight } from "@/components/mailbox/highlight";
import { sanitizeMailHTML } from "@/lib/sanitize";
import { cn } from "@/lib/utils";

// isSentFolder reports whether a mailbox path is the Sent Items folder.
function isSentFolder(name: string): boolean {
  return ["sent", "sent items", "sentitems", "已发送"].includes(name.toLowerCase().trim());
}

function ThreadMessage({
  message,
  isExpanded,
  loading,
  onToggleExpand,
  onClose,
  detailsOpen,
  onToggleDetails,
  expandedQuotes,
  onToggleQuote,
  highlightTerms,
  remoteLoaded,
  onShowActions,
}: {
  message: MailMessage;
  isExpanded: boolean;
  loading?: boolean;
  onToggleExpand: () => void;
  onClose: () => void;
  detailsOpen: boolean;
  onToggleDetails: () => void;
  expandedQuotes: Set<number>;
  onToggleQuote: (i: number) => void;
  highlightTerms?: string[];
  remoteLoaded: boolean;
  onShowActions: () => void;
}) {
  const t = useTranslations("mail");
  const mounted = useMounted();
  const sender = message.from[0];
  const senderName = sender?.name || sender?.email || "?";

  const segments = useMemo(
    () => parseBody(message.text_body || ""),
    [message.text_body],
  );
  const remoteImages = useMemo(
    () => hasRemoteImages(message.html_body),
    [message.html_body],
  );
  const htmlBody = useMemo(() => {
    if (!message.html_body) return "";
    const clean = sanitizeMailHTML(message.html_body);
    return remoteImages && !remoteLoaded
      ? blockRemoteImages(clean)
      : clean;
  }, [message.html_body, remoteImages, remoteLoaded]);

  if (!isExpanded) {
    return (
      <button
        onClick={onToggleExpand}
        className="flex w-full items-start gap-3 border-b border-border py-3 text-left transition-colors hover:bg-muted/40 last:border-b-0"
      >
        <span
          className={cn(
            "mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-full text-xs font-semibold",
            "bg-accent/60 text-accent-foreground",
          )}
        >
          {senderName.charAt(0).toUpperCase()}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-baseline gap-2">
            <span className="truncate text-sm font-medium">{senderName}</span>
            <span className="shrink-0 text-[11px] text-muted-foreground">
              {fmtShort(message.date)}
            </span>
          </div>
          <p className="mt-0.5 line-clamp-2 text-xs leading-relaxed text-muted-foreground">
            <Highlight text={getSnippet(message.text_body || "")} terms={highlightTerms} />
          </p>
          {message.attachments && message.attachments.length > 0 && (
            <div className="mt-1 flex items-center gap-1 text-[11px] text-muted-foreground">
              <span>📎</span>
              <span>
                {t("attachments", { count: message.attachments.length })}
              </span>
            </div>
          )}
        </div>
        <ChevronDown className="mt-1 size-3.5 shrink-0 text-muted-foreground" />
      </button>
    );
  }

  return (
    <div className="border-b border-border py-3 last:border-b-0">
      {/* Sender header */}
      <div className="mb-3 flex items-start gap-3">
        <span
          className={cn(
            "mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-full text-xs font-semibold",
            "bg-accent text-accent-foreground",
          )}
        >
          {senderName.charAt(0).toUpperCase()}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-baseline justify-between gap-2">
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">{senderName}</p>
              <button
                onClick={onToggleDetails}
                className="flex items-center gap-0.5 text-xs text-muted-foreground hover:text-foreground"
              >
                {sender?.email}
                {detailsOpen ? (
                  <ChevronUp className="size-3" />
                ) : (
                  <ChevronDown className="size-3" />
                )}
              </button>
            </div>
            <span className="shrink-0 text-xs text-muted-foreground">
              {fmtFullDate(message.date)}
            </span>
          </div>
        </div>
        <Button
          size="sm"
          variant="ghost"
          onClick={onClose}
          className="size-7"
        >
          <X className="size-3.5" />
        </Button>
      </div>

      {message.invitation && message.invitation.method === "REQUEST" && (
        <InvitationBanner invitation={message.invitation} />
      )}

      {/* Expanded details */}
      {detailsOpen && (
        <div className="mb-3 space-y-0.5 rounded-lg border border-border bg-muted/40 p-3 text-xs text-muted-foreground">
          <p>
            <span className="font-medium text-foreground/80">From: </span>
            {sender?.name ? `${sender.name} <${sender.email}>` : sender?.email}
          </p>
          <p>
            <span className="font-medium text-foreground/80">To: </span>
            {message.to.map((a) => a.name || a.email).join(", ") || "—"}
          </p>
          {message.cc && message.cc.length > 0 && (
            <p>
              <span className="font-medium text-foreground/80">Cc: </span>
              {message.cc.map((a) => a.name || a.email).join(", ")}
            </p>
          )}
          <p>
            <span className="font-medium text-foreground/80">Date: </span>
            {fmtFullDate(message.date)}
          </p>
        </div>
      )}

      {/* Message body: HTML preferred, plain text as fallback */}
      <div className="mail-body">
        {mounted && htmlBody ? (
          <div dangerouslySetInnerHTML={{ __html: htmlBody }} />
        ) : segments.length === 0 && loading ? (
          <p className="flex items-center gap-1.5 text-muted-foreground">
            <Loader2 className="size-3.5 animate-spin" />
            {t("loading")}
          </p>
        ) : segments.length === 0 ? (
          <p className="text-muted-foreground">{t("noTextBody")}</p>
        ) : (
          segments.map((seg, i) =>
            seg.type === "p" ? (
              <p key={i} className="text-sm leading-6">
                {seg.lines.map((l, j) => (
                  <span key={j}>
                    <Highlight text={l} terms={highlightTerms} />
                    {j < seg.lines.length - 1 && <br />}
                  </span>
                ))}
              </p>
            ) : (
              <QuoteBlock
                key={i}
                lines={seg.lines}
                expanded={expandedQuotes.has(i)}
                onToggle={() => onToggleQuote(i)}
              />
            ),
          )
        )}
      </div>

      {/* Attachments */}
      {message.attachments && message.attachments.length > 0 && (
        <div className="mt-4 border-t border-border pt-3">
          <p className="mb-2 text-xs font-medium text-muted-foreground">
            {t("attachments", { count: message.attachments.length })}
          </p>
          <div className="flex flex-wrap gap-2">
            {message.attachments.map((a, i) => (
              <AttachmentCard key={i} attachment={a} />
            ))}
          </div>
        </div>
      )}

      {/* Action buttons below message */}
      <div className="mt-4 flex flex-wrap items-center gap-2 border-t border-border pt-3">
        <Button size="sm" variant="outline" onClick={onShowActions}>
          <Reply className="size-3.5" />
          {t("reply")}
        </Button>
        <Button size="sm" variant="outline" onClick={onShowActions}>
          <ReplyAll className="size-3.5" />
          {t("replyAll")}
        </Button>
        <Button size="sm" variant="outline" onClick={onShowActions}>
          <Send className="size-3.5" />
          {t("forward")}
        </Button>
      </div>
    </div>
  );
}

// InvitationBanner shows the meeting details of a received REQUEST and lets
// the attendee accept / tentatively accept / decline (sends an iTIP REPLY).
function InvitationBanner({ invitation }: { invitation: MailInvitation }) {
  const t = useTranslations("mail");
  const [busy, setBusy] = useState<"" | "accept" | "tentative" | "decline">("");
  const [done, setDone] = useState("");
  const [error, setError] = useState("");

  async function respond(action: "accept" | "decline" | "tentative") {
    setBusy(action);
    setError("");
    setDone("");
    try {
      await inviteRespond(invitation.ics, action);
      setDone(action === "accept" ? t("inviteAccepted") : action === "tentative" ? t("inviteTentative") : t("inviteDeclined"));
    } catch (e) {
      setError(e instanceof Error ? e.message : t("inviteFailed"));
    } finally {
      setBusy("");
    }
  }

  const fmtWhen = () => {
    if (!invitation.start) return "";
    const s = new Date(invitation.start);
    if (Number.isNaN(s.getTime())) return invitation.start;
    if (!invitation.end) return s.toLocaleString();
    const e = new Date(invitation.end);
    if (Number.isNaN(e.getTime())) return s.toLocaleString();
    return `${s.toLocaleString()} – ${e.toLocaleString()}`;
  };

  return (
    <div className="mb-3 rounded-lg border border-border bg-accent/40 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <CalendarDays className="size-4 shrink-0 text-primary" />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium">{invitation.summary || t("inviteMeeting")}</p>
          <p className="truncate text-xs text-muted-foreground">
            {fmtWhen()}
            {invitation.location ? ` · ${invitation.location}` : ""}
          </p>
          {invitation.organizer && (
            <p className="truncate text-xs text-muted-foreground">
              {t("inviteOrganizer")}: {invitation.organizer}
            </p>
          )}
        </div>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-2">
        <Button size="sm" onClick={() => respond("accept")} disabled={!!busy || !!done}>
          {busy === "accept" ? <Loader2 className="size-3.5 animate-spin" /> : null}
          {t("inviteAccept")}
        </Button>
        <Button size="sm" variant="outline" onClick={() => respond("tentative")} disabled={!!busy || !!done}>
          {busy === "tentative" ? <Loader2 className="size-3.5 animate-spin" /> : null}
          {t("inviteTentative")}
        </Button>
        <Button size="sm" variant="outline" onClick={() => respond("decline")} disabled={!!busy || !!done} className="text-destructive hover:text-destructive">
          {busy === "decline" ? <Loader2 className="size-3.5 animate-spin" /> : null}
          {t("inviteDecline")}
        </Button>
        {done && <span className="text-xs text-emerald-600">{done}</span>}
        {error && <span className="text-xs text-destructive">{error}</span>}
      </div>
    </div>
  );
}

export function ReadingPane({
  detail,
  detailLoading,
  aiEnabled,
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
  threadOpen,
  threadLoading,
  onToggleThread,
  onSelectThread,
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
}: {
  detail: MailMessage;
  detailLoading: boolean;
  aiEnabled: boolean;
  summary: string;
  summarizing: boolean;
  onSummarize: () => void;
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
  onToggleThread: () => void;
  onSelectThread: (uid: number) => void;
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
}) {
  const t = useTranslations("mail");
  const fontPx = readerFont === "sm" ? 13 : readerFont === "lg" ? 16 : readerFont === "xl" ? 18 : 14;
  const paneMax = paneWidth === "narrow" ? 720 : paneWidth === "wide" ? 1100 : 0;
  const mounted = useMounted();
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [expandedQuotes, setExpandedQuotes] = useState<Set<number>>(new Set());
  const [summaryCollapsed, setSummaryCollapsed] = useState(false);
  const [feedback, setFeedback] = useState<"up" | "down" | null>(null);
  const [receiptBusy, setReceiptBusy] = useState(false);
  const [recallBusy, setRecallBusy] = useState(false);
  const [recallHandled, setRecallHandled] = useState(false);
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
  const [quickReplyText, setQuickReplyText] = useState("");
  const [quickReplyAll, setQuickReplyAll] = useState(false);
  const [quickSending, setQuickSending] = useState(false);
  const [translation, setTranslation] = useState("");
  const [translating, setTranslating] = useState(false);
  const [translatedView, setTranslatedView] = useState(false);

  // Track which thread message is expanded
  const [expandedUid, setExpandedUid] = useState<number | null>(detail.uid);

  // Reset transient panel state when a different mail is opened. The panel is
  // kept mounted (no key remount) so switching mails doesn't flash empty
  // content behind it; we only clear the per-message state here.
  useEffect(() => {
    const domain = (detail.from[0]?.email || "").split("@").pop() || "";
    setExpandedUid(detail.uid);
    setDetailsOpen(false);
    setExpandedQuotes(new Set());
    setSummaryCollapsed(false);
    setFeedback(null);
    setPgpPlaintext(null);
    setPgpError("");
    setRawText("");
    setRemoteLoaded(rememberedRemoteSenders().includes(domain));
  }, [detail.uid, detail.from]);

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

  const senderDomain = (detail.from[0]?.email || "").split("@").pop() || "";
  const [remoteLoaded, setRemoteLoaded] = useState(() =>
    rememberedRemoteSenders().includes(senderDomain),
  );

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
    return remoteImages && !remoteLoaded
      ? blockRemoteImages(clean)
      : clean;
  }, [detail.html_body, remoteImages, remoteLoaded, pgpPlaintext, isPgpEncrypted]);

  const starred = detail.flags.includes("\\Flagged");
  const sender = detail.from[0];
  const senderName = sender?.name || sender?.email || "?";

  // Determine thread messages to display
  const threadMessages = thread?.messages ?? [detail];
  const isThreadView = threadMessages.length > 1;

  function handleToggleMessage(uid: number) {
    setExpandedUid((prev) => (prev === uid ? null : uid));
  }

  function handleSelectThreadMessage(uid: number) {
    onSelectThread(uid);
    setExpandedUid(uid);
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

  function doPrint() {
    const w = window.open("", "_blank");
    if (!w) return;
    const sender = detail.from.map((a) => a.name || a.email).join(", ");
    const recipients = [...detail.to, ...(detail.cc || [])].map((a) => a.email).join(", ");
    const body =
      detail.html_body ||
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

  async function doQuickReply() {
    const text = quickReplyText.trim();
    if (!text) return;
    setQuickSending(true);
    try {
      const sender = detail.from[0]?.email;
      if (!sender) return;
      const meLower = meEmail.toLowerCase();
      const recipients = new Set<string>();
      if (quickReplyAll) {
        [...detail.from, ...(detail.cc || []), ...detail.to].forEach((a) => {
          if (a.email && a.email.toLowerCase() !== meLower) recipients.add(a.email);
        });
      } else {
        recipients.add(sender);
      }
      const subject = detail.subject.startsWith("Re:") ? detail.subject : `Re: ${detail.subject}`;
      const ok = await onQuickReply(
        [...recipients],
        [],
        subject,
        text,
        detail.id,
        detail.id,
      );
      if (ok) setQuickReplyText("");
    } finally {
      setQuickSending(false);
    }
  }

  return (
    <main
      className="mail-scroll min-w-0 flex-1 overflow-y-auto bg-card"
      style={paneMax ? {maxWidth: paneMax, width: "100%", marginInline: "auto"} : undefined}
    >
      {/* Subject header */}
      <div className="sticky top-0 z-10 border-b border-border bg-card/95 px-4 py-3 backdrop-blur-sm md:px-6">
        {/* Mobile back */}
        <div className="mb-1.5 md:hidden">
          <Button variant="ghost" size="sm" onClick={onBack}>
            <ArrowLeft className="size-4" />
            {t("back")}
          </Button>
        </div>

        <div className="flex items-start gap-2">
          <h1 className="min-w-0 flex-1 break-words text-lg font-semibold tracking-tight md:text-xl">
            {detail.subject ? (
              <Highlight text={detail.subject} terms={highlightTerms} />
            ) : (
              t("noSubject")
            )}
          </h1>
          <div className="flex shrink-0 items-center gap-1">
            <Button
              size="sm"
              variant="ghost"
              onClick={onStar}
              title={starred ? t("unstar") : t("star")}
              className={cn("size-8", starred && "text-[#C9A227]")}
            >
              <Star className={cn("size-4", starred && "fill-current")} />
            </Button>
            {isSentFolder(folder) && (
              <Button
                size="sm"
                variant="ghost"
                onClick={() => onRecall(detail.folder || folder, detail.uid)}
                title={t("recall")}
                className="size-8 text-muted-foreground hover:text-foreground"
              >
                <Undo2 className="size-4" />
              </Button>
            )}
            <Button
              size="sm"
              variant="ghost"
              onClick={onArchive}
              title={t("archive")}
              className="size-8"
            >
              <Archive className="size-4" />
            </Button>
            {!showNotSpam && (
              <Button
                size="sm"
                variant="ghost"
                onClick={onSpam}
                title={t("spam")}
                className="size-8 text-muted-foreground hover:text-destructive"
              >
                <Ban className="size-4" />
              </Button>
            )}
            <Button
              size="sm"
              variant="ghost"
              onClick={onDelete}
              className="size-8 text-muted-foreground hover:text-destructive"
            >
              <Trash2 className="size-4" />
            </Button>
            {aiEnabled && (
              <Button
                size="sm"
                variant="ghost"
                onClick={doTranslate}
                title={t("translate")}
                className="size-8 text-muted-foreground"
              >
                {translating ? <Loader2 className="size-4 animate-spin" /> : <Languages className="size-4" />}
              </Button>
            )}
            <Button
              size="sm"
              variant="ghost"
              onClick={onToggleMute}
              title={muted ? t("unmute") : t("mute")}
              className={cn("size-8", muted && "text-primary")}
            >
              {muted ? <VolumeX className="size-4" /> : <Volume2 className="size-4" />}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={doPrint}
              title={t("print")}
              className="size-8 text-muted-foreground"
            >
              <Printer className="size-4" />
            </Button>
          </div>
        </div>

        {/* Labels / tags */}
        <div className="mt-1.5 flex flex-wrap items-center gap-1">
          {muted && (
            <span className="inline-flex items-center rounded-full border border-border bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">
              {t("mutedBadge")}
            </span>
          )}
          {labels
            .filter((l) => detail.flags.includes(l))
            .map((l) => {
              const color = labelColor(l, labelColors?.[l]);
              return (
                <span
                  key={l}
                  className="inline-flex items-center rounded-full border px-2 py-0.5 text-[11px] font-medium"
                  style={{
                    backgroundColor: `${color}1f`,
                    borderColor: `${color}59`,
                    color,
                  }}
                >
                  {l}
                </span>
              );
            })}
          <div className="relative" ref={labelMenuRef}>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setLabelOpen((v) => !v)}
              title={t("labels")}
              className="h-5 px-1.5 text-[11px]"
            >
              <Tag className="size-3" />
            </Button>
            {labelOpen && (
              <div className="absolute top-full left-0 z-20 mt-1 w-52 rounded-lg border border-border bg-popover p-2 shadow-lg">
                <div className="mb-2 flex flex-wrap gap-1">
                  {labels.map((l) => {
                    const has = detail.flags.includes(l);
                    return (
                      <button
                        key={l}
                        type="button"
                        onClick={() => onToggleLabel(l)}
                        className={cn(
                          "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] transition-colors",
                          has
                            ? "border-transparent font-medium"
                            : "border-border text-muted-foreground hover:bg-muted",
                        )}
                        style={
                          has
                            ? {
                                backgroundColor: `${labelColor(l, labelColors?.[l])}1f`,
                                color: labelColor(l, labelColors?.[l]),
                              }
                            : undefined
                        }
                      >
                        <span
                          className="size-1.5 rounded-full"
                          style={{ backgroundColor: labelColor(l, labelColors?.[l]) }}
                        />
                        {l}
                      </button>
                    );
                  })}
                </div>
                <div className="flex gap-1">
                  <Input
                    value={newLabel}
                    onChange={(e) => setNewLabel(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && newLabel.trim()) {
                        onToggleLabel(newLabel.trim());
                        setNewLabel("");
                        setLabelOpen(false);
                      }
                    }}
                    placeholder={t("newLabel")}
                    className="h-7 text-xs"
                  />
                  <Button
                    size="xs"
                    onClick={() => {
                      if (newLabel.trim()) {
                        onToggleLabel(newLabel.trim());
                        setNewLabel("");
                        setLabelOpen(false);
                      }
                    }}
                  >
                    +
                  </Button>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Message content area */}
      <div className="px-4 py-4 md:px-6">
        {/* Thread view */}
        {isThreadView ? (
          <div className="space-y-0">
            {threadMessages.map((msg) => (
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
                onShowActions={() => {
                  // When clicking reply on a thread message, select it first
                  handleSelectThreadMessage(msg.uid);
                }}
              />
            ))}
          </div>
        ) : (
          /* Single message view (no thread) */
          <div>
            {/* Sender header */}
            <div className="mb-4 flex items-start gap-3">
              <span
                className={cn(
                  "mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-full text-xs font-semibold",
                  "bg-accent text-accent-foreground",
                )}
              >
                {senderName.charAt(0).toUpperCase()}
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex items-baseline justify-between gap-2">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{senderName}</p>
                    <button
                      onClick={() => setDetailsOpen((v) => !v)}
                      className="flex items-center gap-0.5 text-xs text-muted-foreground hover:text-foreground"
                    >
                      {sender?.email}
                      {detailsOpen ? (
                        <ChevronUp className="size-3" />
                      ) : (
                        <ChevronDown className="size-3" />
                      )}
                    </button>
                  </div>
                  <span className="shrink-0 text-xs text-muted-foreground">
                    {fmtFullDate(detail.date)}
                  </span>
                </div>
            </div>
          </div>

            {/* Meeting invitation */}
            {detail.invitation && detail.invitation.method === "REQUEST" && (
              <InvitationBanner invitation={detail.invitation} />
            )}

            {/* Read-receipt request (RFC 3798) */}
            {detail.receipt_requested && !detail.flags.includes("$MDNSent") && (
              <div className="mb-3 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/40 p-3 text-xs">
                <MailCheck className="size-4 text-muted-foreground" />
                <span className="flex-1">{t("receiptRequested")}</span>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={receiptBusy}
                  onClick={async () => {
                    setReceiptBusy(true);
                    await onSendReceipt(detail.folder || folder, detail.uid);
                    setReceiptBusy(false);
                  }}
                >
                  {receiptBusy ? <Loader2 className="size-3.5 animate-spin" /> : null}
                  {t("sendReceipt")}
                </Button>
              </div>
            )}

            {/* Recall notice (Outlook-style X-MS-Recall) */}
            {detail.recall && !recallHandled && (
              <div className="mb-3 flex flex-wrap items-center gap-2 rounded-lg border border-amber-300/60 bg-amber-50 p-3 text-xs dark:border-amber-700/50 dark:bg-amber-950/30">
                <MailWarning className="size-4 text-amber-600" />
                <span className="flex-1">
                  {t("recallNotice", { subject: detail.recall.subject })}
                </span>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={recallBusy}
                  onClick={async () => {
                    setRecallBusy(true);
                    const ok = await onApplyRecall(detail.recall!.message_id, detail.folder || folder, detail.uid);
                    if (ok) setRecallHandled(true);
                    setRecallBusy(false);
                  }}
                >
                  {recallBusy ? <Loader2 className="size-3.5 animate-spin" /> : null}
                  {t("recallDeleteOriginal")}
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setRecallHandled(true)}>
                  {t("recallDismiss")}
                </Button>
              </div>
            )}

            {/* Expanded details */}
            {detailsOpen && (
              <div className="mb-4 space-y-0.5 rounded-lg border border-border bg-muted/40 p-3 text-xs text-muted-foreground">
                <p>
                  <span className="font-medium text-foreground/80">From: </span>
                  {sender?.name ? `${sender.name} <${sender.email}>` : sender?.email}
                </p>
                <p>
                  <span className="font-medium text-foreground/80">To: </span>
                  {detail.to.map((a) => a.name || a.email).join(", ") || "—"}
                </p>
                {detail.cc && detail.cc.length > 0 && (
                  <p>
                    <span className="font-medium text-foreground/80">Cc: </span>
                    {detail.cc.map((a) => a.name || a.email).join(", ")}
                  </p>
                )}
                <p>
                  <span className="font-medium text-foreground/80">Date: </span>
                  {fmtFullDate(detail.date)}
                </p>
              </div>
            )}

            {/* Action buttons */}
            <div className="mb-4 flex flex-wrap items-center gap-2">
              {showNotSpam && (
                <Button size="sm" variant="outline" onClick={onNotSpam}>
                  <ShieldCheck className="size-3.5" />
                  {t("notSpam")}
                </Button>
              )}
              {detail.unsubscribe_url && (
                <Button size="sm" variant="outline" onClick={onUnsubscribe} title={detail.unsubscribe_url}>
                  <X className="size-3.5" />
                  {t("unsubscribe")}
                </Button>
              )}
              <Button size="sm" variant="outline" onClick={onReply}>
                <Reply className="size-3.5" />
                {t("reply")}
              </Button>
              <Button size="sm" variant="outline" onClick={onReplyAll}>
                <ReplyAll className="size-3.5" />
                {t("replyAll")}
              </Button>
              <Button size="sm" variant="outline" onClick={onForward}>
                <Send className="size-3.5" />
                {t("forward")}
              </Button>
              <div className="ml-auto flex items-center gap-1">
                <Button size="sm" variant="ghost" onClick={loadRaw} title={t("viewRaw")}>
                  <FileCode className="size-3.5" />
                </Button>
                <Button size="sm" variant="ghost" onClick={downloadRaw} title={t("downloadEml")}>
                  <Download className="size-3.5" />
                </Button>
                <select
                  value=""
                  onChange={(e) => {
                    if (e.target.value) onMoveToFolder(e.target.value);
                  }}
                  title={t("moveTo")}
                  className="h-7 rounded-md border border-border bg-transparent px-1 text-xs text-muted-foreground outline-none focus-visible:border-ring"
                >
                  <option value="">{t("moveTo")}…</option>
                  {flattenTree(buildFolderTree(folders), (leaf) => folderLabel(t, leaf))
                    .filter((o) => !/^(trash|drafts)$/i.test(o.value))
                    .map((o) => (
                      <option key={o.value} value={o.value}>
                        {o.label}
                      </option>
                    ))}
                </select>
              </div>
            </div>

            {/* Inline quick reply */}
            <div className="mt-4 rounded-lg border border-border p-3">
              <div className="mb-2 flex items-center gap-2">
                <p className="text-xs font-medium text-muted-foreground">{t("quickReply")}</p>
                <button
                  type="button"
                  onClick={() => setQuickReplyAll((v) => !v)}
                  className={cn(
                    "rounded-full border px-2 py-0.5 text-[11px] transition-colors",
                    quickReplyAll
                      ? "border-primary bg-accent font-medium text-accent-foreground"
                      : "border-border text-muted-foreground hover:bg-muted",
                  )}
                >
                  {quickReplyAll ? t("replyAll") : t("reply")}
                </button>
              </div>
              <textarea
                value={quickReplyText}
                onChange={(e) => setQuickReplyText(e.target.value)}
                placeholder={t("quickReply")}
                rows={3}
                className="w-full resize-y rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
              <div className="mt-2 flex justify-end">
                <Button size="sm" onClick={doQuickReply} disabled={!quickReplyText.trim() || quickSending}>
                  {quickSending ? <Loader2 className="size-3.5 animate-spin" /> : <Send className="size-3.5" />}
                  {t("send")}
                </Button>
              </div>
            </div>

            {/* PGP encrypted message */}
            {isPgpEncrypted && (
              <div className="mb-4 flex flex-wrap items-center gap-2 rounded-lg border border-ai/30 bg-ai/10 px-3 py-2 text-xs text-ai">
                <Lock className="size-3.5" />
                {pgpPlaintext !== null ? (
                  <span>{t("pgpDecrypted")}</span>
                ) : (
                  <>
                    <span>{t("pgpEncrypted")}</span>
                    <Button size="xs" variant="outline" onClick={decryptPgp} disabled={decrypting}>
                      {decrypting ? (
                        <Loader2 className="size-3 animate-spin" />
                      ) : (
                        <Lock className="size-3" />
                      )}
                      {decrypting ? t("pgpDecrypting") : t("pgpDecrypt")}
                    </Button>
                  </>
                )}
                {pgpError && <span className="text-destructive">{pgpError}</span>}
              </div>
            )}

            {/* Remote image policy */}
            {remoteImages && !remoteLoaded && (
              <div className="mb-4 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/50 px-3 py-2 text-xs text-muted-foreground">
                <span>{t("remoteImages")}</span>
                <Button size="xs" variant="outline" onClick={() => setRemoteLoaded(true)}>
                  {t("loadRemoteImages")}
                </Button>
                <Button
                  size="xs"
                  variant="outline"
                  onClick={() => {
                    rememberRemoteSender(senderDomain);
                    setRemoteLoaded(true);
                  }}
                >
                  {t("alwaysLoadRemote")}
                </Button>
              </div>
            )}

            {/* Translation panel */}
            {translatedView && translation && (
              <div className="mb-4 rounded-lg border border-border p-3">
                <div className="mb-1.5 flex items-center justify-between">
                  <p className="text-xs font-medium text-muted-foreground">{t("translate")}</p>
                  <Button size="xs" variant="ghost" onClick={() => setTranslatedView(false)}>
                    {t("showOriginal")}
                  </Button>
                </div>
                <div className="whitespace-pre-wrap text-sm leading-6">{translation}</div>
              </div>
            )}

            {/* Message body: HTML preferred, plain text as fallback */}
            <div className="mail-body">
              {mounted && htmlBody ? (
                <div dangerouslySetInnerHTML={{ __html: htmlBody }} style={{fontSize: fontPx}} />
              ) : segments.length === 0 && detailLoading ? (
                <p className="flex items-center gap-1.5 text-muted-foreground">
                  <Loader2 className="size-3.5 animate-spin" />
                  {t("loading")}
                </p>
              ) : segments.length === 0 ? (
                <p className="text-muted-foreground">{t("noTextBody")}</p>
              ) : (
                segments.map((seg, i) =>
                  seg.type === "p" ? (
                    <p key={i} className="leading-6" style={{fontSize: fontPx}}>
                      {seg.lines.map((l, j) => (
                        <span key={j}>
                          <Highlight text={l} terms={highlightTerms} />
                          {j < seg.lines.length - 1 && <br />}
                        </span>
                      ))}
                    </p>
                  ) : (
                    <QuoteBlock
                      key={i}
                      lines={seg.lines}
                      expanded={expandedQuotes.has(i)}
                      onToggle={() => toggleQuote(i)}
                    />
                  ),
                )
              )}
            </div>

            {/* Attachments */}
            {detail.attachments && detail.attachments.length > 0 && (
              <div className="mt-4 border-t border-border pt-3">
                <p className="mb-2 text-xs font-medium text-muted-foreground">
                  {t("attachments", { count: detail.attachments.length })}
                </p>
                <div className="flex flex-wrap gap-2">
                  {detail.attachments.map((a, i) => (
                    <AttachmentCard key={i} attachment={a} />
                  ))}
                </div>
              </div>
            )}
          </div>
        )}

        {/* AI summary */}
        {aiEnabled && (
          <div className="mt-4 rounded-lg border border-ai/30 bg-ai/10 p-3">
            <div className="flex items-center gap-2">
              <span className="flex items-center gap-1 rounded bg-ai px-1.5 py-0.5 text-[11px] font-semibold text-ai-foreground">
                <Sparkles className="size-3" />
                AI
              </span>
              {summary ? (
                <>
                  <button
                    onClick={() => setSummaryCollapsed((v) => !v)}
                    className="rounded p-0.5 text-muted-foreground hover:text-foreground"
                    title={summaryCollapsed ? t("expandQuote") : t("collapseQuote")}
                  >
                    {summaryCollapsed ? (
                      <ChevronDown className="size-3.5" />
                    ) : (
                      <ChevronUp className="size-3.5" />
                    )}
                  </button>
                  <Button
                    size="xs"
                    variant="ghost"
                    onClick={onSummarize}
                    disabled={summarizing}
                  >
                    <RotateCw className="size-3" />
                    {t("regenerate")}
                  </Button>
                  <div className="ml-auto flex items-center gap-0.5">
                    <button
                      onClick={() => setFeedback(feedback === "up" ? null : "up")}
                      title={t("helpful")}
                      className={cn(
                        "rounded p-1 transition-colors",
                        feedback === "up"
                          ? "text-primary"
                          : "text-muted-foreground hover:text-foreground",
                      )}
                    >
                      <ThumbsUp className="size-3.5" />
                    </button>
                    <button
                      onClick={() => setFeedback(feedback === "down" ? null : "down")}
                      title={t("notHelpful")}
                      className={cn(
                        "rounded p-1 transition-colors",
                        feedback === "down"
                          ? "text-destructive"
                          : "text-muted-foreground hover:text-foreground",
                      )}
                    >
                      <ThumbsDown className="size-3.5" />
                    </button>
                  </div>
                </>
              ) : !summarizing ? (
                <Button size="xs" variant="outline" onClick={onSummarize}>
                  {t("summarize")}
                </Button>
              ) : (
                <span className="text-xs text-muted-foreground">{t("summarizing")}</span>
              )}
            </div>
            {summary && !summaryCollapsed && (
              <>
                <p className="mt-2 text-sm leading-6 whitespace-pre-wrap">{summary}</p>
                <p className="mt-2 flex items-center gap-1 text-[11px] text-muted-foreground">
                  <Info className="size-3" />
                  {t("aiGenerated")}
                </p>
                {feedback && (
                  <p className="mt-1 text-[11px] text-muted-foreground">
                    {t("feedbackThanks")}
                  </p>
                )}
              </>
            )}
          </div>
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
