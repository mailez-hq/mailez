"use client";

import { useEffect, useMemo, useState } from "react";
import {
  Archive,
  ArrowLeft,
  ChevronDown,
  ChevronUp,
  Download,
  FileCode,
  Info,
  Loader2,
  Lock,
  Printer,
  RotateCw,
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
import { mailRaw, pgpDecrypt } from "@/lib/api";
import type { MailAttachment, MailMessage, MailThread } from "@/lib/api";
import { AttachmentCard } from "@/components/mailbox/reader/attachment-card";
import { QuoteBlock } from "@/components/mailbox/reader/quote-block";
import { useMounted } from "@/components/mailbox/reader/use-mounted";
import {
  blockRemoteImages, hasRemoteImages, rememberedRemoteSenders, rememberRemoteSender,
} from "@/components/mailbox/reader/remote-images";
import {
  fmtFullDate, fmtShort, getSnippet, parseBody, type Segment,
} from "@/components/mailbox/reader/body";
import { Highlight } from "@/components/mailbox/highlight";
import { sanitizeMailHTML } from "@/lib/sanitize";
import { cn } from "@/lib/utils";

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

      {/* Message body */}
      <div className="mail-body">
        {segments.length === 0 && !htmlBody && loading && (
          <p className="flex items-center gap-1.5 text-muted-foreground">
            <Loader2 className="size-3.5 animate-spin" />
            {t("loading")}
          </p>
        )}
        {segments.length === 0 && !htmlBody && !loading && (
          <p className="text-muted-foreground">{t("noTextBody")}</p>
        )}
        {segments.map((seg, i) =>
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
        )}

          {mounted && htmlBody && (
          <div className="mt-4 border-t border-border pt-3">
            <p className="mb-2 text-xs text-muted-foreground">{t("htmlVersion")}</p>
            <div className="mail-body" dangerouslySetInnerHTML={{ __html: htmlBody }} />
          </div>
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

      {/* Action buttons below message — FastMail style */}
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
  onBack,
  folder,
  folders,
  onMoveToFolder,
  showNotSpam,
  onNotSpam,
  labels,
  onToggleLabel,
  thread,
  threadOpen,
  threadLoading,
  onToggleThread,
  onSelectThread,
  highlightTerms,
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
  onBack: () => void;
  folder: string;
  folders: string[];
  onMoveToFolder: (destination: string) => void;
  showNotSpam: boolean;
  onNotSpam: () => void;
  labels: string[];
  onToggleLabel: (label: string) => void;
  thread: MailThread | null;
  threadOpen: boolean;
  threadLoading: boolean;
  onToggleThread: () => void;
  onSelectThread: (uid: number) => void;
  highlightTerms?: string[];
}) {
  const t = useTranslations("mail");
  const mounted = useMounted();
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [expandedQuotes, setExpandedQuotes] = useState<Set<number>>(new Set());
  const [summaryCollapsed, setSummaryCollapsed] = useState(false);
  const [feedback, setFeedback] = useState<"up" | "down" | null>(null);
  const [pgpPlaintext, setPgpPlaintext] = useState<string | null>(null);
  const [decrypting, setDecrypting] = useState(false);
  const [pgpError, setPgpError] = useState("");
  const [labelOpen, setLabelOpen] = useState(false);
  const [newLabel, setNewLabel] = useState("");
  const [rawOpen, setRawOpen] = useState(false);
  const [rawText, setRawText] = useState("");
  const [rawLoading, setRawLoading] = useState(false);
  const [rawError, setRawError] = useState("");

  // FastMail-style: track which thread message is expanded
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

  return (
    <main className="mail-scroll min-w-0 flex-1 overflow-y-auto bg-card">
      {/* Subject header — FastMail style */}
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
            <Button
              size="sm"
              variant="ghost"
              onClick={onArchive}
              title={t("archive")}
              className="size-8"
            >
              <Archive className="size-4" />
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={onDelete}
              className="size-8 text-muted-foreground hover:text-destructive"
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        </div>

        {/* Labels / tags */}
        <div className="mt-1.5 flex flex-wrap items-center gap-1">
          {labels
            .filter((l) => detail.flags.includes(l))
            .map((l) => (
              <span
                key={l}
                className="inline-flex items-center rounded-full border border-primary/30 bg-accent px-2 py-0.5 text-[11px] font-medium text-accent-foreground"
              >
                {l}
              </span>
            ))}
          <div className="relative">
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
                          "rounded-full border px-2 py-0.5 text-[11px] transition-colors",
                          has
                            ? "border-primary bg-accent font-medium text-accent-foreground"
                            : "border-border text-muted-foreground hover:bg-muted",
                        )}
                      >
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
        {/* Thread view — FastMail style conversation */}
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

            {/* Action buttons — FastMail style */}
            <div className="mb-4 flex flex-wrap items-center gap-2">
              {showNotSpam && (
                <Button size="sm" variant="outline" onClick={onNotSpam}>
                  <ShieldCheck className="size-3.5" />
                  {t("notSpam")}
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
                <Button size="sm" variant="ghost" onClick={() => window.print()} title={t("print")}>
                  <Printer className="size-3.5" />
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
                  {folders
                    .filter((f) => !/^(trash|drafts)$/i.test(f))
                    .map((f) => (
                      <option key={f} value={f}>
                        {f}
                      </option>
                    ))}
                </select>
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

            {/* Message body */}
            <div className="mail-body">
              {segments.length === 0 && !htmlBody && detailLoading && (
                <p className="flex items-center gap-1.5 text-muted-foreground">
                  <Loader2 className="size-3.5 animate-spin" />
                  {t("loading")}
                </p>
              )}
              {segments.length === 0 && !htmlBody && !detailLoading && (
                <p className="text-muted-foreground">{t("noTextBody")}</p>
              )}
              {segments.map((seg, i) =>
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
                    onToggle={() => toggleQuote(i)}
                  />
                ),
              )}

          {mounted && htmlBody && (
                <div className="mt-4 border-t border-border pt-3">
                  <p className="mb-2 text-xs text-muted-foreground">{t("htmlVersion")}</p>
                  <div className="mail-body" dangerouslySetInnerHTML={{ __html: htmlBody }} />
                </div>
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
        <DialogContent className="max-h-[85vh] max-w-3xl">
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
